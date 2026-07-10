package control

import (
	"context"
	"crypto/ed25519"
	"crypto/rand"
	"encoding/base64"
	"errors"
	"slices"
	"sync"
	"testing"
	"time"

	"github.com/redis/go-redis/v9"

	strawpb "github.com/beremaran/straw/v2/api/proto/straw/v1"
)

const (
	workerRegTestWcred1    = "wcred_1"
	workerRegTestEgress    = "egress"
	workerRegTestTenantA   = "ten_a"
	workerRegTestTenantB   = "ten_b"
	workerRegTestTenantC   = "ten_c"
	workerRegTestPool1     = "pool_1"
	workerRegTestChrome120 = "chrome_120"
	workerRegTestWorker1   = "worker-1"
	workerRegTestWorker2   = "worker-2"
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
	err = store.Create(context.Background(), cred)
	if err != nil {
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
		ID:           workerRegTestWcred1,
		Status:       WorkerCredentialStatusActive,
		ExecutorType: workerRegTestEgress,
		TenantScope:  []string{workerRegTestTenantA},
		AllowedPools: []AllowedPool{{TenantID: workerRegTestTenantA, PoolID: workerRegTestPool1}},
	}
}

// signedRegister builds a signed RegisterRequest, then applies mutators.
func (h *regHarness) signedRegister(workerID string, mut ...func(*strawpb.RegisterRequest)) *strawpb.RegisterRequest {
	req := &strawpb.RegisterRequest{
		WorkerId:       workerID,
		ExecutorType:   h.cred.ExecutorType,
		CredentialId:   h.cred.ID,
		ProtocolMajor:  ProtocolMajor,
		AllowedPools:   []*strawpb.RegisterRequest_PoolRef{{TenantId: workerRegTestTenantA, PoolId: workerRegTestPool1}},
		Nonce:          newTestNonce(),
		IssuedAtUnixMs: h.clock.Now().UnixMilli(),
	}
	req.SignedToken = strawpb.SignRegistration(h.priv, req)
	for _, m := range mut {
		m(req)
	}

	return req
}

// newTestNonce returns a fresh random nonce for tests that build
// RegisterRequests directly.
func newTestNonce() []byte {
	nonce := make([]byte, 8)
	_, _ = rand.Read(nonce)

	return nonce
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
	out, err := h.reg.Register(context.Background(), h.signedRegister(workerRegTestWorker1))
	if err != nil {
		t.Fatalf("Register error: %v", err)
	}
	if !out.OK || out.SessionID == "" {
		t.Fatalf("Register outcome = %+v, want OK with session id", out)
	}
	if got := h.reg.RuntimeState(workerRegTestWorker1); got != RuntimeRegistered {
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
	if out.OK || out.Reason != RejectRevokedCredential {
		t.Fatalf("outcome = %+v, want reject credential_revoked", out)
	}
}

func TestRegisterCapabilityOutOfScope(t *testing.T) {
	t.Parallel()
	cred := defaultCred()
	cred.AllowedCapabilities = WorkerCapabilities{Tags: []string{ipTypeDatacenter}}
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

func TestRegisterIngressCapabilityOutOfScope(t *testing.T) {
	t.Parallel()
	cred := defaultCred()
	cred.AllowedCapabilities = WorkerCapabilities{SupportedIngressModes: []string{IngressTypeREST}}
	h := newRegHarness(t, cred)
	req := h.signedRegister("worker-1", func(r *strawpb.RegisterRequest) {
		r.SupportedIngressModes = []string{IngressTypeConnect}
	})
	out, err := h.reg.Register(context.Background(), req)
	if err != nil {
		t.Fatalf("Register error: %v", err)
	}
	if out.OK || out.Reason != RejectCapabilityScope {
		t.Fatalf("outcome = %+v, want reject capability_out_of_scope", out)
	}
}

func TestRegisterFingerprintCapabilitySubset(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name       string
		allowed    []string
		claimed    []string
		wantReason string
	}{
		{name: "allowed exact capability", allowed: []string{workerRegTestChrome120}, claimed: []string{workerRegTestChrome120}},
		{name: "empty allowlist is unrestricted", claimed: []string{workerRegTestChrome120}},
		{name: "claim outside allowlist", allowed: []string{workerRegTestChrome120}, claimed: []string{"firefox_121"}, wantReason: RejectCapabilityScope},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cred := defaultCred()
			cred.AllowedCapabilities.SupportedFingerprintProfiles = tt.allowed
			h := newRegHarness(t, cred)
			req := h.signedRegister(workerRegTestWorker1, func(r *strawpb.RegisterRequest) {
				r.ProtocolMinor = 1
				r.SupportedFingerprintProfiles = append([]string(nil), tt.claimed...)
				r.SignedToken = strawpb.SignRegistration(h.priv, r)
			})

			out, err := h.reg.Register(context.Background(), req)
			if err != nil {
				t.Fatalf("Register error: %v", err)
			}
			if tt.wantReason == "" {
				if !out.OK {
					t.Fatalf("Register rejected: %s", out.Reason)
				}

				return
			}
			if out.OK || out.Reason != tt.wantReason {
				t.Fatalf("outcome = %+v, want rejection %q", out, tt.wantReason)
			}
		})
	}
}

func TestRegisterMultiTenantCredentialScope(t *testing.T) {
	t.Parallel()
	cred := defaultCred()
	cred.TenantScope = []string{workerRegTestTenantA, workerRegTestTenantB}
	cred.AllowedPools = []AllowedPool{
		{TenantID: workerRegTestTenantA, PoolID: workerRegTestPool1},
		{TenantID: workerRegTestTenantB, PoolID: workerRegTestPool1},
	}
	h := newRegHarness(t, cred)

	out, err := h.reg.Register(context.Background(), h.signedRegister("worker-1", func(r *strawpb.RegisterRequest) {
		r.AllowedPools = []*strawpb.RegisterRequest_PoolRef{
			{TenantId: workerRegTestTenantA, PoolId: workerRegTestPool1},
			{TenantId: workerRegTestTenantB, PoolId: workerRegTestPool1},
		}
	}))
	if err != nil {
		t.Fatalf("Register error: %v", err)
	}
	if !out.OK {
		t.Fatalf("Register rejected: %s", out.Reason)
	}

	out, err = h.reg.Register(context.Background(), h.signedRegister("worker-2", func(r *strawpb.RegisterRequest) {
		r.AllowedPools = []*strawpb.RegisterRequest_PoolRef{{TenantId: workerRegTestTenantC, PoolId: workerRegTestPool1}}
	}))
	if err != nil {
		t.Fatalf("Register error: %v", err)
	}
	if out.OK || out.Reason != RejectPoolScope {
		t.Fatalf("outcome = %+v, want reject pool_scope", out)
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

func TestRegisterReplacementUsesOnlyNewFingerprintCapabilities(t *testing.T) {
	t.Parallel()

	h := newRegHarness(t, defaultCred())
	firstReq := h.signedRegister(workerRegTestWorker1, func(r *strawpb.RegisterRequest) {
		r.ProtocolMinor = 1
		r.SupportedFingerprintProfiles = []string{workerRegTestChrome120}
		r.SignedToken = strawpb.SignRegistration(h.priv, r)
	})
	first := h.mustRegister(t, firstReq)
	firstReq.SupportedFingerprintProfiles[0] = "mutated_after_register"

	secondReq := h.signedRegister(workerRegTestWorker1, func(r *strawpb.RegisterRequest) {
		r.ProtocolMinor = 1
		r.SignedToken = strawpb.SignRegistration(h.priv, r)
	})
	second := h.mustRegister(t, secondReq)

	e := h.reg.workers[workerRegTestWorker1]
	if e.current.sessionID != second || len(e.current.supportedFingerprintProfiles) != 0 {
		t.Fatalf("current session = %+v, want new baseline-only session", e.current)
	}
	if e.superseded.sessionID != first || !slices.Equal(e.superseded.supportedFingerprintProfiles, []string{workerRegTestChrome120}) {
		t.Fatalf("superseded session = %+v, want old session with immutable chrome_120", e.superseded)
	}
}

func TestStaleSessionHeartbeatCannotRestoreFingerprintCapabilities(t *testing.T) {
	t.Parallel()

	h := newRegHarness(t, defaultCred())
	first := h.mustRegister(t, h.signedRegister(workerRegTestWorker1, func(r *strawpb.RegisterRequest) {
		r.ProtocolMinor = 1
		r.SupportedFingerprintProfiles = []string{workerRegTestChrome120}
		r.SignedToken = strawpb.SignRegistration(h.priv, r)
	}))
	second := h.mustRegister(t, h.signedRegister(workerRegTestWorker1, func(r *strawpb.RegisterRequest) {
		r.ProtocolMinor = 1
		r.SignedToken = strawpb.SignRegistration(h.priv, r)
	}))

	if ok, _ := h.reg.Heartbeat(readyHeartbeat(workerRegTestWorker1, first)); !ok {
		t.Fatal("superseded-session heartbeat should be recognized")
	}
	e := h.reg.workers[workerRegTestWorker1]
	if e.current.sessionID != second || len(e.current.supportedFingerprintProfiles) != 0 {
		t.Fatalf("current session after stale heartbeat = %+v, want replacement capabilities unchanged", e.current)
	}
}

func TestTenantViewsCopyFingerprintCapabilitiesWithoutSessionInternals(t *testing.T) {
	t.Parallel()

	h := newRegHarness(t, defaultCred())
	sessionID := h.mustRegister(t, h.signedRegister(workerRegTestWorker1, func(r *strawpb.RegisterRequest) {
		r.ProtocolMinor = 1
		r.SupportedFingerprintProfiles = []string{workerRegTestChrome120}
		r.SignedToken = strawpb.SignRegistration(h.priv, r)
	}))
	if ok, _ := h.reg.Heartbeat(readyHeartbeat(workerRegTestWorker1, sessionID)); !ok {
		t.Fatal("current-session heartbeat should be accepted")
	}

	candidates := h.reg.CandidatesForPool(workerRegTestTenantA, workerRegTestPool1)
	views := h.reg.ListWorkersForTenant(workerRegTestTenantA)
	if len(candidates) != 1 || !slices.Equal(candidates[0].SupportedFingerprintProfiles, []string{workerRegTestChrome120}) {
		t.Fatalf("tenant candidates = %+v, want copied chrome_120 capability", candidates)
	}
	if len(views) != 1 || views[0].SessionID != "" || views[0].AssignSubject != "" ||
		!slices.Equal(views[0].SupportedFingerprintProfiles, []string{workerRegTestChrome120}) {
		t.Fatalf("tenant views = %+v, want capability without session internals", views)
	}

	candidates[0].SupportedFingerprintProfiles[0] = "mutated_candidate"
	views[0].SupportedFingerprintProfiles[0] = "mutated_view"
	refreshed := h.reg.ListWorkersForTenant(workerRegTestTenantA)
	if !slices.Equal(refreshed[0].SupportedFingerprintProfiles, []string{workerRegTestChrome120}) {
		t.Fatalf("refreshed tenant profiles = %v, want immutable chrome_120", refreshed[0].SupportedFingerprintProfiles)
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
			sess := h.mustRegister(t, h.signedRegister(workerRegTestWorker1))
			hb := &strawpb.HeartbeatRequest{WorkerId: workerRegTestWorker1, SessionId: sess, Health: tc.health}
			if ok, _ := h.reg.Heartbeat(hb); !ok {
				t.Fatal("heartbeat not accepted")
			}
			if got := h.reg.RuntimeState(workerRegTestWorker1); got != tc.want {
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
	var err error
	_, err = h.reg.Heartbeat(readyHeartbeat("worker-1", sess))
	if err != nil {
		t.Fatalf("heartbeat: %v", err)
	}
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
	var err error
	_, err = h.reg.Heartbeat(readyHeartbeat("worker-1", sess))
	if err != nil {
		t.Fatalf("heartbeat: %v", err)
	}

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
	_, err = h.reg.Heartbeat(readyHeartbeat("worker-1", sess))
	if err != nil {
		t.Fatalf("heartbeat: %v", err)
	}
	if got := h.reg.RuntimeState("worker-1"); got != RuntimeReady {
		t.Fatalf("state after cooldown = %s, want ready", got)
	}
}

func TestCooldownWindowExpiry(t *testing.T) {
	t.Parallel()
	h := newRegHarness(t, defaultCred())
	sess := h.mustRegister(t, h.signedRegister("worker-1"))
	var err error
	_, err = h.reg.Heartbeat(readyHeartbeat("worker-1", sess))
	if err != nil {
		t.Fatalf("heartbeat: %v", err)
	}

	// Two failures, then wait past the window, then one more: should not trip
	// cooldown because the first two aged out.
	h.reg.RecordFailure("worker-1")
	h.reg.RecordFailure("worker-1")
	h.clock.Advance(61 * time.Second)
	_, err = h.reg.Heartbeat(readyHeartbeat("worker-1", sess))
	if err != nil {
		t.Fatalf("heartbeat: %v", err)
	}
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
	cred.TenantScope = []string{adminTestTenantA, adminTestTenantB}
	cred.AllowedPools = []AllowedPool{{TenantID: adminTestTenantA, PoolID: routingTestPool1}, {TenantID: adminTestTenantB, PoolID: routingTestPool1}}
	h := newRegHarness(t, cred)
	sess := h.mustRegister(t, h.signedRegister("worker-1"))
	var err error
	_, err = h.reg.Heartbeat(readyHeartbeat("worker-1", sess))
	if err != nil {
		t.Fatalf("heartbeat: %v", err)
	}

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
	otherCred.TenantScope = []string{workerRegTestTenantC}
	otherCred.AllowedPools = []AllowedPool{{TenantID: workerRegTestTenantC, PoolID: routingTestPool1}}
	pub := base64.StdEncoding.EncodeToString(h.priv.Public().(ed25519.PublicKey))
	otherCred.PublicKeyEd25519Base64 = pub
	err := h.creds.Create(context.Background(), otherCred)
	if err != nil {
		t.Fatalf("create other cred: %v", err)
	}
	otherReq := &strawpb.RegisterRequest{
		WorkerId: "worker-2", ExecutorType: "egress", CredentialId: "wcred_other",
		ProtocolMajor:  ProtocolMajor,
		AllowedPools:   []*strawpb.RegisterRequest_PoolRef{{TenantId: workerRegTestTenantC, PoolId: routingTestPool1}},
		Nonce:          newTestNonce(),
		IssuedAtUnixMs: h.clock.Now().UnixMilli(),
	}
	otherReq.SignedToken = strawpb.SignRegistration(h.priv, otherReq)
	h.mustRegister(t, otherReq)

	// Tenant ten_a sees worker-1 but not worker-2, and no session id.
	views := h.reg.ListWorkersForTenant(adminTestTenantA)
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
	if out.OK || out.Reason != RejectInvalidKeyMaterial {
		t.Fatalf("outcome = %+v, want reject invalid_credential_key", out)
	}
}

// fakeNonceStore is a controllable WorkerNonceStore for registration replay
// tests: it can simulate a store outage (err set) independent of Redis.
type fakeNonceStore struct {
	mu   sync.Mutex
	seen map[string]bool
	err  error
}

func (f *fakeNonceStore) Consume(_ context.Context, credentialID string, nonce []byte, _ time.Duration) (bool, error) {
	f.mu.Lock()
	defer f.mu.Unlock()

	if f.err != nil {
		return false, f.err
	}

	if f.seen == nil {
		f.seen = make(map[string]bool)
	}

	key := credentialID + ":" + string(nonce)
	if f.seen[key] {
		return false, nil
	}

	f.seen[key] = true

	return true, nil
}

func TestRegisterStaleIssuedAtRejected(t *testing.T) {
	t.Parallel()
	h := newRegHarness(t, defaultCred())
	req := h.signedRegister(workerRegTestWorker1, func(r *strawpb.RegisterRequest) {
		r.IssuedAtUnixMs = h.clock.Now().Add(-5 * time.Minute).UnixMilli()
		r.SignedToken = strawpb.SignRegistration(h.priv, r)
	})
	out, err := h.reg.Register(context.Background(), req)
	if err != nil {
		t.Fatalf("Register error: %v", err)
	}
	if out.OK || out.Reason != RejectStaleIssuedAt {
		t.Fatalf("outcome = %+v, want reject stale_issued_at", out)
	}
}

func TestRegisterFutureIssuedAtRejected(t *testing.T) {
	t.Parallel()
	h := newRegHarness(t, defaultCred())
	req := h.signedRegister(workerRegTestWorker1, func(r *strawpb.RegisterRequest) {
		r.IssuedAtUnixMs = h.clock.Now().Add(5 * time.Minute).UnixMilli()
		r.SignedToken = strawpb.SignRegistration(h.priv, r)
	})
	out, err := h.reg.Register(context.Background(), req)
	if err != nil {
		t.Fatalf("Register error: %v", err)
	}
	if out.OK || out.Reason != RejectStaleIssuedAt {
		t.Fatalf("outcome = %+v, want reject stale_issued_at", out)
	}
}

func TestRegisterIssuedAtWithinSkewAccepted(t *testing.T) {
	t.Parallel()
	h := newRegHarness(t, defaultCred())
	req := h.signedRegister(workerRegTestWorker1, func(r *strawpb.RegisterRequest) {
		r.IssuedAtUnixMs = h.clock.Now().Add(50 * time.Second).UnixMilli()
		r.SignedToken = strawpb.SignRegistration(h.priv, r)
	})
	h.mustRegister(t, req)
}

func TestRegisterNonceReplayRejectedDespiteValidSignature(t *testing.T) {
	t.Parallel()
	h := newRegHarness(t, defaultCred())
	h.reg.SetNonceStore(&fakeNonceStore{}, WorkerRegistrationPolicy{ClockSkew: time.Minute, NonceTTL: time.Minute})

	req := h.signedRegister(workerRegTestWorker1)
	h.mustRegister(t, req)

	// Re-submitting the exact same signed request (same nonce, same
	// issued-at) must be rejected even though its signature is still valid.
	out, err := h.reg.Register(context.Background(), req)
	if err != nil {
		t.Fatalf("Register error: %v", err)
	}
	if out.OK || out.Reason != RejectNonceReplayed {
		t.Fatalf("replay outcome = %+v, want reject nonce_replayed", out)
	}
}

func TestRegisterNonceStoreOutageFailsClosedByDefault(t *testing.T) {
	t.Parallel()
	h := newRegHarness(t, defaultCred())
	h.reg.SetNonceStore(&fakeNonceStore{err: errRegNonceStoreDown}, WorkerRegistrationPolicy{ClockSkew: time.Minute, NonceTTL: time.Minute})

	out, err := h.reg.Register(context.Background(), h.signedRegister(workerRegTestWorker1))
	if err != nil {
		t.Fatalf("Register error: %v", err)
	}
	if out.OK || out.Reason != RejectNonceStoreUnavailable {
		t.Fatalf("outcome = %+v, want reject nonce_store_unavailable (fail-closed default)", out)
	}
}

func TestRegisterNonceStoreOutageFailsOpenWhenConfigured(t *testing.T) {
	t.Parallel()
	h := newRegHarness(t, defaultCred())
	h.reg.SetNonceStore(&fakeNonceStore{err: errRegNonceStoreDown}, WorkerRegistrationPolicy{
		ClockSkew: time.Minute, NonceTTL: time.Minute, FailOpenOnNonceStoreError: true,
	})

	h.mustRegister(t, h.signedRegister(workerRegTestWorker1))
}

var errRegNonceStoreDown = errors.New("nonce store unreachable")

func TestRedisWorkerRuntimeTTLAndRestartReconstruction(t *testing.T) {
	client := newTestRedisClient(t)
	store := NewRedisWorkerRuntimeStore(client)
	h := newRegHarness(t, defaultCred())
	h.reg.SetRuntimeStore(store)

	sess := h.mustRegister(t, h.signedRegister(workerRegTestWorker1))
	ok, err := h.reg.Heartbeat(&strawpb.HeartbeatRequest{
		WorkerId: workerRegTestWorker1, SessionId: sess,
		Health:         strawpb.WorkerHealth_WORKER_HEALTH_DEGRADED,
		ActiveRequests: 2, MaxConcurrency: 10, AvailableCapacity: 8,
	})
	if err != nil || !ok {
		t.Fatalf("Heartbeat() = (%v, %v), want accepted nil", ok, err)
	}

	ttl := client.TTL(t.Context(), workerRuntimeKey(workerRegTestWorker1)).Val()
	if ttl <= 0 {
		t.Fatalf("worker runtime TTL = %s, want positive TTL", ttl)
	}

	fresh := NewWorkerRegistry(h.creds, DefaultWorkerTimings(), h.clock.Now)
	fresh.SetRuntimeStore(store)

	if got := fresh.RuntimeState(workerRegTestWorker1); got != RuntimeDegraded {
		t.Fatalf("fresh RuntimeState() = %s, want degraded", got)
	}

	candidates := fresh.CandidatesForPool(workerRegTestTenantA, workerRegTestPool1)
	if len(candidates) != 1 {
		t.Fatalf("fresh CandidatesForPool len = %d, want 1: %+v", len(candidates), candidates)
	}
	if candidates[0].SessionID != sess || candidates[0].ActiveRequests != 2 || candidates[0].AvailableCap != 8 {
		t.Fatalf("fresh candidate = %+v, want restored session/load", candidates[0])
	}
}

func TestRedisWorkerRuntimePreservesDuplicateAndCooldown(t *testing.T) {
	client := newTestRedisClient(t)
	store := NewRedisWorkerRuntimeStore(client)
	h := newRegHarness(t, defaultCred())
	h.reg.SetRuntimeStore(store)

	first := h.mustRegister(t, h.signedRegister(workerRegTestWorker1))
	second := h.mustRegister(t, h.signedRegister(workerRegTestWorker1))

	ok, err := h.reg.Heartbeat(readyHeartbeat(workerRegTestWorker1, second))
	if err != nil || !ok {
		t.Fatalf("Heartbeat() = (%v, %v), want accepted nil", ok, err)
	}
	for range 3 {
		h.reg.RecordFailure(workerRegTestWorker1)
	}

	fresh := NewWorkerRegistry(h.creds, DefaultWorkerTimings(), h.clock.Now)
	fresh.SetRuntimeStore(store)

	if ok, _ := fresh.Heartbeat(readyHeartbeat(workerRegTestWorker1, first)); !ok {
		t.Fatal("fresh registry did not recognize superseded session heartbeat")
	}
	if got := fresh.RuntimeState(workerRegTestWorker1); got != RuntimeCooldown {
		t.Fatalf("fresh RuntimeState() = %s, want cooldown", got)
	}
	if fresh.EligibleForTenant(workerRegTestWorker1, workerRegTestTenantA) {
		t.Fatal("fresh registry made cooldown worker eligible")
	}
}

func TestWorkerRuntimeRedisOutageUsesLocalSnapshotThenFailsSafe(t *testing.T) {
	unreachable := redis.NewClient(&redis.Options{Addr: testUnreachableRedisAddr, DialTimeout: 100 * time.Millisecond, MaxRetries: -1})
	t.Cleanup(func() { _ = unreachable.Close() })

	store := NewRedisWorkerRuntimeStore(unreachable)
	h := newRegHarness(t, defaultCred())
	h.reg.SetRuntimeStore(store)

	sess := h.mustRegister(t, h.signedRegister(workerRegTestWorker1))

	ok, err := h.reg.Heartbeat(readyHeartbeat(workerRegTestWorker1, sess))
	if err != nil || !ok {
		t.Fatalf("Heartbeat() = (%v, %v), want accepted nil", ok, err)
	}
	if !h.reg.EligibleForTenant(workerRegTestWorker1, workerRegTestTenantA) {
		t.Fatal("existing registry lost its local worker snapshot during Redis outage")
	}

	fresh := NewWorkerRegistry(h.creds, DefaultWorkerTimings(), h.clock.Now)
	fresh.SetRuntimeStore(store)
	if got := fresh.RuntimeState(workerRegTestWorker1); got != RuntimeUnregistered {
		t.Fatalf("fresh RuntimeState() = %s, want unregistered with no local snapshot", got)
	}
	if candidates := fresh.CandidatesForPool(workerRegTestTenantA, workerRegTestPool1); len(candidates) != 0 {
		t.Fatalf("fresh CandidatesForPool() = %+v, want fail-safe empty", candidates)
	}
}
