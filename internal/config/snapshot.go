package config

// Default deployment identifiers keep the existing wire protocol internal.
const (
	DefaultDeploymentID = "default"
	DefaultPoolID       = "default"
	defaultTimeoutMS    = 60_000
	maximumTimeoutMS    = 300_000
)

// Snapshot is the immutable policy used by one Straw deployment.
type Snapshot struct {
	ConfigVersion       uint64
	DefaultTimeoutMs    uint64
	MaxTimeoutMs        uint64
	RoutingRules        []RoutingRule
	ExecutorPools       []ExecutorPool
	DenyRules           []DenyRule
	InjectionPolicies   []InjectionPolicy
	FingerprintProfiles []FingerprintProfile
}

// MatchConditions determines whether a route applies.
type MatchConditions struct {
	Tags        []string
	Country     string
	Region      string
	IPType      string
	IngressType string
	TargetHost  string
}

// RoutingRule selects a worker pool for matching requests.
type RoutingRule struct {
	ID                      string
	Priority                int
	Enabled                 bool
	Match                   MatchConditions
	TargetPoolID            string
	StickySessionTTLSeconds uint32
	AllowStickyFallback     bool
}

// ExecutorPool describes an eligible worker group.
type ExecutorPool struct {
	ID                   string
	ExecutorType         string
	Tags                 []string
	Enabled              bool
	AllowDegradedWorkers bool
	AllowedIPTypes       []string
	AllowedCountries     []string
	AllowedRegions       []string
}

// DenyRule blocks or explicitly allows a destination pattern.
type DenyRule struct {
	ID             string
	RuleType       string
	Action         string
	Enabled        bool
	Reason         string
	RawPattern     string
	NormalizedHost string
	NormalizedCIDR string
	NormalizedIP   string
	NormalizedName string
}

// InjectionOperation changes one upstream request header.
type InjectionOperation struct {
	Op          string
	HeaderName  string
	ValueBase64 string
}

// InjectionPolicy is an ordered group of header operations.
type InjectionPolicy struct {
	ID         string
	Enabled    bool
	Operations []InjectionOperation
}

// FingerprintProfile describes a supported outbound TLS profile.
type FingerprintProfile struct {
	Name              string
	ScopeType         string
	SupportedByWorker bool
	Enabled           bool
	ExecutorType      string
	ProfileRef        string
	ContractRevision  string
}

// NewSnapshot creates deployment policy with default timeouts.
func NewSnapshot(version uint64) Snapshot {
	return Snapshot{ConfigVersion: version, DefaultTimeoutMs: defaultTimeoutMS, MaxTimeoutMs: maximumTimeoutMS}
}

// Clone returns a deep copy of the policy.
func (s Snapshot) Clone() Snapshot {
	out := s
	out.RoutingRules = cloneRoutingRules(s.RoutingRules)
	out.ExecutorPools = cloneExecutorPools(s.ExecutorPools)
	out.DenyRules = append([]DenyRule(nil), s.DenyRules...)
	out.InjectionPolicies = cloneInjectionPolicies(s.InjectionPolicies)
	out.FingerprintProfiles = append([]FingerprintProfile(nil), s.FingerprintProfiles...)

	return out
}

func cloneRoutingRules(in []RoutingRule) []RoutingRule {
	out := make([]RoutingRule, len(in))
	for i, rule := range in {
		rule.Match.Tags = append([]string(nil), rule.Match.Tags...)
		out[i] = rule
	}

	return out
}

func cloneExecutorPools(in []ExecutorPool) []ExecutorPool {
	out := make([]ExecutorPool, len(in))
	for i, pool := range in {
		pool.Tags = append([]string(nil), pool.Tags...)
		pool.AllowedIPTypes = append([]string(nil), pool.AllowedIPTypes...)
		pool.AllowedCountries = append([]string(nil), pool.AllowedCountries...)
		pool.AllowedRegions = append([]string(nil), pool.AllowedRegions...)
		out[i] = pool
	}

	return out
}

func cloneInjectionPolicies(in []InjectionPolicy) []InjectionPolicy {
	out := make([]InjectionPolicy, len(in))
	for i, policy := range in {
		policy.Operations = append([]InjectionOperation(nil), policy.Operations...)
		out[i] = policy
	}

	return out
}
