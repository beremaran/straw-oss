package control

import (
	"sync"
	"time"
)

// MatchConditions is a routing rule's match shape
// (docs/planning/10-routing-model.md). Any non-empty field is a hard
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

// RoutingRule is one tenant-scoped routing rule from the immutable snapshot
// evaluated at request start.
type RoutingRule struct {
	ID                      string
	TenantID                string
	Priority                int
	Enabled                 bool
	Match                   MatchConditions
	TargetPoolID            string
	StickySessionTTLSeconds uint32
	AllowStickyFallback     bool
	ConfigVersion           uint64
}

// PoolPolicy is the per-pool routing policy (docs/planning/10: "health is
// ready or degraded with pool policy allow_degraded_workers=true").
// AllowedCountries/AllowedRegions/AllowedIPTypes are the pool's P0 capability
// restrictions (docs/planning/26): a non-empty list requires every one of a
// candidate's claimed capability values to be in the list; empty means
// unrestricted.
type PoolPolicy struct {
	TenantID             string
	PoolID               string
	AllowDegradedWorkers bool
	AllowedCountries     []string
	AllowedRegions       []string
	AllowedIPTypes       []string
}

// RouteRequest is the evaluated request context: client hints plus
// request-derived attributes (ingress_type is always "rest" for P0 REST
// transport; target_host comes from the request URL).
type RouteRequest struct {
	TenantID        string
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
	OK            bool
	ErrorCode     string
	RuleID        string
	PoolID        string
	WorkerID      string
	ExecutorType  string
	SessionID     string
	AssignSubject string
	Sticky        bool
}

// RuleProvider returns the immutable routing-rule snapshot for a tenant,
// captured once at request start (docs/planning/10).
type RuleProvider interface {
	RulesForTenant(tenantID string) []RoutingRule
}

// PoolPolicyProvider returns the routing policy for a tenant+pool. Unknown
// pools default to AllowDegradedWorkers=false.
type PoolPolicyProvider interface {
	PoolPolicy(tenantID, poolID string) PoolPolicy
}

// CandidateSource returns eligible worker candidates for a tenant+pool
// (implemented by *WorkerRegistry).
type CandidateSource interface {
	CandidatesForPool(tenantID, poolID string) []PoolCandidate
}

// Router evaluates routing rules and selects an eligible executor.
type Router struct {
	rules      RuleProvider
	policies   PoolPolicyProvider
	candidates CandidateSource
	sticky     StickyBackend
	now        func() time.Time

	mu    sync.Mutex
	rrIdx map[string]int // round-robin cursor per tenant+pool
}

// NewRouter builds a Router. sticky may be an in-process *StickyStore (tests,
// no-Redis dev) or a *RedisStickyStore (P0 durable-ephemeral backing); now
// may be nil (defaults to time.Now).
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

	for _, rule := range rules {
		policy := rt.policies.PoolPolicy(req.TenantID, rule.TargetPoolID)
		ordinary := rt.eligibleCandidates(req, rule, policy)

		candidates := filterFingerprintCandidates(ordinary, req.FingerprintProfile)
		if len(ordinary) > 0 && len(candidates) == 0 && namedFingerprintRequested(req.FingerprintProfile) {
			profileRejected = true

			continue
		}

		if req.StickySessionID != "" {
			if outcome, handled := rt.evaluateSticky(req, rule, candidates); handled {
				return outcome
			}

			continue
		}

		if picked, ok := rt.selectExecutor(req.TenantID, rule.TargetPoolID, candidates); ok {
			return RouteOutcome{
				OK: true, RuleID: rule.ID, PoolID: rule.TargetPoolID,
				WorkerID: picked.WorkerID, ExecutorType: picked.ExecutorType,
				SessionID: picked.SessionID, AssignSubject: picked.AssignSubject,
			}
		}
	}

	if profileRejected {
		return RouteOutcome{ErrorCode: RouteErrUnsupportedFingerprint}
	}

	return RouteOutcome{ErrorCode: RouteErrUnavailable}
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
func (rt *Router) evaluateSticky(req RouteRequest, rule RoutingRule, candidates []PoolCandidate) (RouteOutcome, bool) {
	pinned, ok := rt.sticky.Get(req.TenantID, req.StickySessionID)
	if ok {
		for _, c := range candidates {
			if c.WorkerID == pinned {
				rt.sticky.Refresh(req.TenantID, req.StickySessionID, pinned, stickyTTL(rule))

				return RouteOutcome{
					OK: true, RuleID: rule.ID, PoolID: rule.TargetPoolID,
					WorkerID: c.WorkerID, ExecutorType: c.ExecutorType,
					SessionID: c.SessionID, AssignSubject: c.AssignSubject, Sticky: true,
				}, true
			}
		}
		// Pinned target not in this rule's eligible pool: unavailable.
		if !rule.AllowStickyFallback {
			return RouteOutcome{ErrorCode: RouteErrStickyUnavailable}, true
		}
		// Fallback permitted: fall through to a fresh selection below.
	}

	picked, ok := rt.selectExecutor(req.TenantID, rule.TargetPoolID, candidates)
	if !ok {
		return RouteOutcome{}, false
	}

	rt.sticky.Set(req.TenantID, req.StickySessionID, picked.WorkerID, stickyTTL(rule))

	return RouteOutcome{
		OK: true, RuleID: rule.ID, PoolID: rule.TargetPoolID,
		WorkerID: picked.WorkerID, ExecutorType: picked.ExecutorType,
		SessionID: picked.SessionID, AssignSubject: picked.AssignSubject, Sticky: true,
	}, true
}

func stickyTTL(rule RoutingRule) time.Duration {
	return time.Duration(rule.StickySessionTTLSeconds) * time.Second
}

// matchingRules returns enabled rules for the tenant whose match conditions
// are satisfied by req, in ascending priority order.
func (rt *Router) matchingRules(req RouteRequest) []RoutingRule {
	all := rt.rules.RulesForTenant(req.TenantID)

	out := make([]RoutingRule, 0, len(all))
	for _, r := range all {
		if r.TenantID != req.TenantID || !r.Enabled {
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

// matchesField reports whether an optional exact-match constraint is
// satisfied: unset on either side means no preference.
func matchesField(want, got string) bool {
	return want == "" || got == "" || want == got
}

func matchesTags(want, got []string) bool {
	return len(want) == 0 || subset(want, got)
}

func matchesHost(pattern, host string) bool {
	return pattern == "" || host == "" || hostMatches(pattern, host)
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
// request capability constraints (docs/planning/10 "capabilities satisfy
// all request constraints").
func (rt *Router) eligibleCandidates(req RouteRequest, rule RoutingRule, policy PoolPolicy) []PoolCandidate {
	all := rt.candidates.CandidatesForPool(req.TenantID, rule.TargetPoolID)

	out := make([]PoolCandidate, 0, len(all))
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

		// AvailableCap is the worker's authoritative admission signal. A
		// zero value means the session cannot accept an assignment even when
		// the reported active/max pair has not caught up yet.
		if c.AvailableCap == 0 {
			continue
		}

		out = append(out, c)
	}

	return out
}

func capabilitySatisfies(c PoolCandidate, req RouteRequest) bool {
	checks := []bool{
		len(req.Tags) == 0 || subset(req.Tags, c.Tags),
		req.Country == "" || len(c.Countries) == 0 || containsString(c.Countries, req.Country),
		req.Region == "" || len(c.Regions) == 0 || containsString(c.Regions, req.Region),
		req.IPType == "" || len(c.IPTypes) == 0 || containsString(c.IPTypes, req.IPType),
		req.IngressType == "" || len(c.IngressModes) == 0 || containsString(c.IngressModes, req.IngressType),
	}

	return allTrue(checks)
}

// poolAllows reports whether a candidate's claimed capabilities fall within
// the pool's restrictions (docs/planning/26 allowed_ip_types/allowed_countries/
// allowed_regions): a non-empty restriction requires every value the
// candidate claims for that dimension to be in the allowed list; an empty
// restriction is unrestricted regardless of what the candidate claims.
func poolAllows(c PoolCandidate, policy PoolPolicy) bool {
	checks := []bool{
		len(policy.AllowedCountries) == 0 || subset(c.Countries, policy.AllowedCountries),
		len(policy.AllowedRegions) == 0 || subset(c.Regions, policy.AllowedRegions),
		len(policy.AllowedIPTypes) == 0 || subset(c.IPTypes, policy.AllowedIPTypes),
	}

	return allTrue(checks)
}

// selectExecutor picks the least-loaded eligible candidate, with a
// round-robin tie breaker per tenant+pool (docs/planning/10 "Executor
// Selection").
func (rt *Router) selectExecutor(tenantID, poolID string, candidates []PoolCandidate) (PoolCandidate, bool) {
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
	key := tenantID + "\x00" + poolID
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

// StaticRuleProvider serves an in-memory routing-rule set, grouped by
// tenant. Tests and P0 wiring construct this directly; a Postgres-backed
// snapshot provider is future work.
type StaticRuleProvider struct {
	mu       sync.Mutex
	byTenant map[string][]RoutingRule
}

// NewStaticRuleProvider builds a StaticRuleProvider from a flat rule list.
func NewStaticRuleProvider(rules []RoutingRule) *StaticRuleProvider {
	p := &StaticRuleProvider{byTenant: make(map[string][]RoutingRule)}
	for _, r := range rules {
		p.byTenant[r.TenantID] = append(p.byTenant[r.TenantID], r)
	}

	return p
}

// RulesForTenant returns a copy of the rules registered for tenantID.
func (p *StaticRuleProvider) RulesForTenant(tenantID string) []RoutingRule {
	p.mu.Lock()
	defer p.mu.Unlock()

	rules := p.byTenant[tenantID]
	out := make([]RoutingRule, len(rules))
	copy(out, rules)

	return out
}

// StaticPoolPolicyProvider serves an in-memory pool-policy set.
type StaticPoolPolicyProvider struct {
	mu       sync.Mutex
	policies map[string]PoolPolicy // key: tenantID + "\x00" + poolID
}

// NewStaticPoolPolicyProvider builds a StaticPoolPolicyProvider from a flat
// policy list.
func NewStaticPoolPolicyProvider(policies []PoolPolicy) *StaticPoolPolicyProvider {
	p := &StaticPoolPolicyProvider{policies: make(map[string]PoolPolicy)}
	for _, pol := range policies {
		p.policies[pol.TenantID+"\x00"+pol.PoolID] = pol
	}

	return p
}

// PoolPolicy returns the registered policy for tenantID+poolID, or the zero
// value (AllowDegradedWorkers=false) if none was registered.
func (p *StaticPoolPolicyProvider) PoolPolicy(tenantID, poolID string) PoolPolicy {
	p.mu.Lock()
	defer p.mu.Unlock()

	return p.policies[tenantID+"\x00"+poolID]
}

// ---- sticky session store ----

type stickyEntry struct {
	workerID  string
	expiresAt time.Time
}

// StickyStore is an in-process emulation of the canonical Redis sticky key
// structure (docs/planning/10): key
// straw:sticky:<tenant_id>:<sticky_session_id>, TTL from the matched rule,
// refreshed on each use. It is used for tests and no-Redis dev; production
// P0 wiring uses RedisStickyStore (sticky_redis.go), which shares the same
// key shape and TTL-refresh semantics.
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

func stickyKey(tenantID, sessionID string) string {
	return "straw:sticky:" + tenantID + ":" + sessionID
}

// Get returns the pinned worker_id if present and unexpired.
func (s *StickyStore) Get(tenantID, sessionID string) (string, bool) {
	s.mu.Lock()
	defer s.mu.Unlock()

	e, ok := s.entries[stickyKey(tenantID, sessionID)]
	if !ok || s.now().After(e.expiresAt) {
		return "", false
	}

	return e.workerID, true
}

// Set pins sessionID to workerID with the given TTL.
func (s *StickyStore) Set(tenantID, sessionID, workerID string, ttl time.Duration) {
	s.mu.Lock()
	defer s.mu.Unlock()

	s.entries[stickyKey(tenantID, sessionID)] = stickyEntry{workerID: workerID, expiresAt: s.now().Add(ttl)}
}

// Refresh extends the TTL of an existing pin on use.
func (s *StickyStore) Refresh(tenantID, sessionID, workerID string, ttl time.Duration) {
	s.Set(tenantID, sessionID, workerID, ttl)
}
