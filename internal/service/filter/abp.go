package filter

import (
	"bufio"
	"context"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"time"

	"github.com/beremaran/straw/internal/infra/redis"
	"github.com/pmezard/adblock/adblock"
)

const (
	EasyListURL      = "https://easylist.to/easylist/easylist.txt"
	EasyPrivacyURL   = "https://easylist.to/easylist/easyprivacy.txt"
	UBlockFiltersURL = "https://raw.githubusercontent.com/uBlockOrigin/uAssets/master/filters/filters.txt"
)

var DefaultLists = map[string]string{
	"easylist":    EasyListURL,
	"easyprivacy": EasyPrivacyURL,
	"ublock":      UBlockFiltersURL,
}

type ABPMatcher struct {
	matchers map[string]*adblock.RuleMatcher
	redis    *redis.Client
	mu       sync.RWMutex

	httpClient     *http.Client
	updateInterval time.Duration
	stopChan       chan struct{}
}

type ABPMatcherConfig struct {
	UpdateInterval time.Duration

	HTTPTimeout time.Duration
}

func DefaultABPMatcherConfig() ABPMatcherConfig {
	return ABPMatcherConfig{
		UpdateInterval: 24 * time.Hour,
		HTTPTimeout:    30 * time.Second,
	}
}

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

func (m *ABPMatcher) LoadDefaultLists(ctx context.Context) error {
	for name, url := range DefaultLists {
		err := m.LoadList(ctx, name, url)
		if err != nil {
			continue
		}
	}

	return nil
}

func (m *ABPMatcher) LoadList(ctx context.Context, listName string, listURL string) error {
	cacheKey := fmt.Sprintf("abp:list:%s", listName)
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
		return fmt.Errorf("unexpected status code %d for list %s", resp.StatusCode, listName)
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return fmt.Errorf("failed to read list body: %w", err)
	}

	if m.redis != nil {
		_ = m.redis.Client.Set(ctx, cacheKey, string(body), 25*time.Hour).Err()
	}

	return m.parseAndStore(listName, strings.NewReader(string(body)))
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

func (m *ABPMatcher) Stop() {
	close(m.stopChan)
}

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
