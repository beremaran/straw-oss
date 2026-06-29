package dto

import "time"

// CreateRoutingRuleRequest is the request body for creating a routing rule.
type CreateRoutingRuleRequest struct {
	Name                 string            `json:"name"                             validate:"required"`
	RequiredTags         []string          `json:"required_tags"`
	ExcludedTags         []string          `json:"excluded_tags,omitempty"`
	Priority             int               `json:"priority"`
	HardTimeout          string            `json:"hard_timeout,omitempty"`
	RateLimitPerMinute   int               `json:"rate_limit_per_minute,omitempty"`
	RateLimitPerSecond   int               `json:"rate_limit_per_second,omitempty"`
	AllowedEndpointTypes []string          `json:"allowed_endpoint_types,omitempty"`
	RequiredEndpointCaps []string          `json:"required_endpoint_caps,omitempty"`
	FingerprintPreset    string            `json:"fingerprint_preset,omitempty"`
	FingerprintABTest    *ABConfigDTO      `json:"fingerprint_ab_test,omitempty"`
	QuotaKey             string            `json:"quota_key,omitempty"`
	AllowInsecureTLS     bool              `json:"allow_insecure_tls,omitempty"`
	PinnedCertHash       string            `json:"pinned_cert_hash,omitempty"`
	RequestFilters       *RequestFilterDTO `json:"request_filters,omitempty"`
	EndpointPools        []EndpointPoolDTO `json:"endpoint_pools,omitempty"`
	IsActive             bool              `json:"is_active"`
}

// UpdateRoutingRuleRequest is the request body for updating a routing rule.
type UpdateRoutingRuleRequest struct {
	CreateRoutingRuleRequest
	Version int `json:"version" validate:"required"`
}

// RoutingRuleResponse represents a routing rule in API responses.
type RoutingRuleResponse struct {
	ID                   string            `json:"id"`
	Name                 string            `json:"name"`
	RequiredTags         []string          `json:"required_tags"`
	ExcludedTags         []string          `json:"excluded_tags,omitempty"`
	Priority             int               `json:"priority"`
	HardTimeout          string            `json:"hard_timeout,omitempty"`
	RateLimitPerMinute   int               `json:"rate_limit_per_minute,omitempty"`
	RateLimitPerSecond   int               `json:"rate_limit_per_second,omitempty"`
	AllowedEndpointTypes []string          `json:"allowed_endpoint_types,omitempty"`
	RequiredEndpointCaps []string          `json:"required_endpoint_caps,omitempty"`
	FingerprintPreset    string            `json:"fingerprint_preset,omitempty"`
	FingerprintABTest    *ABConfigDTO      `json:"fingerprint_ab_test,omitempty"`
	QuotaKey             string            `json:"quota_key,omitempty"`
	AllowInsecureTLS     bool              `json:"allow_insecure_tls,omitempty"`
	PinnedCertHash       string            `json:"pinned_cert_hash,omitempty"`
	RequestFilters       *RequestFilterDTO `json:"request_filters,omitempty"`
	EndpointPools        []EndpointPoolDTO `json:"endpoint_pools,omitempty"`
	IsActive             bool              `json:"is_active"`
	Version              int               `json:"version"`
	CreatedAt            time.Time         `json:"created_at"`
	UpdatedAt            time.Time         `json:"updated_at"`
}

// ListRoutingRulesResponse is a paginated list of routing rules.
type ListRoutingRulesResponse = PaginatedResponse[RoutingRuleResponse]

// ABConfigDTO defines an A/B testing configuration.
type ABConfigDTO struct {
	Variants []ABVariantDTO `json:"variants"`
	Strategy string         `json:"strategy"`
}

// ABVariantDTO represents a variant in an A/B test.
type ABVariantDTO struct {
	Fingerprint string `json:"fingerprint"`
	Weight      int    `json:"weight"`
}

// RequestFilterDTO defines request filtering rules.
type RequestFilterDTO struct {
	BlockContentTypes []string `json:"block_content_types,omitempty"`
	BlockURLPatterns  []string `json:"block_url_patterns,omitempty"`
	BlockDomains      []string `json:"block_domains,omitempty"`
	EnableAdblock     bool     `json:"enable_adblock,omitempty"`
	AdblockLists      []string `json:"adblock_lists,omitempty"`
}

// EndpointPoolDTO represents a pool of endpoints for routing.
type EndpointPoolDTO struct {
	Tier       int      `json:"tier"`
	Endpoints  []string `json:"endpoints"`
	MaxRetries int      `json:"max_retries"`
}
