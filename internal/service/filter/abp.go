package filter

import (
	"bufio"
	"context"
	"fmt"
	"io"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/kwilabs/straw-proxy-server/internal/infra/redis"
	"github.com/pmezard/adblock/adblock"
)

// Default ABP list URLs.
const (
	EasyListURL      = "https://easylist.to/easylist/easylist.txt"
	EasyPrivacyURL   = "https://easylist.to/easylist/easyprivacy.txt"
	UBlockFiltersURL = "https://raw.githubusercontent.com/uBlockOrigin/uAssets/master/filters/filters.txt"
)

// DefaultLists maps list names to their download URLs.
var DefaultLists = map[string]string{
	"easylist":    EasyListURL,
	"easyprivacy": EasyPrivacyURL,
	"ublock":      UBlockFiltersURL,
}

// ABPMatcher provides Adblock Plus filter list matching.
type ABPMatcher struct {
	matchers map[string]*adblock.RuleMatcher // keyed by list name
	redis    *redis.Client
	mu       sync.RWMutex

	httpClient     *http.Client
	updateInterval time.Duration
	stopChan       chan struct{}
}

// ABPMatcherConfig holds configuration for the ABP matcher.
type ABPMatcherConfig struct {
	// UpdateInterval is how often to check for list updates (default: 24h).
	UpdateInterval time.Duration

	// HTTPTimeout is the timeout for fetching lists (default: 30s).
	HTTPTimeout time.Duration
}

// DefaultABPMatcherConfig returns a default configuration.
func DefaultABPMatcherConfig() ABPMatcherConfig {
	return ABPMatcherConfig{
		UpdateInterval: 24 * time.Hour,
		HTTPTimeout:    30 * time.Second,
	}
}

// NewABPMatcher creates a new ABP matcher.
func NewABPMatcher(redisClient *redis.Client, config ABPMatcherConfig) *ABPMatcher {
	if config.UpdateInterval == 0 {
		config.UpdateInterval = 24 * time.Hour
	}
	if config.HTTPTimeout == 0 {
		config.HTTPTimeout = 30 * time.Second
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

// LoadDefaultLists loads the common ABP filter lists.
func (m *ABPMatcher) LoadDefaultLists(ctx context.Context) error {
	for name, url := range DefaultLists {
		if err := m.LoadList(ctx, name, url); err != nil {
			// Log error but continue with other lists
			// TODO: Add proper logging
			continue
		}
	}
	return nil
}

// LoadList downloads and parses an ABP filter list.
func (m *ABPMatcher) LoadList(ctx context.Context, listName string, listURL string) error {
	// Try to load from cache first
	cacheKey := fmt.Sprintf("abp:list:%s", listName)
	if m.redis != nil {
		if cached, err := m.redis.Client.Get(ctx, cacheKey).Result(); err == nil && cached != "" {
			if err := m.parseAndStore(listName, strings.NewReader(cached)); err == nil {
				return nil
			}
		}
	}

	// Fetch from URL
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, listURL, nil)
	if err != nil {
		return fmt.Errorf("failed to create request: %w", err)
	}

	resp, err := m.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("failed to fetch list %s: %w", listName, err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("unexpected status code %d for list %s", resp.StatusCode, listName)
	}

	// Read the body
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return fmt.Errorf("failed to read list body: %w", err)
	}

	// Cache in Redis (TTL: 25 hours to ensure we have data even if update fails)
	if m.redis != nil {
		_ = m.redis.Client.Set(ctx, cacheKey, string(body), 25*time.Hour).Err()
	}

	// Parse and store
	return m.parseAndStore(listName, strings.NewReader(string(body)))
}

// parseAndStore parses ABP rules and stores the matcher.
func (m *ABPMatcher) parseAndStore(listName string, reader io.Reader) error {
	matcher := adblock.NewMatcher()
	scanner := bufio.NewScanner(reader)

	lineNum := 0
	for scanner.Scan() {
		lineNum++
		line := strings.TrimSpace(scanner.Text())

		// Skip empty lines and comments
		if line == "" || strings.HasPrefix(line, "!") || strings.HasPrefix(line, "[") {
			continue
		}

		// Parse the rule
		rule, err := adblock.ParseRule(line)
		if err != nil {
			// Skip invalid rules
			continue
		}

		if rule != nil {
			matcher.AddRule(rule, lineNum)
		}
	}

	if err := scanner.Err(); err != nil {
		return fmt.Errorf("error reading list: %w", err)
	}

	m.mu.Lock()
	m.matchers[listName] = matcher
	m.mu.Unlock()

	return nil
}

// Match checks if a URL matches any ABP rules in the specified lists.
// Returns (blocked, matchedRule) where matchedRule is the pattern that matched.
func (m *ABPMatcher) Match(url string, lists []string) (bool, string) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	for _, listName := range lists {
		matcher, ok := m.matchers[listName]
		if !ok {
			continue
		}

		// Create a request with minimal info for matching
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

// StartAutoUpdate starts a background goroutine that periodically updates the filter lists.
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
				// Update all default lists
				for name, url := range DefaultLists {
					_ = m.LoadList(ctx, name, url)
				}
			}
		}
	}()
}

// Stop stops the auto-update goroutine.
func (m *ABPMatcher) Stop() {
	close(m.stopChan)
}

// HasList checks if a list is loaded.
func (m *ABPMatcher) HasList(listName string) bool {
	m.mu.RLock()
	defer m.mu.RUnlock()
	_, ok := m.matchers[listName]
	return ok
}

// extractDomain extracts the domain from a URL.
func extractDomain(rawURL string) string {
	// Simple extraction - find the host part
	url := rawURL

	// Remove protocol
	if idx := strings.Index(url, "://"); idx != -1 {
		url = url[idx+3:]
	}

	// Remove path
	if idx := strings.Index(url, "/"); idx != -1 {
		url = url[:idx]
	}

	// Remove port
	if idx := strings.LastIndex(url, ":"); idx != -1 {
		url = url[:idx]
	}

	return url
}
