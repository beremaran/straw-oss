package config

// TenantSnapshot is the immutable tenant config view consumed by control-plane
// admission and routing decisions. It is assembled from the Postgres config
// stores keyed by (TenantID, ConfigVersion); in-flight requests keep the
// snapshot they captured at request start even if config changes during
// execution (docs/planning/25-dynamic-configuration.md).
//
// The carried policy types are config-layer data carriers, deliberately
// separate from the control-package runtime types (control.RoutingRule, ...)
// so this low-level package stays free of control imports. Tasks 22 and 24 map
// these carriers into the runtime router/dispatch types when they consume the
// snapshot.
type TenantSnapshot struct {
	TenantID      string
	ConfigVersion uint64

	RevokedAPIKeyIDs      []string
	RoutingRules          []RoutingRule
	ExecutorPools         []ExecutorPool
	DenyRules             []DenyRule
	InjectionPolicies     []InjectionPolicy
	FingerprintProfiles   []FingerprintProfile
	RateLimits            []RateLimitRule
	Quota                 QuotaConfig
	WorkerAdminStates     []WorkerAdminState
	TenantWorkerOverrides []TenantWorkerOverride
}

// MatchConditions is a routing rule's match shape (docs/planning/10).
type MatchConditions struct {
	Tags        []string
	Country     string
	Region      string
	IPType      string
	IngressType string
	TargetHost  string
}

// RoutingRule is one tenant-scoped routing rule (docs/planning/10). Deleted
// rules are excluded from the snapshot.
type RoutingRule struct {
	ID                      string
	Priority                int
	Enabled                 bool
	Match                   MatchConditions
	TargetPoolID            string
	StickySessionTTLSeconds uint32
	AllowStickyFallback     bool
}

// ExecutorPool is a tenant-visible pool (docs/planning/21). AllowDegradedWorkers
// sources the degraded-pool routing policy (docs/planning/10): a pool with
// this set to true lets Router select a degraded-health worker when no ready
// worker is available.
type ExecutorPool struct {
	ID                   string
	ExecutorType         string
	Tags                 []string
	Enabled              bool
	AllowDegradedWorkers bool
}

// DenyRule is one host/CIDR/CNAME deny or allow override (docs/planning/21).
type DenyRule struct {
	ID             string
	RuleType       string
	Action         string
	Enabled        bool
	RawPattern     string
	NormalizedHost string
	NormalizedCIDR string
	NormalizedIP   string
	NormalizedName string
}

// InjectionOperation is one ordered header operation. ValueBase64 is secret and
// is redacted in audit events (docs/planning/21).
type InjectionOperation struct {
	Op          string
	HeaderName  string
	ValueBase64 string
}

// InjectionPolicy is one tenant-scoped ordered set of header operations.
type InjectionPolicy struct {
	ID         string
	Enabled    bool
	Operations []InjectionOperation
}

// FingerprintProfile is an allowed profile name and worker compatibility. P0
// rows are seeded built-in globals only; there is no P0 write path.
type FingerprintProfile struct {
	Name              string
	ScopeType         string
	SupportedByWorker bool
	Enabled           bool
}

// RateLimitRule is one rate-limit dimension's configured limit (docs/planning/20).
type RateLimitRule struct {
	Dimension     string
	Key           string
	WindowSeconds uint32
	MaxRequests   uint32
	FailPolicy    string
}

// QuotaConfig is the tenant's monthly quota (docs/planning/20). A zero value
// means the tenant has no configured quota.
type QuotaConfig struct {
	Period              string
	RequestCountLimit   int64
	BandwidthBytesLimit int64
	CountOnAdmission    bool
	FailPolicy          string
	Enabled             bool
}

// WorkerAdminState is a durable global worker disable (docs/planning/21).
type WorkerAdminState struct {
	WorkerID       string
	Disabled       bool
	DisabledReason string
}

// TenantWorkerOverride is a durable tenant worker routing override (disable
// only in P0) (docs/planning/21).
type TenantWorkerOverride struct {
	WorkerID       string
	Disabled       bool
	DisabledReason string
}

// NewTenantSnapshot builds a snapshot with only identity, version, and revoked
// keys set, copying the slice so callers can keep their input buffer mutable.
// The richer policy fields are set directly by the Postgres assembler.
func NewTenantSnapshot(tenantID string, configVersion uint64, revokedAPIKeyIDs []string) TenantSnapshot {
	return TenantSnapshot{
		TenantID:         tenantID,
		ConfigVersion:    configVersion,
		RevokedAPIKeyIDs: append([]string(nil), revokedAPIKeyIDs...),
	}
}

// Clone returns a deep copy of the snapshot so cached snapshots never share
// mutable backing arrays with callers.
func (s TenantSnapshot) Clone() TenantSnapshot {
	out := s
	out.RevokedAPIKeyIDs = append([]string(nil), s.RevokedAPIKeyIDs...)
	out.RoutingRules = cloneRoutingRules(s.RoutingRules)
	out.ExecutorPools = cloneExecutorPools(s.ExecutorPools)
	out.DenyRules = append([]DenyRule(nil), s.DenyRules...)
	out.InjectionPolicies = cloneInjectionPolicies(s.InjectionPolicies)
	out.FingerprintProfiles = append([]FingerprintProfile(nil), s.FingerprintProfiles...)
	out.RateLimits = append([]RateLimitRule(nil), s.RateLimits...)
	out.WorkerAdminStates = append([]WorkerAdminState(nil), s.WorkerAdminStates...)
	out.TenantWorkerOverrides = append([]TenantWorkerOverride(nil), s.TenantWorkerOverrides...)

	return out
}

func cloneRoutingRules(in []RoutingRule) []RoutingRule {
	if in == nil {
		return nil
	}

	out := make([]RoutingRule, len(in))
	for i, r := range in {
		r.Match.Tags = append([]string(nil), r.Match.Tags...)
		out[i] = r
	}

	return out
}

func cloneExecutorPools(in []ExecutorPool) []ExecutorPool {
	if in == nil {
		return nil
	}

	out := make([]ExecutorPool, len(in))
	for i, p := range in {
		p.Tags = append([]string(nil), p.Tags...)
		out[i] = p
	}

	return out
}

func cloneInjectionPolicies(in []InjectionPolicy) []InjectionPolicy {
	if in == nil {
		return nil
	}

	out := make([]InjectionPolicy, len(in))
	for i, p := range in {
		p.Operations = append([]InjectionOperation(nil), p.Operations...)
		out[i] = p
	}

	return out
}
