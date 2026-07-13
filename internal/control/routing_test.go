package control

import (
	"context"
	"testing"
	"time"

	"github.com/beremaran/straw-oss/internal/config"
	strawpb "github.com/beremaran/straw-protos-go/straw/v1"
)

const (
	routingDeploymentID = defaultFingerprintProfileName
	routingExecutorType = errorCategoryEgress
	routingPoolID       = "pool"
	routingHost         = "api.example.com"
	routingRegion       = "ap-southeast-2"
	routingIPType       = "residential"
	routingTag          = "datacenter"
	routingSessionID    = "session-1"
	routingFallbackID   = "fallback"
	routingPreferredID  = "preferred"
	routingHostPoolID   = denyRuleTypeHost
	routingCountryID    = "country"
	routingRegionTag    = "region:au"
	routingWorkerB      = "worker-b"
	disabledPoolID      = "disabled-pool"
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
