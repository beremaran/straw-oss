package control

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

// testAdmin bundles an AdminHandlers with its backing stores for direct
// inspection in tests (e.g. asserting on AuditStore contents).
type testAdmin struct {
	h           *AdminHandlers
	apiKeys     *InMemoryAPIKeyStore
	workerCreds *InMemoryWorkerCredentialStore
	tenants     *InMemoryTenantStore
	quotas      *InMemoryQuotaStore
	audit       *InMemoryAuditStore
	pepper      []byte
}

func newTestAdmin(t *testing.T) *testAdmin {
	t.Helper()

	apiKeys := NewInMemoryAPIKeyStore()
	pepper := []byte("test-pepper")
	snapshotStore := NewInMemorySnapshotStore()
	cache := NewConfigCache(snapshotStore, nil)

	ta := &testAdmin{
		apiKeys:     apiKeys,
		workerCreds: NewInMemoryWorkerCredentialStore(),
		tenants:     NewInMemoryTenantStore(),
		quotas:      NewInMemoryQuotaStore(),
		audit:       NewInMemoryAuditStore(),
		pepper:      pepper,
	}
	ta.h = &AdminHandlers{
		Authenticator: NewAuthenticator(apiKeys, pepper),
		APIKeys:       apiKeys,
		WorkerCreds:   ta.workerCreds,
		Tenants:       ta.tenants,
		Quotas:        ta.quotas,
		Audit:         ta.audit,
		ConfigCache:   cache,
		Pepper:        pepper,
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
	req := httptest.NewRequest(method, path, strings.NewReader(body))
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
	if err := json.Unmarshal(w.Body.Bytes(), &created); err != nil {
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
	if err := json.Unmarshal(w.Body.Bytes(), &listed); err != nil {
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
	if _, err := auth.Authenticate(context.Background(), "Bearer "+created.Secret); err != nil {
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
	if _, err := auth.Authenticate(context.Background(), "Bearer "+created.Secret); err != ErrAuthFailure {
		t.Fatalf("Authenticate() after revoke error = %v, want ErrAuthFailure", err)
	}
}

func TestPlatformAPIKeyLifecycleRequiresSystemAdmin(t *testing.T) {
	t.Parallel()

	ta := newTestAdmin(t)
	tenantToken := ta.seedTenantKey(t, "key_tenant_admin", "ten_a", RoleTenantAdmin)

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
	tenantAdminToken := ta.seedTenantKey(t, "key_tenant_admin", "ten_a", RoleTenantAdmin)

	w := httptest.NewRecorder()
	ta.h.CreateTenant(w, newAdminRequest(http.MethodPost, "/tenants", tenantAdminToken, `{"name":"Sneaky Tenant"}`))
	if w.Code != http.StatusForbidden {
		t.Fatalf("status = %d, want %d", w.Code, http.StatusForbidden)
	}
	var errResp ErrorResponse
	if err := json.Unmarshal(w.Body.Bytes(), &errResp); err != nil {
		t.Fatalf("unmarshal error: %v", err)
	}
	if errResp.Code != "insufficient_permissions" {
		t.Fatalf("code = %q, want %q", errResp.Code, "insufficient_permissions")
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
	if rec.ActorType != "api_key" {
		t.Fatalf("actor_type = %q, want %q", rec.ActorType, "api_key")
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
	tenantAToken := ta.seedTenantKey(t, "key_a_admin", "ten_a", RoleTenantAdmin)
	tenantBToken := ta.seedTenantKey(t, "key_b_admin", "ten_b", RoleTenantAdmin)

	// Tenant A creates a key.
	w := httptest.NewRecorder()
	ta.h.CreateTenantAPIKey(w, newAdminRequest(http.MethodPost, "/api-keys", tenantAToken, `{"role":"requester"}`))
	if w.Code != http.StatusCreated {
		t.Fatalf("create status = %d, want %d, body=%s", w.Code, http.StatusCreated, w.Body.String())
	}
	var created apiKeyCreateResponse
	if err := json.Unmarshal(w.Body.Bytes(), &created); err != nil {
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
	if err := json.Unmarshal(w.Body.Bytes(), &listed); err != nil {
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
	tenantAdminToken := ta.seedTenantKey(t, "key_a_admin", "ten_a", RoleTenantAdmin)

	w := httptest.NewRecorder()
	ta.h.CreateTenantAPIKey(w, newAdminRequest(http.MethodPost, "/api-keys", tenantAdminToken, `{"role":"requester"}`))
	var created apiKeyCreateResponse
	if err := json.Unmarshal(w.Body.Bytes(), &created); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}

	before, err := ta.h.ConfigCache.Snapshot(context.Background(), "ten_a")
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

	after, err := ta.h.ConfigCache.Snapshot(context.Background(), "ten_a")
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
	tenant := Tenant{ID: "ten_a", Name: "A", Status: TenantStatusActive, CreatedAt: time.Now().UTC()}
	if err := ta.tenants.Create(context.Background(), tenant); err != nil {
		t.Fatalf("seed tenant: %v", err)
	}
	tenantAdminToken := ta.seedTenantKey(t, "key_a_admin", "ten_a", RoleTenantAdmin)
	adminToken := ta.seedPlatformKey(t, "key_admin", RoleSystemAdmin)

	body := `{"expected_config_version":0,"period":"monthly","max_requests":1000,"max_bandwidth_bytes":100000,"request_count_policy":"count_on_admission","redis_fail_policy":"closed"}`

	// Tenant key rejected.
	w := httptest.NewRecorder()
	req := newAdminRequest(http.MethodPut, "/tenants/ten_a/quotas", tenantAdminToken, body)
	req.SetPathValue("id", "ten_a")
	ta.h.PutTenantQuotas(w, req)
	if w.Code != http.StatusForbidden {
		t.Fatalf("tenant key quota write status = %d, want %d", w.Code, http.StatusForbidden)
	}

	// Platform key succeeds.
	w = httptest.NewRecorder()
	req = newAdminRequest(http.MethodPut, "/tenants/ten_a/quotas", adminToken, body)
	req.SetPathValue("id", "ten_a")
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
	if err := json.Unmarshal(w.Body.Bytes(), &quota); err != nil {
		t.Fatalf("unmarshal quota: %v", err)
	}
	if quota.MaxRequests != 1000 {
		t.Fatalf("max_requests = %d, want 1000", quota.MaxRequests)
	}
}

// ---- worker-credential create rejects foreign tenant scope ----

func TestWorkerCredentialCreateRejectsForeignTenantScope(t *testing.T) {
	t.Parallel()

	ta := newTestAdmin(t)
	tenantAdminToken := ta.seedTenantKey(t, "key_a_admin", "ten_a", RoleTenantAdmin)

	body := `{"executor_type":"egress","allowed_pools":[{"tenant_id":"ten_b","pool_id":"pool_x"}],"public_key_ed25519_base64":"YWJjZA=="}`
	w := httptest.NewRecorder()
	ta.h.CreateWorkerCredential(w, newAdminRequest(http.MethodPost, "/worker-credentials", tenantAdminToken, body))
	if w.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want %d, body=%s", w.Code, http.StatusBadRequest, w.Body.String())
	}
	var errResp ErrorResponse
	if err := json.Unmarshal(w.Body.Bytes(), &errResp); err != nil {
		t.Fatalf("unmarshal error: %v", err)
	}
	if errResp.Code != "invalid_request" {
		t.Fatalf("code = %q, want %q", errResp.Code, "invalid_request")
	}
}

func TestWorkerCredentialCreateForcesCallerTenantScope(t *testing.T) {
	t.Parallel()

	ta := newTestAdmin(t)
	tenantAdminToken := ta.seedTenantKey(t, "key_a_admin", "ten_a", RoleTenantAdmin)

	body := `{"executor_type":"egress","allowed_pools":[{"tenant_id":"ten_a","pool_id":"pool_x"}],"public_key_ed25519_base64":"YWJjZA=="}`
	w := httptest.NewRecorder()
	ta.h.CreateWorkerCredential(w, newAdminRequest(http.MethodPost, "/worker-credentials", tenantAdminToken, body))
	if w.Code != http.StatusCreated {
		t.Fatalf("status = %d, want %d, body=%s", w.Code, http.StatusCreated, w.Body.String())
	}
	var created workerCredentialResponse
	if err := json.Unmarshal(w.Body.Bytes(), &created); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if len(created.TenantScope) != 1 || created.TenantScope[0] != "ten_a" {
		t.Fatalf("tenant_scope = %v, want [ten_a]", created.TenantScope)
	}
}

func TestWorkerCredentialRevokeInvalidatesAcrossTenants(t *testing.T) {
	t.Parallel()

	ta := newTestAdmin(t)
	tenantAToken := ta.seedTenantKey(t, "key_a_admin", "ten_a", RoleTenantAdmin)
	tenantBToken := ta.seedTenantKey(t, "key_b_admin", "ten_b", RoleTenantAdmin)

	body := `{"executor_type":"egress","allowed_pools":[],"public_key_ed25519_base64":"YWJjZA=="}`
	w := httptest.NewRecorder()
	ta.h.CreateWorkerCredential(w, newAdminRequest(http.MethodPost, "/worker-credentials", tenantAToken, body))
	var created workerCredentialResponse
	if err := json.Unmarshal(w.Body.Bytes(), &created); err != nil {
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
