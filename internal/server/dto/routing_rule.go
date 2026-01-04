package dto

import "time"

// CreateRoutingRuleRequest is the request to create a routing rule.
//
//	@Description Request body for creating a new routing rule
type CreateRoutingRuleRequest struct {
	// Name is a human-readable name for the rule (required)
	Name string `json:"name" validate:"required"`

	// RequiredTags are the tags that a request MUST have to match this rule
	RequiredTags []string `json:"required_tags"`

	// ExcludedTags are tags that a request must NOT have to match this rule
	ExcludedTags []string `json:"excluded_tags,omitempty"`

	// Priority determines rule precedence (higher = evaluated first)
	Priority int `json:"priority"`

	// HardTimeout is the maximum time allowed for a request (e.g., "30s")
	HardTimeout string `json:"hard_timeout,omitempty"`

	// RateLimitPerMinute is the maximum requests per minute
	RateLimitPerMinute int `json:"rate_limit_per_minute,omitempty"`

	// RateLimitPerSecond is the maximum requests per second
	RateLimitPerSecond int `json:"rate_limit_per_second,omitempty"`

	// AllowedEndpointTypes specifies which endpoint types can handle requests
	AllowedEndpointTypes []string `json:"allowed_endpoint_types,omitempty"`

	// RequiredEndpointCaps are capability tags that endpoints must have
	RequiredEndpointCaps []string `json:"required_endpoint_caps,omitempty"`

	// FingerprintPreset specifies the default browser TLS fingerprint
	FingerprintPreset string `json:"fingerprint_preset,omitempty"`

	// FingerprintABTest allows A/B testing different fingerprints
	FingerprintABTest *ABConfigDTO `json:"fingerprint_ab_test,omitempty"`

	// QuotaKey is the key used for rate limit bucketing
	QuotaKey string `json:"quota_key,omitempty"`

	// AllowInsecureTLS allows skipping TLS certificate verification
	AllowInsecureTLS bool `json:"allow_insecure_tls,omitempty"`

	// PinnedCertHash for certificate pinning
	PinnedCertHash string `json:"pinned_cert_hash,omitempty"`

	// RequestFilters defines bandwidth optimization filters
	RequestFilters *RequestFilterDTO `json:"request_filters,omitempty"`

	// EndpointPools defines tiered endpoint fallback pools
	EndpointPools []EndpointPoolDTO `json:"endpoint_pools,omitempty"`

	// IsActive indicates whether this rule is currently active
	IsActive bool `json:"is_active"`
}

// UpdateRoutingRuleRequest is the request to update a routing rule.
//
//	@Description Request body for updating an existing routing rule
type UpdateRoutingRuleRequest struct {
	CreateRoutingRuleRequest

	// Version for optimistic locking (required for updates)
	Version int `json:"version" validate:"required"`
}

// RoutingRuleResponse is the API response for a routing rule.
//
//	@Description Routing rule data returned by the API
type RoutingRuleResponse struct {
	// ID is the unique identifier for this rule
	ID string `json:"id"`

	// Name is a human-readable name for the rule
	Name string `json:"name"`

	// RequiredTags are the tags that a request MUST have to match this rule
	RequiredTags []string `json:"required_tags"`

	// ExcludedTags are tags that a request must NOT have to match this rule
	ExcludedTags []string `json:"excluded_tags,omitempty"`

	// Priority determines rule precedence
	Priority int `json:"priority"`

	// HardTimeout is the maximum time allowed for a request
	HardTimeout string `json:"hard_timeout,omitempty"`

	// RateLimitPerMinute is the maximum requests per minute
	RateLimitPerMinute int `json:"rate_limit_per_minute,omitempty"`

	// RateLimitPerSecond is the maximum requests per second
	RateLimitPerSecond int `json:"rate_limit_per_second,omitempty"`

	// AllowedEndpointTypes specifies which endpoint types can handle requests
	AllowedEndpointTypes []string `json:"allowed_endpoint_types,omitempty"`

	// RequiredEndpointCaps are capability tags that endpoints must have
	RequiredEndpointCaps []string `json:"required_endpoint_caps,omitempty"`

	// FingerprintPreset specifies the default browser TLS fingerprint
	FingerprintPreset string `json:"fingerprint_preset,omitempty"`

	// FingerprintABTest allows A/B testing different fingerprints
	FingerprintABTest *ABConfigDTO `json:"fingerprint_ab_test,omitempty"`

	// QuotaKey is the key used for rate limit bucketing
	QuotaKey string `json:"quota_key,omitempty"`

	// AllowInsecureTLS allows skipping TLS certificate verification
	AllowInsecureTLS bool `json:"allow_insecure_tls,omitempty"`

	// PinnedCertHash for certificate pinning
	PinnedCertHash string `json:"pinned_cert_hash,omitempty"`

	// RequestFilters defines bandwidth optimization filters
	RequestFilters *RequestFilterDTO `json:"request_filters,omitempty"`

	// EndpointPools defines tiered endpoint fallback pools
	EndpointPools []EndpointPoolDTO `json:"endpoint_pools,omitempty"`

	// IsActive indicates whether this rule is currently active
	IsActive bool `json:"is_active"`

	// Version for optimistic locking on updates
	Version int `json:"version"`

	// CreatedAt is when the rule was created
	CreatedAt time.Time `json:"created_at"`

	// UpdatedAt is when the rule was last modified
	UpdatedAt time.Time `json:"updated_at"`
}

// ListRoutingRulesResponse is the paginated list of routing rules
type ListRoutingRulesResponse = PaginatedResponse[RoutingRuleResponse]

// ABConfigDTO represents A/B test configuration
type ABConfigDTO struct {
	// Variants are the different fingerprint options to test
	Variants []ABVariantDTO `json:"variants"`

	// Strategy determines how variants are selected (random, round_robin, weighted)
	Strategy string `json:"strategy"`
}

// ABVariantDTO represents an A/B test variant
type ABVariantDTO struct {
	// Fingerprint is the TLS fingerprint preset ID
	Fingerprint string `json:"fingerprint"`

	// Weight is the percentage weight for weighted selection (0-100)
	Weight int `json:"weight"`
}

// RequestFilterDTO represents request filtering configuration
type RequestFilterDTO struct {
	// BlockContentTypes blocks responses by Content-Type
	BlockContentTypes []string `json:"block_content_types,omitempty"`

	// BlockURLPatterns blocks requests matching URL patterns
	BlockURLPatterns []string `json:"block_url_patterns,omitempty"`

	// BlockDomains blocks requests to specific domains
	BlockDomains []string `json:"block_domains,omitempty"`

	// EnableAdblock enables EasyList-style ad blocking
	EnableAdblock bool `json:"enable_adblock,omitempty"`

	// AdblockLists specifies which blocklists to use
	AdblockLists []string `json:"adblock_lists,omitempty"`
}

// EndpointPoolDTO represents an endpoint pool tier
type EndpointPoolDTO struct {
	// Tier is the priority of this pool (1 = Primary, 2 = Secondary, etc.)
	Tier int `json:"tier"`

	// Endpoints are endpoint IDs or tag selectors for this pool
	Endpoints []string `json:"endpoints"`

	// MaxRetries is the maximum retry attempts within this pool
	MaxRetries int `json:"max_retries"`
}
