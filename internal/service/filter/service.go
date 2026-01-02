package filter

import (
	"context"
	"fmt"
	"net/url"
	"path/filepath"
	"strings"

	"github.com/kwilabs/straw-proxy-server/internal/domain"
)

// FilterType represents the type of filter that matched.
type FilterType string

const (
	FilterTypeContentType FilterType = "content-type"
	FilterTypeURLPattern  FilterType = "url-pattern"
	FilterTypeDomain      FilterType = "domain"
	FilterTypeABP         FilterType = "abp"
)

// FilterResult represents the result of a filter evaluation.
type FilterResult struct {
	// Blocked indicates whether the request should be blocked.
	Blocked bool

	// Reason provides a human-readable explanation of why the request was blocked.
	// Example: "content-type:image/*", "domain:googleanalytics.com"
	Reason string

	// FilterType identifies which filter caused the block.
	FilterType FilterType
}

// Service handles request filtering for bandwidth optimization.
type Service struct {
	abpMatcher *ABPMatcher
}

// NewService creates a new filter Service.
func NewService(abpMatcher *ABPMatcher) *Service {
	return &Service{
		abpMatcher: abpMatcher,
	}
}

// ShouldBlock evaluates whether a request should be blocked based on the provided filters.
// Returns a FilterResult indicating whether the request is blocked and why.
func (s *Service) ShouldBlock(ctx context.Context, req *FilterRequest, filter *domain.RequestFilter) (*FilterResult, error) {
	// No filter configured means nothing is blocked
	if filter == nil {
		return &FilterResult{Blocked: false}, nil
	}

	// Check content-type blocking
	if result := s.checkContentType(req, filter.BlockContentTypes); result.Blocked {
		return result, nil
	}

	// Check URL pattern blocking
	if result := s.checkURLPatterns(req, filter.BlockURLPatterns); result.Blocked {
		return result, nil
	}

	// Check domain blocklist
	if result := s.checkDomains(req, filter.BlockDomains); result.Blocked {
		return result, nil
	}

	// Check ABP lists if enabled
	if filter.EnableAdblock && s.abpMatcher != nil {
		if result := s.checkABP(req, filter.AdblockLists); result.Blocked {
			return result, nil
		}
	}

	return &FilterResult{Blocked: false}, nil
}

// checkContentType checks if the request's expected content-type matches any blocked patterns.
func (s *Service) checkContentType(req *FilterRequest, patterns []string) *FilterResult {
	if req.ContentType == "" || len(patterns) == 0 {
		return &FilterResult{Blocked: false}
	}

	// Normalize content-type (remove params like charset)
	contentType := strings.Split(req.ContentType, ";")[0]
	contentType = strings.TrimSpace(strings.ToLower(contentType))

	for _, pattern := range patterns {
		pattern = strings.ToLower(strings.TrimSpace(pattern))
		if matchGlob(contentType, pattern) {
			return &FilterResult{
				Blocked:    true,
				Reason:     fmt.Sprintf("content-type:%s", pattern),
				FilterType: FilterTypeContentType,
			}
		}
	}

	return &FilterResult{Blocked: false}
}

// checkURLPatterns checks if the request URL matches any blocked patterns.
func (s *Service) checkURLPatterns(req *FilterRequest, patterns []string) *FilterResult {
	if req.URL == "" || len(patterns) == 0 {
		return &FilterResult{Blocked: false}
	}

	for _, pattern := range patterns {
		pattern = strings.TrimSpace(pattern)
		if matchURLPattern(req.URL, pattern) {
			return &FilterResult{
				Blocked:    true,
				Reason:     fmt.Sprintf("url-pattern:%s", pattern),
				FilterType: FilterTypeURLPattern,
			}
		}
	}

	return &FilterResult{Blocked: false}
}

// checkDomains checks if the request host matches any blocked domains.
func (s *Service) checkDomains(req *FilterRequest, domains []string) *FilterResult {
	if req.Host == "" || len(domains) == 0 {
		return &FilterResult{Blocked: false}
	}

	host := strings.ToLower(req.Host)
	// Remove port if present
	if idx := strings.LastIndex(host, ":"); idx != -1 {
		host = host[:idx]
	}

	for _, domain := range domains {
		domain = strings.ToLower(strings.TrimSpace(domain))
		if matchDomain(host, domain) {
			return &FilterResult{
				Blocked:    true,
				Reason:     fmt.Sprintf("domain:%s", domain),
				FilterType: FilterTypeDomain,
			}
		}
	}

	return &FilterResult{Blocked: false}
}

// checkABP checks if the request URL matches any ABP rules.
func (s *Service) checkABP(req *FilterRequest, lists []string) *FilterResult {
	if req.URL == "" || s.abpMatcher == nil {
		return &FilterResult{Blocked: false}
	}

	// Default to common lists if none specified
	if len(lists) == 0 {
		lists = []string{"easylist", "easyprivacy"}
	}

	if blocked, rule := s.abpMatcher.Match(req.URL, lists); blocked {
		return &FilterResult{
			Blocked:    true,
			Reason:     fmt.Sprintf("abp:%s", rule),
			FilterType: FilterTypeABP,
		}
	}

	return &FilterResult{Blocked: false}
}

// matchGlob performs glob-style pattern matching.
// Supports * as wildcard for any characters.
func matchGlob(s, pattern string) bool {
	// Use filepath.Match for glob matching
	matched, err := filepath.Match(pattern, s)
	if err != nil {
		return false
	}
	return matched
}

// matchURLPattern matches a URL against a pattern.
// Supports:
// - * as wildcard for any characters
// - Patterns starting with * match any host
// - Patterns like */ads/* match path segments
func matchURLPattern(rawURL, pattern string) bool {
	// Parse the URL
	parsedURL, err := url.Parse(rawURL)
	if err != nil {
		return false
	}

	// Build the match string (host + path)
	matchStr := parsedURL.Host + parsedURL.Path

	// Handle patterns starting with *. (wildcard domain)
	if strings.HasPrefix(pattern, "*.") {
		// Match any subdomain
		suffix := pattern[1:] // Remove leading *
		if strings.HasSuffix(matchStr, suffix) || strings.Contains(matchStr, suffix) {
			return true
		}
	}

	// Handle patterns with * as path wildcard (e.g., */ads/*)
	if strings.Contains(pattern, "*") {
		// Convert to regex-like matching
		// Split by * and check if parts appear in order
		parts := strings.Split(pattern, "*")
		remaining := matchStr

		for _, part := range parts {
			if part == "" {
				continue
			}
			idx := strings.Index(remaining, part)
			if idx == -1 {
				return false
			}
			remaining = remaining[idx+len(part):]
		}
		return true
	}

	// Exact match
	return matchStr == pattern || parsedURL.Host == pattern
}

// matchDomain checks if a host matches a domain pattern.
// Supports:
// - Exact match: example.com matches example.com
// - Subdomain match: sub.example.com matches example.com
// - Wildcard: *.example.com matches any subdomain
func matchDomain(host, pattern string) bool {
	// Exact match
	if host == pattern {
		return true
	}

	// Wildcard pattern (*.example.com)
	if strings.HasPrefix(pattern, "*.") {
		suffix := pattern[1:] // ".example.com"
		return strings.HasSuffix(host, suffix)
	}

	// Subdomain match: host ends with .pattern
	return strings.HasSuffix(host, "."+pattern)
}
