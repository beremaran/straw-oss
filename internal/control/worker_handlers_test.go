package control

import (
	"context"
	"crypto/ed25519"
	"encoding/base64"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	strawpb "github.com/beremaran/straw/v2/api/proto/straw/v1"
)

// seedRegisteredWorker wires a WorkerRegistry into the admin harness, creates
// a worker credential for the given tenants, and registers+heartbeats a ready
// worker. It returns the registry so tests can inspect/act on it.
func (ta *testAdmin) seedRegisteredWorker(t *testing.T, workerID string, tenants []string) *WorkerRegistry {
	t.Helper()
	pub, priv, err := ed25519.GenerateKey(nil)
	if err != nil {
		t.Fatalf("GenerateKey: %v", err)
	}
	pools := make([]AllowedPool, 0, len(tenants))
	poolRefs := make([]*strawpb.RegisterRequest_PoolRef, 0, len(tenants))
	for _, tid := range tenants {
		pools = append(pools, AllowedPool{TenantID: tid, PoolID: "pool_1"})
		poolRefs = append(poolRefs, &strawpb.RegisterRequest_PoolRef{TenantId: tid, PoolId: "pool_1"})
	}
	cred := WorkerCredential{
		ID: "wcred_1", Status: WorkerCredentialStatusActive, ExecutorType: "egress",
		TenantScope: tenants, AllowedPools: pools,
		PublicKeyEd25519Base64: base64.StdEncoding.EncodeToString(pub),
	}
	if err := ta.workerCreds.Create(context.Background(), cred); err != nil {
		t.Fatalf("create cred: %v", err)
	}
	reg := NewWorkerRegistry(ta.workerCreds, DefaultWorkerTimings(), time.Now)
	ta.h.Workers = reg

	req := &strawpb.RegisterRequest{
		WorkerId: workerID, ExecutorType: "egress", CredentialId: "wcred_1",
		ProtocolMajor: ProtocolMajor, AllowedPools: poolRefs,
	}
	req.SignedToken = strawpb.SignRegistration(priv, req)
	out, err := reg.Register(context.Background(), req)
	if err != nil || !out.OK {
		t.Fatalf("register worker: %+v err=%v", out, err)
	}
	reg.Heartbeat(&strawpb.HeartbeatRequest{WorkerId: workerID, SessionId: out.SessionID, Health: strawpb.WorkerHealth_WORKER_HEALTH_READY})
	return reg
}

func TestListWorkersPlatformSeesSessionTenantDoesNot(t *testing.T) {
	t.Parallel()
	ta := newTestAdmin(t)
	reg := ta.seedRegisteredWorker(t, "worker-1", []string{"ten_a"})

	platformToken := ta.seedPlatformKey(t, "key_admin", RoleSystemAdmin)
	w := httptest.NewRecorder()
	ta.h.ListWorkers(w, newAdminRequest(http.MethodGet, "/api/v1/admin/workers", platformToken, ""))
	if w.Code != http.StatusOK {
		t.Fatalf("platform list status = %d", w.Code)
	}
	var platform []workerAdminView
	if err := json.Unmarshal(w.Body.Bytes(), &platform); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if len(platform) != 1 || platform[0].SessionID == "" || platform[0].AssignSubject == "" {
		t.Fatalf("platform view missing session/subject: %+v", platform)
	}

	tenantToken := ta.seedTenantKey(t, "key_ta", "ten_a", RoleTenantAdmin)
	w = httptest.NewRecorder()
	ta.h.ListWorkers(w, newAdminRequest(http.MethodGet, "/api/v1/admin/workers", tenantToken, ""))
	if w.Code != http.StatusOK {
		t.Fatalf("tenant list status = %d", w.Code)
	}
	var tenant []workerAdminView
	if err := json.Unmarshal(w.Body.Bytes(), &tenant); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if len(tenant) != 1 || tenant[0].WorkerID != "worker-1" {
		t.Fatalf("tenant view = %+v, want worker-1", tenant)
	}
	if tenant[0].SessionID != "" || tenant[0].AssignSubject != "" {
		t.Fatalf("tenant view leaked session/subject: %+v", tenant[0])
	}
	_ = reg
}

func TestTenantWorkerListOmitsOtherTenants(t *testing.T) {
	t.Parallel()
	ta := newTestAdmin(t)
	ta.seedRegisteredWorker(t, "worker-1", []string{"ten_a"})

	// A tenant_admin for ten_b sees no workers (worker-1 is scoped to ten_a).
	tenantToken := ta.seedTenantKey(t, "key_tb", "ten_b", RoleTenantAdmin)
	w := httptest.NewRecorder()
	ta.h.ListWorkers(w, newAdminRequest(http.MethodGet, "/api/v1/admin/workers", tenantToken, ""))
	var views []workerAdminView
	if err := json.Unmarshal(w.Body.Bytes(), &views); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if len(views) != 0 {
		t.Fatalf("ten_b listing = %+v, want empty", views)
	}
}

func TestGlobalWorkerActionRequiresSystemAdmin(t *testing.T) {
	t.Parallel()
	ta := newTestAdmin(t)
	ta.seedRegisteredWorker(t, "worker-1", []string{"ten_a"})

	// A tenant_admin cannot perform a global disable.
	tenantToken := ta.seedTenantKey(t, "key_ta", "ten_a", RoleTenantAdmin)
	w := httptest.NewRecorder()
	req := newAdminRequest(http.MethodPost, "/api/v1/admin/workers/worker-1/disable", tenantToken, "")
	req.SetPathValue("worker_id", "worker-1")
	ta.h.DisableWorker(w, req)
	if w.Code != http.StatusForbidden {
		t.Fatalf("tenant global disable status = %d, want 403", w.Code)
	}

	// system_admin can, and it takes effect.
	platformToken := ta.seedPlatformKey(t, "key_admin", RoleSystemAdmin)
	w = httptest.NewRecorder()
	req = newAdminRequest(http.MethodPost, "/api/v1/admin/workers/worker-1/disable", platformToken, "")
	req.SetPathValue("worker_id", "worker-1")
	ta.h.DisableWorker(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("system_admin disable status = %d, want 200", w.Code)
	}
	if ta.h.Workers.EligibleForTenant("worker-1", "ten_a") {
		t.Fatal("worker should be globally disabled")
	}
}

func TestTenantWorkerActionAffectsOnlyThatTenant(t *testing.T) {
	t.Parallel()
	ta := newTestAdmin(t)
	ta.seedRegisteredWorker(t, "worker-1", []string{"ten_a", "ten_b"})

	tenantToken := ta.seedTenantKey(t, "key_ta", "ten_a", RoleTenantAdmin)
	w := httptest.NewRecorder()
	req := newAdminRequest(http.MethodPost, "/api/v1/admin/workers/worker-1/tenant-disable", tenantToken, "")
	req.SetPathValue("worker_id", "worker-1")
	ta.h.TenantDisableWorker(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("tenant-disable status = %d, want 200", w.Code)
	}
	if ta.h.Workers.EligibleForTenant("worker-1", "ten_a") {
		t.Fatal("ten_a should be disabled for this worker")
	}
	if !ta.h.Workers.EligibleForTenant("worker-1", "ten_b") {
		t.Fatal("ten_b eligibility must be unaffected by ten_a tenant-disable")
	}
}

func TestTenantDisableRequiresTenantAdminNotOperator(t *testing.T) {
	t.Parallel()
	ta := newTestAdmin(t)
	ta.seedRegisteredWorker(t, "worker-1", []string{"ten_a"})

	// operator may drain but not disable (per doc 26 role table).
	operatorToken := ta.seedTenantKey(t, "key_op", "ten_a", RoleOperator)
	w := httptest.NewRecorder()
	req := newAdminRequest(http.MethodPost, "/api/v1/admin/workers/worker-1/tenant-disable", operatorToken, "")
	req.SetPathValue("worker_id", "worker-1")
	ta.h.TenantDisableWorker(w, req)
	if w.Code != http.StatusForbidden {
		t.Fatalf("operator tenant-disable status = %d, want 403", w.Code)
	}

	// operator may tenant-drain.
	w = httptest.NewRecorder()
	req = newAdminRequest(http.MethodPost, "/api/v1/admin/workers/worker-1/tenant-drain", operatorToken, "")
	req.SetPathValue("worker_id", "worker-1")
	ta.h.TenantDrainWorker(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("operator tenant-drain status = %d, want 200", w.Code)
	}
	if ta.h.Workers.EligibleForTenant("worker-1", "ten_a") {
		t.Fatal("operator tenant-drain should exclude the worker")
	}
}
