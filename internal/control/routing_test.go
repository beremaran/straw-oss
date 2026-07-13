package control

import (
	"testing"
	"time"
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
		policies = append(policies, PoolPolicy{DeploymentID: routingDeploymentID, PoolID: rule.TargetPoolID})
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
	sticky.Set(routingDeploymentID, routingSessionID, "worker-b", time.Minute)
	rule := RoutingRule{ID: "sticky", DeploymentID: routingDeploymentID, Priority: 1, Enabled: true, TargetPoolID: routingPoolID, StickySessionTTLSeconds: 60}
	router := NewRouter(
		NewStaticRuleProvider([]RoutingRule{rule}),
		NewStaticPoolPolicyProvider([]PoolPolicy{{DeploymentID: routingDeploymentID, PoolID: routingPoolID}}),
		testRoutingCandidates{routingPoolID: {routingCandidate("worker-a"), routingCandidate("worker-b")}},
		sticky,
		nil,
	)

	outcome := router.Evaluate(RouteRequest{DeploymentID: routingDeploymentID, StickySessionID: routingSessionID, IngressType: IngressTypeREST})
	if !outcome.OK || !outcome.Sticky || outcome.WorkerID != "worker-b" {
		t.Fatalf("sticky outcome = %+v, want worker-b sticky selection", outcome)
	}
}

func TestRouterStickyFallback(t *testing.T) {
	sticky := NewStickyStore(nil)
	sticky.Set(routingDeploymentID, routingSessionID, "gone", time.Minute)
	rule := RoutingRule{ID: "fallback", DeploymentID: routingDeploymentID, Priority: 1, Enabled: true, TargetPoolID: routingPoolID, StickySessionTTLSeconds: 60, AllowStickyFallback: true}
	router := NewRouter(
		NewStaticRuleProvider([]RoutingRule{rule}),
		NewStaticPoolPolicyProvider([]PoolPolicy{{DeploymentID: routingDeploymentID, PoolID: routingPoolID}}),
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
