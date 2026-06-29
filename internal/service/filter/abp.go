// Package filter provides request filtering capabilities including ADBlock Plus list matching.
package filter

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"time"

	"github.com/pmezard/adblock/adblock"

	"github.com/beremaran/straw/internal/infra/redis"
)

// ErrUnexpectedListStatusCode is returned when a filter list URL responds with a non-200 status code.
var ErrUnexpectedListStatusCode = errors.New("unexpected status code for list")

const (
	// DefaultUpdateInterval is the default interval for auto-updating filter lists.
	DefaultUpdateInterval = 24 * time.Hour
	// DefaultHTTPTimeout is the default HTTP client timeout for fetching filter lists.
	DefaultHTTPTimeout = 30 * time.Second
	// DefaultCacheTTL is the default TTL for cached filter lists in Redis.
	DefaultCacheTTL = 25 * time.Hour

	// EasyListURL is the URL for the EasyList filter list.
	EasyListURL = "https://easylist.to/easylist/easylist.txt"
	// EasyPrivacyURL is the URL for the EasyPrivacy filter list.
	EasyPrivacyURL = "https://easylist.to/easylist/easyprivacy.txt"
	// UBlockFiltersURL is the URL for the uBlock Origin filters list.
	UBlockFiltersURL = "https://raw.githubusercontent.com/uBlockOrigin/uAssets/master/filters/filters.txt"
)

// DefaultLists maps list names to their download URLs.
var DefaultLists = map[string]string{
	"easylist":    EasyListURL,
	"easyprivacy": EasyPrivacyURL,
	"ublock":      UBlockFiltersURL,
}

// ABPMatcher applies ADBlock Plus filter lists to determine if requests should be blocked.
type ABPMatcher struct {
	matchers       map[string]*adblock.RuleMatcher
	redis          *redis.Client
	mu             sync.RWMutex
	httpClient     *http.Client
	updateInterval time.Duration
	stopChan       chan struct{}
}

// ABPMatcherConfig configures the behavior of ABPMatcher.
type ABPMatcherConfig struct {
	UpdateInterval time.Duration
	HTTPTimeout    time.Duration
}

// DefaultABPMatcherConfig returns a configuration with sensible defaults.
func DefaultABPMatcherConfig() ABPMatcherConfig {
	return ABPMatcherConfig{
		UpdateInterval: DefaultUpdateInterval,
		HTTPTimeout:    DefaultHTTPTimeout,
	}
}

// NewABPMatcher creates a new ABPMatcher with the given configuration.
func NewABPMatcher(redisClient *redis.Client, config ABPMatcherConfig) *ABPMatcher {
	if config.UpdateInterval == 0 {
		config.UpdateInterval = DefaultUpdateInterval
	}

	if config.HTTPTimeout == 0 {
		config.HTTPTimeout = DefaultHTTPTimeout
	}

	return &ABPMatcher{
		matchers: make(map[string]*adblock.RuleMatcher),
		redis:    redisClient,
		httpClient: &http.Client{
			Timeout: config.HTTPTimeout,
		},
		updateInterval: config.UpdateInterval,
		stopChan:       make(chan struct{}),
	}
}

// LoadDefaultLists downloads and parses all default filter lists.
func (m *ABPMatcher) LoadDefaultLists(ctx context.Context) error {
	for name, url := range DefaultLists {
		err := m.LoadList(ctx, name, url)
		if err != nil {
			continue
		}
	}

	return nil
}

// LoadList downloads and parses a filter list from the given URL.
func (m *ABPMatcher) LoadList(ctx context.Context, listName string, listURL string) error {
	cacheKey := "abp:list:" + listName
	if m.redis != nil {
		cached, err := m.redis.Client.Get(ctx, cacheKey).Result()
		if err == nil && cached != "" {
			err := m.parseAndStore(listName, strings.NewReader(cached))
			if err == nil {
				return nil
			}
		}
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, listURL, nil)
	if err != nil {
		return fmt.Errorf("failed to create request: %w", err)
	}

	resp, err := m.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("failed to fetch list %s: %w", listName, err)
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("%w: %d for %s", ErrUnexpectedListStatusCode, resp.StatusCode, listName)
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return fmt.Errorf("failed to read list body: %w", err)
	}

	if m.redis != nil {
		_ = m.redis.Client.Set(ctx, cacheKey, string(body), DefaultCacheTTL).Err()
	}

	return m.parseAndStore(listName, strings.NewReader(string(body)))
}

// Match checks if the given URL is blocked by any of the specified filter lists.
func (m *ABPMatcher) Match(url string, lists []string) (bool, string) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	for _, listName := range lists {
		matcher, ok := m.matchers[listName]
		if !ok {
			continue
		}

		rq := &adblock.Request{
			URL:    url,
			Domain: extractDomain(url),
		}

		matched, _, err := matcher.Match(rq)
		if err == nil && matched {
			return true, listName
		}
	}

	return false, ""
}

// StartAutoUpdate begins periodic refreshing of all default filter lists.
func (m *ABPMatcher) StartAutoUpdate(ctx context.Context) {
	go func() {
		ticker := time.NewTicker(m.updateInterval)
		defer ticker.Stop()

		for {
			select {
			case <-ctx.Done():
				return
			case <-m.stopChan:
				return
			case <-ticker.C:
				for name, url := range DefaultLists {
					_ = m.LoadList(ctx, name, url)
				}
			}
		}
	}()
}

// Stop signals the auto-update goroutine to terminate.
func (m *ABPMatcher) Stop() {
	close(m.stopChan)
}

// HasList reports whether the given filter list has been loaded.
func (m *ABPMatcher) HasList(listName string) bool {
	m.mu.RLock()
	defer m.mu.RUnlock()

	_, ok := m.matchers[listName]

	return ok
}

func extractDomain(rawURL string) string {
	uStr := rawURL
	if !strings.Contains(rawURL, "://") && !strings.HasPrefix(rawURL, "//") {
		uStr = "http://" + rawURL
	}

	u, err := url.Parse(uStr)
	if err != nil {
		return rawURL
	}

	return u.Hostname()
}

func (m *ABPMatcher) parseAndStore(listName string, reader io.Reader) error {
	matcher := adblock.NewMatcher()
	scanner := bufio.NewScanner(reader)

	lineNum := 0
	for scanner.Scan() {
		lineNum++
		line := strings.TrimSpace(scanner.Text())

		if line == "" || strings.HasPrefix(line, "!") || strings.HasPrefix(line, "[") {
			continue
		}

		rule, err := adblock.ParseRule(line)
		if err != nil {
			continue
		}

		if rule != nil {
			_ = matcher.AddRule(rule, lineNum)
		}
	}

	err := scanner.Err()
	if err != nil {
		return fmt.Errorf("error reading list: %w", err)
	}

	m.mu.Lock()
	m.matchers[listName] = matcher
	m.mu.Unlock()

	return nil
}
