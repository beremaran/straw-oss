package control

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

const (
	adminTestTenantA           = "ten_a"
	adminTestTenantB           = "ten_b"
	adminTestKeyTenantAdmin    = "key_tenant_admin"
	adminTestKeyAAdmin         = "key_a_admin"
	adminTestKeyBAdmin         = "key_b_admin"
	adminTestInvalidRequest    = errorCodeInvalidRequest
	adminTestInsufficientPerms = "insufficient_permissions"
	adminTestEgress            = errorCategoryEgress
	adminTestTenantX           = "tenant_x"
	adminTestPoolX             = "pool_x"
	tenantUpdateTestName       = "Renamed"
)

// testAdmin bundles an AdminHandlers with its backing stores for direct
// inspection in tests (e.g. asserting on AuditStore contents).
type testAdmin struct {
	h                 *AdminHandlers
	apiKeys           *InMemoryAPIKeyStore
	workerCreds       *InMemoryWorkerCredentialStore
	tenants           *InMemoryTenantStore
	quotas            *InMemoryQuotaStore
	rateLimits        *InMemoryRateLimitConfigStore
	audit             *InMemoryAuditStore
	routingRules      *InMemoryRoutingRuleStore
	executorPools     *InMemoryExecutorPoolStore
	denyRules         *InMemoryDenyRuleStore
	injectionPolicies *InMemoryInjectionPolicyStore
	fingerprints      *InMemoryFingerprintProfileStore
	pepper            []byte
}

type recordingConfigWrites struct {
	quotaCalled     bool
	quota           QuotaConfig
	quotaExpected   uint64
	quotaActor      ConfigActor
	rateCalled      bool
	rate            RateLimitConfig
	rateExpected    uint64
	rateCeiling     *RateLimitCeiling
	rateActor       ConfigActor
	globalWorkerID  string
	tenantWorkerID  string
	tenantWorkerTid string
}

func (w *recordingConfigWrites) PutQuotaConfig(_ context.Context, quota QuotaConfig, expectedVersion uint64, actor ConfigActor) (QuotaConfig, error) {
	w.quotaCalled = true
	w.quota = quota
	w.quotaExpected = expectedVersion
	w.quotaActor = actor
	quota.ConfigVersion = expectedVersion + 1

	return quota, nil
}

func (w *recordingConfigWrites) PutRateLimitConfig(_ context.Context, cfg RateLimitConfig, expectedVersion uint64, ceiling *RateLimitCeiling, actor ConfigActor) (RateLimitConfig, error) {
	w.rateCalled = true
	w.rate = cfg
	w.rateExpected = expectedVersion
	w.rateCeiling = ceiling
	w.rateActor = actor
	cfg.ConfigVersion = expectedVersion + 1

	return cfg, nil
}

func (w *recordingConfigWrites) SetGlobalWorkerAdminConfig(_ context.Context, workerID string, _ bool, _ string, _ ConfigActor) error {
	w.globalWorkerID = workerID

	return nil
}

func (w *recordingConfigWrites) SetTenantWorkerOverrideConfig(_ context.Context, tenantID, workerID string, _ bool, _ string, _ ConfigActor) error {
	w.tenantWorkerTid = tenantID
	w.tenantWorkerID = workerID

	return nil
}

func newTestAdmin(t *testing.T) *testAdmin {
	t.Helper()

	apiKeys := NewInMemoryAPIKeyStore()
	pepper := []byte("test-pepper")
	snapshotStore := NewInMemorySnapshotStore()
	cache := NewConfigCache(snapshotStore, nil)

	ta := &testAdmin{
		apiKeys:           apiKeys,
		workerCreds:       NewInMemoryWorkerCredentialStore(),
		tenants:           NewInMemoryTenantStore(),
		quotas:            NewInMemoryQuotaStore(),
		rateLimits:        NewInMemoryRateLimitConfigStore(),
		audit:             NewInMemoryAuditStore(),
		routingRules:      NewInMemoryRoutingRuleStore(),
		executorPools:     NewInMemoryExecutorPoolStore(),
		denyRules:         NewInMemoryDenyRuleStore(),
		injectionPolicies: NewInMemoryInjectionPolicyStore(),
		fingerprints:      NewInMemoryFingerprintProfileStore(),
		pepper:            pepper,
	}
	ta.h = &AdminHandlers{
		Authenticator:       NewAuthenticator(apiKeys, pepper),
		APIKeys:             apiKeys,
		WorkerCreds:         ta.workerCreds,
		Tenants:             ta.tenants,
		Quotas:              ta.quotas,
		RateLimits:          ta.rateLimits,
		Audit:               ta.audit,
		ConfigCache:         cache,
		Pepper:              pepper,
		RoutingRules:        ta.routingRules,
		ExecutorPools:       ta.executorPools,
		DenyRules:           ta.denyRules,
		InjectionPolicies:   ta.injectionPolicies,
		FingerprintProfiles: ta.fingerprints,
	}

	return ta
}

// seedPlatformKey inserts an active platform-scoped key directly into the
// store and returns its plaintext secret for use in Authorization headers.
func (ta *testAdmin) seedPlatformKey(t *testing.T, id string, role Role) string {
	t.Helper()
	gen, err := GenerateAPIKey()
	if err != nil {
		t.Fatalf("GenerateAPIKey() error = %v", err)
	}
	mustCreate(t, ta.apiKeys, APIKeyRecord{
		ID: id, ScopeType: ScopePlatform, Role: role,
		Prefix: gen.Prefix, SecretHash: HashAPIKeySecret(gen.Secret, ta.pepper),
		Status: APIKeyStatusActive, CreatedAt: time.Now().UTC(),
	})

	return gen.Secret
}

// seedTenantKey inserts an active tenant-scoped key directly into the store
// and returns its plaintext secret.
func (ta *testAdmin) seedTenantKey(t *testing.T, id, tenantID string, role Role) string {
	t.Helper()
	gen, err := GenerateAPIKey()
	if err != nil {
		t.Fatalf("GenerateAPIKey() error = %v", err)
	}
	mustCreate(t, ta.apiKeys, APIKeyRecord{
		ID: id, ScopeType: ScopeTenant, TenantID: tenantID, Role: role,
		Prefix: gen.Prefix, SecretHash: HashAPIKeySecret(gen.Secret, ta.pepper),
		Status: APIKeyStatusActive, CreatedAt: time.Now().UTC(),
	})

	return gen.Secret
}

func newAdminRequest(method, path, token, body string) *http.Request {
	req := httptest.NewRequestWithContext(context.Background(), method, path, strings.NewReader(body))
	if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}
	req.Header.Set("Content-Type", "application/json")

	return req
}

// ---- platform key lifecycle ----

func TestPlatformAPIKeyLifecycle(t *testing.T) {
	t.Parallel()

	ta := newTestAdmin(t)
	adminToken := ta.seedPlatformKey(t, "key_admin", RoleSystemAdmin)

	// Create.
	w := httptest.NewRecorder()
	ta.h.CreatePlatformAPIKey(w, newAdminRequest(http.MethodPost, "/platform-api-keys", adminToken, `{"role":"system_admin"}`))
	if w.Code != http.StatusCreated {
		t.Fatalf("create status = %d, want %d, body=%s", w.Code, http.StatusCreated, w.Body.String())
	}
	var created apiKeyCreateResponse
	err := json.Unmarshal(w.Body.Bytes(), &created)
	if err != nil {
		t.Fatalf("unmarshal create response: %v", err)
	}
	if created.Secret == "" {
		t.Fatal("created key secret is empty, want plaintext key shown once")
	}
	if created.ScopeType != "platform" || created.TenantID != nil {
		t.Fatalf("created key scope = %q tenant_id = %v, want platform/nil", created.ScopeType, created.TenantID)
	}

	// List.
	w = httptest.NewRecorder()
	ta.h.ListPlatformAPIKeys(w, newAdminRequest(http.MethodGet, "/platform-api-keys", adminToken, ""))
	if w.Code != http.StatusOK {
		t.Fatalf("list status = %d, want %d", w.Code, http.StatusOK)
	}
	var listed []apiKeyReadResponse
	err = json.Unmarshal(w.Body.Bytes(), &listed)
	if err != nil {
		t.Fatalf("unmarshal list response: %v", err)
	}
	found := false
	for _, k := range listed {
		if k.ID == created.ID {
			found = true
		}
		if k.Prefix == "" {
			t.Fatal("list response must include prefix")
		}
	}
	if !found {
		t.Fatal("created key not present in list response")
	}

	// Authenticate with the new key before revocation succeeds.
	auth := NewAuthenticator(ta.apiKeys, ta.pepper)
	_, err = auth.Authenticate(context.Background(), "Bearer "+created.Secret)
	if err != nil {
		t.Fatalf("Authenticate() before revoke error = %v", err)
	}

	// Revoke.
	w = httptest.NewRecorder()
	req := newAdminRequest(http.MethodPost, "/platform-api-keys/"+created.ID+"/revoke", adminToken, "")
	req.SetPathValue("id", created.ID)
	ta.h.RevokePlatformAPIKey(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("revoke status = %d, want %d, body=%s", w.Code, http.StatusOK, w.Body.String())
	}

	// Revocation takes effect immediately.
	_, err = auth.Authenticate(context.Background(), "Bearer "+created.Secret)
	if !errors.Is(err, ErrAuthFailure) {
		t.Fatalf("Authenticate() after revoke error = %v, want ErrAuthFailure", err)
	}
}

func TestPlatformAPIKeyLifecycleRequiresSystemAdmin(t *testing.T) {
	t.Parallel()

	ta := newTestAdmin(t)
	tenantToken := ta.seedTenantKey(t, adminTestKeyTenantAdmin, adminTestTenantA, RoleTenantAdmin)

	w := httptest.NewRecorder()
	ta.h.CreatePlatformAPIKey(w, newAdminRequest(http.MethodPost, "/platform-api-keys", tenantToken, `{"role":"system_admin"}`))
	if w.Code != http.StatusForbidden {
		t.Fatalf("status = %d, want %d", w.Code, http.StatusForbidden)
	}
}

// ---- tenant key cannot create tenants ----

func TestTenantKeyCannotCreateTenants(t *testing.T) {
	t.Parallel()

	ta := newTestAdmin(t)
	tenantAdminToken := ta.seedTenantKey(t, adminTestKeyTenantAdmin, adminTestTenantA, RoleTenantAdmin)

	w := httptest.NewRecorder()
	ta.h.CreateTenant(w, newAdminRequest(http.MethodPost, "/tenants", tenantAdminToken, `{"name":"Sneaky Tenant"}`))
	if w.Code != http.StatusForbidden {
		t.Fatalf("status = %d, want %d", w.Code, http.StatusForbidden)
	}
	var errResp ErrorResponse
	err := json.Unmarshal(w.Body.Bytes(), &errResp)
	if err != nil {
		t.Fatalf("unmarshal error: %v", err)
	}
	if errResp.Code != adminTestInsufficientPerms {
		t.Fatalf("code = %q, want %q", errResp.Code, adminTestInsufficientPerms)
	}
}

func TestSystemAdminCanCreateTenants(t *testing.T) {
	t.Parallel()

	ta := newTestAdmin(t)
	adminToken := ta.seedPlatformKey(t, "key_admin", RoleSystemAdmin)

	w := httptest.NewRecorder()
	ta.h.CreateTenant(w, newAdminRequest(http.MethodPost, "/tenants", adminToken, `{"name":"Real Tenant"}`))
	if w.Code != http.StatusCreated {
		t.Fatalf("status = %d, want %d, body=%s", w.Code, http.StatusCreated, w.Body.String())
	}
}

// ---- tenant lifecycle (docs/tasks/p0/29) ----

func createTenantForTest(t *testing.T, ta *testAdmin, adminToken, name string) string {
	t.Helper()

	w := httptest.NewRecorder()
	ta.h.CreateTenant(w, newAdminRequest(http.MethodPost, "/tenants", adminToken, `{"name":"`+name+`"}`))
	if w.Code != http.StatusCreated {
		t.Fatalf("CreateTenant() status = %d, body=%s", w.Code, w.Body.String())
	}

	var resp tenantResponse

	err := json.Unmarshal(w.Body.Bytes(), &resp)
	if err != nil {
		t.Fatalf("unmarshal tenant response: %v", err)
	}

	return resp.ID
}

func TestListTenantsRequiresSystemAdmin(t *testing.T) {
	t.Parallel()

	ta := newTestAdmin(t)
	adminToken := ta.seedPlatformKey(t, "key_admin", RoleSystemAdmin)
	tenantID := createTenantForTest(t, ta, adminToken, "Tenant One")
	tenantAdminToken := ta.seedTenantKey(t, adminTestKeyTenantAdmin, tenantID, RoleTenantAdmin)

	w := httptest.NewRecorder()
	ta.h.ListTenants(w, newAdminRequest(http.MethodGet, "/api/v1/config/tenants", tenantAdminToken, ""))
	if w.Code != http.StatusForbidden {
		t.Fatalf("tenant-scoped ListTenants() status = %d, want %d", w.Code, http.StatusForbidden)
	}

	w = httptest.NewRecorder()
	ta.h.ListTenants(w, newAdminRequest(http.MethodGet, "/api/v1/config/tenants", adminToken, ""))
	if w.Code != http.StatusOK {
		t.Fatalf("system_admin ListTenants() status = %d, body=%s", w.Code, w.Body.String())
	}

	var out []tenantResponse

	err := json.Unmarshal(w.Body.Bytes(), &out)
	if err != nil {
		t.Fatalf("unmarshal list: %v", err)
	}

	if len(out) != 1 || out[0].ID != tenantID {
		t.Fatalf("ListTenants() = %+v, want one tenant %q", out, tenantID)
	}
}

func TestGetTenantAllowsOwnTenantRoleButNotOtherTenant(t *testing.T) {
	t.Parallel()

	ta := newTestAdmin(t)
	adminToken := ta.seedPlatformKey(t, "key_admin", RoleSystemAdmin)
	tenantID := createTenantForTest(t, ta, adminToken, "Tenant One")
	ownViewerToken := ta.seedTenantKey(t, "key_own_viewer", tenantID, RoleViewer)
	otherToken := ta.seedTenantKey(t, "key_other", adminTestTenantB, RoleViewer)

	req := newAdminRequest(http.MethodGet, "/api/v1/config/tenants/"+tenantID, ownViewerToken, "")
	req.SetPathValue("id", tenantID)
	w := httptest.NewRecorder()
	ta.h.GetTenant(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("own-tenant viewer GetTenant() status = %d, body=%s", w.Code, w.Body.String())
	}

	req = newAdminRequest(http.MethodGet, "/api/v1/config/tenants/"+tenantID, otherToken, "")
	req.SetPathValue("id", tenantID)
	w = httptest.NewRecorder()
	ta.h.GetTenant(w, req)
	if w.Code != http.StatusForbidden {
		t.Fatalf("other-tenant GetTenant() status = %d, want %d", w.Code, http.StatusForbidden)
	}
}

func TestUpdateTenantPersistsRateLimitCeilingAndForcesInvalidation(t *testing.T) {
	t.Parallel()

	ta := newTestAdmin(t)
	adminToken := ta.seedPlatformKey(t, "key_admin", RoleSystemAdmin)
	tenantID := createTenantForTest(t, ta, adminToken, "Tenant One")

	// Prime the cache so we can observe the version bump after update.
	before, err := ta.h.ConfigCache.Snapshot(context.Background(), tenantID)
	if err != nil {
		t.Fatalf("Snapshot() before update error = %v", err)
	}

	body := `{"name":"` + tenantUpdateTestName + `","status":"active","rate_limit_ceiling":{"window_seconds":60,"max_requests":6000},"expected_config_version":0}`
	req := newAdminRequest(http.MethodPut, "/api/v1/config/tenants/"+tenantID, adminToken, body)
	req.SetPathValue("id", tenantID)
	w := httptest.NewRecorder()
	ta.h.UpdateTenant(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("UpdateTenant() status = %d, body=%s", w.Code, w.Body.String())
	}

	var resp tenantResponse

	err = json.Unmarshal(w.Body.Bytes(), &resp)
	if err != nil {
		t.Fatalf("unmarshal update response: %v", err)
	}

	if resp.Name != tenantUpdateTestName || resp.RateLimitCeiling == nil || resp.RateLimitCeiling.MaxRequests != 6000 {
		t.Fatalf("UpdateTenant() response = %+v, want renamed with ceiling", resp)
	}

	after, err := ta.h.ConfigCache.Snapshot(context.Background(), tenantID)
	if err != nil {
		t.Fatalf("Snapshot() after update error = %v", err)
	}
	if after.ConfigVersion <= before.ConfigVersion {
		t.Fatalf("tenant config version = %d, want > %d (invalidation forced)", after.ConfigVersion, before.ConfigVersion)
	}

	// A rate-limit write above the ceiling is now rejected end to end.
	tenantAdminToken := ta.seedTenantKey(t, "key_tenant_admin_ceiling", tenantID, RoleTenantAdmin)
	rlBody := `{"limits":[{"dimension":"tenant","key":"*","window_seconds":60,"max_requests":7000,"fail_policy":"open"}],"expected_config_version":0}`
	rlReq := newAdminRequest(http.MethodPut, "/rate-limits", tenantAdminToken, rlBody)
	rlW := httptest.NewRecorder()
	ta.h.PutRateLimits(rlW, rlReq)
	if rlW.Code != http.StatusBadRequest {
		t.Fatalf("PutRateLimits() above ceiling status = %d, body=%s, want %d", rlW.Code, rlW.Body.String(), http.StatusBadRequest)
	}
}

func TestUpdateTenantRejectsVersionConflict(t *testing.T) {
	t.Parallel()

	ta := newTestAdmin(t)
	adminToken := ta.seedPlatformKey(t, "key_admin", RoleSystemAdmin)
	tenantID := createTenantForTest(t, ta, adminToken, "Tenant One")

	body := `{"name":"` + tenantUpdateTestName + `","status":"active","expected_config_version":5}`
	req := newAdminRequest(http.MethodPut, "/api/v1/config/tenants/"+tenantID, adminToken, body)
	req.SetPathValue("id", tenantID)
	w := httptest.NewRecorder()
	ta.h.UpdateTenant(w, req)
	if w.Code != http.StatusConflict {
		t.Fatalf("UpdateTenant() version conflict status = %d, want %d", w.Code, http.StatusConflict)
	}
}

func TestSoftDeleteTenantThenKeyRejectedWithTenantNotFound(t *testing.T) {
	t.Parallel()

	ta := newTestAdmin(t)
	adminToken := ta.seedPlatformKey(t, "key_admin", RoleSystemAdmin)
	tenantID := createTenantForTest(t, ta, adminToken, "Tenant One")
	tenantToken := ta.seedTenantKey(t, "key_tenant_requester", tenantID, RoleRequester)

	req := newAdminRequest(http.MethodDelete, "/api/v1/config/tenants/"+tenantID, adminToken, "")
	req.SetPathValue("id", tenantID)
	w := httptest.NewRecorder()
	ta.h.SoftDeleteTenant(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("SoftDeleteTenant() status = %d, body=%s", w.Code, w.Body.String())
	}

	auth := NewAuthenticator(ta.apiKeys, ta.pepper).SetTenantStore(ta.tenants)
	_, err := auth.Authenticate(context.Background(), "Bearer "+tenantToken)
	if !errors.Is(err, ErrTenantNotFound) {
		t.Fatalf("Authenticate() after soft delete error = %v, want ErrTenantNotFound", err)
	}

	// A second soft delete on the same tenant reports not found.
	req2 := newAdminRequest(http.MethodDelete, "/api/v1/config/tenants/"+tenantID, adminToken, "")
	req2.SetPathValue("id", tenantID)
	w2 := httptest.NewRecorder()
	ta.h.SoftDeleteTenant(w2, req2)
	if w2.Code != http.StatusNotFound {
		t.Fatalf("second SoftDeleteTenant() status = %d, want %d", w2.Code, http.StatusNotFound)
	}
}

func TestSuspendedTenantKeyRejectedWithTenantNotFound(t *testing.T) {
	t.Parallel()

	ta := newTestAdmin(t)
	adminToken := ta.seedPlatformKey(t, "key_admin", RoleSystemAdmin)
	tenantID := createTenantForTest(t, ta, adminToken, "Tenant One")
	tenantToken := ta.seedTenantKey(t, "key_tenant_requester_susp", tenantID, RoleRequester)

	body := `{"name":"Tenant One","status":"suspended","expected_config_version":0}`
	req := newAdminRequest(http.MethodPut, "/api/v1/config/tenants/"+tenantID, adminToken, body)
	req.SetPathValue("id", tenantID)
	w := httptest.NewRecorder()
	ta.h.UpdateTenant(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("UpdateTenant() to suspended status = %d, body=%s", w.Code, w.Body.String())
	}

	auth := NewAuthenticator(ta.apiKeys, ta.pepper).SetTenantStore(ta.tenants)
	_, err := auth.Authenticate(context.Background(), "Bearer "+tenantToken)
	if !errors.Is(err, ErrTenantNotFound) {
		t.Fatalf("Authenticate() for suspended tenant error = %v, want ErrTenantNotFound", err)
	}
}

// ---- actor audit source recorded ----

func TestActorAuditSourceRecorded(t *testing.T) {
	t.Parallel()

	ta := newTestAdmin(t)
	adminToken := ta.seedPlatformKey(t, "key_admin", RoleSystemAdmin)

	w := httptest.NewRecorder()
	ta.h.CreatePlatformAPIKey(w, newAdminRequest(http.MethodPost, "/platform-api-keys", adminToken, `{"role":"system_admin"}`))
	if w.Code != http.StatusCreated {
		t.Fatalf("status = %d, want %d", w.Code, http.StatusCreated)
	}

	if len(ta.audit.records) == 0 {
		t.Fatal("expected at least one audit record")
	}
	rec := ta.audit.records[len(ta.audit.records)-1]
	if rec.ActorType != configActorTypeAPIKey {
		t.Fatalf("actor_type = %q, want %q", rec.ActorType, configActorTypeAPIKey)
	}
	if rec.ActorID != "key_admin" {
		t.Fatalf("actor_id = %q, want %q", rec.ActorID, "key_admin")
	}
	if rec.ResourceType != "platform_api_key" || rec.Action != "create" {
		t.Fatalf("resource_type/action = %q/%q, want platform_api_key/create", rec.ResourceType, rec.Action)
	}
}

// ---- tenant isolation (cross-tenant access blocked) ----

func TestTenantIsolationBlocksCrossTenantKeyRevoke(t *testing.T) {
	t.Parallel()

	ta := newTestAdmin(t)
	tenantAToken := ta.seedTenantKey(t, adminTestKeyAAdmin, adminTestTenantA, RoleTenantAdmin)
	tenantBToken := ta.seedTenantKey(t, adminTestKeyBAdmin, adminTestTenantB, RoleTenantAdmin)

	// Tenant A creates a key.
	w := httptest.NewRecorder()
	ta.h.CreateTenantAPIKey(w, newAdminRequest(http.MethodPost, "/api-keys", tenantAToken, `{"role":"requester"}`))
	if w.Code != http.StatusCreated {
		t.Fatalf("create status = %d, want %d, body=%s", w.Code, http.StatusCreated, w.Body.String())
	}
	var created apiKeyCreateResponse
	err := json.Unmarshal(w.Body.Bytes(), &created)
	if err != nil {
		t.Fatalf("unmarshal: %v", err)
	}

	// Tenant B must not be able to revoke tenant A's key.
	w = httptest.NewRecorder()
	req := newAdminRequest(http.MethodPost, "/api-keys/"+created.ID+"/revoke", tenantBToken, "")
	req.SetPathValue("id", created.ID)
	ta.h.RevokeTenantAPIKey(w, req)
	if w.Code != http.StatusForbidden {
		t.Fatalf("cross-tenant revoke status = %d, want %d", w.Code, http.StatusForbidden)
	}

	// Tenant B must not see tenant A's key in its own list.
	w = httptest.NewRecorder()
	ta.h.ListTenantAPIKeys(w, newAdminRequest(http.MethodGet, "/api-keys", tenantBToken, ""))
	var listed []apiKeyReadResponse
	err = json.Unmarshal(w.Body.Bytes(), &listed)
	if err != nil {
		t.Fatalf("unmarshal list: %v", err)
	}
	for _, k := range listed {
		if k.ID == created.ID {
			t.Fatal("tenant B's key list must not include tenant A's key")
		}
	}
}

func TestRevokeTenantAPIKeyInvalidatesConfigCache(t *testing.T) {
	t.Parallel()

	ta := newTestAdmin(t)
	tenantAdminToken := ta.seedTenantKey(t, adminTestKeyAAdmin, adminTestTenantA, RoleTenantAdmin)
	var err error
	w := httptest.NewRecorder()
	ta.h.CreateTenantAPIKey(w, newAdminRequest(http.MethodPost, "/api-keys", tenantAdminToken, `{"role":"requester"}`))
	var created apiKeyCreateResponse
	err = json.Unmarshal(w.Body.Bytes(), &created)
	if err != nil {
		t.Fatalf("unmarshal: %v", err)
	}

	before, err := ta.h.ConfigCache.Snapshot(context.Background(), adminTestTenantA)
	if err != nil {
		t.Fatalf("Snapshot() before revoke error = %v", err)
	}

	w = httptest.NewRecorder()
	req := newAdminRequest(http.MethodPost, "/api-keys/"+created.ID+"/revoke", tenantAdminToken, "")
	req.SetPathValue("id", created.ID)
	ta.h.RevokeTenantAPIKey(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("revoke status = %d, want %d, body=%s", w.Code, http.StatusOK, w.Body.String())
	}

	after, err := ta.h.ConfigCache.Snapshot(context.Background(), adminTestTenantA)
	if err != nil {
		t.Fatalf("Snapshot() after revoke error = %v", err)
	}
	if after.ConfigVersion <= before.ConfigVersion {
		t.Fatalf("tenant config version after revoke = %d, want > %d", after.ConfigVersion, before.ConfigVersion)
	}
	found := false
	for _, id := range after.RevokedAPIKeyIDs {
		if id == created.ID {
			found = true
		}
	}
	if !found {
		t.Fatalf("RevokedAPIKeyIDs = %v, want to include %q", after.RevokedAPIKeyIDs, created.ID)
	}
}

// ---- quota write requires platform key ----

func TestQuotaWriteRequiresPlatformKey(t *testing.T) {
	t.Parallel()

	ta := newTestAdmin(t)
	tenant := Tenant{ID: adminTestTenantA, Name: "A", Status: TenantStatusActive, CreatedAt: time.Now().UTC()}
	err := ta.tenants.Create(context.Background(), tenant)
	if err != nil {
		t.Fatalf("seed tenant: %v", err)
	}
	tenantAdminToken := ta.seedTenantKey(t, adminTestKeyAAdmin, adminTestTenantA, RoleTenantAdmin)
	adminToken := ta.seedPlatformKey(t, "key_admin", RoleSystemAdmin)

	body := `{"expected_config_version":0,"period":"monthly","max_requests":1000,"max_bandwidth_bytes":100000,"request_count_policy":"count_on_admission","redis_fail_policy":"closed"}`

	// Tenant key rejected.
	w := httptest.NewRecorder()
	req := newAdminRequest(http.MethodPut, "/tenants/ten_a/quotas", tenantAdminToken, body)
	req.SetPathValue("id", adminTestTenantA)
	ta.h.PutTenantQuotas(w, req)
	if w.Code != http.StatusForbidden {
		t.Fatalf("tenant key quota write status = %d, want %d", w.Code, http.StatusForbidden)
	}

	// Platform key succeeds.
	w = httptest.NewRecorder()
	req = newAdminRequest(http.MethodPut, "/tenants/ten_a/quotas", adminToken, body)
	req.SetPathValue("id", adminTestTenantA)
	ta.h.PutTenantQuotas(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("platform key quota write status = %d, want %d, body=%s", w.Code, http.StatusOK, w.Body.String())
	}

	// Tenant retains read-only access.
	w = httptest.NewRecorder()
	ta.h.GetQuotas(w, newAdminRequest(http.MethodGet, "/quotas", tenantAdminToken, ""))
	if w.Code != http.StatusOK {
		t.Fatalf("tenant quota read status = %d, want %d", w.Code, http.StatusOK)
	}
	var quota quotaResponse
	err = json.Unmarshal(w.Body.Bytes(), &quota)
	if err != nil {
		t.Fatalf("unmarshal quota: %v", err)
	}
	if quota.MaxRequests != 1000 {
		t.Fatalf("max_requests = %d, want 1000", quota.MaxRequests)
	}
}

// ---- rate limit config: dimension write, tenant read, ceiling rejection ----

func TestRateLimitsWriteRequiresTenantAdmin(t *testing.T) {
	t.Parallel()

	ta := newTestAdmin(t)
	tenant := Tenant{ID: adminTestTenantA, Name: "A", Status: TenantStatusActive, CreatedAt: time.Now().UTC()}
	err := ta.tenants.Create(context.Background(), tenant)
	if err != nil {
		t.Fatalf("seed tenant: %v", err)
	}

	tenantAdminToken := ta.seedTenantKey(t, adminTestKeyAAdmin, adminTestTenantA, RoleTenantAdmin)
	viewerToken := ta.seedTenantKey(t, "key_a_viewer", adminTestTenantA, RoleViewer)

	body := `{"expected_config_version":0,"limits":[{"dimension":"tenant","key":"*","window_seconds":60,"max_requests":600,"fail_policy":"open"}]}`

	// Viewer rejected.
	w := httptest.NewRecorder()
	ta.h.PutRateLimits(w, newAdminRequest(http.MethodPut, "/rate-limits", viewerToken, body))
	if w.Code != http.StatusForbidden {
		t.Fatalf("viewer write status = %d, want %d", w.Code, http.StatusForbidden)
	}

	// Tenant admin succeeds.
	w = httptest.NewRecorder()
	ta.h.PutRateLimits(w, newAdminRequest(http.MethodPut, "/rate-limits", tenantAdminToken, body))
	if w.Code != http.StatusOK {
		t.Fatalf("tenant_admin write status = %d, want %d, body=%s", w.Code, http.StatusOK, w.Body.String())
	}

	var saved rateLimitResponse

	err = json.Unmarshal(w.Body.Bytes(), &saved)
	if err != nil {
		t.Fatalf("unmarshal: %v", err)
	}

	if len(saved.Limits) != 1 || saved.Limits[0].MaxRequests != 600 {
		t.Fatalf("saved limits = %+v, want one rule with max_requests=600", saved.Limits)
	}

	// Viewer retains read access.
	w = httptest.NewRecorder()
	ta.h.GetRateLimits(w, newAdminRequest(http.MethodGet, "/rate-limits", viewerToken, ""))
	if w.Code != http.StatusOK {
		t.Fatalf("viewer read status = %d, want %d", w.Code, http.StatusOK)
	}
}

// TestRateLimitsCeilingRejectsExceedingWrite proves the HTTP surface for
// docs/planning/26: tenant-managed rate-limit values above the
// system_admin-set rate_limit_ceiling are rejected with invalid_request.
func TestRateLimitsCeilingRejectsExceedingWrite(t *testing.T) {
	t.Parallel()

	ta := newTestAdmin(t)
	tenant := Tenant{
		ID: adminTestTenantA, Name: "A", Status: TenantStatusActive, CreatedAt: time.Now().UTC(),
		RateLimitCeiling: &RateLimitCeiling{WindowSeconds: 60, MaxRequests: 100},
	}

	err := ta.tenants.Create(context.Background(), tenant)
	if err != nil {
		t.Fatalf("seed tenant: %v", err)
	}

	tenantAdminToken := ta.seedTenantKey(t, adminTestKeyAAdmin, adminTestTenantA, RoleTenantAdmin)

	// 200 req / 60s exceeds the 100 req / 60s ceiling.
	body := `{"expected_config_version":0,"limits":[{"dimension":"tenant","key":"*","window_seconds":60,"max_requests":200,"fail_policy":"open"}]}`

	w := httptest.NewRecorder()
	ta.h.PutRateLimits(w, newAdminRequest(http.MethodPut, "/rate-limits", tenantAdminToken, body))
	if w.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want %d, body=%s", w.Code, http.StatusBadRequest, w.Body.String())
	}

	var errResp ErrorResponse

	err = json.Unmarshal(w.Body.Bytes(), &errResp)
	if err != nil {
		t.Fatalf("unmarshal error: %v", err)
	}

	if errResp.Code != adminTestInvalidRequest {
		t.Fatalf("code = %q, want %q", errResp.Code, adminTestInvalidRequest)
	}
}

func TestQuotaAndRateLimitWritesUseTransactionalConfigStore(t *testing.T) {
	t.Parallel()

	ta := newTestAdmin(t)
	writes := &recordingConfigWrites{}
	ta.h.ConfigWrites = writes
	adminToken := ta.seedPlatformKey(t, "platform_cfg_admin", RoleSystemAdmin)
	tenantToken := ta.seedTenantKey(t, adminTestKeyTenantAdmin, adminTestTenantA, RoleTenantAdmin)
	err := ta.tenants.Create(context.Background(), Tenant{
		ID: adminTestTenantA, Name: "A", Status: TenantStatusActive, CreatedAt: time.Now().UTC(),
		RateLimitCeiling: &RateLimitCeiling{WindowSeconds: 60, MaxRequests: 100},
	})
	if err != nil {
		t.Fatalf("seed tenant: %v", err)
	}

	quotaBody := `{"expected_config_version":0,"period":"monthly","max_requests":10,"max_bandwidth_bytes":2048,"request_count_policy":"count_on_admission","redis_fail_policy":"fail_closed"}`
	quotaReq := newAdminRequest(http.MethodPut, "/tenants/"+adminTestTenantA+"/quotas", adminToken, quotaBody)
	quotaReq.SetPathValue("id", adminTestTenantA)
	quotaResp := httptest.NewRecorder()
	ta.h.PutTenantQuotas(quotaResp, quotaReq)
	if quotaResp.Code != http.StatusOK {
		t.Fatalf("quota status = %d, want %d, body=%s", quotaResp.Code, http.StatusOK, quotaResp.Body.String())
	}
	if !writes.quotaCalled {
		t.Fatal("quota write did not use transactional config writer")
	}
	if writes.quota.TenantID != adminTestTenantA || writes.quota.MaxRequests != 10 || writes.quotaExpected != 0 {
		t.Fatalf("quota write = %+v expected=%d", writes.quota, writes.quotaExpected)
	}
	if writes.quotaActor.ActorID != "platform_cfg_admin" || writes.quotaActor.ActorType != configActorTypeAPIKey {
		t.Fatalf("quota actor = %+v, want platform API key actor", writes.quotaActor)
	}

	rateBody := `{"expected_config_version":0,"limits":[{"dimension":"tenant","key":"*","window_seconds":60,"max_requests":50,"fail_policy":"open"}]}`
	rateReq := newAdminRequest(http.MethodPut, "/rate-limits", tenantToken, rateBody)
	rateResp := httptest.NewRecorder()
	ta.h.PutRateLimits(rateResp, rateReq)
	if rateResp.Code != http.StatusOK {
		t.Fatalf("rate status = %d, want %d, body=%s", rateResp.Code, http.StatusOK, rateResp.Body.String())
	}
	if !writes.rateCalled {
		t.Fatal("rate-limit write did not use transactional config writer")
	}
	if writes.rate.TenantID != adminTestTenantA || len(writes.rate.Limits) != 1 || writes.rateExpected != 0 {
		t.Fatalf("rate write = %+v expected=%d", writes.rate, writes.rateExpected)
	}
	if writes.rateCeiling == nil || writes.rateCeiling.MaxRequests != 100 {
		t.Fatalf("rate ceiling = %+v, want tenant ceiling", writes.rateCeiling)
	}
	if writes.rateActor.ActorID != adminTestKeyTenantAdmin || writes.rateActor.ActorType != configActorTypeAPIKey {
		t.Fatalf("rate actor = %+v, want tenant API key actor", writes.rateActor)
	}
}

// ---- worker-credential create rejects foreign tenant scope ----

func TestWorkerCredentialCreateRejectsForeignTenantScope(t *testing.T) {
	t.Parallel()

	ta := newTestAdmin(t)
	tenantAdminToken := ta.seedTenantKey(t, adminTestKeyAAdmin, adminTestTenantA, RoleTenantAdmin)

	body := `{"executor_type":"egress","allowed_pools":[{"tenant_id":"ten_b","pool_id":"pool_x"}],"public_key_ed25519_base64":"YWJjZA=="}`
	w := httptest.NewRecorder()
	ta.h.CreateWorkerCredential(w, newAdminRequest(http.MethodPost, "/worker-credentials", tenantAdminToken, body))
	if w.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want %d, body=%s", w.Code, http.StatusBadRequest, w.Body.String())
	}
	var errResp ErrorResponse
	err := json.Unmarshal(w.Body.Bytes(), &errResp)
	if err != nil {
		t.Fatalf("unmarshal error: %v", err)
	}
	if errResp.Code != adminTestInvalidRequest {
		t.Fatalf("code = %q, want %q", errResp.Code, adminTestInvalidRequest)
	}
}

func TestWorkerCredentialCreateForcesCallerTenantScope(t *testing.T) {
	t.Parallel()

	ta := newTestAdmin(t)
	tenantAdminToken := ta.seedTenantKey(t, adminTestKeyAAdmin, adminTestTenantA, RoleTenantAdmin)

	body := `{"executor_type":"egress","allowed_pools":[{"tenant_id":"ten_a","pool_id":"pool_x"}],"public_key_ed25519_base64":"YWJjZA=="}`
	w := httptest.NewRecorder()
	ta.h.CreateWorkerCredential(w, newAdminRequest(http.MethodPost, "/worker-credentials", tenantAdminToken, body))
	if w.Code != http.StatusCreated {
		t.Fatalf("status = %d, want %d, body=%s", w.Code, http.StatusCreated, w.Body.String())
	}
	var created workerCredentialResponse
	err := json.Unmarshal(w.Body.Bytes(), &created)
	if err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if len(created.TenantScope) != 1 || created.TenantScope[0] != adminTestTenantA {
		t.Fatalf("tenant_scope = %v, want [ten_a]", created.TenantScope)
	}
}

func TestWorkerCredentialRevokeInvalidatesAcrossTenants(t *testing.T) {
	t.Parallel()

	ta := newTestAdmin(t)
	tenantAToken := ta.seedTenantKey(t, adminTestKeyAAdmin, adminTestTenantA, RoleTenantAdmin)
	tenantBToken := ta.seedTenantKey(t, adminTestKeyBAdmin, adminTestTenantB, RoleTenantAdmin)

	body := `{"executor_type":"egress","allowed_pools":[],"public_key_ed25519_base64":"YWJjZA=="}`
	w := httptest.NewRecorder()
	ta.h.CreateWorkerCredential(w, newAdminRequest(http.MethodPost, "/worker-credentials", tenantAToken, body))
	var created workerCredentialResponse
	err := json.Unmarshal(w.Body.Bytes(), &created)
	if err != nil {
		t.Fatalf("unmarshal: %v", err)
	}

	w = httptest.NewRecorder()
	req := newAdminRequest(http.MethodPost, "/worker-credentials/"+created.ID+"/revoke", tenantBToken, "")
	req.SetPathValue("id", created.ID)
	ta.h.RevokeWorkerCredential(w, req)
	if w.Code != http.StatusForbidden {
		t.Fatalf("cross-tenant revoke status = %d, want %d", w.Code, http.StatusForbidden)
	}
}
