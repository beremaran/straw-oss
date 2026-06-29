package router

import (
	"context"
	"fmt"
	"log"
	"sync"
	"sync/atomic"
	"time"

	"github.com/beremaran/straw/internal/domain"
)

const cacheAsyncTimeout = 5 * time.Second

// Matcher matches incoming requests against routing rules with caching support.
type Matcher struct {
	repo           domain.RoutingRuleRepository
	cache          *RuleCache
	rules          []domain.RoutingRule
	currentVersion int64
	mu             sync.RWMutex
	lastUpdate     time.Time
	cacheHits      atomic.Int64
	cacheMisses    atomic.Int64
}

// NewMatcher creates a new Matcher with the given repository and cache.
func NewMatcher(repo domain.RoutingRuleRepository, cache *RuleCache) *Matcher {
	return &Matcher{
		repo:  repo,
		cache: cache,
	}
}

// LoadRules loads the current routing rules, using cache when available.
func (m *Matcher) LoadRules(ctx context.Context) error {
	if m.cache == nil {
		return m.loadFromDB(ctx)
	}

	version, err := m.cache.GetRulesVersion(ctx)
	if err != nil {
		log.Printf("Failed to get rules version from cache, falling back to DB: %v", err)

		return m.loadFromDB(ctx)
	}

	if matcherHasCurrentVersion(m, version) {
		return nil
	}

	if loadCachedRules(ctx, m, version) {
		return nil
	}

	m.cacheMisses.Add(1)
	log.Printf("Loading rules from DB (cache miss/error, version=%d)", version)

	rules, err := m.repo.GetActiveRules(ctx)
	if err != nil {
		return fmt.Errorf("failed to load rules from repo: %w", err)
	}

	if version > 0 {
		cacheRulesAsync(ctx, m, version, rules)
	}

	m.updateRules(rules, version)
	log.Printf("Routing rules loaded from DB: count=%d, version=%d", len(rules), version)

	return nil
}

func matcherHasCurrentVersion(m *Matcher, version int64) bool {
	m.mu.RLock()
	defer m.mu.RUnlock()

	return m.currentVersion == version && version > 0 && len(m.rules) > 0
}

func loadCachedRules(ctx context.Context, m *Matcher, version int64) bool {
	if version <= 0 {
		return false
	}

	rules, err := m.cache.GetRulesByVersion(ctx, version)
	if err != nil {
		log.Printf("Failed to get rules (v%d) from cache: %v", version, err)

		return false
	}

	if rules == nil {
		return false
	}

	m.updateRules(rules, version)
	m.cacheHits.Add(1)
	log.Printf("Routing rules loaded from cache (v%d): count=%d", version, len(rules))

	return true
}

func cacheRulesAsync(ctx context.Context, m *Matcher, version int64, rules []domain.RoutingRule) {
	go func() {
		cacheCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), cacheAsyncTimeout)
		defer cancel()

		err := m.cache.SetRulesByVersion(cacheCtx, version, rules)
		if err != nil {
			log.Printf("Failed to update rules cache (v%d): %v", version, err)
		}
	}()
}

// Match finds the first routing rule that matches the given tags.
func (m *Matcher) Match(tags []domain.Tag) *domain.RoutingRule {
	m.mu.RLock()
	defer m.mu.RUnlock()

	for i := range m.rules {
		rule := &m.rules[i]
		if rule.MatchesTags(tags) {
			return rule
		}
	}

	return nil
}

// StartAutoRefresh periodically reloads routing rules at the given interval.
func (m *Matcher) StartAutoRefresh(ctx context.Context, interval time.Duration) {
	ticker := time.NewTicker(interval)
	go func() {
		defer ticker.Stop()

		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				err := m.LoadRules(ctx)
				if err != nil {
					log.Printf("Failed to auto-refresh routing rules: %v", err)
				}
			}
		}
	}()
}

// GetStats returns the cache hits and misses counts.
func (m *Matcher) GetStats() (int64, int64) {
	return m.cacheHits.Load(), m.cacheMisses.Load()
}

func (m *Matcher) loadFromDB(ctx context.Context) error {
	rules, err := m.repo.GetActiveRules(ctx)
	if err != nil {
		return fmt.Errorf("failed to load rules from repo (fallback): %w", err)
	}

	m.mu.Lock()
	m.rules = rules
	m.lastUpdate = time.Now()
	m.mu.Unlock()
	log.Printf("Routing rules loaded from DB (fallback): count=%d", len(rules))

	return nil
}

func (m *Matcher) updateRules(rules []domain.RoutingRule, version int64) {
	m.mu.Lock()
	m.rules = rules
	m.currentVersion = version
	m.lastUpdate = time.Now()
	m.mu.Unlock()
}
