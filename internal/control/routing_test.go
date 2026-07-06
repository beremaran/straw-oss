package control

import (
	"testing"
	"time"

	strawpb "github.com/beremaran/straw/v2/api/proto/straw/v1"
)

const (
	routingTestWcred1  = "wcred_1"
	routingTestEgress  = errorCategoryEgress
	routingTestTenantA = adminTestTenantA
	routingTestTenantB = "ten_b"
	routingTestPool1   = "pool_1"
	routingTestPool2   = "pool_2"
	routingTestWorker1 = "worker-1"
	routingTestSticky1 = "sticky-1"
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
		ID:           routingTestWcred1,
		Status:       WorkerCredentialStatusActive,
		ExecutorType: routingTestEgress,
		TenantScope:  []string{routingTestTenantA, routingTestTenantB},
		AllowedPools: []AllowedPool{
			{TenantID: routingTestTenantA, PoolID: routingTestPool1},
			{TenantID: routingTestTenantA, PoolID: routingTestPool2},
			{TenantID: routingTestTenantB, PoolID: routingTestPool1},
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
		{ID: "r_low", TenantID: routingTestTenantA, Priority: 10, Enabled: true, TargetPoolID: routingTestPool2},
		{ID: "r_high", TenantID: routingTestTenantA, Priority: 1, Enabled: true, TargetPoolID: routingTestPool1},
	}, nil)
	h.registerReady(t, "w1", routingTestTenantA, routingTestPool1)
	h.registerReady(t, "w2", routingTestTenantA, routingTestPool2)

	out := h.router.Evaluate(RouteRequest{TenantID: routingTestTenantA})
	if !out.OK || out.RuleID != "r_high" || out.PoolID != routingTestPool1 {
		t.Fatalf("Evaluate = %+v, want rule r_high/pool_1", out)
	}
}

func TestRoutingTenantIsolation(t *testing.T) {
	t.Parallel()
	h := newRouteHarness(t, []RoutingRule{
		{ID: "r_a", TenantID: routingTestTenantA, Priority: 1, Enabled: true, TargetPoolID: routingTestPool1},
		{ID: "r_b", TenantID: routingTestTenantB, Priority: 1, Enabled: true, TargetPoolID: routingTestPool1},
	}, nil)
	h.registerReady(t, "w1", routingTestTenantA, routingTestPool1)
	h.registerReady(t, "w2", routingTestTenantB, routingTestPool1)

	out := h.router.Evaluate(RouteRequest{TenantID: routingTestTenantA})
	if !out.OK || out.WorkerID != "w1" {
		t.Fatalf("Evaluate = %+v, want w1 (ten_a scope only)", out)
	}

	out = h.router.Evaluate(RouteRequest{TenantID: routingTestTenantB})
	if !out.OK || out.WorkerID != "w2" {
		t.Fatalf("Evaluate = %+v, want w2 (ten_b scope only)", out)
	}
}

func TestRoutingHardClientHints(t *testing.T) {
	t.Parallel()
	h := newRouteHarness(t, []RoutingRule{
		{
			ID: "r_us", TenantID: routingTestTenantA, Priority: 1, Enabled: true, TargetPoolID: routingTestPool1,
			Match: MatchConditions{Country: "US"},
		},
	}, nil)
	h.registerReady(t, "w1", routingTestTenantA, routingTestPool1)

	out := h.router.Evaluate(RouteRequest{TenantID: routingTestTenantA, Country: "DE"})
	if out.OK || out.ErrorCode != RouteErrNoMatch {
		t.Fatalf("Evaluate(country=DE) = %+v, want route_no_match", out)
	}

	out = h.router.Evaluate(RouteRequest{TenantID: routingTestTenantA, Country: "US"})
	if !out.OK || out.WorkerID != "w1" {
		t.Fatalf("Evaluate(country=US) = %+v, want w1", out)
	}
}

func TestRoutingIngressTypeMatchAndCapability(t *testing.T) {
	t.Parallel()
	h := newRouteHarness(t, []RoutingRule{
		{
			ID: "r_proxy", TenantID: routingTestTenantA, Priority: 1, Enabled: true, TargetPoolID: routingTestPool1,
			Match: MatchConditions{IngressType: IngressTypeHTTPProxy},
		},
		{
			ID: "r_rest", TenantID: routingTestTenantA, Priority: 2, Enabled: true, TargetPoolID: routingTestPool2,
			Match: MatchConditions{IngressType: IngressTypeREST},
		},
	}, nil)
	h.registerReady(t, "proxy-worker", routingTestTenantA, routingTestPool1, func(r *strawpb.RegisterRequest) {
		r.SupportedIngressModes = []string{IngressTypeHTTPProxy}
	})
	h.registerReady(t, "rest-worker", routingTestTenantA, routingTestPool2, func(r *strawpb.RegisterRequest) {
		r.SupportedIngressModes = []string{IngressTypeREST}
	})

	out := h.router.Evaluate(RouteRequest{TenantID: routingTestTenantA, IngressType: IngressTypeHTTPProxy})
	if !out.OK || out.RuleID != "r_proxy" || out.WorkerID != "proxy-worker" {
		t.Fatalf("Evaluate(http_proxy) = %+v, want proxy-worker", out)
	}

	out = h.router.Evaluate(RouteRequest{TenantID: routingTestTenantA, IngressType: IngressTypeREST})
	if !out.OK || out.RuleID != "r_rest" || out.WorkerID != "rest-worker" {
		t.Fatalf("Evaluate(rest) = %+v, want rest-worker", out)
	}
}

func TestRoutingMatchesEveryDocumentedIngressType(t *testing.T) {
	t.Parallel()

	for _, mode := range []string{IngressTypeREST, IngressTypeHTTPProxy, IngressTypeConnect, IngressTypeMITM} {
		h := newRouteHarness(t, []RoutingRule{
			{
				ID: "r_" + mode, TenantID: routingTestTenantA, Priority: 1, Enabled: true, TargetPoolID: routingTestPool1,
				Match: MatchConditions{IngressType: mode},
			},
		}, nil)
		h.registerReady(t, "worker-"+mode, routingTestTenantA, routingTestPool1, func(r *strawpb.RegisterRequest) {
			r.SupportedIngressModes = []string{mode}
		})

		out := h.router.Evaluate(RouteRequest{TenantID: routingTestTenantA, IngressType: mode})
		if !out.OK || out.RuleID != "r_"+mode || out.WorkerID != "worker-"+mode {
			t.Fatalf("Evaluate(%s) = %+v, want matching route and worker", mode, out)
		}
	}
}

func TestRoutingIngressCapabilityDefaultAndUnsupported(t *testing.T) {
	t.Parallel()
	h := newRouteHarness(t, []RoutingRule{
		{ID: "r1", TenantID: routingTestTenantA, Priority: 1, Enabled: true, TargetPoolID: routingTestPool1},
	}, nil)
	h.registerReady(t, "old-worker", routingTestTenantA, routingTestPool1)
	h.registerReady(t, "connect-worker", routingTestTenantA, routingTestPool1, func(r *strawpb.RegisterRequest) {
		r.SupportedIngressModes = []string{IngressTypeConnect}
	})

	out := h.router.Evaluate(RouteRequest{TenantID: routingTestTenantA, IngressType: IngressTypeMITM})
	if !out.OK || out.WorkerID != "old-worker" {
		t.Fatalf("Evaluate(mitm) = %+v, want old worker with empty ingress capability default", out)
	}

	out = h.router.Evaluate(RouteRequest{TenantID: routingTestTenantA, IngressType: IngressTypeConnect})
	if !out.OK {
		t.Fatalf("Evaluate(connect) = %+v, want routable", out)
	}

	blocked := newRouteHarness(t, []RoutingRule{
		{ID: "r1", TenantID: routingTestTenantA, Priority: 1, Enabled: true, TargetPoolID: routingTestPool1},
	}, nil)
	blocked.registerReady(t, "rest-worker", routingTestTenantA, routingTestPool1, func(r *strawpb.RegisterRequest) {
		r.SupportedIngressModes = []string{IngressTypeREST}
	})

	out = blocked.router.Evaluate(RouteRequest{TenantID: routingTestTenantA, IngressType: IngressTypeConnect})
	if out.OK || out.ErrorCode != RouteErrUnavailable {
		t.Fatalf("Evaluate(connect with rest-only worker) = %+v, want route_unavailable", out)
	}
}

func TestRoutingDegradedPoolPolicy(t *testing.T) {
	t.Parallel()
	h := newRouteHarness(t, []RoutingRule{
		{ID: "r1", TenantID: routingTestTenantA, Priority: 1, Enabled: true, TargetPoolID: routingTestPool1},
	}, []PoolPolicy{
		{TenantID: routingTestTenantA, PoolID: routingTestPool1, AllowDegradedWorkers: false},
	})
	h.registerReady(t, "w1", routingTestTenantA, routingTestPool1)
	_, err := h.reg.Heartbeat(&strawpb.HeartbeatRequest{
		WorkerId: "w1", SessionId: h.reg.ListWorkersPlatform()[0].SessionID,
		Health: strawpb.WorkerHealth_WORKER_HEALTH_DEGRADED, MaxConcurrency: 10, AvailableCapacity: 10,
	})
	if err != nil {
		t.Fatalf("heartbeat: %v", err)
	}

	out := h.router.Evaluate(RouteRequest{TenantID: routingTestTenantA})
	if out.OK || out.ErrorCode != RouteErrUnavailable {
		t.Fatalf("Evaluate (degraded, policy=false) = %+v, want route_unavailable", out)
	}

	h.pols = NewStaticPoolPolicyProvider([]PoolPolicy{{TenantID: routingTestTenantA, PoolID: routingTestPool1, AllowDegradedWorkers: true}})
	h.router = NewRouter(h.rules, h.pols, h.reg, h.sticky, h.clock.Now)
	out = h.router.Evaluate(RouteRequest{TenantID: routingTestTenantA})
	if !out.OK || out.WorkerID != "w1" {
		t.Fatalf("Evaluate (degraded, policy=true) = %+v, want w1", out)
	}
}

func TestRoutingNoMatch(t *testing.T) {
	t.Parallel()
	h := newRouteHarness(t, nil, nil)
	out := h.router.Evaluate(RouteRequest{TenantID: routingTestTenantA})
	if out.OK || out.ErrorCode != RouteErrNoMatch {
		t.Fatalf("Evaluate (no rules) = %+v, want route_no_match", out)
	}
}

func TestRoutingUnavailableNoEligibleExecutor(t *testing.T) {
	t.Parallel()
	h := newRouteHarness(t, []RoutingRule{
		{ID: "r1", TenantID: routingTestTenantA, Priority: 1, Enabled: true, TargetPoolID: routingTestPool1},
	}, nil)
	out := h.router.Evaluate(RouteRequest{TenantID: routingTestTenantA})
	if out.OK || out.ErrorCode != RouteErrUnavailable {
		t.Fatalf("Evaluate (no workers) = %+v, want route_unavailable", out)
	}
}

func TestRoutingStickySuccess(t *testing.T) {
	t.Parallel()
	h := newRouteHarness(t, []RoutingRule{
		{ID: "r1", TenantID: routingTestTenantA, Priority: 1, Enabled: true, TargetPoolID: routingTestPool1, StickySessionTTLSeconds: 60},
	}, nil)
	h.registerReady(t, "w1", routingTestTenantA, routingTestPool1)
	h.registerReady(t, "w2", routingTestTenantA, routingTestPool1)

	first := h.router.Evaluate(RouteRequest{TenantID: routingTestTenantA, StickySessionID: routingTestSticky1})
	if !first.OK || !first.Sticky {
		t.Fatalf("first Evaluate = %+v, want sticky OK", first)
	}

	for i := range 5 {
		out := h.router.Evaluate(RouteRequest{TenantID: routingTestTenantA, StickySessionID: routingTestSticky1})
		if !out.OK || out.WorkerID != first.WorkerID {
			t.Fatalf("Evaluate[%d] = %+v, want pinned worker %s", i, out, first.WorkerID)
		}
	}
}

func TestRoutingStickyFailure(t *testing.T) {
	t.Parallel()
	h := newRouteHarness(t, []RoutingRule{
		{
			ID: "r1", TenantID: routingTestTenantA, Priority: 1, Enabled: true, TargetPoolID: routingTestPool1,
			StickySessionTTLSeconds: 60, AllowStickyFallback: false,
		},
	}, nil)
	sess := h.registerReady(t, "w1", routingTestTenantA, routingTestPool1)

	first := h.router.Evaluate(RouteRequest{TenantID: routingTestTenantA, StickySessionID: routingTestSticky1})
	if !first.OK || first.WorkerID != "w1" {
		t.Fatalf("first Evaluate = %+v, want w1", first)
	}

	h.reg.SetGlobalAdmin("w1", AdminDisabled)
	_ = sess

	out := h.router.Evaluate(RouteRequest{TenantID: routingTestTenantA, StickySessionID: "sticky-1"})
	if out.OK || out.ErrorCode != RouteErrStickyUnavailable {
		t.Fatalf("Evaluate (pinned target gone, no fallback) = %+v, want sticky_session_unavailable", out)
	}
}

func TestRoutingStickyFallback(t *testing.T) {
	t.Parallel()
	h := newRouteHarness(t, []RoutingRule{
		{
			ID: "r1", TenantID: routingTestTenantA, Priority: 1, Enabled: true, TargetPoolID: routingTestPool1,
			StickySessionTTLSeconds: 60, AllowStickyFallback: true,
		},
	}, nil)
	h.registerReady(t, "w1", routingTestTenantA, routingTestPool1)
	h.registerReady(t, "w2", routingTestTenantA, routingTestPool1)

	first := h.router.Evaluate(RouteRequest{TenantID: routingTestTenantA, StickySessionID: routingTestSticky1})
	if !first.OK {
		t.Fatalf("first Evaluate = %+v, want OK", first)
	}
	h.reg.SetGlobalAdmin(first.WorkerID, AdminDisabled)

	out := h.router.Evaluate(RouteRequest{TenantID: routingTestTenantA, StickySessionID: routingTestSticky1})
	if !out.OK || out.WorkerID == first.WorkerID {
		t.Fatalf("Evaluate (fallback permitted) = %+v, want re-pinned to a different worker", out)
	}

	// Re-pinned: subsequent calls stick to the new target.
	again := h.router.Evaluate(RouteRequest{TenantID: routingTestTenantA, StickySessionID: routingTestSticky1})
	if !again.OK || again.WorkerID != out.WorkerID {
		t.Fatalf("Evaluate (after re-pin) = %+v, want pinned to %s", again, out.WorkerID)
	}
}

func TestRoutingRuleDisabledSkipped(t *testing.T) {
	t.Parallel()
	h := newRouteHarness(t, []RoutingRule{
		{ID: "r_disabled", TenantID: routingTestTenantA, Priority: 1, Enabled: false, TargetPoolID: routingTestPool1},
		{ID: "r_enabled", TenantID: routingTestTenantA, Priority: 2, Enabled: true, TargetPoolID: routingTestPool2},
	}, nil)
	h.registerReady(t, "w1", routingTestTenantA, routingTestPool1)
	h.registerReady(t, "w2", routingTestTenantA, routingTestPool2)

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
