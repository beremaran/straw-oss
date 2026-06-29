package domain

import (
	"context"
	"time"
)

// RoutingRule defines how requests are routed based on tags and other criteria.
type RoutingRule struct {
	ID                   string         `json:"id"`
	Name                 string         `json:"name"`
	RequiredTags         []string       `json:"required_tags"`
	ExcludedTags         []string       `json:"excluded_tags,omitempty"`
	Priority             int            `json:"priority"`
	HardTimeout          time.Duration  `json:"hard_timeout,omitempty"`
	RateLimitPerMinute   int            `json:"rate_limit_per_minute,omitempty"`
	RateLimitPerSecond   int            `json:"rate_limit_per_second,omitempty"`
	AllowedEndpointTypes []string       `json:"allowed_endpoint_types,omitempty"`
	RequiredEndpointCaps []string       `json:"required_endpoint_caps,omitempty"`
	FingerprintPreset    string         `json:"fingerprint_preset,omitempty"`
	FingerprintABTest    *ABConfig      `json:"fingerprint_ab_test,omitempty"`
	QuotaKey             string         `json:"quota_key,omitempty"`
	AllowInsecureTLS     bool           `json:"allow_insecure_tls,omitempty"`
	PinnedCertHash       string         `json:"pinned_cert_hash,omitempty"`
	RequestFilters       *RequestFilter `json:"request_filters,omitempty"`
	EndpointPools        []EndpointPool `json:"endpoint_pools,omitempty"`
	IsActive             bool           `json:"is_active"`
	Version              int            `json:"version"`
	CreatedAt            time.Time      `json:"created_at"`
	UpdatedAt            time.Time      `json:"updated_at"`
}

// ABConfig defines an A/B test configuration with variants.
type ABConfig struct {
	Variants []ABVariant `json:"variants"`
	Strategy string      `json:"strategy"`
}

// ABVariant represents a single variant in an A/B test.
type ABVariant struct {
	Fingerprint string `json:"fingerprint"`
	Weight      int    `json:"weight"`
}

// RoutingRuleReference is a lightweight reference to a routing rule.
type RoutingRuleReference struct {
	ID   string `json:"id"`
	Name string `json:"name"`
}

// RequestFilter defines request-level filtering rules.
type RequestFilter struct {
	BlockContentTypes []string `json:"block_content_types,omitempty"`
	BlockURLPatterns  []string `json:"block_url_patterns,omitempty"`
	BlockDomains      []string `json:"block_domains,omitempty"`
	EnableAdblock     bool     `json:"enable_adblock,omitempty"`
	AdblockLists      []string `json:"adblock_lists,omitempty"`
}

// EndpointPool defines a tiered pool of endpoint IDs.
type EndpointPool struct {
	Tier       int      `json:"tier"`
	Endpoints  []string `json:"endpoints"`
	MaxRetries int      `json:"max_retries"`
}

// MatchesTags reports whether the request tags satisfy this routing rule's required and excluded tags.
func (r *RoutingRule) MatchesTags(requestTags []Tag) bool {
	required, err := StringsToTags(r.RequiredTags)
	if err != nil {
		return false
	}

	excluded, err := StringsToTags(r.ExcludedTags)
	if err != nil {
		return false
	}

	return MatchesAll(requestTags, required) && MatchesNone(requestTags, excluded)
}

// RoutingRuleRepository provides persistence operations for RoutingRule entities.
type RoutingRuleRepository interface {
	GetActiveRules(ctx context.Context) ([]RoutingRule, error)
	CreateRule(ctx context.Context, rule *RoutingRule) error
	GetRuleByID(ctx context.Context, id string) (*RoutingRule, error)
	UpdateRule(ctx context.Context, rule *RoutingRule) error
	DeleteRule(ctx context.Context, id string) error
	ListRules(ctx context.Context, limit, offset int) ([]RoutingRule, int, error)
}
