package control

import (
	"context"
	"crypto/ed25519"
	"crypto/rand"
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
		pools = append(pools, AllowedPool{TenantID: tid, PoolID: routingTestPool1})
		poolRefs = append(poolRefs, &strawpb.RegisterRequest_PoolRef{TenantId: tid, PoolId: routingTestPool1})
	}
	cred := WorkerCredential{
		ID: "wcred_1", Status: WorkerCredentialStatusActive, ExecutorType: routingTestEgress,
		TenantScope: tenants, AllowedPools: pools,
		PublicKeyEd25519Base64: base64.StdEncoding.EncodeToString(pub),
	}
	createErr := ta.workerCreds.Create(context.Background(), cred)
	if createErr != nil {
		t.Fatalf("create cred: %v", createErr)
	}
	reg := NewWorkerRegistry(ta.workerCreds, DefaultWorkerTimings(), time.Now)
	ta.h.Workers = reg

	nonce := make([]byte, 8)
	_, _ = rand.Read(nonce)
	req := &strawpb.RegisterRequest{
		WorkerId: workerID, ExecutorType: routingTestEgress, CredentialId: "wcred_1",
		ProtocolMajor: ProtocolMajor, AllowedPools: poolRefs,
		Nonce: nonce, IssuedAtUnixMs: time.Now().UnixMilli(),
	}
	req.SignedToken = strawpb.SignRegistration(priv, req)
	out, err := reg.Register(context.Background(), req)
	if err != nil || !out.OK {
		t.Fatalf("register worker: %+v err=%v", out, err)
	}
	_, err = reg.Heartbeat(&strawpb.HeartbeatRequest{WorkerId: workerID, SessionId: out.SessionID, Health: strawpb.WorkerHealth_WORKER_HEALTH_READY})
	if err != nil {
		t.Fatalf("heartbeat: %v", err)
	}

	return reg
}

func TestListWorkersPlatformSeesSessionTenantDoesNot(t *testing.T) {
	t.Parallel()
	ta := newTestAdmin(t)
	reg := ta.seedRegisteredWorker(t, routingTestWorker1, []string{adminTestTenantA})

	platformToken := ta.seedPlatformKey(t, "key_admin", RoleSystemAdmin)
	w := httptest.NewRecorder()
	ta.h.ListWorkers(w, newAdminRequest(http.MethodGet, "/api/v1/admin/workers", platformToken, ""))
	if w.Code != http.StatusOK {
		t.Fatalf("platform list status = %d", w.Code)
	}
	var platform []workerAdminView
	err := json.Unmarshal(w.Body.Bytes(), &platform)
	if err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if len(platform) != 1 || platform[0].SessionID == "" || platform[0].AssignSubject == "" {
		t.Fatalf("platform view missing session/subject: %+v", platform)
	}

	tenantToken := ta.seedTenantKey(t, "key_ta", adminTestTenantA, RoleTenantAdmin)
	w = httptest.NewRecorder()
	ta.h.ListWorkers(w, newAdminRequest(http.MethodGet, "/api/v1/admin/workers", tenantToken, ""))
	if w.Code != http.StatusOK {
		t.Fatalf("tenant list status = %d", w.Code)
	}
	var tenant []workerAdminView
	err = json.Unmarshal(w.Body.Bytes(), &tenant)
	if err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if len(tenant) != 1 || tenant[0].WorkerID != routingTestWorker1 {
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
	ta.seedRegisteredWorker(t, routingTestWorker1, []string{adminTestTenantA})

	// A tenant_admin for ten_b sees no workers (worker-1 is scoped to ten_a).
	tenantToken := ta.seedTenantKey(t, "key_tb", "ten_b", RoleTenantAdmin)
	w := httptest.NewRecorder()
	ta.h.ListWorkers(w, newAdminRequest(http.MethodGet, "/api/v1/admin/workers", tenantToken, ""))
	var views []workerAdminView
	err := json.Unmarshal(w.Body.Bytes(), &views)
	if err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if len(views) != 0 {
		t.Fatalf("ten_b listing = %+v, want empty", views)
	}
}

func TestGlobalWorkerActionRequiresSystemAdmin(t *testing.T) {
	t.Parallel()
	ta := newTestAdmin(t)
	ta.seedRegisteredWorker(t, routingTestWorker1, []string{adminTestTenantA})

	// A tenant_admin cannot perform a global disable.
	tenantToken := ta.seedTenantKey(t, "key_ta", adminTestTenantA, RoleTenantAdmin)
	w := httptest.NewRecorder()
	req := newAdminRequest(http.MethodPost, "/api/v1/admin/workers/worker-1/disable", tenantToken, "")
	req.SetPathValue("worker_id", routingTestWorker1)
	ta.h.DisableWorker(w, req)
	if w.Code != http.StatusForbidden {
		t.Fatalf("tenant global disable status = %d, want 403", w.Code)
	}

	// system_admin can, and it takes effect.
	platformToken := ta.seedPlatformKey(t, "key_admin", RoleSystemAdmin)
	w = httptest.NewRecorder()
	req = newAdminRequest(http.MethodPost, "/api/v1/admin/workers/worker-1/disable", platformToken, "")
	req.SetPathValue("worker_id", routingTestWorker1)
	ta.h.DisableWorker(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("system_admin disable status = %d, want 200", w.Code)
	}
	if ta.h.Workers.EligibleForTenant(routingTestWorker1, adminTestTenantA) {
		t.Fatal("worker should be globally disabled")
	}
}

func TestTenantWorkerActionAffectsOnlyThatTenant(t *testing.T) {
	t.Parallel()
	ta := newTestAdmin(t)
	ta.seedRegisteredWorker(t, routingTestWorker1, []string{adminTestTenantA, adminTestTenantB})

	tenantToken := ta.seedTenantKey(t, "key_ta", adminTestTenantA, RoleTenantAdmin)
	w := httptest.NewRecorder()
	req := newAdminRequest(http.MethodPost, "/api/v1/admin/workers/worker-1/tenant-disable", tenantToken, "")
	req.SetPathValue("worker_id", routingTestWorker1)
	ta.h.TenantDisableWorker(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("tenant-disable status = %d, want 200", w.Code)
	}
	if ta.h.Workers.EligibleForTenant(routingTestWorker1, adminTestTenantA) {
		t.Fatal("ten_a should be disabled for this worker")
	}
	if !ta.h.Workers.EligibleForTenant(routingTestWorker1, adminTestTenantB) {
		t.Fatal("ten_b eligibility must be unaffected by ten_a tenant-disable")
	}
}

func TestTenantDisableRequiresTenantAdminNotOperator(t *testing.T) {
	t.Parallel()
	ta := newTestAdmin(t)
	ta.seedRegisteredWorker(t, routingTestWorker1, []string{adminTestTenantA})

	// operator may drain but not disable (per doc 26 role table).
	operatorToken := ta.seedTenantKey(t, "key_op", adminTestTenantA, RoleOperator)
	w := httptest.NewRecorder()
	req := newAdminRequest(http.MethodPost, "/api/v1/admin/workers/worker-1/tenant-disable", operatorToken, "")
	req.SetPathValue("worker_id", routingTestWorker1)
	ta.h.TenantDisableWorker(w, req)
	if w.Code != http.StatusForbidden {
		t.Fatalf("operator tenant-disable status = %d, want 403", w.Code)
	}

	// operator may tenant-drain.
	w = httptest.NewRecorder()
	req = newAdminRequest(http.MethodPost, "/api/v1/admin/workers/worker-1/tenant-drain", operatorToken, "")
	req.SetPathValue("worker_id", routingTestWorker1)
	ta.h.TenantDrainWorker(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("operator tenant-drain status = %d, want 200", w.Code)
	}
	if ta.h.Workers.EligibleForTenant(routingTestWorker1, adminTestTenantA) {
		t.Fatal("operator tenant-drain should exclude the worker")
	}
}
