package domain

import (
	"context"
	"time"
)

// RoutingRule defines how requests with specific tags should be handled.
// Rules are matched based on required/excluded tags and prioritized by Priority field.
type RoutingRule struct {
	// ID is the unique identifier for this rule.
	ID string `json:"id"`

	// Name is a human-readable name for the rule.
	Name string `json:"name"`

	// RequiredTags are the tags that a request MUST have to match this rule (AND logic).
	RequiredTags []string `json:"required_tags"`

	// ExcludedTags are tags that a request must NOT have to match this rule (NOT logic).
	ExcludedTags []string `json:"excluded_tags,omitempty"`

	// Priority determines rule precedence. Higher priority rules are evaluated first.
	Priority int `json:"priority"`

	// HardTimeout is the maximum time allowed for a request to complete.
	HardTimeout time.Duration `json:"hard_timeout,omitempty"`

	// RateLimitPerMinute is the maximum requests per minute for this rule's quota key.
	RateLimitPerMinute int `json:"rate_limit_per_minute,omitempty"`

	// RateLimitPerSecond is the maximum requests per second for this rule's quota key.
	RateLimitPerSecond int `json:"rate_limit_per_second,omitempty"`

	// AllowedEndpointTypes specifies which endpoint types can handle these requests.
	// Example: ["residential", "mobile"]
	AllowedEndpointTypes []string `json:"allowed_endpoint_types,omitempty"`

	// RequiredEndpointCaps are capability tags that endpoints must have.
	// Example: ["capability:stealth"]
	RequiredEndpointCaps []string `json:"required_endpoint_caps,omitempty"`

	// FingerprintPreset specifies the default browser TLS fingerprint to use.
	// Example: "chrome-130"
	FingerprintPreset string `json:"fingerprint_preset,omitempty"`

	// FingerprintABTest allows A/B testing different fingerprints.
	FingerprintABTest *ABConfig `json:"fingerprint_ab_test,omitempty"`

	// QuotaKey is the key used for rate limit bucketing (e.g., "target:amazon").
	QuotaKey string `json:"quota_key,omitempty"`

	// AllowInsecureTLS allows skipping TLS certificate verification for targets.
	AllowInsecureTLS bool `json:"allow_insecure_tls,omitempty"`

	// PinnedCertHash for certificate pinning (optional).
	PinnedCertHash string `json:"pinned_cert_hash,omitempty"`

	// RequestFilters defines bandwidth optimization filters.
	RequestFilters *RequestFilter `json:"request_filters,omitempty"`

	// EndpointPools defines tiered endpoint fallback pools.
	EndpointPools []EndpointPool `json:"endpoint_pools,omitempty"`

	// IsActive indicates whether this rule is currently active.
	IsActive bool `json:"is_active"`

	// Version for optimistic locking on updates.
	Version int `json:"version"`

	// CreatedAt is when the rule was created.
	CreatedAt time.Time `json:"created_at"`

	// UpdatedAt is when the rule was last modified.
	UpdatedAt time.Time `json:"updated_at"`
}

// ABConfig defines A/B testing configuration for fingerprints.
type ABConfig struct {
	// Variants are the different fingerprint options to test.
	Variants []ABVariant `json:"variants"`

	// Strategy determines how variants are selected.
	// Valid values: "random", "round_robin", "weighted"
	Strategy string `json:"strategy"`
}

// ABVariant represents a single variant in an A/B test.
type ABVariant struct {
	// Fingerprint is the TLS fingerprint preset ID.
	Fingerprint string `json:"fingerprint"`

	// Weight is the percentage weight for weighted selection (0-100).
	Weight int `json:"weight"`
}

// RequestFilter defines filters for bandwidth optimization.
// Matching requests are blocked before reaching endpoints.
type RequestFilter struct {
	// BlockContentTypes blocks responses by Content-Type.
	// Example: ["image/*", "font/*", "video/*"]
	BlockContentTypes []string `json:"block_content_types,omitempty"`

	// BlockURLPatterns blocks requests matching URL patterns (glob/regex).
	// Example: ["*.google-analytics.com", "*/ads/*"]
	BlockURLPatterns []string `json:"block_url_patterns,omitempty"`

	// BlockDomains blocks requests to specific domains.
	BlockDomains []string `json:"block_domains,omitempty"`

	// EnableAdblock enables EasyList-style ad blocking.
	EnableAdblock bool `json:"enable_adblock,omitempty"`

	// AdblockLists specifies which blocklists to use.
	// Example: ["easylist", "easyprivacy"]
	AdblockLists []string `json:"adblock_lists,omitempty"`
}

// EndpointPool defines a tier of endpoints for retry/fallback logic.
type EndpointPool struct {
	// Tier is the priority of this pool (1 = Primary, 2 = Secondary, etc.).
	Tier int `json:"tier"`

	// Endpoints are endpoint IDs or tag selectors for this pool.
	Endpoints []string `json:"endpoints"`

	// MaxRetries is the maximum retry attempts within this pool before escalating.
	MaxRetries int `json:"max_retries"`
}

// MatchesTags checks if this rule matches the given request tags.
// Returns true if all required tags match and no excluded tags match.
func (r *RoutingRule) MatchesTags(requestTags []Tag) bool {
	// Parse required tags
	required, err := StringsToTags(r.RequiredTags)
	if err != nil {
		return false
	}

	// Parse excluded tags
	excluded, err := StringsToTags(r.ExcludedTags)
	if err != nil {
		return false
	}

	return MatchesAll(requestTags, required) && MatchesNone(requestTags, excluded)
}

// RoutingRuleRepository defines the interface for routing rule storage.
type RoutingRuleRepository interface {
	GetActiveRules(ctx context.Context) ([]RoutingRule, error)
	CreateRule(ctx context.Context, rule *RoutingRule) error
	GetRuleByID(ctx context.Context, id string) (*RoutingRule, error)
	UpdateRule(ctx context.Context, rule *RoutingRule) error
	DeleteRule(ctx context.Context, id string) error
	ListRules(ctx context.Context, limit, offset int) ([]RoutingRule, int, error)
}
