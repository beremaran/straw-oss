package control

import (
	"testing"
	"time"

	strawpb "github.com/beremaran/straw/v2/api/proto/straw/v1"
)

// routeHarness bundles a registry (with two pools registered on one
// credential) and a router for routing tests.
type routeHarness struct {
	*regHarness
	router *Router
	rules  *StaticRuleProvider
	pols   *StaticPoolPolicyProvider
	sticky *StickyStore
}

func multiPoolCred() WorkerCredential {
	return WorkerCredential{
		ID:           "wcred_1",
		Status:       WorkerCredentialStatusActive,
		ExecutorType: "egress",
		TenantScope:  []string{"ten_a", "ten_b"},
		AllowedPools: []AllowedPool{
			{TenantID: "ten_a", PoolID: "pool_1"},
			{TenantID: "ten_a", PoolID: "pool_2"},
			{TenantID: "ten_b", PoolID: "pool_1"},
		},
	}
}

func newRouteHarness(t *testing.T, rules []RoutingRule, policies []PoolPolicy) *routeHarness {
	t.Helper()
	h := newRegHarness(t, multiPoolCred())
	rp := NewStaticRuleProvider(rules)
	pp := NewStaticPoolPolicyProvider(policies)
	sticky := NewStickyStore(h.clock.Now)
	router := NewRouter(rp, pp, h.reg, sticky, h.clock.Now)

	return &routeHarness{regHarness: h, router: router, rules: rp, pols: pp, sticky: sticky}
}

// registerReady registers workerID scoped to tenantID/poolID and brings it
// to the ready runtime state via heartbeat.
func (h *routeHarness) registerReady(t *testing.T, workerID, tenantID, poolID string, mut ...func(*strawpb.RegisterRequest)) string {
	t.Helper()
	req := h.signedRegister(workerID, append([]func(*strawpb.RegisterRequest){
		func(r *strawpb.RegisterRequest) {
			r.AllowedPools = []*strawpb.RegisterRequest_PoolRef{{TenantId: tenantID, PoolId: poolID}}
		},
	}, mut...)...)
	sess := h.mustRegister(t, req)
	ok, err := h.reg.Heartbeat(&strawpb.HeartbeatRequest{
		WorkerId: workerID, SessionId: sess,
		Health: strawpb.WorkerHealth_WORKER_HEALTH_READY, MaxConcurrency: 10, AvailableCapacity: 10,
	})
	if err != nil || !ok {
		t.Fatalf("heartbeat: ok=%v err=%v", ok, err)
	}

	return sess
}

func TestRoutingPriorityOrder(t *testing.T) {
	t.Parallel()
	h := newRouteHarness(t, []RoutingRule{
		{ID: "r_low", TenantID: "ten_a", Priority: 10, Enabled: true, TargetPoolID: "pool_2"},
		{ID: "r_high", TenantID: "ten_a", Priority: 1, Enabled: true, TargetPoolID: "pool_1"},
	}, nil)
	h.registerReady(t, "w1", "ten_a", "pool_1")
	h.registerReady(t, "w2", "ten_a", "pool_2")

	out := h.router.Evaluate(RouteRequest{TenantID: "ten_a"})
	if !out.OK || out.RuleID != "r_high" || out.PoolID != "pool_1" {
		t.Fatalf("Evaluate = %+v, want rule r_high/pool_1", out)
	}
}

func TestRoutingTenantIsolation(t *testing.T) {
	t.Parallel()
	h := newRouteHarness(t, []RoutingRule{
		{ID: "r_a", TenantID: "ten_a", Priority: 1, Enabled: true, TargetPoolID: "pool_1"},
		{ID: "r_b", TenantID: "ten_b", Priority: 1, Enabled: true, TargetPoolID: "pool_1"},
	}, nil)
	h.registerReady(t, "w1", "ten_a", "pool_1")
	h.registerReady(t, "w2", "ten_b", "pool_1")

	out := h.router.Evaluate(RouteRequest{TenantID: "ten_a"})
	if !out.OK || out.WorkerID != "w1" {
		t.Fatalf("Evaluate = %+v, want w1 (ten_a scope only)", out)
	}

	out = h.router.Evaluate(RouteRequest{TenantID: "ten_b"})
	if !out.OK || out.WorkerID != "w2" {
		t.Fatalf("Evaluate = %+v, want w2 (ten_b scope only)", out)
	}
}

func TestRoutingHardClientHints(t *testing.T) {
	t.Parallel()
	h := newRouteHarness(t, []RoutingRule{
		{
			ID: "r_us", TenantID: "ten_a", Priority: 1, Enabled: true, TargetPoolID: "pool_1",
			Match: MatchConditions{Country: "US"},
		},
	}, nil)
	h.registerReady(t, "w1", "ten_a", "pool_1")

	out := h.router.Evaluate(RouteRequest{TenantID: "ten_a", Country: "DE"})
	if out.OK || out.ErrorCode != RouteErrNoMatch {
		t.Fatalf("Evaluate(country=DE) = %+v, want route_no_match", out)
	}

	out = h.router.Evaluate(RouteRequest{TenantID: "ten_a", Country: "US"})
	if !out.OK || out.WorkerID != "w1" {
		t.Fatalf("Evaluate(country=US) = %+v, want w1", out)
	}
}

func TestRoutingDegradedPoolPolicy(t *testing.T) {
	t.Parallel()
	h := newRouteHarness(t, []RoutingRule{
		{ID: "r1", TenantID: "ten_a", Priority: 1, Enabled: true, TargetPoolID: "pool_1"},
	}, []PoolPolicy{
		{TenantID: "ten_a", PoolID: "pool_1", AllowDegradedWorkers: false},
	})
	h.registerReady(t, "w1", "ten_a", "pool_1")
	if _, err := h.reg.Heartbeat(&strawpb.HeartbeatRequest{
		WorkerId: "w1", SessionId: h.reg.ListWorkersPlatform()[0].SessionID,
		Health: strawpb.WorkerHealth_WORKER_HEALTH_DEGRADED, MaxConcurrency: 10, AvailableCapacity: 10,
	}); err != nil {
		t.Fatalf("heartbeat: %v", err)
	}

	out := h.router.Evaluate(RouteRequest{TenantID: "ten_a"})
	if out.OK || out.ErrorCode != RouteErrUnavailable {
		t.Fatalf("Evaluate (degraded, policy=false) = %+v, want route_unavailable", out)
	}

	h.pols = NewStaticPoolPolicyProvider([]PoolPolicy{{TenantID: "ten_a", PoolID: "pool_1", AllowDegradedWorkers: true}})
	h.router = NewRouter(h.rules, h.pols, h.reg, h.sticky, h.clock.Now)
	out = h.router.Evaluate(RouteRequest{TenantID: "ten_a"})
	if !out.OK || out.WorkerID != "w1" {
		t.Fatalf("Evaluate (degraded, policy=true) = %+v, want w1", out)
	}
}

func TestRoutingNoMatch(t *testing.T) {
	t.Parallel()
	h := newRouteHarness(t, nil, nil)
	out := h.router.Evaluate(RouteRequest{TenantID: "ten_a"})
	if out.OK || out.ErrorCode != RouteErrNoMatch {
		t.Fatalf("Evaluate (no rules) = %+v, want route_no_match", out)
	}
}

func TestRoutingUnavailableNoEligibleExecutor(t *testing.T) {
	t.Parallel()
	h := newRouteHarness(t, []RoutingRule{
		{ID: "r1", TenantID: "ten_a", Priority: 1, Enabled: true, TargetPoolID: "pool_1"},
	}, nil)
	out := h.router.Evaluate(RouteRequest{TenantID: "ten_a"})
	if out.OK || out.ErrorCode != RouteErrUnavailable {
		t.Fatalf("Evaluate (no workers) = %+v, want route_unavailable", out)
	}
}

func TestRoutingStickySuccess(t *testing.T) {
	t.Parallel()
	h := newRouteHarness(t, []RoutingRule{
		{ID: "r1", TenantID: "ten_a", Priority: 1, Enabled: true, TargetPoolID: "pool_1", StickySessionTTLSeconds: 60},
	}, nil)
	h.registerReady(t, "w1", "ten_a", "pool_1")
	h.registerReady(t, "w2", "ten_a", "pool_1")

	first := h.router.Evaluate(RouteRequest{TenantID: "ten_a", StickySessionID: "sticky-1"})
	if !first.OK || !first.Sticky {
		t.Fatalf("first Evaluate = %+v, want sticky OK", first)
	}

	for i := range 5 {
		out := h.router.Evaluate(RouteRequest{TenantID: "ten_a", StickySessionID: "sticky-1"})
		if !out.OK || out.WorkerID != first.WorkerID {
			t.Fatalf("Evaluate[%d] = %+v, want pinned worker %s", i, out, first.WorkerID)
		}
	}
}

func TestRoutingStickyFailure(t *testing.T) {
	t.Parallel()
	h := newRouteHarness(t, []RoutingRule{
		{
			ID: "r1", TenantID: "ten_a", Priority: 1, Enabled: true, TargetPoolID: "pool_1",
			StickySessionTTLSeconds: 60, AllowStickyFallback: false,
		},
	}, nil)
	sess := h.registerReady(t, "w1", "ten_a", "pool_1")

	first := h.router.Evaluate(RouteRequest{TenantID: "ten_a", StickySessionID: "sticky-1"})
	if !first.OK || first.WorkerID != "w1" {
		t.Fatalf("first Evaluate = %+v, want w1", first)
	}

	h.reg.SetGlobalAdmin("w1", AdminDisabled)
	_ = sess

	out := h.router.Evaluate(RouteRequest{TenantID: "ten_a", StickySessionID: "sticky-1"})
	if out.OK || out.ErrorCode != RouteErrStickyUnavailable {
		t.Fatalf("Evaluate (pinned target gone, no fallback) = %+v, want sticky_session_unavailable", out)
	}
}

func TestRoutingStickyFallback(t *testing.T) {
	t.Parallel()
	h := newRouteHarness(t, []RoutingRule{
		{
			ID: "r1", TenantID: "ten_a", Priority: 1, Enabled: true, TargetPoolID: "pool_1",
			StickySessionTTLSeconds: 60, AllowStickyFallback: true,
		},
	}, nil)
	h.registerReady(t, "w1", "ten_a", "pool_1")
	h.registerReady(t, "w2", "ten_a", "pool_1")

	first := h.router.Evaluate(RouteRequest{TenantID: "ten_a", StickySessionID: "sticky-1"})
	if !first.OK {
		t.Fatalf("first Evaluate = %+v, want OK", first)
	}
	h.reg.SetGlobalAdmin(first.WorkerID, AdminDisabled)

	out := h.router.Evaluate(RouteRequest{TenantID: "ten_a", StickySessionID: "sticky-1"})
	if !out.OK || out.WorkerID == first.WorkerID {
		t.Fatalf("Evaluate (fallback permitted) = %+v, want re-pinned to a different worker", out)
	}

	// Re-pinned: subsequent calls stick to the new target.
	again := h.router.Evaluate(RouteRequest{TenantID: "ten_a", StickySessionID: "sticky-1"})
	if !again.OK || again.WorkerID != out.WorkerID {
		t.Fatalf("Evaluate (after re-pin) = %+v, want pinned to %s", again, out.WorkerID)
	}
}

func TestRoutingRuleDisabledSkipped(t *testing.T) {
	t.Parallel()
	h := newRouteHarness(t, []RoutingRule{
		{ID: "r_disabled", TenantID: "ten_a", Priority: 1, Enabled: false, TargetPoolID: "pool_1"},
		{ID: "r_enabled", TenantID: "ten_a", Priority: 2, Enabled: true, TargetPoolID: "pool_2"},
	}, nil)
	h.registerReady(t, "w1", "ten_a", "pool_1")
	h.registerReady(t, "w2", "ten_a", "pool_2")

	out := h.router.Evaluate(RouteRequest{TenantID: "ten_a"})
	if !out.OK || out.RuleID != "r_enabled" {
		t.Fatalf("Evaluate = %+v, want disabled rule skipped", out)
	}
}

func TestStickyStoreTTLExpiry(t *testing.T) {
	t.Parallel()
	now := time.Unix(1_700_000_000, 0)
	s := NewStickyStore(func() time.Time { return now })
	s.Set("ten_a", "sess", "w1", 10*time.Second)
	if _, ok := s.Get("ten_a", "sess"); !ok {
		t.Fatalf("Get immediately after Set = not found, want found")
	}
	now = now.Add(11 * time.Second)
	if _, ok := s.Get("ten_a", "sess"); ok {
		t.Fatalf("Get after TTL expiry = found, want not found")
	}
}
