package control

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"slices"
	"time"

	"github.com/beremaran/straw/v2/internal/config"
)

// AdminHandlers implements the config-management API surface needed for
// API key, worker credential, tenant, and quota lifecycle
// (docs/planning/26-config-management-api-surface.md). All handlers
// authenticate the caller and enforce RBAC before touching any store.
type AdminHandlers struct {
	Authenticator *Authenticator
	APIKeys       APIKeyStore
	WorkerCreds   WorkerCredentialStore
	Tenants       TenantStore
	Quotas        QuotaStore
	RateLimits    RateLimitConfigStore
	Audit         AuditStore
	ConfigCache   *ConfigCache
	Workers       *WorkerRegistry
	ConfigWrites  ConfigWriteStore
	// WorkerAdmin persists durable worker disable state. Optional: nil keeps
	// runtime-only (single-Control) behavior for unit tests; the binary wires
	// the Postgres store so disables survive restarts and reach snapshots.
	WorkerAdmin WorkerAdminStore
	// InFlight registers dispatched requests so CancelRequest
	// (docs/tasks/p0/27) can reach a running dispatch. Optional: nil makes
	// CancelRequest respond control_internal_error instead of panicking.
	InFlight *InFlightRegistry
	Pepper   []byte

	// Config admin API surface (docs/tasks/p0/20): routing rules, deny rules,
	// injection policies, and read-only fingerprint profiles. The binary
	// wires PostgresConfigStore for all four; unit tests may use the
	// InMemory* doubles in config_resource_store.go or leave a field nil
	// (its handlers then respond control_internal_error instead of panicking).
	RoutingRules        RoutingRuleStore
	ExecutorPools       ExecutorPoolStore
	DenyRules           DenyRuleStore
	InjectionPolicies   InjectionPolicyStore
	FingerprintProfiles FingerprintProfileStore

	// Runtime Redis-backed admission components (docs/tasks/p0/21). The
	// binary constructs these against a live Redis client; no admin handler
	// in this task reads them. They exist here so the request dispatch
	// pipeline (docs/tasks/p0/24) has a constructed instance to consume
	// instead of building its own. Nil in tests that do not need them.
	RateLimiter        *RateLimiter
	RateLimitAdmission *RateLimitAdmission
	QuotaAdmission     *QuotaAdmission
	StickySessions     StickyBackend
}

// ConfigWriteStore persists mutable tenant/platform config and its audit row in
// the same Postgres transaction as the tenant version bump.
type ConfigWriteStore interface {
	PutQuotaConfig(ctx context.Context, quota QuotaConfig, expectedVersion uint64, actor ConfigActor) (QuotaConfig, error)
	PutRateLimitConfig(ctx context.Context, cfg RateLimitConfig, expectedVersion uint64, ceiling *RateLimitCeiling, actor ConfigActor) (RateLimitConfig, error)
	SetGlobalWorkerAdminConfig(ctx context.Context, workerID string, disabled bool, reason string, actor ConfigActor) error
	SetTenantWorkerOverrideConfig(ctx context.Context, tenantID, workerID string, disabled bool, reason string, actor ConfigActor) error
}

func configActor(identity Identity) ConfigActor {
	return ConfigActor{ActorType: configActorTypeAPIKey, ActorID: identity.APIKeyID}
}

func writeAuthOrRBACError(w http.ResponseWriter, err error) {
	switch {
	case errors.Is(err, ErrAuthFailure):
		WriteError(w, http.StatusUnauthorized, ErrorResponseFromCode(AuthFailure, "", nil))
	case errors.Is(err, ErrTenantNotFound):
		WriteError(w, http.StatusUnauthorized, ErrorResponseFromCode(TenantNotFound, "", nil))
	case errors.Is(err, ErrInsufficientPermissions):
		WriteError(w, http.StatusForbidden, ErrorResponseFromCode(InsufficientPermissions, "", nil))
	default:
		WriteError(w, http.StatusInternalServerError, ErrorResponseFromCode(ControlInternalError, "", nil))
	}
}

func writeJSON(w http.ResponseWriter, status int, body any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)

	err := json.NewEncoder(w).Encode(body)
	if err != nil {
		return
	}
}

func decodeJSONBody(r *http.Request, dst any) error {
	dec := json.NewDecoder(r.Body)

	err := dec.Decode(dst)
	if err != nil {
		return fmt.Errorf("decode json body: %w", err)
	}

	return nil
}

// ---- Tenant lifecycle (docs/planning/26 tenant endpoint table) ----

type tenantCreateRequest struct {
	Name string `json:"name"`
}

// tenantRateLimitCeilingJSON is the wire shape of Tenant.RateLimitCeiling.
type tenantRateLimitCeilingJSON struct {
	WindowSeconds uint32 `json:"window_seconds"`
	MaxRequests   uint32 `json:"max_requests"`
}

type tenantUpdateRequest struct {
	Name                  string                      `json:"name"`
	Status                string                      `json:"status"`
	RateLimitCeiling      *tenantRateLimitCeilingJSON `json:"rate_limit_ceiling"`
	ExpectedConfigVersion uint64                      `json:"expected_config_version"`
}

type tenantResponse struct {
	ID               string                      `json:"id"`
	Name             string                      `json:"name"`
	Status           string                      `json:"status"`
	RateLimitCeiling *tenantRateLimitCeilingJSON `json:"rate_limit_ceiling"`
	CreatedAt        string                      `json:"created_at"`
	ConfigVersion    uint64                      `json:"config_version"`
}

func toTenantResponse(t Tenant) tenantResponse {
	resp := tenantResponse{
		ID:            t.ID,
		Name:          t.Name,
		Status:        string(t.Status),
		CreatedAt:     t.CreatedAt.Format(time.RFC3339),
		ConfigVersion: t.ConfigVersion,
	}

	if t.RateLimitCeiling != nil {
		resp.RateLimitCeiling = &tenantRateLimitCeilingJSON{
			WindowSeconds: t.RateLimitCeiling.WindowSeconds,
			MaxRequests:   t.RateLimitCeiling.MaxRequests,
		}
	}

	return resp
}

// validTenantUpdateStatuses excludes "deleted": soft delete is a dedicated
// endpoint (DELETE /tenants/{id}), not a status value PUT may set directly.
var validTenantUpdateStatuses = map[string]bool{
	string(TenantStatusActive):    true,
	string(TenantStatusSuspended): true,
}

// CreateTenant handles POST /tenants. Only system_admin may create tenant
// boundaries (docs/planning/06: "P0 must not rely on a tenant-scoped role
// to create that same tenant.").
func (h *AdminHandlers) CreateTenant(w http.ResponseWriter, r *http.Request) {
	identity, err := h.authenticate(r)
	if err != nil {
		writeAuthOrRBACError(w, err)

		return
	}

	err = RequireRole(identity, RoleSystemAdmin)
	if err != nil {
		writeAuthOrRBACError(w, err)

		return
	}

	var req tenantCreateRequest

	err = decodeJSONBody(r, &req)
	if err != nil || req.Name == "" {
		WriteError(w, http.StatusBadRequest, ErrorResponseFromCode(InvalidRequest, "", nil))

		return
	}

	id, err := newResourceID()
	if err != nil {
		WriteError(w, http.StatusInternalServerError, ErrorResponseFromCode(ControlInternalError, "", nil))

		return
	}

	tenant := Tenant{ID: id, Name: req.Name, Status: TenantStatusActive, CreatedAt: time.Now().UTC()}

	err = h.Tenants.Create(r.Context(), tenant)
	if err != nil {
		WriteError(w, http.StatusInternalServerError, ErrorResponseFromCode(ControlInternalError, "", nil))

		return
	}

	recordAudit(r.Context(), h.Audit, identity, "tenant", id, "create")

	writeJSON(w, http.StatusCreated, toTenantResponse(tenant))
}

// ListTenants handles GET /api/v1/config/tenants. system_admin only.
func (h *AdminHandlers) ListTenants(w http.ResponseWriter, r *http.Request) {
	identity, err := h.authenticate(r)
	if err != nil {
		writeAuthOrRBACError(w, err)

		return
	}

	err = RequireRole(identity, RoleSystemAdmin)
	if err != nil {
		writeAuthOrRBACError(w, err)

		return
	}

	limit, offset := parsePagination(r)

	tenants, err := h.Tenants.List(r.Context(), limit, offset)
	if err != nil {
		WriteError(w, http.StatusInternalServerError, ErrorResponseFromCode(ControlInternalError, "", nil))

		return
	}

	out := make([]tenantResponse, 0, len(tenants))
	for _, t := range tenants {
		out = append(out, toTenantResponse(t))
	}

	writeJSON(w, http.StatusOK, out)
}

// GetTenant handles GET /api/v1/config/tenants/{id}. Visible to system_admin
// or any role belonging to the tenant itself (docs/planning/26: "system_admin,
// tenant roles").
func (h *AdminHandlers) GetTenant(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")

	identity, err := h.authenticate(r)
	if err != nil {
		writeAuthOrRBACError(w, err)

		return
	}

	allowed := RequireRole(identity, RoleSystemAdmin) == nil
	if !allowed {
		allowed = RequireOwnTenant(identity, id) == nil
	}

	if !allowed {
		writeAuthOrRBACError(w, ErrInsufficientPermissions)

		return
	}

	tenant, err := h.Tenants.Get(r.Context(), id)
	if err != nil {
		WriteError(w, http.StatusNotFound, ErrorResponseFromCode(TenantNotFound, "", nil))

		return
	}

	writeJSON(w, http.StatusOK, toTenantResponse(tenant))
}

// UpdateTenant handles PUT /api/v1/config/tenants/{id}. system_admin only.
func (h *AdminHandlers) UpdateTenant(w http.ResponseWriter, r *http.Request) {
	identity, err := h.authenticate(r)
	if err != nil {
		writeAuthOrRBACError(w, err)

		return
	}

	err = RequireRole(identity, RoleSystemAdmin)
	if err != nil {
		writeAuthOrRBACError(w, err)

		return
	}

	var req tenantUpdateRequest

	err = decodeJSONBody(r, &req)
	if err != nil || req.Name == "" || !validTenantUpdateStatuses[req.Status] {
		WriteError(w, http.StatusBadRequest, ErrorResponseFromCode(InvalidRequest, "", map[string]string{
			errorDetailReasonKey: "name is required and status must be active or suspended",
		}))

		return
	}

	id := r.PathValue("id")

	var ceiling *RateLimitCeiling
	if req.RateLimitCeiling != nil {
		ceiling = &RateLimitCeiling{
			WindowSeconds: req.RateLimitCeiling.WindowSeconds,
			MaxRequests:   req.RateLimitCeiling.MaxRequests,
		}
	}

	tenant := Tenant{ID: id, Name: req.Name, Status: TenantStatus(req.Status), RateLimitCeiling: ceiling}

	updated, err := h.Tenants.Update(r.Context(), tenant, req.ExpectedConfigVersion)
	if err != nil {
		h.writeTenantResourceError(r, w, id, err)

		return
	}

	_, err = h.bumpTenantVersion(r.Context(), id, nil)
	if err != nil {
		WriteError(w, http.StatusInternalServerError, ErrorResponseFromCode(ControlInternalError, "", nil))

		return
	}

	h.ConfigCache.PublishInvalidation(r.Context(), id, updated.ConfigVersion)
	recordAudit(r.Context(), h.Audit, identity, "tenant", id, configActionUpdate)

	writeJSON(w, http.StatusOK, toTenantResponse(updated))
}

// SoftDeleteTenant handles DELETE /api/v1/config/tenants/{id}. system_admin
// only. Soft delete forces config-cache/auth invalidation before returning
// success, matching the API-key/worker-credential revocation pattern
// (docs/planning/25).
func (h *AdminHandlers) SoftDeleteTenant(w http.ResponseWriter, r *http.Request) {
	identity, err := h.authenticate(r)
	if err != nil {
		writeAuthOrRBACError(w, err)

		return
	}

	err = RequireRole(identity, RoleSystemAdmin)
	if err != nil {
		writeAuthOrRBACError(w, err)

		return
	}

	id := r.PathValue("id")

	deleted, err := h.Tenants.SoftDelete(r.Context(), id)
	if err != nil {
		h.writeTenantResourceError(r, w, id, err)

		return
	}

	_, err = h.bumpTenantVersion(r.Context(), id, nil)
	if err != nil {
		WriteError(w, http.StatusInternalServerError, ErrorResponseFromCode(ControlInternalError, "", nil))

		return
	}

	h.ConfigCache.PublishInvalidation(r.Context(), id, deleted.ConfigVersion)
	recordAudit(r.Context(), h.Audit, identity, "tenant", id, configActionDelete)

	writeJSON(w, http.StatusOK, toTenantResponse(deleted))
}

// ---- Platform API key lifecycle (system_admin only, after bootstrap) ----

type platformKeyCreateRequest struct {
	Role string `json:"role"`
}

type apiKeyCreateResponse struct {
	ID            string  `json:"id"`
	ScopeType     string  `json:"scope_type"`
	TenantID      *string `json:"tenant_id"`
	Role          string  `json:"role"`
	Prefix        string  `json:"prefix"`
	Secret        string  `json:"secret"`
	CreatedAt     string  `json:"created_at"`
	ConfigVersion uint64  `json:"config_version"`
}

type apiKeyReadResponse struct {
	ID            string  `json:"id"`
	ScopeType     string  `json:"scope_type"`
	TenantID      *string `json:"tenant_id"`
	Role          string  `json:"role"`
	Prefix        string  `json:"prefix"`
	Status        string  `json:"status"`
	CreatedAt     string  `json:"created_at"`
	RevokedAt     *string `json:"revoked_at"`
	ConfigVersion uint64  `json:"config_version"`
}

func toAPIKeyReadResponse(r APIKeyRecord) apiKeyReadResponse {
	resp := apiKeyReadResponse{
		ID:            r.ID,
		ScopeType:     string(r.ScopeType),
		Role:          string(r.Role),
		Prefix:        r.Prefix,
		Status:        string(r.Status),
		CreatedAt:     r.CreatedAt.Format(time.RFC3339),
		ConfigVersion: r.ConfigVersion,
	}
	if r.TenantID != "" {
		tid := r.TenantID
		resp.TenantID = &tid
	}

	if r.RevokedAt != nil {
		s := r.RevokedAt.Format(time.RFC3339)
		resp.RevokedAt = &s
	}

	return resp
}

// CreatePlatformAPIKey handles POST /platform-api-keys.
func (h *AdminHandlers) CreatePlatformAPIKey(w http.ResponseWriter, r *http.Request) {
	identity, err := h.authenticate(r)
	if err != nil {
		writeAuthOrRBACError(w, err)

		return
	}

	err = RequireRole(identity, RoleSystemAdmin)
	if err != nil {
		writeAuthOrRBACError(w, err)

		return
	}

	role, ok := decodeAndValidatePlatformKeyRole(w, r)
	if !ok {
		return
	}

	h.createPlatformAPIKey(w, r, identity, role)
}

// ListPlatformAPIKeys handles GET /platform-api-keys.
func (h *AdminHandlers) ListPlatformAPIKeys(w http.ResponseWriter, r *http.Request) {
	identity, err := h.authenticate(r)
	if err != nil {
		writeAuthOrRBACError(w, err)

		return
	}

	err = RequireRole(identity, RoleSystemAdmin)
	if err != nil {
		writeAuthOrRBACError(w, err)

		return
	}

	records, err := h.APIKeys.ListPlatform(r.Context())
	if err != nil {
		WriteError(w, http.StatusInternalServerError, ErrorResponseFromCode(ControlInternalError, "", nil))

		return
	}

	out := make([]apiKeyReadResponse, 0, len(records))
	for _, rec := range records {
		out = append(out, toAPIKeyReadResponse(rec))
	}

	writeJSON(w, http.StatusOK, out)
}

// RevokePlatformAPIKey handles POST /platform-api-keys/{id}/revoke.
func (h *AdminHandlers) RevokePlatformAPIKey(w http.ResponseWriter, r *http.Request) {
	identity, err := h.authenticate(r)
	if err != nil {
		writeAuthOrRBACError(w, err)

		return
	}

	err = RequireRole(identity, RoleSystemAdmin)
	if err != nil {
		writeAuthOrRBACError(w, err)

		return
	}

	id := r.PathValue("id")

	existing, err := h.APIKeys.Get(r.Context(), id)
	if err != nil || existing.ScopeType != ScopePlatform {
		WriteError(w, http.StatusNotFound, ErrorResponseFromCode(TenantNotFound, "", nil))

		return
	}

	revoked, err := h.APIKeys.Revoke(r.Context(), id, time.Now().UTC())
	if err != nil {
		WriteError(w, http.StatusInternalServerError, ErrorResponseFromCode(ControlInternalError, "", nil))

		return
	}
	// Platform key authentication always reads the live store directly
	// (see Authenticator.Authenticate), so revocation takes effect on the
	// very next authentication attempt with no separate cache to
	// invalidate.
	recordAudit(r.Context(), h.Audit, identity, "platform_api_key", id, "revoke")

	writeJSON(w, http.StatusOK, toAPIKeyReadResponse(revoked))
}

// ---- Tenant API key lifecycle (tenant_admin only, scoped to caller's
// tenant) ----

type tenantKeyCreateRequest struct {
	Role string `json:"role"`
}

// CreateTenantAPIKey handles POST /api-keys.
func (h *AdminHandlers) CreateTenantAPIKey(w http.ResponseWriter, r *http.Request) {
	identity, err := h.authenticate(r)
	if err != nil {
		writeAuthOrRBACError(w, err)

		return
	}

	err = RequireRole(identity, RoleTenantAdmin)
	if err != nil {
		writeAuthOrRBACError(w, err)

		return
	}

	role, ok := decodeAndValidateTenantKeyRole(w, r)
	if !ok {
		return
	}

	h.createTenantAPIKey(w, r, identity, identity.TenantID, role)
}

// CreateTenantKeyForTenant handles POST /api/v1/config/tenants/{id}/api-keys.
// Unlike CreateTenantAPIKey (own-tenant, tenant_admin only), this lets a
// platform system_admin mint a tenant's FIRST key — the bootstrap path that
// was previously missing, forcing direct DB inserts. tenant_admin may still
// use it for their own tenant. Mirrors the sysadmin-or-own-tenant pattern of
// GET /api/v1/config/tenants/{id}.
func (h *AdminHandlers) CreateTenantKeyForTenant(w http.ResponseWriter, r *http.Request) {
	identity, err := h.authenticate(r)
	if err != nil {
		writeAuthOrRBACError(w, err)

		return
	}

	tenantID := r.PathValue("id")

	allowed := RequireRole(identity, RoleSystemAdmin) == nil
	if !allowed {
		allowed = RequireRole(identity, RoleTenantAdmin) == nil && RequireOwnTenant(identity, tenantID) == nil
	}

	if !allowed {
		writeAuthOrRBACError(w, ErrInsufficientPermissions)

		return
	}

	role, ok := decodeAndValidateTenantKeyRole(w, r)
	if !ok {
		return
	}

	_, err = h.Tenants.Get(r.Context(), tenantID)
	if err != nil {
		WriteError(w, http.StatusNotFound, ErrorResponseFromCode(TenantNotFound, "", nil))

		return
	}

	h.createTenantAPIKey(w, r, identity, tenantID, role)
}

// ListTenantAPIKeys handles GET /api-keys, scoped to the caller's tenant.
func (h *AdminHandlers) ListTenantAPIKeys(w http.ResponseWriter, r *http.Request) {
	identity, err := h.authenticate(r)
	if err != nil {
		writeAuthOrRBACError(w, err)

		return
	}

	err = RequireRole(identity, RoleTenantAdmin, RoleOperator)
	if err != nil {
		writeAuthOrRBACError(w, err)

		return
	}

	records, err := h.APIKeys.ListTenant(r.Context(), identity.TenantID)
	if err != nil {
		WriteError(w, http.StatusInternalServerError, ErrorResponseFromCode(ControlInternalError, "", nil))

		return
	}

	out := make([]apiKeyReadResponse, 0, len(records))
	for _, rec := range records {
		out = append(out, toAPIKeyReadResponse(rec))
	}

	writeJSON(w, http.StatusOK, out)
}

// RevokeTenantAPIKey handles POST /api-keys/{id}/revoke.
func (h *AdminHandlers) RevokeTenantAPIKey(w http.ResponseWriter, r *http.Request) {
	identity, err := h.authenticate(r)
	if err != nil {
		writeAuthOrRBACError(w, err)

		return
	}

	err = RequireRole(identity, RoleTenantAdmin)
	if err != nil {
		writeAuthOrRBACError(w, err)

		return
	}

	id := r.PathValue("id")

	existing, err := h.APIKeys.Get(r.Context(), id)
	if err != nil || existing.ScopeType != ScopeTenant || existing.TenantID != identity.TenantID {
		// Tenant isolation: never confirm existence of another tenant's key.
		WriteError(w, http.StatusForbidden, ErrorResponseFromCode(InsufficientPermissions, "", nil))

		return
	}

	revoked, err := h.APIKeys.Revoke(r.Context(), id, time.Now().UTC())
	if err != nil {
		WriteError(w, http.StatusInternalServerError, ErrorResponseFromCode(ControlInternalError, "", nil))

		return
	}

	// Force cache invalidation before returning success
	// (docs/planning/25-dynamic-configuration.md: "API key revocation and
	// worker credential revocation force cache invalidation before
	// returning success.").
	_, err = h.bumpTenantVersion(r.Context(), identity.TenantID, func(revokedIDs []string) []string {
		return append(append([]string(nil), revokedIDs...), id)
	})
	if err != nil {
		WriteError(w, http.StatusInternalServerError, ErrorResponseFromCode(ControlInternalError, "", nil))

		return
	}

	recordAudit(r.Context(), h.Audit, identity, "tenant_api_key", id, "revoke")
	writeJSON(w, http.StatusOK, toAPIKeyReadResponse(revoked))
}

// ---- Worker credential lifecycle (tenant_admin only; P0 forces
// single-tenant scope) ----

type workerCredentialCreateRequest struct {
	ExecutorType           string        `json:"executor_type"`
	AllowedPools           []AllowedPool `json:"allowed_pools"`
	PublicKeyEd25519Base64 string        `json:"public_key_ed25519_base64"`
}

type workerCredentialResponse struct {
	ID                     string        `json:"id"`
	TenantScope            []string      `json:"tenant_scope"`
	ExecutorType           string        `json:"executor_type"`
	AllowedPools           []AllowedPool `json:"allowed_pools"`
	PublicKeyEd25519Base64 string        `json:"public_key_ed25519_base64"`
	Status                 string        `json:"status"`
	ConfigVersion          uint64        `json:"config_version"`
}

func toWorkerCredentialResponse(c WorkerCredential) workerCredentialResponse {
	return workerCredentialResponse{
		ID:                     c.ID,
		TenantScope:            c.TenantScope,
		ExecutorType:           c.ExecutorType,
		AllowedPools:           c.AllowedPools,
		PublicKeyEd25519Base64: c.PublicKeyEd25519Base64,
		Status:                 string(c.Status),
		ConfigVersion:          c.ConfigVersion,
	}
}

// CreateWorkerCredential handles POST /worker-credentials. P0 forces
// tenant_scope to the caller's tenant and rejects allowed_pools entries
// referencing any other tenant (docs/planning/06, docs/planning/26).
// Multi-tenant worker credentials are a P1 system_admin-only operation.
func (h *AdminHandlers) CreateWorkerCredential(w http.ResponseWriter, r *http.Request) {
	identity, err := h.authenticate(r)
	if err != nil {
		writeAuthOrRBACError(w, err)

		return
	}

	err = RequireRole(identity, RoleTenantAdmin)
	if err != nil {
		writeAuthOrRBACError(w, err)

		return
	}

	var req workerCredentialCreateRequest

	err = decodeJSONBody(r, &req)
	if err != nil {
		WriteError(w, http.StatusBadRequest, ErrorResponseFromCode(InvalidRequest, "", nil))

		return
	}

	req, ok := validateWorkerCredentialRequest(w, req, identity.TenantID)
	if !ok {
		return
	}

	if !h.createWorkerCredential(w, r, identity, req) {
		return
	}
}

// ListWorkerCredentials handles GET /worker-credentials, scoped to the
// caller's tenant.
func (h *AdminHandlers) ListWorkerCredentials(w http.ResponseWriter, r *http.Request) {
	identity, err := h.authenticate(r)
	if err != nil {
		writeAuthOrRBACError(w, err)

		return
	}

	err = RequireRole(identity, RoleTenantAdmin)
	if err != nil {
		writeAuthOrRBACError(w, err)

		return
	}

	records, err := h.WorkerCreds.ListTenant(r.Context(), identity.TenantID)
	if err != nil {
		WriteError(w, http.StatusInternalServerError, ErrorResponseFromCode(ControlInternalError, "", nil))

		return
	}

	out := make([]workerCredentialResponse, 0, len(records))
	for _, rec := range records {
		out = append(out, toWorkerCredentialResponse(rec))
	}

	writeJSON(w, http.StatusOK, out)
}

// RevokeWorkerCredential handles POST /worker-credentials/{id}/revoke.
func (h *AdminHandlers) RevokeWorkerCredential(w http.ResponseWriter, r *http.Request) {
	identity, err := h.authenticate(r)
	if err != nil {
		writeAuthOrRBACError(w, err)

		return
	}

	err = RequireRole(identity, RoleTenantAdmin)
	if err != nil {
		writeAuthOrRBACError(w, err)

		return
	}

	id := r.PathValue("id")

	existing, err := h.WorkerCreds.Get(r.Context(), id)
	if err != nil || !containsString(existing.TenantScope, identity.TenantID) {
		WriteError(w, http.StatusForbidden, ErrorResponseFromCode(InsufficientPermissions, "", nil))

		return
	}

	revoked, err := h.WorkerCreds.Revoke(r.Context(), id, time.Now().UTC())
	if err != nil {
		WriteError(w, http.StatusInternalServerError, ErrorResponseFromCode(ControlInternalError, "", nil))

		return
	}
	// Force cache invalidation before returning success, same as API key
	// revocation.
	_, err = h.bumpTenantVersion(r.Context(), identity.TenantID, nil)
	if err != nil {
		WriteError(w, http.StatusInternalServerError, ErrorResponseFromCode(ControlInternalError, "", nil))

		return
	}

	recordAudit(r.Context(), h.Audit, identity, "worker_credential", id, "revoke")

	writeJSON(w, http.StatusOK, toWorkerCredentialResponse(revoked))
}

func containsString(list []string, want string) bool {
	return slices.Contains(list, want)
}

// ---- Quota config (platform-managed writes, tenant-readable) ----

type quotaResponse struct {
	TenantID           string `json:"tenant_id"`
	Period             string `json:"period"`
	MaxRequests        int64  `json:"max_requests"`
	MaxBandwidthBytes  int64  `json:"max_bandwidth_bytes"`
	RequestCountPolicy string `json:"request_count_policy"`
	RedisFailPolicy    string `json:"redis_fail_policy"`
	ConfigVersion      uint64 `json:"config_version"`
}

func toQuotaResponse(q QuotaConfig) quotaResponse {
	return quotaResponse{
		TenantID:           q.TenantID,
		Period:             q.Period,
		MaxRequests:        q.MaxRequests,
		MaxBandwidthBytes:  q.MaxBandwidthBytes,
		RequestCountPolicy: q.RequestCountPolicy,
		RedisFailPolicy:    q.RedisFailPolicy,
		ConfigVersion:      q.ConfigVersion,
	}
}

// GetQuotas handles GET /quotas, scoped to the caller's tenant. Tenant keys
// retain read-only access; only system_admin may write quotas.
func (h *AdminHandlers) GetQuotas(w http.ResponseWriter, r *http.Request) {
	identity, err := h.authenticate(r)
	if err != nil {
		writeAuthOrRBACError(w, err)

		return
	}

	err = RequireRole(identity, RoleTenantAdmin, RoleOperator, RoleViewer)
	if err != nil {
		writeAuthOrRBACError(w, err)

		return
	}

	quota, err := h.Quotas.Get(r.Context(), identity.TenantID)
	if err != nil {
		WriteError(w, http.StatusInternalServerError, ErrorResponseFromCode(ControlInternalError, "", nil))

		return
	}

	writeJSON(w, http.StatusOK, toQuotaResponse(quota))
}

type quotaPutRequest struct {
	ExpectedConfigVersion uint64 `json:"expected_config_version"`
	Period                string `json:"period"`
	MaxRequests           int64  `json:"max_requests"`
	MaxBandwidthBytes     int64  `json:"max_bandwidth_bytes"`
	RequestCountPolicy    string `json:"request_count_policy"`
	RedisFailPolicy       string `json:"redis_fail_policy"`
}

// PutTenantQuotas handles PUT /tenants/{id}/quotas. Requires system_admin;
// platform keys carry no tenant identity, so the tenant is taken from the
// path, not from the caller's identity (docs/planning/26).
func (h *AdminHandlers) PutTenantQuotas(w http.ResponseWriter, r *http.Request) {
	identity, err := h.authenticate(r)
	if err != nil {
		writeAuthOrRBACError(w, err)

		return
	}

	err = RequireRole(identity, RoleSystemAdmin)
	if err != nil {
		writeAuthOrRBACError(w, err)

		return
	}

	tenantID := r.PathValue("id")

	_, err = h.Tenants.Get(r.Context(), tenantID)
	if err != nil {
		WriteError(w, http.StatusNotFound, ErrorResponseFromCode(TenantNotFound, "", nil))

		return
	}

	req, ok := decodeQuotaPutRequest(w, r)
	if !ok {
		return
	}

	h.putTenantQuotas(w, r, identity, tenantID, req)
}

// ---- Rate limit config (tenant-managed, bounded by optional
// system_admin-set ceiling) ----

type rateLimitRuleJSON struct {
	Dimension     string `json:"dimension"`
	Key           string `json:"key"`
	WindowSeconds uint32 `json:"window_seconds"`
	MaxRequests   uint32 `json:"max_requests"`
	FailPolicy    string `json:"fail_policy"`
}

type rateLimitResponse struct {
	TenantID      string              `json:"tenant_id"`
	Limits        []rateLimitRuleJSON `json:"limits"`
	ConfigVersion uint64              `json:"config_version"`
}

func toRateLimitResponse(cfg RateLimitConfig) rateLimitResponse {
	limits := make([]rateLimitRuleJSON, 0, len(cfg.Limits))
	for _, l := range cfg.Limits {
		limits = append(limits, rateLimitRuleJSON{
			Dimension:     string(l.Dimension),
			Key:           l.Key,
			WindowSeconds: l.WindowSeconds,
			MaxRequests:   l.MaxRequests,
			FailPolicy:    string(l.FailPolicy),
		})
	}

	return rateLimitResponse{TenantID: cfg.TenantID, Limits: limits, ConfigVersion: cfg.ConfigVersion}
}

// GetRateLimits handles GET /rate-limits, scoped to the caller's tenant.
func (h *AdminHandlers) GetRateLimits(w http.ResponseWriter, r *http.Request) {
	identity, err := h.authenticate(r)
	if err != nil {
		writeAuthOrRBACError(w, err)

		return
	}

	err = RequireRole(identity, RoleTenantAdmin, RoleOperator, RoleViewer)
	if err != nil {
		writeAuthOrRBACError(w, err)

		return
	}

	cfg, err := h.RateLimits.Get(r.Context(), identity.TenantID)
	if err != nil {
		WriteError(w, http.StatusInternalServerError, ErrorResponseFromCode(ControlInternalError, "", nil))

		return
	}

	writeJSON(w, http.StatusOK, toRateLimitResponse(cfg))
}

type rateLimitPutRequest struct {
	ExpectedConfigVersion uint64              `json:"expected_config_version"`
	Limits                []rateLimitRuleJSON `json:"limits"`
}

// PutRateLimits handles PUT /rate-limits. Requires tenant_admin. Values
// exceeding the tenant's system_admin-set rate_limit_ceiling are rejected
// with invalid_request (docs/planning/26).
func (h *AdminHandlers) PutRateLimits(w http.ResponseWriter, r *http.Request) {
	identity, err := h.authenticate(r)
	if err != nil {
		writeAuthOrRBACError(w, err)

		return
	}

	err = RequireRole(identity, RoleTenantAdmin)
	if err != nil {
		writeAuthOrRBACError(w, err)

		return
	}

	var req rateLimitPutRequest

	err = decodeJSONBody(r, &req)
	if err != nil {
		WriteError(w, http.StatusBadRequest, ErrorResponseFromCode(InvalidRequest, "", nil))

		return
	}

	tenant, err := h.Tenants.Get(r.Context(), identity.TenantID)
	if err != nil {
		WriteError(w, http.StatusNotFound, ErrorResponseFromCode(TenantNotFound, "", nil))

		return
	}

	h.putRateLimits(w, r, identity, tenant, req)
}

func (h *AdminHandlers) putRateLimits(w http.ResponseWriter, r *http.Request, identity Identity, tenant Tenant, req rateLimitPutRequest) {
	limits := make([]RateLimitRule, 0, len(req.Limits))
	for _, l := range req.Limits {
		limits = append(limits, RateLimitRule{
			Dimension:     RateLimitDimension(l.Dimension),
			Key:           l.Key,
			WindowSeconds: l.WindowSeconds,
			MaxRequests:   l.MaxRequests,
			FailPolicy:    RateLimitFailPolicy(l.FailPolicy),
		})
	}

	cfg := RateLimitConfig{TenantID: identity.TenantID, Limits: limits}

	if h.ConfigWrites != nil {
		saved, err := h.ConfigWrites.PutRateLimitConfig(r.Context(), cfg, req.ExpectedConfigVersion, tenant.RateLimitCeiling, configActor(identity))
		if err != nil {
			h.writeRateLimitError(w, err)

			return
		}

		writeJSON(w, http.StatusOK, toRateLimitResponse(saved))

		return
	}

	saved, err := h.RateLimits.Put(r.Context(), cfg, req.ExpectedConfigVersion, tenant.RateLimitCeiling)
	if err != nil {
		h.writeRateLimitError(w, err)

		return
	}

	recordAudit(r.Context(), h.Audit, identity, "rate_limit_config", identity.TenantID, "update")

	writeJSON(w, http.StatusOK, toRateLimitResponse(saved))
}

func (h *AdminHandlers) writeRateLimitError(w http.ResponseWriter, err error) {
	switch {
	case errors.Is(err, ErrRateLimitVersionConflict):
		WriteError(w, http.StatusConflict, ErrorResponseFromCode(Conflict, "", nil))
	case errors.Is(err, ErrRateLimitCeilingExceeded):
		WriteError(w, http.StatusBadRequest, ErrorResponseFromCode(InvalidRequest, "", map[string]string{
			errorDetailReasonKey: "rate limit exceeds tenant rate_limit_ceiling",
		}))
	default:
		WriteError(w, http.StatusInternalServerError, ErrorResponseFromCode(ControlInternalError, "", nil))
	}
}

// writeTenantResourceError maps a TenantStore write error to its HTTP
// response, including details.current_config_version on a version conflict
// (docs/planning/26 "Shared Config API Contract").
func (h *AdminHandlers) writeTenantResourceError(r *http.Request, w http.ResponseWriter, id string, err error) {
	switch {
	case errors.Is(err, ErrTenantVersionConflict):
		WriteError(w, http.StatusConflict, ErrorResponseFromCode(Conflict, "", conflictDetails(r.Context(), func(ctx context.Context) (uint64, error) {
			got, getErr := h.Tenants.Get(ctx, id)
			if getErr != nil {
				return 0, fmt.Errorf("get tenant: %w", getErr)
			}

			return got.ConfigVersion, nil
		})))
	case errors.Is(err, ErrTenantNotFound):
		WriteError(w, http.StatusNotFound, ErrorResponseFromCode(TenantNotFound, "", nil))
	default:
		WriteError(w, http.StatusInternalServerError, ErrorResponseFromCode(ControlInternalError, "", nil))
	}
}

func decodeAndValidatePlatformKeyRole(w http.ResponseWriter, r *http.Request) (Role, bool) {
	var req platformKeyCreateRequest

	err := decodeJSONBody(r, &req)
	if err != nil {
		WriteError(w, http.StatusBadRequest, ErrorResponseFromCode(InvalidRequest, "", nil))

		return "", false
	}

	role := Role(req.Role)
	if !ValidPlatformRole(role) {
		WriteError(w, http.StatusBadRequest, ErrorResponseFromCode(InvalidRequest, "", nil))

		return "", false
	}

	return role, true
}

func decodeAndValidateTenantKeyRole(w http.ResponseWriter, r *http.Request) (Role, bool) {
	var req tenantKeyCreateRequest

	err := decodeJSONBody(r, &req)
	if err != nil {
		WriteError(w, http.StatusBadRequest, ErrorResponseFromCode(InvalidRequest, "", nil))

		return "", false
	}

	role := Role(req.Role)
	if !ValidTenantRole(role) {
		WriteError(w, http.StatusBadRequest, ErrorResponseFromCode(InvalidRequest, "", nil))

		return "", false
	}

	return role, true
}

func decodeQuotaPutRequest(w http.ResponseWriter, r *http.Request) (quotaPutRequest, bool) {
	var req quotaPutRequest

	err := decodeJSONBody(r, &req)
	if err != nil {
		WriteError(w, http.StatusBadRequest, ErrorResponseFromCode(InvalidRequest, "", nil))

		return quotaPutRequest{}, false
	}

	return req, true
}

func createAPIKeyRecord(ctx context.Context, store APIKeyStore, w http.ResponseWriter, record APIKeyRecord) bool {
	err := store.Create(ctx, record)
	if err != nil {
		WriteError(w, http.StatusInternalServerError, ErrorResponseFromCode(ControlInternalError, "", nil))

		return false
	}

	return true
}

func (h *AdminHandlers) createPlatformAPIKey(w http.ResponseWriter, r *http.Request, identity Identity, role Role) {
	generated, err := GenerateAPIKey()
	if err != nil {
		WriteError(w, http.StatusInternalServerError, ErrorResponseFromCode(ControlInternalError, "", nil))

		return
	}

	id, err := newResourceID()
	if err != nil {
		WriteError(w, http.StatusInternalServerError, ErrorResponseFromCode(ControlInternalError, "", nil))

		return
	}

	record := APIKeyRecord{
		ID:            id,
		ScopeType:     ScopePlatform,
		Role:          role,
		Prefix:        generated.Prefix,
		SecretHash:    HashAPIKeySecret(generated.Secret, h.Pepper),
		Status:        APIKeyStatusActive,
		CreatedAt:     time.Now().UTC(),
		ConfigVersion: 0,
	}

	if !createAPIKeyRecord(r.Context(), h.APIKeys, w, record) {
		return
	}

	recordAudit(r.Context(), h.Audit, identity, "platform_api_key", id, "create")

	writeJSON(w, http.StatusCreated, apiKeyCreateResponse{
		ID:            record.ID,
		ScopeType:     string(record.ScopeType),
		TenantID:      nil,
		Role:          string(record.Role),
		Prefix:        record.Prefix,
		Secret:        generated.Secret,
		CreatedAt:     record.CreatedAt.Format(time.RFC3339),
		ConfigVersion: record.ConfigVersion,
	})
}

func (h *AdminHandlers) createTenantAPIKey(w http.ResponseWriter, r *http.Request, identity Identity, tenantID string, role Role) {
	generated, err := GenerateAPIKey()
	if err != nil {
		WriteError(w, http.StatusInternalServerError, ErrorResponseFromCode(ControlInternalError, "", nil))

		return
	}

	id, err := newResourceID()
	if err != nil {
		WriteError(w, http.StatusInternalServerError, ErrorResponseFromCode(ControlInternalError, "", nil))

		return
	}

	saved, err := h.bumpTenantVersion(r.Context(), tenantID, nil)
	if err != nil {
		WriteError(w, http.StatusInternalServerError, ErrorResponseFromCode(ControlInternalError, "", nil))

		return
	}

	record := APIKeyRecord{
		ID:            id,
		ScopeType:     ScopeTenant,
		TenantID:      tenantID,
		Role:          role,
		Prefix:        generated.Prefix,
		SecretHash:    HashAPIKeySecret(generated.Secret, h.Pepper),
		Status:        APIKeyStatusActive,
		CreatedAt:     time.Now().UTC(),
		ConfigVersion: saved.ConfigVersion,
	}

	if !createAPIKeyRecord(r.Context(), h.APIKeys, w, record) {
		return
	}

	recordAudit(r.Context(), h.Audit, identity, "tenant_api_key", id, "create")

	tid := tenantID
	writeJSON(w, http.StatusCreated, apiKeyCreateResponse{
		ID:            record.ID,
		ScopeType:     string(record.ScopeType),
		TenantID:      &tid,
		Role:          string(record.Role),
		Prefix:        record.Prefix,
		Secret:        generated.Secret,
		CreatedAt:     record.CreatedAt.Format(time.RFC3339),
		ConfigVersion: record.ConfigVersion,
	})
}

func (h *AdminHandlers) putTenantQuotas(w http.ResponseWriter, r *http.Request, identity Identity, tenantID string, req quotaPutRequest) {
	quota := QuotaConfig{
		TenantID:           tenantID,
		Period:             req.Period,
		MaxRequests:        req.MaxRequests,
		MaxBandwidthBytes:  req.MaxBandwidthBytes,
		RequestCountPolicy: req.RequestCountPolicy,
		RedisFailPolicy:    req.RedisFailPolicy,
		UpdatedAt:          time.Now().UTC(),
	}

	if h.ConfigWrites != nil {
		saved, err := h.ConfigWrites.PutQuotaConfig(r.Context(), quota, req.ExpectedConfigVersion, configActor(identity))
		if err != nil {
			if errors.Is(err, ErrQuotaVersionConflict) {
				WriteError(w, http.StatusConflict, ErrorResponseFromCode(Conflict, "", nil))

				return
			}

			WriteError(w, http.StatusInternalServerError, ErrorResponseFromCode(ControlInternalError, "", nil))

			return
		}

		writeJSON(w, http.StatusOK, toQuotaResponse(saved))

		return
	}

	saved, err := h.Quotas.Put(r.Context(), quota, req.ExpectedConfigVersion)
	if err != nil {
		if errors.Is(err, ErrQuotaVersionConflict) {
			WriteError(w, http.StatusConflict, ErrorResponseFromCode(Conflict, "", nil))

			return
		}

		WriteError(w, http.StatusInternalServerError, ErrorResponseFromCode(ControlInternalError, "", nil))

		return
	}

	_, err = h.bumpTenantVersion(r.Context(), tenantID, nil)
	if err != nil {
		WriteError(w, http.StatusInternalServerError, ErrorResponseFromCode(ControlInternalError, "", nil))

		return
	}

	recordAudit(r.Context(), h.Audit, identity, "quota_config", tenantID, "update")

	writeJSON(w, http.StatusOK, toQuotaResponse(saved))
}

func (h *AdminHandlers) createWorkerCredential(w http.ResponseWriter, r *http.Request, identity Identity, req workerCredentialCreateRequest) bool {
	id, err := newResourceID()
	if err != nil {
		WriteError(w, http.StatusInternalServerError, ErrorResponseFromCode(ControlInternalError, "", nil))

		return false
	}

	record := WorkerCredential{
		ID:                     id,
		Status:                 WorkerCredentialStatusActive,
		ExecutorType:           req.ExecutorType,
		PublicKeyEd25519Base64: req.PublicKeyEd25519Base64,
		TenantScope:            []string{identity.TenantID}, // forced single-tenant scope in P0
		AllowedPools:           req.AllowedPools,
		CreatedAt:              time.Now().UTC(),
		UpdatedAt:              time.Now().UTC(),
		ConfigVersion:          1,
	}

	err = h.WorkerCreds.Create(r.Context(), record)
	if err != nil {
		WriteError(w, http.StatusInternalServerError, ErrorResponseFromCode(ControlInternalError, "", nil))

		return false
	}

	_, err = h.bumpTenantVersion(r.Context(), identity.TenantID, nil)
	if err != nil {
		WriteError(w, http.StatusInternalServerError, ErrorResponseFromCode(ControlInternalError, "", nil))

		return false
	}

	recordAudit(r.Context(), h.Audit, identity, "worker_credential", id, "create")

	writeJSON(w, http.StatusCreated, toWorkerCredentialResponse(record))

	return true
}

func validateWorkerCredentialRequest(w http.ResponseWriter, req workerCredentialCreateRequest, tenantID string) (workerCredentialCreateRequest, bool) {
	if req.PublicKeyEd25519Base64 == "" {
		WriteError(w, http.StatusBadRequest, ErrorResponseFromCode(InvalidRequest, "", nil))

		return workerCredentialCreateRequest{}, false
	}

	if req.ExecutorType == "" {
		req.ExecutorType = errorCategoryEgress
	}

	for _, pool := range req.AllowedPools {
		if pool.TenantID != tenantID {
			WriteError(w, http.StatusBadRequest, ErrorResponseFromCode(InvalidRequest, "", map[string]string{
				errorDetailReasonKey: "allowed_pools entries must reference the caller's tenant in P0",
			}))

			return workerCredentialCreateRequest{}, false
		}
	}

	return req, true
}

func (h *AdminHandlers) authenticate(r *http.Request) (Identity, error) {
	return h.Authenticator.Authenticate(r.Context(), r.Header.Get("Authorization"))
}

// bumpTenantVersion increments the tenant's aggregate config version
// through ConfigCache, forcing invalidation to propagate before the caller
// observes success. mutateRevoked, if non-nil, transforms the current
// RevokedAPIKeyIDs list (used by API key revocation).
func (h *AdminHandlers) bumpTenantVersion(ctx context.Context, tenantID string, mutateRevoked func([]string) []string) (config.TenantSnapshot, error) {
	current, err := h.ConfigCache.Snapshot(ctx, tenantID)
	if err != nil {
		return config.TenantSnapshot{}, err
	}

	revoked := current.RevokedAPIKeyIDs
	if mutateRevoked != nil {
		revoked = mutateRevoked(revoked)
	}

	next := config.NewTenantSnapshot(tenantID, current.ConfigVersion+1, revoked)

	return h.ConfigCache.Save(ctx, next, current.ConfigVersion)
}
