package filter

import (
	"context"
	"fmt"
	"net/url"
	"path/filepath"
	"strings"

	"github.com/kwilabs/straw-proxy-server/internal/domain"
)

// Type represents the type of filter that matched.
type Type string

const (
	TypeContentType Type = "content-type"
	TypeURLPattern  Type = "url-pattern"
	TypeDomain      Type = "domain"
	TypeABP         Type = "abp"
)

// Result represents the result of a filter evaluation.
type Result struct {
	// Blocked indicates whether the request should be blocked.
	Blocked bool

	// Reason provides a human-readable explanation of why the request was blocked.
	// Example: "content-type:image/*", "domain:googleanalytics.com"
	Reason string

	// FilterType identifies which filter caused the block.
	FilterType Type
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
func (s *Service) ShouldBlock(ctx context.Context, req *Request, filter *domain.RequestFilter) (*Result, error) {
	// No filter configured means nothing is blocked
	if filter == nil {
		return &Result{Blocked: false}, nil
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

	return &Result{Blocked: false}, nil
}

// checkContentType checks if the request's expected content-type matches any blocked patterns.
func (s *Service) checkContentType(req *Request, patterns []string) *Result {
	if req.ContentType == "" || len(patterns) == 0 {
		return &Result{Blocked: false}
	}

	// Normalize content-type (remove params like charset)
	contentType := strings.Split(req.ContentType, ";")[0]
	contentType = strings.TrimSpace(strings.ToLower(contentType))

	for _, pattern := range patterns {
		pattern = strings.ToLower(strings.TrimSpace(pattern))
		if matchGlob(contentType, pattern) {
			return &Result{
				Blocked:    true,
				Reason:     fmt.Sprintf("content-type:%s", pattern),
				FilterType: TypeContentType,
			}
		}
	}

	return &Result{Blocked: false}
}

// checkURLPatterns checks if the request URL matches any blocked patterns.
func (s *Service) checkURLPatterns(req *Request, patterns []string) *Result {
	if req.URL == "" || len(patterns) == 0 {
		return &Result{Blocked: false}
	}

	for _, pattern := range patterns {
		pattern = strings.TrimSpace(pattern)
		if matchURLPattern(req.URL, pattern) {
			return &Result{
				Blocked:    true,
				Reason:     fmt.Sprintf("url-pattern:%s", pattern),
				FilterType: TypeURLPattern,
			}
		}
	}

	return &Result{Blocked: false}
}

// checkDomains checks if the request host matches any blocked domains.
func (s *Service) checkDomains(req *Request, domains []string) *Result {
	if req.Host == "" || len(domains) == 0 {
		return &Result{Blocked: false}
	}

	host := strings.ToLower(req.Host)
	// Remove port if present
	if idx := strings.LastIndex(host, ":"); idx != -1 {
		host = host[:idx]
	}

	for _, domain_ := range domains {
		domain_ = strings.ToLower(strings.TrimSpace(domain_))
		if matchDomain(host, domain_) {
			return &Result{
				Blocked:    true,
				Reason:     fmt.Sprintf("domain_:%s", domain_),
				FilterType: TypeDomain,
			}
		}
	}

	return &Result{Blocked: false}
}

// checkABP checks if the request URL matches any ABP rules.
func (s *Service) checkABP(req *Request, lists []string) *Result {
	if req.URL == "" || s.abpMatcher == nil {
		return &Result{Blocked: false}
	}

	// Default to common lists if none specified
	if len(lists) == 0 {
		lists = []string{"easylist", "easyprivacy"}
	}

	if blocked, rule := s.abpMatcher.Match(req.URL, lists); blocked {
		return &Result{
			Blocked:    true,
			Reason:     fmt.Sprintf("abp:%s", rule),
			FilterType: TypeABP,
		}
	}

	return &Result{Blocked: false}
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
