package filter

import (
	"context"
	"fmt"
	"net/url"
	"path"
	"strings"

	"github.com/beremaran/straw/internal/domain"
)

type Type string

const (
	TypeContentType Type = "content-type"
	TypeURLPattern  Type = "url-pattern"
	TypeDomain      Type = "domain"
	TypeABP         Type = "abp"
)

type Result struct {
	Blocked    bool
	Reason     string
	FilterType Type
}

type Service struct {
	abpMatcher *ABPMatcher
}

func NewService(abpMatcher *ABPMatcher) *Service {
	return &Service{
		abpMatcher: abpMatcher,
	}
}

func (s *Service) ShouldBlock(ctx context.Context, req *Request, filter *domain.RequestFilter) (*Result, error) {
	if filter == nil {
		return &Result{Blocked: false}, nil
	}

	if result := s.checkContentType(req, filter.BlockContentTypes); result.Blocked {
		return result, nil
	}

	if result := s.checkURLPatterns(req, filter.BlockURLPatterns); result.Blocked {
		return result, nil
	}

	if result := s.checkDomains(req, filter.BlockDomains); result.Blocked {
		return result, nil
	}

	if filter.EnableAdblock && s.abpMatcher != nil {
		if result := s.checkABP(req, filter.AdblockLists); result.Blocked {
			return result, nil
		}
	}

	return &Result{Blocked: false}, nil
}

func (s *Service) checkContentType(req *Request, patterns []string) *Result {
	if req.ContentType == "" || len(patterns) == 0 {
		return &Result{Blocked: false}
	}

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

func (s *Service) checkDomains(req *Request, domains []string) *Result {
	if req.Host == "" || len(domains) == 0 {
		return &Result{Blocked: false}
	}

	host := strings.ToLower(req.Host)

	if idx := strings.LastIndex(host, ":"); idx != -1 {
		host = host[:idx]
	}

	for _, domain_ := range domains {
		domain_ = strings.ToLower(strings.TrimSpace(domain_))
		if matchDomain(host, domain_) {
			return &Result{
				Blocked:    true,
				Reason:     fmt.Sprintf("domain:%s", domain_),
				FilterType: TypeDomain,
			}
		}
	}

	return &Result{Blocked: false}
}

func (s *Service) checkABP(req *Request, lists []string) *Result {
	if req.URL == "" || s.abpMatcher == nil {
		return &Result{Blocked: false}
	}

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

func matchGlob(s, pattern string) bool {
	matched, err := path.Match(pattern, s)
	if err != nil {
		return false
	}

	return matched
}

func matchURLPattern(rawURL, pattern string) bool {
	parsedURL, err := url.Parse(rawURL)
	if err != nil {
		return false
	}

	matchStr := parsedURL.Host + parsedURL.Path

	if strings.HasPrefix(pattern, "*.") {
		suffix := pattern[1:]
		if strings.HasSuffix(matchStr, suffix) || strings.Contains(matchStr, suffix) {
			return true
		}
	}

	if strings.Contains(pattern, "*") {
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

	return matchStr == pattern || parsedURL.Host == pattern
}

func matchDomain(host, pattern string) bool {
	if host == pattern {
		return true
	}

	if strings.HasPrefix(pattern, "*.") {
		suffix := pattern[1:]

		return strings.HasSuffix(host, suffix)
	}

	return strings.HasSuffix(host, "."+pattern)
}
