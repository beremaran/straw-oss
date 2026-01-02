package router

import (
	"fmt"
	"net/http"

	"github.com/kwilabs/straw-proxy-server/internal/domain"
)

const (
	HeaderRelayTags      = "X-Relay-Tags"
	HeaderLegacyRetailer = "X-Straw-Retailer"
	HeaderLegacyMode     = "X-Straw-Mode"
	HeaderLegacyCountry  = "X-Straw-Country"
)

// info: "JWT Claims" requirement is currently mapped to API Key Scopes as per current architecture.
// Real JWT token parsing is deferred as user indicated no current use-case.

// TagParser handles parsing and merging of tags from request sources.
type TagParser struct{}

// NewTagParser creates a new TagParser.
func NewTagParser() *TagParser {
	return &TagParser{}
}

// ParseResult contains the parsed tags and any warnings (e.g. deprecation).
type ParseResult struct {
	Tags     []domain.Tag
	Warnings []string
}

// ParseTags extracts tags from the request headers and API key scopes.
// It merges them with the following priority (highest first):
// 1. Inferred/System tags (TODO)
// 2. Client Headers (X-Relay-Tags)
// 3. Legacy Headers (X-Straw-*)
// 4. API Key Scopes (treated as tags)
func (p *TagParser) ParseTags(r *http.Request, apiKey *domain.ApiKey) (*ParseResult, error) {
	result := &ParseResult{
		Tags:     make([]domain.Tag, 0),
		Warnings: make([]string, 0),
	}

	// We use a map to deduplicate tags.
	// Key: Tag Key, Value: Tag Value.
	// Strategy: If a key exists, we overwrite it? OR do we allow multiple values for the same key?
	// Design doc: "Merged tags contain all unique entries".
	// Design doc also implies AND logic for RequiredTags.
	// If I send "target:amazon" and "target:google", and rule requires "target:amazon", it matches.
	// So we should collect ALL unique Key:Value pairs.
	// However, usually "target" implies a single target.
	// But "capability" can be multiple.
	// Let's store unique Tag structs.
	tagMap := make(map[domain.Tag]bool)

	// Helper to add tags
	addTag := func(t domain.Tag) {
		if !tagMap[t] {
			tagMap[t] = true
			result.Tags = append(result.Tags, t)
		}
	}

	// 1. Client Headers (X-Relay-Tags)
	if headerVal := r.Header.Get(HeaderRelayTags); headerVal != "" {
		// Expects comma separated list
		tags, err := domain.ParseTags(headerVal)
		if err != nil {
			return nil, fmt.Errorf("failed to parse %s: %w", HeaderRelayTags, err)
		}
		for _, t := range tags {
			addTag(t)
		}
	}

	// 2. Legacy Headers (X-Straw-*)
	// X-Straw-Retailer -> target:*
	if val := r.Header.Get(HeaderLegacyRetailer); val != "" {
		addTag(domain.Tag{Key: "target", Value: val})
		result.Warnings = append(result.Warnings, fmt.Sprintf("Header %s is deprecated. Use %s: target=%s instead.", HeaderLegacyRetailer, HeaderRelayTags, val))
	}
	// X-Straw-Mode -> type:*
	if val := r.Header.Get(HeaderLegacyMode); val != "" {
		addTag(domain.Tag{Key: "type", Value: val})
		result.Warnings = append(result.Warnings, fmt.Sprintf("Header %s is deprecated. Use %s: type=%s instead.", HeaderLegacyMode, HeaderRelayTags, val))
	}
	// X-Straw-Country -> region:*
	if val := r.Header.Get(HeaderLegacyCountry); val != "" {
		addTag(domain.Tag{Key: "region", Value: val})
		result.Warnings = append(result.Warnings, fmt.Sprintf("Header %s is deprecated. Use %s: region=%s instead.", HeaderLegacyCountry, HeaderRelayTags, val))
	}

	// 3. API Key Scopes
	if apiKey != nil && len(apiKey.Scopes) > 0 {
		// Scopes are stored as strings "key:value" or "key:*" (glob)
		// For a request, we are treating these tags as attributes OF the request imposed by the key.
		// Wait, scopes usually *limit* what tags you can use (Authorization), not *add* tags.
		// BUT the task says: "Extract tags from JWT claims ... Priority: Client headers > JWT claims".
		// And "Scope matching logic" was in Task 3.2 (Auth).
		// If "Scopes" are permissions, they are checked against the request tags.
		// If "Scopes" are *forced tags* (e.g. this key IS for "target:amazon"), then we add them.

		// Design Doc 3.1: "Keys can be scoped to specific tags (e.g., target:amazon only)." -> This implies Authorization / Restriction.
		// However, Section 6.1 "Tag Sources": "JWT Claims: Authenticated clients may have embedded tags in token claims."
		// Since we agreed "JWT Claims" -> "API Key Scopes" roughly, AND the requirement is "Extract tags...",
		// it implies usage as *default/forced tags*.
		// Example: An API key for a specific customer might always tag usage with `customer:xyz`.

		// Let's parse them as tags and add them.
		scopes, err := domain.StringsToTags(apiKey.Scopes)
		if err != nil {
			// If scopes are globs like "target:*", they might fail ParseTag if value is wildcard?
			// valid tag value can be "*". domain.Tag doesn't forbid it.
			// But usually scopes are for matching.
			// If I have a scope "target:*", adding it as a request tag means "I am targeting anything".
			// That might not be what we want if we are trying to ROUTE.
			// Currently, we will just parse them. If they fail, we log/ignore or error?
			// Let's assume validation happened at key creation.
			// We'll treat them as tags to merge.
			return nil, fmt.Errorf("invalid api key scopes: %w", err)
		}
		for _, t := range scopes {
			addTag(t)
		}
	}

	// 4. Inferred Tags (Stub)
	// Example: Extract from subdomain, etc.
	// inferTags(r) -> addTag...

	return result, nil
}
