package control

import (
	"context"
	"testing"
	"time"

	"github.com/beremaran/straw-oss/internal/config"
	strawpb "github.com/beremaran/straw-protos-go/straw/v1"
)

const (
	routingDeploymentID  = defaultFingerprintProfileName
	routingExecutorType  = errorCategoryEgress
	routingPoolID        = "pool"
	routingHost          = "api.example.com"
	routingRegion        = "ap-southeast-2"
	routingIPType        = "residential"
	routingTag           = "datacenter"
	routingSessionID     = "session-1"
	routingFallbackID    = "fallback"
	routingPreferredID   = "preferred"
	routingHostPoolID    = denyRuleTypeHost
	routingCountryID     = "country"
	routingRegionTag     = "region:au"
	routingWorkerB       = "worker-b"
	routingSharedRouteID = "shared-route"
	routingSharedPoolID  = "shared-pool"
	disabledPoolID       = "disabled-pool"
)

type testRoutingCandidates map[string][]PoolCandidate

func (c testRoutingCandidates) CandidatesForPool(_ string, poolID string) []PoolCandidate {
	return append([]PoolCandidate(nil), c[poolID]...)
}

func routingCandidate(id string) PoolCandidate {
	return PoolCandidate{
		WorkerID:       id,
		SessionID:      id + "-session",
		AssignSubject:  "assign." + id,
		ExecutorType:   routingExecutorType,
		IngressModes:   []string{IngressTypeREST},
		MaxConcurrency: 4,
		AvailableCap:   4,
	}
}

func testRouter(rules []RoutingRule, candidates testRoutingCandidates) *Router {
	policies := make([]PoolPolicy, 0)
	for _, rule := range rules {
		policies = append(policies, PoolPolicy{DeploymentID: routingDeploymentID, PoolID: rule.TargetPoolID, Enabled: true})
	}

	return NewRouter(
		NewStaticRuleProvider(rules),
		NewStaticPoolPolicyProvider(policies),
		candidates,
		NewStickyStore(nil),
		nil,
	)
}

func TestRouterRulePriority(t *testing.T) {
	router := testRouter([]RoutingRule{
		{ID: routingFallbackID, DeploymentID: routingDeploymentID, Priority: 20, Enabled: true, TargetPoolID: routingFallbackID},
		{ID: routingPreferredID, DeploymentID: routingDeploymentID, Priority: 10, Enabled: true, TargetPoolID: routingPreferredID},
	}, testRoutingCandidates{
		routingFallbackID:  {routingCandidate("fallback-worker")},
		routingPreferredID: {routingCandidate("preferred-worker")},
	})

	outcome := router.Evaluate(RouteRequest{DeploymentID: routingDeploymentID, IngressType: IngressTypeREST, TargetHost: "example.com"})
	if !outcome.OK || outcome.RuleID != routingPreferredID || outcome.WorkerID != "preferred-worker" {
		t.Fatalf("route outcome = %+v, want preferred rule and worker", outcome)
	}
}

func TestRouterHostAndIngressMatching(t *testing.T) {
	rules := []RoutingRule{
		{ID: routingHostPoolID, DeploymentID: routingDeploymentID, Priority: 1, Enabled: true, TargetPoolID: routingHostPoolID, Match: MatchConditions{TargetHost: routingHost, IngressType: IngressTypeREST}},
	}
	router := testRouter(rules, testRoutingCandidates{routingHostPoolID: {routingCandidate("host-worker")}})

	if got := router.Evaluate(RouteRequest{DeploymentID: routingDeploymentID, IngressType: IngressTypeREST, TargetHost: "other.example.com"}); got.ErrorCode != RouteErrNoMatch {
		t.Fatalf("wrong host outcome = %+v, want %s", got, RouteErrNoMatch)
	}
	if got := router.Evaluate(RouteRequest{DeploymentID: routingDeploymentID, TargetHost: routingHost}); got.ErrorCode != RouteErrNoMatch {
		t.Fatalf("omitted ingress outcome = %+v, want %s", got, RouteErrNoMatch)
	}
	if got := router.Evaluate(RouteRequest{DeploymentID: routingDeploymentID, IngressType: IngressTypeREST, TargetHost: routingHost}); !got.OK || got.WorkerID != "host-worker" {
		t.Fatalf("matching host/ingress outcome = %+v", got)
	}
}

func TestRouterEquivalentAcrossRESTProxyAndConnectIngresses(t *testing.T) {
	t.Parallel()
	rule := RoutingRule{
		ID: routingSharedRouteID, DeploymentID: routingDeploymentID, Priority: 1, Enabled: true, TargetPoolID: routingPoolID,
		Match: MatchConditions{Country: "AU", Region: routingRegion, IPType: routingIPType},
	}
	candidate := routingCandidate("shared-worker")
	candidate.Countries = []string{"AU"}
	candidate.Regions = []string{routingRegion}
	candidate.IPTypes = []string{routingIPType}
	candidate.IngressModes = []string{IngressTypeREST, IngressTypeHTTPProxy, IngressTypeConnect}
	router := testRouter([]RoutingRule{rule}, testRoutingCandidates{routingPoolID: {candidate}})

	var first RouteOutcome
	for i, ingress := range []string{IngressTypeREST, IngressTypeHTTPProxy, IngressTypeConnect} {
		outcome := router.Evaluate(RouteRequest{
			DeploymentID: routingDeploymentID, Country: "AU", Region: routingRegion, IPType: routingIPType,
			IngressType: ingress, TargetHost: routingHost,
		})
		if i == 0 {
			first = outcome
		}
		if !outcome.OK || outcome.RuleID != first.RuleID || outcome.PoolID != first.PoolID || outcome.WorkerID != first.WorkerID {
			t.Fatalf("%s outcome = %+v, want same decision as REST %+v", ingress, outcome, first)
		}
	}
}

func TestRouterTagsCountryRegionAndIPTypeFilterWorkers(t *testing.T) {
	matching := routingCandidate("matching")
	matching.Tags = []string{routingTag, "blue"}
	matching.Countries = []string{"AU"}
	matching.Regions = []string{routingRegion}
	matching.IPTypes = []string{routingIPType}

	wrong := routingCandidate("wrong")
	wrong.Tags = []string{routingTag}
	wrong.Countries = []string{"US"}
	wrong.Regions = []string{"us-west-1"}
	wrong.IPTypes = []string{routingTag}

	rule := RoutingRule{
		ID: "constrained", DeploymentID: routingDeploymentID, Priority: 1, Enabled: true, TargetPoolID: routingPoolID,
		Match: MatchConditions{Tags: []string{routingTag}, Country: "AU", Region: routingRegion, IPType: routingIPType},
	}
	router := testRouter([]RoutingRule{rule}, testRoutingCandidates{routingPoolID: {wrong, matching}})

	outcome := router.Evaluate(RouteRequest{
		DeploymentID: routingDeploymentID, Tags: []string{routingTag}, Country: "AU", Region: routingRegion, IPType: routingIPType, IngressType: IngressTypeREST,
	})
	if !outcome.OK || outcome.WorkerID != "matching" {
		t.Fatalf("constrained outcome = %+v, want matching worker", outcome)
	}
}

func TestRouterMissingWorkerCapabilityIsNotWildcard(t *testing.T) {
	candidate := routingCandidate("missing-country")
	candidate.Countries = nil
	rule := RoutingRule{ID: routingCountryID, DeploymentID: routingDeploymentID, Priority: 1, Enabled: true, TargetPoolID: routingPoolID}
	router := testRouter([]RoutingRule{rule}, testRoutingCandidates{routingPoolID: {candidate}})

	outcome := router.Evaluate(RouteRequest{DeploymentID: routingDeploymentID, Country: "AU", IngressType: IngressTypeREST})
	if outcome.ErrorCode != RouteErrUnavailable {
		t.Fatalf("missing capability outcome = %+v, want %s", outcome, RouteErrUnavailable)
	}
}

func TestRouterCapacity(t *testing.T) {
	candidate := routingCandidate("full")
	candidate.ActiveRequests = candidate.MaxConcurrency
	candidate.AvailableCap = 0
	rule := RoutingRule{ID: "capacity", DeploymentID: routingDeploymentID, Priority: 1, Enabled: true, TargetPoolID: routingPoolID}
	router := testRouter([]RoutingRule{rule}, testRoutingCandidates{routingPoolID: {candidate}})

	outcome := router.Evaluate(RouteRequest{DeploymentID: routingDeploymentID, IngressType: IngressTypeREST})
	if outcome.ErrorCode != RouteErrCapacityExhausted {
		t.Fatalf("capacity outcome = %+v, want %s", outcome, RouteErrCapacityExhausted)
	}
}

func TestRouterStickySelection(t *testing.T) {
	sticky := NewStickyStore(nil)
	sticky.Set(routingDeploymentID, routingSessionID, routingWorkerB, time.Minute)
	rule := RoutingRule{ID: "sticky", DeploymentID: routingDeploymentID, Priority: 1, Enabled: true, TargetPoolID: routingPoolID, StickySessionTTLSeconds: 60}
	router := NewRouter(
		NewStaticRuleProvider([]RoutingRule{rule}),
		NewStaticPoolPolicyProvider([]PoolPolicy{{DeploymentID: routingDeploymentID, PoolID: routingPoolID, Enabled: true}}),
		testRoutingCandidates{routingPoolID: {routingCandidate("worker-a"), routingCandidate(routingWorkerB)}},
		sticky,
		nil,
	)

	outcome := router.Evaluate(RouteRequest{DeploymentID: routingDeploymentID, StickySessionID: routingSessionID, IngressType: IngressTypeREST})
	if !outcome.OK || !outcome.Sticky || outcome.WorkerID != routingWorkerB {
		t.Fatalf("sticky outcome = %+v, want %s sticky selection", outcome, routingWorkerB)
	}
}

func TestRouterStickyFallback(t *testing.T) {
	sticky := NewStickyStore(nil)
	sticky.Set(routingDeploymentID, routingSessionID, "gone", time.Minute)
	rule := RoutingRule{ID: "fallback", DeploymentID: routingDeploymentID, Priority: 1, Enabled: true, TargetPoolID: routingPoolID, StickySessionTTLSeconds: 60, AllowStickyFallback: true}
	router := NewRouter(
		NewStaticRuleProvider([]RoutingRule{rule}),
		NewStaticPoolPolicyProvider([]PoolPolicy{{DeploymentID: routingDeploymentID, PoolID: routingPoolID, Enabled: true}}),
		testRoutingCandidates{routingPoolID: {routingCandidate("available")}},
		sticky,
		nil,
	)

	outcome := router.Evaluate(RouteRequest{DeploymentID: routingDeploymentID, StickySessionID: routingSessionID, IngressType: IngressTypeREST})
	if !outcome.OK || outcome.WorkerID != "available" || !outcome.Sticky {
		t.Fatalf("sticky fallback outcome = %+v", outcome)
	}
	if worker, ok := sticky.Get(routingDeploymentID, routingSessionID); !ok || worker != "available" {
		t.Fatalf("sticky pin = %q, %v; want available", worker, ok)
	}
}

func TestRouterOmittedHintsUseUnconstrainedRule(t *testing.T) {
	rules := []RoutingRule{
		{ID: "country-only", DeploymentID: routingDeploymentID, Priority: 1, Enabled: true, TargetPoolID: "country", Match: MatchConditions{Country: "AU"}},
		{ID: routingDeploymentID, DeploymentID: routingDeploymentID, Priority: 10, Enabled: true, TargetPoolID: routingDeploymentID},
	}
	router := testRouter(rules, testRoutingCandidates{
		"country":           {{WorkerID: "country-worker", ExecutorType: routingExecutorType, AvailableCap: 1, IngressModes: []string{IngressTypeREST}}},
		routingDeploymentID: {{WorkerID: "default-worker", ExecutorType: routingExecutorType, AvailableCap: 1, IngressModes: []string{IngressTypeREST}}},
	})

	outcome := router.Evaluate(RouteRequest{DeploymentID: routingDeploymentID, IngressType: IngressTypeREST})
	if !outcome.OK || outcome.RuleID != routingDeploymentID || outcome.WorkerID != "default-worker" {
		t.Fatalf("omitted hints outcome = %+v, want unconstrained default rule", outcome)
	}
}

func TestRouterEnforcesPoolEnabledTypeTagsAndCapabilities(t *testing.T) {
	t.Parallel()

	wrongType := routingCandidate("wrong-type")
	wrongType.Tags = []string{routingRegionTag, routingIPType}
	wrongType.Countries = []string{"AU"}
	wrongType.Regions = []string{routingRegion}
	wrongType.IPTypes = []string{routingIPType}
	wrongType.ExecutorType = "wrong"
	missingTag := routingCandidate("missing-tag")
	missingTag.Tags = []string{routingRegionTag}
	missingTag.Countries = []string{"AU"}
	missingTag.Regions = []string{routingRegion}
	missingTag.IPTypes = []string{routingIPType}
	wrongCapability := routingCandidate("wrong-capability")
	wrongCapability.Tags = []string{routingRegionTag, routingIPType}
	wrongCapability.Countries = []string{"US"}
	wrongCapability.Regions = []string{"us-west-1"}
	wrongCapability.IPTypes = []string{routingIPType}
	matching := routingCandidate("matching")
	matching.Tags = []string{routingRegionTag, routingIPType}
	matching.Countries = []string{"AU"}
	matching.Regions = []string{routingRegion}
	matching.IPTypes = []string{routingIPType}

	rule := RoutingRule{ID: "restricted", DeploymentID: routingDeploymentID, Enabled: true, TargetPoolID: routingPoolID}
	policy := PoolPolicy{
		DeploymentID: routingDeploymentID, PoolID: routingPoolID, Enabled: true, ExecutorType: routingExecutorType,
		Tags: []string{routingIPType}, AllowedCountries: []string{"AU"}, AllowedRegions: []string{routingRegion}, AllowedIPTypes: []string{routingIPType},
	}
	router := NewRouter(NewStaticRuleProvider([]RoutingRule{rule}), NewStaticPoolPolicyProvider([]PoolPolicy{policy}), testRoutingCandidates{routingPoolID: {wrongType, missingTag, wrongCapability, matching}}, NewStickyStore(nil), nil)

	outcome := router.Evaluate(RouteRequest{DeploymentID: routingDeploymentID, IngressType: IngressTypeREST, Country: "AU", Region: routingRegion, IPType: routingIPType})
	if !outcome.OK || outcome.WorkerID != "matching" {
		t.Fatalf("restricted outcome = %+v, want matching worker", outcome)
	}

	degraded := matching
	degraded.WorkerID = "degraded"
	degraded.Degraded = true
	noDegraded := policy
	noDegraded.AllowDegradedWorkers = false
	noDegradedRouter := NewRouter(NewStaticRuleProvider([]RoutingRule{rule}), NewStaticPoolPolicyProvider([]PoolPolicy{noDegraded}), testRoutingCandidates{routingPoolID: {degraded}}, NewStickyStore(nil), nil)
	if outcome := noDegradedRouter.Evaluate(RouteRequest{DeploymentID: routingDeploymentID, IngressType: IngressTypeREST}); outcome.ErrorCode != RouteErrUnavailable {
		t.Fatalf("degraded-disabled outcome = %+v, want %s", outcome, RouteErrUnavailable)
	}
	allowDegraded := policy
	allowDegraded.AllowDegradedWorkers = true
	allowDegradedRouter := NewRouter(NewStaticRuleProvider([]RoutingRule{rule}), NewStaticPoolPolicyProvider([]PoolPolicy{allowDegraded}), testRoutingCandidates{routingPoolID: {degraded}}, NewStickyStore(nil), nil)
	if outcome := allowDegradedRouter.Evaluate(RouteRequest{DeploymentID: routingDeploymentID, IngressType: IngressTypeREST}); !outcome.OK || outcome.WorkerID != "degraded" {
		t.Fatalf("degraded-enabled outcome = %+v", outcome)
	}

	disabled := policy
	disabled.Enabled = false
	disabledRouter := NewRouter(NewStaticRuleProvider([]RoutingRule{rule}), NewStaticPoolPolicyProvider([]PoolPolicy{disabled}), testRoutingCandidates{routingPoolID: {matching}}, NewStickyStore(nil), nil)
	if outcome := disabledRouter.Evaluate(RouteRequest{DeploymentID: routingDeploymentID, IngressType: IngressTypeREST}); outcome.ErrorCode != RouteErrUnavailable {
		t.Fatalf("disabled outcome = %+v, want %s", outcome, RouteErrUnavailable)
	}
}

func TestRouterUsesRegisteredMembershipAcrossMultiplePools(t *testing.T) {
	t.Parallel()

	registry := NewDeploymentWorkerRegistry(DefaultWorkerTimings(), nil)
	snapshot := config.NewSnapshot(1)
	snapshot.ExecutorPools = []config.ExecutorPool{
		{ID: config.DefaultPoolID, ExecutorType: errorCategoryEgress, Enabled: true},
		{ID: routingIPType, ExecutorType: errorCategoryEgress, Enabled: true, Tags: []string{routingIPType}},
		{ID: disabledPoolID, ExecutorType: errorCategoryEgress, Enabled: false},
	}
	registry.ApplySnapshot(snapshot)

	register := func(id string, refs ...*strawpb.RegisterRequest_PoolRef) string {
		outcome, err := registry.Register(context.Background(), &strawpb.RegisterRequest{WorkerId: id, ExecutorType: errorCategoryEgress, ProtocolMajor: ProtocolMajor, AllowedPools: refs, Tags: []string{routingIPType}, SupportedIngressModes: []string{IngressTypeREST}})
		if err != nil || !outcome.OK {
			t.Fatalf("register %s = %+v, %v", id, outcome, err)
		}
		ok, err := registry.Heartbeat(context.Background(), &strawpb.HeartbeatRequest{WorkerId: id, SessionId: outcome.SessionID, Health: strawpb.WorkerHealth_WORKER_HEALTH_READY, AvailableCapacity: 1})
		if err != nil || !ok {
			t.Fatalf("heartbeat %s = %v, %v", id, ok, err)
		}

		return id
	}
	register("residential-worker", &strawpb.RegisterRequest_PoolRef{PoolId: routingIPType})
	register("default-worker", &strawpb.RegisterRequest_PoolRef{PoolId: config.DefaultPoolID})

	rules := NewStaticRuleProvider([]RoutingRule{
		{ID: routingIPType, DeploymentID: config.DefaultDeploymentID, Enabled: true, TargetPoolID: routingIPType},
		{ID: disabledPoolID, DeploymentID: config.DefaultDeploymentID, Enabled: true, TargetPoolID: disabledPoolID},
	})
	router := NewRouter(rules, NewStaticPoolPolicyProvider(poolPoliciesFromSnapshot(config.DefaultDeploymentID, snapshot.ExecutorPools)), registry, NewStickyStore(nil), nil)

	if outcome := router.Evaluate(RouteRequest{DeploymentID: config.DefaultDeploymentID, IngressType: IngressTypeREST}); !outcome.OK || outcome.WorkerID != "residential-worker" {
		t.Fatalf("residential route = %+v", outcome)
	}
	disabledRouter := NewRouter(NewStaticRuleProvider([]RoutingRule{{ID: disabledPoolID, DeploymentID: config.DefaultDeploymentID, Enabled: true, TargetPoolID: disabledPoolID}}), NewStaticPoolPolicyProvider(poolPoliciesFromSnapshot(config.DefaultDeploymentID, snapshot.ExecutorPools)), registry, NewStickyStore(nil), nil)
	if outcome := disabledRouter.Evaluate(RouteRequest{DeploymentID: config.DefaultDeploymentID, IngressType: IngressTypeREST}); outcome.OK {
		t.Fatalf("disabled route unexpectedly selected worker: %+v", outcome)
	}
}

func TestRouterFiltersProxyClaimsBeforeStickyAndCapacitySelection(t *testing.T) {
	t.Parallel()

	const proxyID = "proxy-profile"
	rule := RoutingRule{
		ID: "proxy-route", DeploymentID: routingDeploymentID, Enabled: true, TargetPoolID: routingPoolID,
		StickySessionTTLSeconds: 90, AllowStickyFallback: true,
	}
	policy := PoolPolicy{
		DeploymentID: routingDeploymentID, PoolID: routingPoolID, Enabled: true,
		UpstreamProxyID: proxyID, TrustedRemoteResolution: true,
	}
	stale := routingCandidate("stale-worker")
	stale.UpstreamProxyID = "stale-profile"
	stale.ProtocolMinor = 2
	old := routingCandidate("old-worker")
	old.UpstreamProxyID = proxyID
	old.ProtocolMinor = 1
	matching := routingCandidate("matching-worker")
	matching.UpstreamProxyID = proxyID
	matching.SupportedProtocolMinor = 2
	matching.ProtocolMinor = 2
	matching.ActiveRequests = 3

	for _, sticky := range []bool{false, true} {
		store := NewStickyStore(nil)
		request := RouteRequest{DeploymentID: routingDeploymentID, IngressType: IngressTypeREST}
		if sticky {
			request.StickySessionID = routingSessionID
			store.Set(routingDeploymentID, routingSessionID, stale.WorkerID, time.Minute)
		}
		router := NewRouter(
			NewStaticRuleProvider([]RoutingRule{rule}), NewStaticPoolPolicyProvider([]PoolPolicy{policy}),
			testRoutingCandidates{routingPoolID: {stale, old, matching}}, store, nil,
		)
		outcome := router.Evaluate(request)
		if !outcome.OK || outcome.WorkerID != matching.WorkerID || outcome.Sticky != sticky {
			t.Fatalf("sticky=%v outcome = %+v", sticky, outcome)
		}
		if outcome.UpstreamProxyID != proxyID || !outcome.TrustedRemoteResolution || outcome.StickySessionTTLSeconds != 90 || outcome.ProtocolMinor != 2 {
			t.Fatalf("sticky=%v route metadata = %+v", sticky, outcome)
		}
	}

	stale.AvailableCap = 0
	old.AvailableCap = 0
	withoutMatching := NewRouter(
		NewStaticRuleProvider([]RoutingRule{rule}), NewStaticPoolPolicyProvider([]PoolPolicy{policy}),
		testRoutingCandidates{routingPoolID: {stale, old}}, NewStickyStore(nil), nil,
	).Evaluate(RouteRequest{DeploymentID: routingDeploymentID, IngressType: IngressTypeREST})
	if withoutMatching.ErrorCode != RouteErrUnavailable {
		t.Fatalf("ineligible full candidates outcome = %+v, want %s", withoutMatching, RouteErrUnavailable)
	}
}

func TestPoolPoliciesFromSnapshotIncludesUpstreamProxy(t *testing.T) {
	t.Parallel()

	const proxyID = "proxy-profile"
	policies := poolPoliciesFromSnapshot(routingDeploymentID, []config.ExecutorPool{
		{ID: "direct", Enabled: true},
		{ID: "proxy", Enabled: true, UpstreamProxy: &config.ExecutorPoolUpstreamProxy{ID: proxyID, TrustedRemoteResolution: true}},
	})
	if len(policies) != 2 {
		t.Fatalf("policies = %+v", policies)
	}
	if policies[0].UpstreamProxyID != "" || policies[0].TrustedRemoteResolution {
		t.Fatalf("direct policy = %+v", policies[0])
	}
	if policies[1].UpstreamProxyID != proxyID || !policies[1].TrustedRemoteResolution {
		t.Fatalf("proxy policy = %+v", policies[1])
	}
}

func TestDeriveProviderSessionID(t *testing.T) {
	t.Parallel()

	route := RouteOutcome{PoolID: "pool-au", UpstreamProxyID: "brightdata-resi", StickySessionTTLSeconds: 900}
	const want = "ab280a385a3bef76e06d414de3d71b46"
	got := deriveProviderSessionID(testDeploymentID, route, "AU", testProxyRegion, routingIPType, testStickySessionID)
	if got != want {
		t.Fatalf("provider session ID = %q, want %q", got, want)
	}

	changedRoutes := []RouteOutcome{
		{PoolID: "pool-nz", UpstreamProxyID: route.UpstreamProxyID, StickySessionTTLSeconds: route.StickySessionTTLSeconds},
		{PoolID: route.PoolID, UpstreamProxyID: "other-proxy", StickySessionTTLSeconds: route.StickySessionTTLSeconds},
	}
	for _, changed := range changedRoutes {
		if changedID := deriveProviderSessionID(testDeploymentID, changed, "AU", testProxyRegion, routingIPType, testStickySessionID); changedID == got {
			t.Errorf("changed route produced unchanged provider session ID: %+v", changed)
		}
	}
	for _, inputs := range [][5]string{
		{"deployment-2", "AU", testProxyRegion, routingIPType, testStickySessionID},
		{testDeploymentID, "NZ", testProxyRegion, routingIPType, testStickySessionID},
		{testDeploymentID, "AU", "vic", routingIPType, testStickySessionID},
		{testDeploymentID, "AU", testProxyRegion, "datacenter", testStickySessionID},
		{testDeploymentID, "AU", testProxyRegion, routingIPType, "checkout-43"},
	} {
		if changedID := deriveProviderSessionID(inputs[0], route, inputs[1], inputs[2], inputs[3], inputs[4]); changedID == got {
			t.Errorf("changed inputs %q produced unchanged provider session ID", inputs)
		}
	}

	withoutProxy := route
	withoutProxy.UpstreamProxyID = ""
	withoutTTL := route
	withoutTTL.StickySessionTTLSeconds = 0
	for name, value := range map[string]string{
		"empty sticky ID": deriveProviderSessionID(testDeploymentID, route, "AU", testProxyRegion, routingIPType, ""),
		"zero TTL":        deriveProviderSessionID(testDeploymentID, withoutTTL, "AU", testProxyRegion, routingIPType, testStickySessionID),
		"empty proxy ID":  deriveProviderSessionID(testDeploymentID, withoutProxy, "AU", testProxyRegion, routingIPType, testStickySessionID),
	} {
		if value != "" {
			t.Errorf("%s provider session ID = %q, want empty", name, value)
		}
	}
}
