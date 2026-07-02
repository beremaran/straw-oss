package control

import (
	"context"
	"crypto/ed25519"
	"encoding/base64"
	"sync"
	"testing"
	"time"

	strawpb "github.com/beremaran/straw/v2/api/proto/straw/v1"
)

// fakeClock is a controllable clock for deterministic state-timing tests.
type fakeClock struct {
	mu sync.Mutex
	t  time.Time
}

func (c *fakeClock) Now() time.Time {
	c.mu.Lock()
	defer c.mu.Unlock()

	return c.t
}

func (c *fakeClock) Advance(d time.Duration) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.t = c.t.Add(d)
}

// regHarness bundles a registry with its credential store and the worker
// keypair so tests can mint signed registrations.
type regHarness struct {
	reg   *WorkerRegistry
	creds *InMemoryWorkerCredentialStore
	clock *fakeClock
	priv  ed25519.PrivateKey
	cred  WorkerCredential
}

func newRegHarness(t *testing.T, cred WorkerCredential) *regHarness {
	t.Helper()
	pub, priv, err := ed25519.GenerateKey(nil)
	if err != nil {
		t.Fatalf("GenerateKey: %v", err)
	}
	cred.PublicKeyEd25519Base64 = base64.StdEncoding.EncodeToString(pub)
	if cred.Status == "" {
		cred.Status = WorkerCredentialStatusActive
	}
	store := NewInMemoryWorkerCredentialStore()
	if err := store.Create(context.Background(), cred); err != nil {
		t.Fatalf("create credential: %v", err)
	}
	clock := &fakeClock{t: time.Unix(1_700_000_000, 0)}

	return &regHarness{
		reg:   NewWorkerRegistry(store, DefaultWorkerTimings(), clock.Now),
		creds: store,
		clock: clock,
		priv:  priv,
		cred:  cred,
	}
}

// defaultCred returns a permissive credential for tenant ten_a.
func defaultCred() WorkerCredential {
	return WorkerCredential{
		ID:           "wcred_1",
		Status:       WorkerCredentialStatusActive,
		ExecutorType: "egress",
		TenantScope:  []string{"ten_a"},
		AllowedPools: []AllowedPool{{TenantID: "ten_a", PoolID: "pool_1"}},
	}
}

// signedRegister builds a signed RegisterRequest, then applies mutators.
func (h *regHarness) signedRegister(workerID string, mut ...func(*strawpb.RegisterRequest)) *strawpb.RegisterRequest {
	req := &strawpb.RegisterRequest{
		WorkerId:      workerID,
		ExecutorType:  h.cred.ExecutorType,
		CredentialId:  h.cred.ID,
		ProtocolMajor: ProtocolMajor,
		AllowedPools:  []*strawpb.RegisterRequest_PoolRef{{TenantId: "ten_a", PoolId: "pool_1"}},
	}
	req.SignedToken = strawpb.SignRegistration(h.priv, req)
	for _, m := range mut {
		m(req)
	}

	return req
}

func (h *regHarness) mustRegister(t *testing.T, req *strawpb.RegisterRequest) string {
	t.Helper()
	out, err := h.reg.Register(context.Background(), req)
	if err != nil {
		t.Fatalf("Register error: %v", err)
	}
	if !out.OK {
		t.Fatalf("Register rejected: %s", out.Reason)
	}

	return out.SessionID
}

func TestRegisterValid(t *testing.T) {
	t.Parallel()
	h := newRegHarness(t, defaultCred())
	out, err := h.reg.Register(context.Background(), h.signedRegister("worker-1"))
	if err != nil {
		t.Fatalf("Register error: %v", err)
	}
	if !out.OK || out.SessionID == "" {
		t.Fatalf("Register outcome = %+v, want OK with session id", out)
	}
	if got := h.reg.RuntimeState("worker-1"); got != RuntimeRegistered {
		t.Fatalf("runtime state before heartbeat = %s, want registered", got)
	}
}

func TestRegisterRejections(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name       string
		mutateCred func(*WorkerCredential)
		mutateReq  func(*strawpb.RegisterRequest)
		wantReason string
	}{
		{
			name:       "invalid signature",
			mutateReq:  func(r *strawpb.RegisterRequest) { r.SignedToken = []byte("nope-not-a-signature-at-all-xx") },
			wantReason: RejectInvalidSignature,
		},
		{
			name:       "unknown credential",
			mutateReq:  func(r *strawpb.RegisterRequest) { r.CredentialId = "wcred_missing" },
			wantReason: RejectUnknownCredential,
		},
		{
			name:       "executor mismatch",
			mutateReq:  func(r *strawpb.RegisterRequest) { r.ExecutorType = "provider_adapter" },
			wantReason: RejectExecutorMismatch,
		},
		{
			name:       "pool out of scope",
			mutateReq:  func(r *strawpb.RegisterRequest) { r.AllowedPools[0].PoolId = "pool_other" },
			wantReason: RejectPoolScope,
		},
		{
			name:       "incompatible protocol",
			mutateReq:  func(r *strawpb.RegisterRequest) { r.ProtocolMajor = 2 },
			wantReason: RejectIncompatibleProto,
		},
		{
			name:       "invalid worker id",
			mutateReq:  func(r *strawpb.RegisterRequest) { r.WorkerId = "bad.worker.id" },
			wantReason: RejectInvalidWorkerID,
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			cred := defaultCred()
			if tc.mutateCred != nil {
				tc.mutateCred(&cred)
			}
			h := newRegHarness(t, cred)
			// Sign first, then mutate so signature-affecting mutations
			// (protocol, executor, worker id) are correctly rejected as
			// mismatches rather than passing verification.
			req := h.signedRegister("worker-1", tc.mutateReq)
			out, err := h.reg.Register(context.Background(), req)
			if err != nil {
				t.Fatalf("Register error: %v", err)
			}
			if out.OK {
				t.Fatalf("Register accepted, want rejection %q", tc.wantReason)
			}
			if out.Reason != tc.wantReason {
				t.Fatalf("reason = %q, want %q", out.Reason, tc.wantReason)
			}
		})
	}
}

func TestRegisterRevokedCredential(t *testing.T) {
	t.Parallel()
	cred := defaultCred()
	cred.Status = WorkerCredentialStatusRevoked
	h := newRegHarness(t, cred)
	out, err := h.reg.Register(context.Background(), h.signedRegister("worker-1"))
	if err != nil {
		t.Fatalf("Register error: %v", err)
	}
	if out.OK || out.Reason != RejectCredentialRevoked {
		t.Fatalf("outcome = %+v, want reject credential_revoked", out)
	}
}

func TestRegisterCapabilityOutOfScope(t *testing.T) {
	t.Parallel()
	cred := defaultCred()
	cred.AllowedCapabilities = WorkerCapabilities{Tags: []string{"datacenter"}}
	h := newRegHarness(t, cred)
	req := h.signedRegister("worker-1", func(r *strawpb.RegisterRequest) {
		r.Tags = []string{"residential"}
		r.SignedToken = strawpb.SignRegistration(h.priv, r) // tags aren't signed, but keep sig valid
	})
	out, err := h.reg.Register(context.Background(), req)
	if err != nil {
		t.Fatalf("Register error: %v", err)
	}
	if out.OK || out.Reason != RejectCapabilityScope {
		t.Fatalf("outcome = %+v, want reject capability_out_of_scope", out)
	}
}

func TestRegisterDuplicateSessionReplacement(t *testing.T) {
	t.Parallel()
	h := newRegHarness(t, defaultCred())
	first := h.mustRegister(t, h.signedRegister("worker-1"))
	second := h.mustRegister(t, h.signedRegister("worker-1"))
	if first == second {
		t.Fatalf("duplicate registration reused session id %q", first)
	}

	// New session heartbeats to ready.
	if ok, _ := h.reg.Heartbeat(readyHeartbeat("worker-1", second)); !ok {
		t.Fatal("heartbeat on new session not accepted")
	}
	if got := h.reg.RuntimeState("worker-1"); got != RuntimeReady {
		t.Fatalf("runtime after new-session heartbeat = %s, want ready", got)
	}

	// A heartbeat on the superseded session is recognized (diagnostics) but
	// must not revive it or affect routing state.
	ok, _ := h.reg.Heartbeat(readyHeartbeat("worker-1", first))
	if !ok {
		t.Fatal("superseded-session heartbeat should be recognized")
	}
	if got := h.reg.RuntimeState("worker-1"); got != RuntimeReady {
		t.Fatalf("runtime after stale heartbeat = %s, want ready (unchanged)", got)
	}
}

func readyHeartbeat(workerID, sessionID string) *strawpb.HeartbeatRequest {
	return &strawpb.HeartbeatRequest{
		WorkerId: workerID, SessionId: sessionID,
		Health: strawpb.WorkerHealth_WORKER_HEALTH_READY,
	}
}

func TestHeartbeatHealthStates(t *testing.T) {
	t.Parallel()
	cases := []struct {
		health strawpb.WorkerHealth
		want   WorkerRuntimeState
	}{
		{strawpb.WorkerHealth_WORKER_HEALTH_READY, RuntimeReady},
		{strawpb.WorkerHealth_WORKER_HEALTH_DEGRADED, RuntimeDegraded},
		{strawpb.WorkerHealth_WORKER_HEALTH_UNHEALTHY, RuntimeUnhealthy},
	}
	for _, tc := range cases {
		t.Run(tc.want.string(), func(t *testing.T) {
			h := newRegHarness(t, defaultCred())
			sess := h.mustRegister(t, h.signedRegister("worker-1"))
			hb := &strawpb.HeartbeatRequest{WorkerId: "worker-1", SessionId: sess, Health: tc.health}
			if ok, _ := h.reg.Heartbeat(hb); !ok {
				t.Fatal("heartbeat not accepted")
			}
			if got := h.reg.RuntimeState("worker-1"); got != tc.want {
				t.Fatalf("state = %s, want %s", got, tc.want)
			}
		})
	}
}

func (s WorkerRuntimeState) string() string { return string(s) }

func TestHeartbeatUnavailableThenDead(t *testing.T) {
	t.Parallel()
	h := newRegHarness(t, defaultCred())
	sess := h.mustRegister(t, h.signedRegister("worker-1"))
	h.reg.Heartbeat(readyHeartbeat("worker-1", sess))
	if got := h.reg.RuntimeState("worker-1"); got != RuntimeReady {
		t.Fatalf("state = %s, want ready", got)
	}

	// 16s after the last heartbeat: past 15s availability timeout.
	h.clock.Advance(16 * time.Second)
	if got := h.reg.RuntimeState("worker-1"); got != RuntimeUnavailable {
		t.Fatalf("state = %s, want unavailable", got)
	}

	// 31s total after the last heartbeat: past 30s dead timeout.
	h.clock.Advance(15 * time.Second)
	if got := h.reg.RuntimeState("worker-1"); got != RuntimeDead {
		t.Fatalf("state = %s, want dead", got)
	}
}

func TestHeartbeatStaleSessionIgnored(t *testing.T) {
	t.Parallel()
	h := newRegHarness(t, defaultCred())
	h.mustRegister(t, h.signedRegister("worker-1"))
	ok, _ := h.reg.Heartbeat(readyHeartbeat("worker-1", "sess_not_current"))
	if ok {
		t.Fatal("heartbeat with unknown session should be rejected")
	}
	if got := h.reg.RuntimeState("worker-1"); got != RuntimeRegistered {
		t.Fatalf("state = %s, want registered (unchanged by stale heartbeat)", got)
	}
}

func TestCooldownExcludesThenRecovers(t *testing.T) {
	t.Parallel()
	h := newRegHarness(t, defaultCred())
	sess := h.mustRegister(t, h.signedRegister("worker-1"))
	h.reg.Heartbeat(readyHeartbeat("worker-1", sess))

	for range 3 {
		h.reg.RecordFailure("worker-1")
	}
	if got := h.reg.RuntimeState("worker-1"); got != RuntimeCooldown {
		t.Fatalf("state = %s, want cooldown", got)
	}
	if h.reg.EligibleForTenant("worker-1", "ten_a") {
		t.Fatal("worker in cooldown must not be eligible")
	}

	// Fresh heartbeat keeps liveness; cooldown expires after its duration.
	h.clock.Advance(31 * time.Second)
	h.reg.Heartbeat(readyHeartbeat("worker-1", sess))
	if got := h.reg.RuntimeState("worker-1"); got != RuntimeReady {
		t.Fatalf("state after cooldown = %s, want ready", got)
	}
}

func TestCooldownWindowExpiry(t *testing.T) {
	t.Parallel()
	h := newRegHarness(t, defaultCred())
	sess := h.mustRegister(t, h.signedRegister("worker-1"))
	h.reg.Heartbeat(readyHeartbeat("worker-1", sess))

	// Two failures, then wait past the window, then one more: should not trip
	// cooldown because the first two aged out.
	h.reg.RecordFailure("worker-1")
	h.reg.RecordFailure("worker-1")
	h.clock.Advance(61 * time.Second)
	h.reg.Heartbeat(readyHeartbeat("worker-1", sess))
	h.reg.RecordFailure("worker-1")
	if got := h.reg.RuntimeState("worker-1"); got != RuntimeReady {
		t.Fatalf("state = %s, want ready (failures aged out of window)", got)
	}
}

// multiTenantHarness registers one worker credentialed for ten_a and ten_b,
// heartbeat to ready, and returns the harness.
func multiTenantHarness(t *testing.T) *regHarness {
	t.Helper()
	cred := defaultCred()
	cred.TenantScope = []string{"ten_a", "ten_b"}
	cred.AllowedPools = []AllowedPool{{TenantID: "ten_a", PoolID: "pool_1"}, {TenantID: "ten_b", PoolID: "pool_1"}}
	h := newRegHarness(t, cred)
	sess := h.mustRegister(t, h.signedRegister("worker-1"))
	h.reg.Heartbeat(readyHeartbeat("worker-1", sess))

	return h
}

func TestGlobalDisablePrecedence(t *testing.T) {
	t.Parallel()
	h := multiTenantHarness(t)
	h.reg.SetTenantAdmin("worker-1", "ten_a", AdminEnabled)
	h.reg.SetGlobalAdmin("worker-1", AdminDisabled)
	if h.reg.EligibleForTenant("worker-1", "ten_a") {
		t.Fatal("global disable must override tenant enable")
	}
}

func TestTenantDisableIsolation(t *testing.T) {
	t.Parallel()
	h := multiTenantHarness(t)
	h.reg.SetTenantAdmin("worker-1", "ten_a", AdminDisabled)
	if h.reg.EligibleForTenant("worker-1", "ten_a") {
		t.Fatal("tenant ten_a disabled, must not be eligible")
	}
	if !h.reg.EligibleForTenant("worker-1", "ten_b") {
		t.Fatal("tenant ten_b must remain eligible (isolation)")
	}
}

func TestDrainingExclusion(t *testing.T) {
	t.Parallel()
	h := multiTenantHarness(t)
	h.reg.SetTenantDrain("worker-1", "ten_a", true)
	if h.reg.EligibleForTenant("worker-1", "ten_a") {
		t.Fatal("tenant drain must exclude ten_a")
	}
	if !h.reg.EligibleForTenant("worker-1", "ten_b") {
		t.Fatal("tenant drain on ten_a must not affect ten_b")
	}

	h.reg.SetGlobalDrain("worker-1", true)
	if h.reg.EligibleForTenant("worker-1", "ten_b") {
		t.Fatal("global drain must exclude all tenants")
	}
}

func TestListWorkersForTenantScoping(t *testing.T) {
	t.Parallel()
	h := multiTenantHarness(t)
	// A second worker credentialed for a different tenant only.
	otherCred := defaultCred()
	otherCred.ID = "wcred_other"
	otherCred.TenantScope = []string{"ten_c"}
	otherCred.AllowedPools = []AllowedPool{{TenantID: "ten_c", PoolID: "pool_1"}}
	pub := base64.StdEncoding.EncodeToString(h.priv.Public().(ed25519.PublicKey))
	otherCred.PublicKeyEd25519Base64 = pub
	err := h.creds.Create(context.Background(), otherCred)
	if err != nil {
		t.Fatalf("create other cred: %v", err)
	}
	otherReq := &strawpb.RegisterRequest{
		WorkerId: "worker-2", ExecutorType: "egress", CredentialId: "wcred_other",
		ProtocolMajor: ProtocolMajor,
		AllowedPools:  []*strawpb.RegisterRequest_PoolRef{{TenantId: "ten_c", PoolId: "pool_1"}},
	}
	otherReq.SignedToken = strawpb.SignRegistration(h.priv, otherReq)
	h.mustRegister(t, otherReq)

	// Tenant ten_a sees worker-1 but not worker-2, and no session id.
	views := h.reg.ListWorkersForTenant("ten_a")
	if len(views) != 1 || views[0].WorkerID != "worker-1" {
		t.Fatalf("ten_a listing = %+v, want only worker-1", views)
	}
	if views[0].SessionID != "" || views[0].AssignSubject != "" {
		t.Fatalf("tenant listing leaked session_id/subject: %+v", views[0])
	}

	// Platform sees both, with session ids.
	all := h.reg.ListWorkersPlatform()
	if len(all) != 2 {
		t.Fatalf("platform listing len = %d, want 2", len(all))
	}
	for _, v := range all {
		if v.SessionID == "" {
			t.Fatalf("platform listing missing session id for %s", v.WorkerID)
		}
	}
}

func TestRegisterInvalidCredentialKey(t *testing.T) {
	t.Parallel()
	cred := defaultCred()
	h := newRegHarness(t, cred)
	// Corrupt the stored public key.
	corrupt := cred
	corrupt.PublicKeyEd25519Base64 = "not-base64!!"
	err := h.creds.Create(context.Background(), corrupt)
	if err != nil {
		t.Fatalf("overwrite cred: %v", err)
	}
	out, _ := h.reg.Register(context.Background(), h.signedRegister("worker-1"))
	if out.OK || out.Reason != RejectInvalidCredKey {
		t.Fatalf("outcome = %+v, want reject invalid_credential_key", out)
	}
}
