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
	ConfigVersion       uint64               `json:"config_version"`
	DefaultTimeoutMs    uint64               `json:"default_timeout_ms"`
	MaxTimeoutMs        uint64               `json:"max_timeout_ms"`
	RoutingRules        []RoutingRule        `json:"routing_rules"`
	ExecutorPools       []ExecutorPool       `json:"executor_pools"`
	DenyRules           []DenyRule           `json:"destination_policy"`
	InjectionPolicies   []InjectionPolicy    `json:"injection_policies"`
	FingerprintProfiles []FingerprintProfile `json:"fingerprint_profiles"`
	WorkerSettings      []WorkerSetting      `json:"worker_settings"`
}

// MatchConditions determines whether a route applies.
type MatchConditions struct {
	Tags        []string `json:"tags,omitempty"`
	Country     string   `json:"country,omitempty"`
	Region      string   `json:"region,omitempty"`
	IPType      string   `json:"ip_type,omitempty"`
	IngressType string   `json:"ingress_type,omitempty"`
	TargetHost  string   `json:"target_host,omitempty"`
}

// RoutingRule selects a worker pool for matching requests.
type RoutingRule struct {
	ID                      string          `json:"id"`
	Priority                int             `json:"priority"`
	Enabled                 bool            `json:"enabled"`
	Match                   MatchConditions `json:"match"`
	TargetPoolID            string          `json:"target_pool_id"`
	StickySessionTTLSeconds uint32          `json:"sticky_session_ttl_seconds,omitempty"`
	AllowStickyFallback     bool            `json:"allow_sticky_fallback,omitempty"`
}

// ExecutorPool describes an eligible worker group.
type ExecutorPool struct {
	ID                   string   `json:"id"`
	ExecutorType         string   `json:"executor_type"`
	Tags                 []string `json:"tags,omitempty"`
	Enabled              bool     `json:"enabled"`
	AllowDegradedWorkers bool     `json:"allow_degraded_workers,omitempty"`
	AllowedIPTypes       []string `json:"allowed_ip_types,omitempty"`
	AllowedCountries     []string `json:"allowed_countries,omitempty"`
	AllowedRegions       []string `json:"allowed_regions,omitempty"`
}

// DenyRule blocks or explicitly allows a destination pattern.
type DenyRule struct {
	ID             string `json:"id"`
	RuleType       string `json:"rule_type"`
	Action         string `json:"action"`
	Enabled        bool   `json:"enabled"`
	Reason         string `json:"reason,omitempty"`
	RawPattern     string `json:"raw_pattern"`
	NormalizedHost string `json:"normalized_host,omitempty"`
	NormalizedCIDR string `json:"normalized_cidr,omitempty"`
	NormalizedIP   string `json:"normalized_ip,omitempty"`
	NormalizedName string `json:"normalized_name,omitempty"`
}

// InjectionOperation changes one upstream request header.
type InjectionOperation struct {
	Op          string `json:"op"`
	HeaderName  string `json:"header_name"`
	ValueBase64 string `json:"value_base64,omitempty"`
}

// InjectionPolicy is an ordered group of header operations.
type InjectionPolicy struct {
	ID         string               `json:"id"`
	Enabled    bool                 `json:"enabled"`
	Operations []InjectionOperation `json:"operations"`
}

// FingerprintProfile describes a supported outbound TLS profile.
type FingerprintProfile struct {
	Name              string `json:"name"`
	ScopeType         string `json:"scope_type,omitempty"`
	SupportedByWorker bool   `json:"supported_by_worker"`
	Enabled           bool   `json:"enabled"`
	ExecutorType      string `json:"executor_type"`
	ProfileRef        string `json:"profile_ref"`
	ContractRevision  string `json:"contract_revision,omitempty"`
}

// WorkerSetting is the durable administrative override for one worker.
// Draining and disabled workers finish existing requests but receive no new assignments.
type WorkerSetting struct {
	WorkerID string `json:"worker_id"`
	Enabled  bool   `json:"enabled"`
	Draining bool   `json:"draining"`
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
	out.WorkerSettings = append([]WorkerSetting(nil), s.WorkerSettings...)

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
