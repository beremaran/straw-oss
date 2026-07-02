package control

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
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
	Audit         AuditStore
	ConfigCache   *ConfigCache
	Pepper        []byte
}

// ---- shared helpers ----

func (h *AdminHandlers) authenticate(r *http.Request) (Identity, error) {
	return h.Authenticator.Authenticate(r.Context(), r.Header.Get("Authorization"))
}

func writeAuthOrRBACError(w http.ResponseWriter, err error) {
	switch {
	case errors.Is(err, ErrAuthFailure):
		WriteError(w, http.StatusUnauthorized, ErrorResponseFromCode(AuthFailure, "", nil))
	case errors.Is(err, ErrInsufficientPermissions):
		WriteError(w, http.StatusForbidden, ErrorResponseFromCode(InsufficientPermissions, "", nil))
	default:
		WriteError(w, http.StatusInternalServerError, ErrorResponseFromCode(ControlInternalError, "", nil))
	}
}

func writeJSON(w http.ResponseWriter, status int, body any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(body)
}

func decodeJSONBody(r *http.Request, dst any) error {
	dec := json.NewDecoder(r.Body)
	return dec.Decode(dst)
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

// ---- Tenant lifecycle (minimal: enough to prove system_admin-only
// creation; the full tenant resource schema is out of this task's scope) ----

type tenantCreateRequest struct {
	Name string `json:"name"`
}

type tenantResponse struct {
	ID        string `json:"id"`
	Name      string `json:"name"`
	Status    string `json:"status"`
	CreatedAt string `json:"created_at"`
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
	if err := RequireRole(identity, RoleSystemAdmin); err != nil {
		writeAuthOrRBACError(w, err)
		return
	}

	var req tenantCreateRequest
	if err := decodeJSONBody(r, &req); err != nil || req.Name == "" {
		WriteError(w, http.StatusBadRequest, ErrorResponseFromCode(InvalidRequest, "", nil))
		return
	}

	id, err := newRandomID("ten")
	if err != nil {
		WriteError(w, http.StatusInternalServerError, ErrorResponseFromCode(ControlInternalError, "", nil))
		return
	}

	tenant := Tenant{ID: id, Name: req.Name, Status: TenantStatusActive, CreatedAt: time.Now().UTC()}
	if err := h.Tenants.Create(r.Context(), tenant); err != nil {
		WriteError(w, http.StatusInternalServerError, ErrorResponseFromCode(ControlInternalError, "", nil))
		return
	}
	recordAudit(r.Context(), h.Audit, identity, "tenant", id, "create")

	writeJSON(w, http.StatusCreated, tenantResponse{
		ID:        tenant.ID,
		Name:      tenant.Name,
		Status:    string(tenant.Status),
		CreatedAt: tenant.CreatedAt.Format(time.RFC3339),
	})
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
	if err := RequireRole(identity, RoleSystemAdmin); err != nil {
		writeAuthOrRBACError(w, err)
		return
	}

	var req platformKeyCreateRequest
	if err := decodeJSONBody(r, &req); err != nil {
		WriteError(w, http.StatusBadRequest, ErrorResponseFromCode(InvalidRequest, "", nil))
		return
	}
	role := Role(req.Role)
	if !ValidPlatformRole(role) {
		WriteError(w, http.StatusBadRequest, ErrorResponseFromCode(InvalidRequest, "", nil))
		return
	}

	generated, err := GenerateAPIKey()
	if err != nil {
		WriteError(w, http.StatusInternalServerError, ErrorResponseFromCode(ControlInternalError, "", nil))
		return
	}
	id, err := newRandomID("key")
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
	if err := h.APIKeys.Create(r.Context(), record); err != nil {
		WriteError(w, http.StatusInternalServerError, ErrorResponseFromCode(ControlInternalError, "", nil))
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

// ListPlatformAPIKeys handles GET /platform-api-keys.
func (h *AdminHandlers) ListPlatformAPIKeys(w http.ResponseWriter, r *http.Request) {
	identity, err := h.authenticate(r)
	if err != nil {
		writeAuthOrRBACError(w, err)
		return
	}
	if err := RequireRole(identity, RoleSystemAdmin); err != nil {
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
	if err := RequireRole(identity, RoleSystemAdmin); err != nil {
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
	if err := RequireRole(identity, RoleTenantAdmin); err != nil {
		writeAuthOrRBACError(w, err)
		return
	}

	var req tenantKeyCreateRequest
	if err := decodeJSONBody(r, &req); err != nil {
		WriteError(w, http.StatusBadRequest, ErrorResponseFromCode(InvalidRequest, "", nil))
		return
	}
	role := Role(req.Role)
	if !ValidTenantRole(role) {
		WriteError(w, http.StatusBadRequest, ErrorResponseFromCode(InvalidRequest, "", nil))
		return
	}

	generated, err := GenerateAPIKey()
	if err != nil {
		WriteError(w, http.StatusInternalServerError, ErrorResponseFromCode(ControlInternalError, "", nil))
		return
	}
	id, err := newRandomID("key")
	if err != nil {
		WriteError(w, http.StatusInternalServerError, ErrorResponseFromCode(ControlInternalError, "", nil))
		return
	}

	saved, err := h.bumpTenantVersion(r.Context(), identity.TenantID, nil)
	if err != nil {
		WriteError(w, http.StatusInternalServerError, ErrorResponseFromCode(ControlInternalError, "", nil))
		return
	}

	record := APIKeyRecord{
		ID:            id,
		ScopeType:     ScopeTenant,
		TenantID:      identity.TenantID,
		Role:          role,
		Prefix:        generated.Prefix,
		SecretHash:    HashAPIKeySecret(generated.Secret, h.Pepper),
		Status:        APIKeyStatusActive,
		CreatedAt:     time.Now().UTC(),
		ConfigVersion: saved.ConfigVersion,
	}
	if err := h.APIKeys.Create(r.Context(), record); err != nil {
		WriteError(w, http.StatusInternalServerError, ErrorResponseFromCode(ControlInternalError, "", nil))
		return
	}
	recordAudit(r.Context(), h.Audit, identity, "tenant_api_key", id, "create")

	tid := identity.TenantID
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

// ListTenantAPIKeys handles GET /api-keys, scoped to the caller's tenant.
func (h *AdminHandlers) ListTenantAPIKeys(w http.ResponseWriter, r *http.Request) {
	identity, err := h.authenticate(r)
	if err != nil {
		writeAuthOrRBACError(w, err)
		return
	}
	if err := RequireRole(identity, RoleTenantAdmin, RoleOperator); err != nil {
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
	if err := RequireRole(identity, RoleTenantAdmin); err != nil {
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
	if _, err := h.bumpTenantVersion(r.Context(), identity.TenantID, func(revokedIDs []string) []string {
		return append(append([]string(nil), revokedIDs...), id)
	}); err != nil {
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
	if err := RequireRole(identity, RoleTenantAdmin); err != nil {
		writeAuthOrRBACError(w, err)
		return
	}

	var req workerCredentialCreateRequest
	if err := decodeJSONBody(r, &req); err != nil {
		WriteError(w, http.StatusBadRequest, ErrorResponseFromCode(InvalidRequest, "", nil))
		return
	}
	if req.PublicKeyEd25519Base64 == "" {
		WriteError(w, http.StatusBadRequest, ErrorResponseFromCode(InvalidRequest, "", nil))
		return
	}
	if req.ExecutorType == "" {
		req.ExecutorType = "egress"
	}

	for _, pool := range req.AllowedPools {
		if pool.TenantID != identity.TenantID {
			WriteError(w, http.StatusBadRequest, ErrorResponseFromCode(InvalidRequest, "", map[string]string{
				"reason": "allowed_pools entries must reference the caller's tenant in P0",
			}))
			return
		}
	}

	id, err := newRandomID("wcred")
	if err != nil {
		WriteError(w, http.StatusInternalServerError, ErrorResponseFromCode(ControlInternalError, "", nil))
		return
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
	if err := h.WorkerCreds.Create(r.Context(), record); err != nil {
		WriteError(w, http.StatusInternalServerError, ErrorResponseFromCode(ControlInternalError, "", nil))
		return
	}
	if _, err := h.bumpTenantVersion(r.Context(), identity.TenantID, nil); err != nil {
		WriteError(w, http.StatusInternalServerError, ErrorResponseFromCode(ControlInternalError, "", nil))
		return
	}
	recordAudit(r.Context(), h.Audit, identity, "worker_credential", id, "create")

	writeJSON(w, http.StatusCreated, toWorkerCredentialResponse(record))
}

// ListWorkerCredentials handles GET /worker-credentials, scoped to the
// caller's tenant.
func (h *AdminHandlers) ListWorkerCredentials(w http.ResponseWriter, r *http.Request) {
	identity, err := h.authenticate(r)
	if err != nil {
		writeAuthOrRBACError(w, err)
		return
	}
	if err := RequireRole(identity, RoleTenantAdmin); err != nil {
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
	if err := RequireRole(identity, RoleTenantAdmin); err != nil {
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
	if _, err := h.bumpTenantVersion(r.Context(), identity.TenantID, nil); err != nil {
		WriteError(w, http.StatusInternalServerError, ErrorResponseFromCode(ControlInternalError, "", nil))
		return
	}
	recordAudit(r.Context(), h.Audit, identity, "worker_credential", id, "revoke")

	writeJSON(w, http.StatusOK, toWorkerCredentialResponse(revoked))
}

func containsString(list []string, want string) bool {
	for _, s := range list {
		if s == want {
			return true
		}
	}
	return false
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
	if err := RequireRole(identity, RoleTenantAdmin, RoleOperator, RoleViewer); err != nil {
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
	if err := RequireRole(identity, RoleSystemAdmin); err != nil {
		writeAuthOrRBACError(w, err)
		return
	}

	tenantID := r.PathValue("id")
	if _, err := h.Tenants.Get(r.Context(), tenantID); err != nil {
		WriteError(w, http.StatusNotFound, ErrorResponseFromCode(TenantNotFound, "", nil))
		return
	}

	var req quotaPutRequest
	if err := decodeJSONBody(r, &req); err != nil {
		WriteError(w, http.StatusBadRequest, ErrorResponseFromCode(InvalidRequest, "", nil))
		return
	}

	quota := QuotaConfig{
		TenantID:           tenantID,
		Period:             req.Period,
		MaxRequests:        req.MaxRequests,
		MaxBandwidthBytes:  req.MaxBandwidthBytes,
		RequestCountPolicy: req.RequestCountPolicy,
		RedisFailPolicy:    req.RedisFailPolicy,
		UpdatedAt:          time.Now().UTC(),
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
	if _, err := h.bumpTenantVersion(r.Context(), tenantID, nil); err != nil {
		WriteError(w, http.StatusInternalServerError, ErrorResponseFromCode(ControlInternalError, "", nil))
		return
	}
	recordAudit(r.Context(), h.Audit, identity, "quota_config", tenantID, "update")

	writeJSON(w, http.StatusOK, toQuotaResponse(saved))
}
