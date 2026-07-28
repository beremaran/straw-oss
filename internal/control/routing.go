package control

import (
	"crypto/sha256"
	"encoding/hex"
	"strings"
	"sync"
	"time"
)

// MatchConditions is a routing rule's match shape
// (docs/public/architecture.md). Any non-empty field is a hard
// constraint against the corresponding request hint; missing hints (empty
// request field) mean no preference and always pass.
type MatchConditions struct {
	Tags        []string
	Country     string
	Region      string
	IPType      string
	IngressType string
	TargetHost  string // exact host, or "*.example.com" suffix wildcard
}

// RoutingRule is one deployment-scoped rule from the immutable snapshot
// evaluated at request start.
type RoutingRule struct {
	ID                      string
	DeploymentID            string
	Priority                int
	Enabled                 bool
	Match                   MatchConditions
	TargetPoolID            string
	StickySessionTTLSeconds uint32
	AllowStickyFallback     bool
	ConfigVersion           uint64
}

// PoolPolicy is the per-pool routing policy (docs/public/architecture.md: "health is
// ready or degraded with pool policy allow_degraded_workers=true").
// AllowedCountries/AllowedRegions/AllowedIPTypes are the pool's P0 capability
// restrictions (docs/public/architecture.md): a non-empty list requires every one of a
// candidate's claimed capability values to be in the list; empty means
// unrestricted.
type PoolPolicy struct {
	DeploymentID            string
	PoolID                  string
	Enabled                 bool
	ExecutorType            string
	Tags                    []string
	AllowDegradedWorkers    bool
	AllowedCountries        []string
	AllowedRegions          []string
	AllowedIPTypes          []string
	UpstreamProxyID         string
	TrustedRemoteResolution bool
}

// RouteRequest is the evaluated request context: client hints plus
// request-derived attributes (ingress_type is always "rest" for P0 REST
// transport; target_host comes from the request URL).
type RouteRequest struct {
	DeploymentID    string
	Tags            []string
	Country         string
	Region          string
	IPType          string
	IngressType     string
	TargetHost      string
	StickySessionID string
	// FingerprintProfile is the raw request intent. Empty and "default" are
	// baseline compatibility values; every other non-empty value requires an
	// exact capability match after ordinary eligibility filtering.
	FingerprintProfile string
}

// Route error codes, matching the canonical ErrorCode registry
// (internal/control/errors.go).
const (
	RouteErrNoMatch                = "route_no_match"
	RouteErrUnavailable            = "route_unavailable"
	RouteErrStickyUnavailable      = "sticky_session_unavailable"
	RouteErrCapacityExhausted      = "executor_capacity_exhausted"
	RouteErrUnsupportedFingerprint = "unsupported_fingerprint"
)

// RouteOutcome is the result of evaluating one RouteRequest.
type RouteOutcome struct {
	OK                      bool
	ErrorCode               string
	RuleID                  string
	PoolID                  string
	WorkerID                string
	ExecutorType            string
	SessionID               string
	AssignSubject           string
	Sticky                  bool
	UpstreamProxyID         string
	TrustedRemoteResolution bool
	StickySessionTTLSeconds uint32
	ProtocolMinor           uint32
}

// RuleProvider returns the immutable routing-rule snapshot for a deployment,
// captured once at request start (docs/public/architecture.md).
type RuleProvider interface {
	RulesForDeployment(deploymentID string) []RoutingRule
}

// PoolPolicyProvider returns the routing policy for a deployment and pool. Unknown
// pools default to AllowDegradedWorkers=false.
type PoolPolicyProvider interface {
	PoolPolicy(deploymentID, poolID string) PoolPolicy
}

// CandidateSource returns eligible worker candidates for a deployment and pool
// (implemented by *WorkerRegistry).
type CandidateSource interface {
	CandidatesForPool(deploymentID, poolID string) []PoolCandidate
}

// Router evaluates routing rules and selects an eligible executor.
type Router struct {
	rules      RuleProvider
	policies   PoolPolicyProvider
	candidates CandidateSource
	sticky     StickyBackend
	now        func() time.Time

	mu    sync.Mutex
	rrIdx map[string]int // round-robin cursor per deployment and pool
}

type ruleEvaluation struct {
	outcome           RouteOutcome
	selected          bool
	terminal          bool
	profileRejected   bool
	capacityExhausted bool
}

// NewRouter builds a Router. sticky may be an in-process StickyStore; now may
// be nil and defaults to time.Now.
func NewRouter(rules RuleProvider, policies PoolPolicyProvider, candidates CandidateSource, sticky StickyBackend, now func() time.Time) *Router {
	if now == nil {
		now = time.Now
	}

	return &Router{
		rules:      rules,
		policies:   policies,
		candidates: candidates,
		sticky:     sticky,
		now:        now,
		rrIdx:      make(map[string]int),
	}
}

// Evaluate selects a rule and eligible executor for req, in ascending
// priority order, applying sticky-session pinning when requested.
func (rt *Router) Evaluate(req RouteRequest) RouteOutcome {
	rules := rt.matchingRules(req)
	if len(rules) == 0 {
		return RouteOutcome{ErrorCode: RouteErrNoMatch}
	}

	profileRejected := false
	capacityRejected := false

	for _, rule := range rules {
		evaluation := rt.evaluateRule(req, rule)
		if evaluation.terminal || evaluation.selected {
			return evaluation.outcome
		}

		profileRejected = profileRejected || evaluation.profileRejected
		capacityRejected = capacityRejected || evaluation.capacityExhausted
	}

	if profileRejected {
		return RouteOutcome{ErrorCode: RouteErrUnsupportedFingerprint}
	}

	if capacityRejected {
		return RouteOutcome{ErrorCode: RouteErrCapacityExhausted}
	}

	return RouteOutcome{ErrorCode: RouteErrUnavailable}
}

func (rt *Router) evaluateRule(req RouteRequest, rule RoutingRule) ruleEvaluation {
	policy := rt.policies.PoolPolicy(req.DeploymentID, rule.TargetPoolID)
	ordinary, atCapacity := rt.eligibleCandidates(req, rule, policy)
	candidates := filterFingerprintCandidates(ordinary, req.FingerprintProfile)

	evaluation := ruleEvaluation{capacityExhausted: atCapacity}
	if len(ordinary) > 0 && len(candidates) == 0 && namedFingerprintRequested(req.FingerprintProfile) {
		evaluation.profileRejected = true

		return evaluation
	}

	if req.StickySessionID != "" {
		outcome, handled := rt.evaluateSticky(req, rule, policy, candidates)
		if handled {
			evaluation.outcome = outcome
			evaluation.terminal = true
			evaluation.selected = outcome.OK

			return evaluation
		}

		return evaluation
	}

	picked, ok := rt.selectExecutor(req.DeploymentID, rule.TargetPoolID, candidates)
	if !ok {
		return evaluation
	}

	evaluation.outcome = successfulRouteOutcome(rule, policy, picked, false)
	evaluation.selected = true

	return evaluation
}

func namedFingerprintRequested(profile string) bool {
	return profile != "" && profile != defaultFingerprintProfileName
}

func filterFingerprintCandidates(candidates []PoolCandidate, requested string) []PoolCandidate {
	if !namedFingerprintRequested(requested) {
		return candidates
	}

	out := make([]PoolCandidate, 0, len(candidates))
	for _, candidate := range candidates {
		if containsString(candidate.SupportedFingerprintProfiles, requested) {
			out = append(out, candidate)
		}
	}

	return out
}

// evaluateSticky handles one rule under a sticky session ID. handled=false
// means "no usable pin under this rule, fall through to the next rule" — it
// does not fall through once a rule has attempted a pin and exhausted
// fallback; that returns a final outcome.
func (rt *Router) evaluateSticky(req RouteRequest, rule RoutingRule, policy PoolPolicy, candidates []PoolCandidate) (RouteOutcome, bool) {
	pinned, ok := rt.sticky.Get(req.DeploymentID, req.StickySessionID)
	if ok {
		for _, c := range candidates {
			if c.WorkerID == pinned {
				rt.sticky.Refresh(req.DeploymentID, req.StickySessionID, pinned, stickyTTL(rule))

				return successfulRouteOutcome(rule, policy, c, true), true
			}
		}
		// Pinned target not in this rule's eligible pool: unavailable.
		if !rule.AllowStickyFallback {
			return RouteOutcome{ErrorCode: RouteErrStickyUnavailable}, true
		}
		// Fallback permitted: fall through to a fresh selection below.
	}

	picked, ok := rt.selectExecutor(req.DeploymentID, rule.TargetPoolID, candidates)
	if !ok {
		return RouteOutcome{}, false
	}

	rt.sticky.Set(req.DeploymentID, req.StickySessionID, picked.WorkerID, stickyTTL(rule))

	return successfulRouteOutcome(rule, policy, picked, true), true
}

func successfulRouteOutcome(rule RoutingRule, policy PoolPolicy, candidate PoolCandidate, sticky bool) RouteOutcome {
	return RouteOutcome{
		OK: true, RuleID: rule.ID, PoolID: rule.TargetPoolID,
		WorkerID: candidate.WorkerID, ExecutorType: candidate.ExecutorType,
		SessionID: candidate.SessionID, AssignSubject: candidate.AssignSubject, Sticky: sticky,
		UpstreamProxyID: policy.UpstreamProxyID, TrustedRemoteResolution: policy.TrustedRemoteResolution,
		StickySessionTTLSeconds: rule.StickySessionTTLSeconds, ProtocolMinor: candidate.ProtocolMinor,
	}
}

func stickyTTL(rule RoutingRule) time.Duration {
	return time.Duration(rule.StickySessionTTLSeconds) * time.Second
}

func deriveProviderSessionID(deploymentID string, route RouteOutcome, country, region, ipType, stickyID string) string {
	if stickyID == "" || route.StickySessionTTLSeconds == 0 || route.UpstreamProxyID == "" {
		return ""
	}

	input := strings.Join([]string{
		"straw-provider-session-v1", deploymentID, route.PoolID, route.UpstreamProxyID,
		country, region, ipType, stickyID,
	}, "\x00")
	sum := sha256.Sum256([]byte(input))

	return hex.EncodeToString(sum[:16])
}

// matchingRules returns enabled rules for the deployment whose match conditions
// are satisfied by req, in ascending priority order.
func (rt *Router) matchingRules(req RouteRequest) []RoutingRule {
	all := rt.rules.RulesForDeployment(req.DeploymentID)

	out := make([]RoutingRule, 0, len(all))
	for _, r := range all {
		if r.DeploymentID != req.DeploymentID || !r.Enabled {
			continue
		}

		if !matches(r.Match, req) {
			continue
		}

		out = append(out, r)
	}

	sortRulesByPriority(out)

	return out
}

func sortRulesByPriority(rules []RoutingRule) {
	for i := 1; i < len(rules); i++ {
		for j := i; j > 0 && rules[j].Priority < rules[j-1].Priority; j-- {
			rules[j], rules[j-1] = rules[j-1], rules[j]
		}
	}
}

func matches(m MatchConditions, req RouteRequest) bool {
	return allTrue([]bool{
		matchesTags(m.Tags, req.Tags),
		matchesField(m.Country, req.Country),
		matchesField(m.Region, req.Region),
		matchesField(m.IPType, req.IPType),
		matchesField(m.IngressType, req.IngressType),
		matchesHost(m.TargetHost, req.TargetHost),
	})
}

// matchesField reports whether an exact-match constraint is satisfied. A
// configured constraint is not a wildcard: the request must supply the value
// and the values must match.
func matchesField(want, got string) bool {
	return want == "" || (got != "" && want == got)
}

func matchesTags(want, got []string) bool {
	return len(want) == 0 || subset(want, got)
}

func matchesHost(pattern, host string) bool {
	return pattern == "" || (host != "" && hostMatches(pattern, host))
}

func allTrue(checks []bool) bool {
	for _, c := range checks {
		if !c {
			return false
		}
	}

	return true
}

func hostMatches(pattern, host string) bool {
	if len(pattern) > 2 && pattern[:2] == "*." {
		suffix := pattern[1:] // ".example.com"

		return len(host) > len(suffix) && host[len(host)-len(suffix):] == suffix
	}

	return pattern == host
}

// eligibleCandidates filters a pool's candidates by degraded policy and
// request capability constraints (docs/public/architecture.md "capabilities satisfy
// all request constraints").
func (rt *Router) eligibleCandidates(req RouteRequest, rule RoutingRule, policy PoolPolicy) ([]PoolCandidate, bool) {
	if policy.PoolID != "" && !policy.Enabled {
		return nil, false
	}

	all := rt.candidates.CandidatesForPool(req.DeploymentID, rule.TargetPoolID)

	out := make([]PoolCandidate, 0, len(all))
	capacityEligible := 0

	for _, c := range all {
		if c.Degraded && !policy.AllowDegradedWorkers {
			continue
		}

		if !capabilitySatisfies(c, req) {
			continue
		}

		if !poolAllows(c, policy) {
			continue
		}

		capacityEligible++

		// AvailableCap is the worker's authoritative admission signal. A
		// zero value means the session cannot accept an assignment even when
		// the reported active/max pair has not caught up yet.
		if c.AvailableCap == 0 {
			continue
		}

		out = append(out, c)
	}

	return out, capacityEligible > 0 && len(out) == 0
}

func capabilitySatisfies(c PoolCandidate, req RouteRequest) bool {
	checks := []bool{
		len(req.Tags) == 0 || subset(req.Tags, c.Tags),
		req.Country == "" || (len(c.Countries) > 0 && containsString(c.Countries, req.Country)),
		req.Region == "" || (len(c.Regions) > 0 && containsString(c.Regions, req.Region)),
		req.IPType == "" || (len(c.IPTypes) > 0 && containsString(c.IPTypes, req.IPType)),
		req.IngressType == "" || (len(c.IngressModes) > 0 && containsString(c.IngressModes, req.IngressType)),
	}

	return allTrue(checks)
}

// poolAllows reports whether a candidate's claimed capabilities fall within
// the pool's restrictions (docs/public/architecture.md allowed_ip_types/allowed_countries/
// allowed_regions): a non-empty restriction requires every value the
// candidate claims for that dimension to be in the allowed list; an empty
// restriction is unrestricted regardless of what the candidate claims.
func poolAllows(c PoolCandidate, policy PoolPolicy) bool {
	if policy.PoolID != "" && !poolClaimsAllowed(c, policy) {
		return false
	}

	checks := []bool{
		len(policy.AllowedCountries) == 0 || subset(c.Countries, policy.AllowedCountries),
		len(policy.AllowedRegions) == 0 || subset(c.Regions, policy.AllowedRegions),
		len(policy.AllowedIPTypes) == 0 || subset(c.IPTypes, policy.AllowedIPTypes),
	}

	return allTrue(checks)
}

func poolClaimsAllowed(c PoolCandidate, policy PoolPolicy) bool {
	if policy.ExecutorType != "" && c.ExecutorType != policy.ExecutorType {
		return false
	}

	if c.UpstreamProxyID != policy.UpstreamProxyID {
		return false
	}

	if policy.UpstreamProxyID != "" && c.ProtocolMinor < upstreamProxyProtocolMinor {
		return false
	}

	return subset(policy.Tags, c.Tags)
}

// selectExecutor picks the least-loaded eligible candidate, with a
// round-robin tie breaker per deployment and pool.
func (rt *Router) selectExecutor(deploymentID, poolID string, candidates []PoolCandidate) (PoolCandidate, bool) {
	if len(candidates) == 0 {
		return PoolCandidate{}, false
	}

	minLoad := load(candidates[0])
	for _, c := range candidates[1:] {
		if l := load(c); l < minLoad {
			minLoad = l
		}
	}

	tied := make([]PoolCandidate, 0, len(candidates))
	for _, c := range candidates {
		if load(c) == minLoad {
			tied = append(tied, c)
		}
	}

	sortCandidatesByWorkerID(tied)

	rt.mu.Lock()
	key := deploymentID + "\x00" + poolID
	idx := rt.rrIdx[key] % len(tied)
	rt.rrIdx[key] = idx + 1
	rt.mu.Unlock()

	return tied[idx], true
}

func sortCandidatesByWorkerID(c []PoolCandidate) {
	for i := 1; i < len(c); i++ {
		for j := i; j > 0 && c[j].WorkerID < c[j-1].WorkerID; j-- {
			c[j], c[j-1] = c[j-1], c[j]
		}
	}
}

// load is a lower-is-less-loaded metric: fraction of capacity in use.
func load(c PoolCandidate) float64 {
	if c.MaxConcurrency == 0 {
		return 0
	}

	return float64(c.ActiveRequests) / float64(c.MaxConcurrency)
}

// ---- rule/policy providers (P0 in-memory) ----

// StaticRuleProvider serves deployment-scoped rules from memory.
type StaticRuleProvider struct {
	mu           sync.Mutex
	byDeployment map[string][]RoutingRule
}

// NewStaticRuleProvider builds a StaticRuleProvider from a flat rule list.
func NewStaticRuleProvider(rules []RoutingRule) *StaticRuleProvider {
	p := &StaticRuleProvider{byDeployment: make(map[string][]RoutingRule)}
	for _, r := range rules {
		p.byDeployment[r.DeploymentID] = append(p.byDeployment[r.DeploymentID], r)
	}

	return p
}

// RulesForDeployment returns a copy of the registered deployment rules.
func (p *StaticRuleProvider) RulesForDeployment(deploymentID string) []RoutingRule {
	p.mu.Lock()
	defer p.mu.Unlock()

	rules := p.byDeployment[deploymentID]
	out := make([]RoutingRule, len(rules))
	copy(out, rules)

	return out
}

// StaticPoolPolicyProvider serves an in-memory pool-policy set.
type StaticPoolPolicyProvider struct {
	mu       sync.Mutex
	policies map[string]PoolPolicy // key: deploymentID + "\x00" + poolID
}

// NewStaticPoolPolicyProvider builds a StaticPoolPolicyProvider from a flat
// policy list.
func NewStaticPoolPolicyProvider(policies []PoolPolicy) *StaticPoolPolicyProvider {
	p := &StaticPoolPolicyProvider{policies: make(map[string]PoolPolicy)}
	for _, pol := range policies {
		p.policies[pol.DeploymentID+"\x00"+pol.PoolID] = pol
	}

	return p
}

// PoolPolicy returns the registered policy for deploymentID+poolID, or the zero
// value (AllowDegradedWorkers=false) if none was registered.
func (p *StaticPoolPolicyProvider) PoolPolicy(deploymentID, poolID string) PoolPolicy {
	p.mu.Lock()
	defer p.mu.Unlock()

	return p.policies[deploymentID+"\x00"+poolID]
}

// ---- sticky session store ----

// StickyBackend stores optional session-to-worker pins.
type StickyBackend interface {
	Get(deploymentID, sessionID string) (string, bool)
	Set(deploymentID, sessionID, workerID string, ttl time.Duration)
	Refresh(deploymentID, sessionID, workerID string, ttl time.Duration)
}

type stickyEntry struct {
	workerID  string
	expiresAt time.Time
}

// StickyStore keeps optional session-to-worker pins in process.
type StickyStore struct {
	mu      sync.Mutex
	now     func() time.Time
	entries map[string]stickyEntry
}

// NewStickyStore builds a sticky store. now may be nil (defaults to
// time.Now).
func NewStickyStore(now func() time.Time) *StickyStore {
	if now == nil {
		now = time.Now
	}

	return &StickyStore{now: now, entries: make(map[string]stickyEntry)}
}

func stickyKey(deploymentID, sessionID string) string {
	return "straw:sticky:" + deploymentID + ":" + sessionID
}

// Get returns the pinned worker_id if present and unexpired.
func (s *StickyStore) Get(deploymentID, sessionID string) (string, bool) {
	s.mu.Lock()
	defer s.mu.Unlock()

	e, ok := s.entries[stickyKey(deploymentID, sessionID)]
	if !ok || s.now().After(e.expiresAt) {
		return "", false
	}

	return e.workerID, true
}

// Set pins sessionID to workerID with the given TTL.
func (s *StickyStore) Set(deploymentID, sessionID, workerID string, ttl time.Duration) {
	s.mu.Lock()
	defer s.mu.Unlock()

	s.entries[stickyKey(deploymentID, sessionID)] = stickyEntry{workerID: workerID, expiresAt: s.now().Add(ttl)}
}

// Refresh extends the TTL of an existing pin on use.
func (s *StickyStore) Refresh(deploymentID, sessionID, workerID string, ttl time.Duration) {
	s.Set(deploymentID, sessionID, workerID, ttl)
}
