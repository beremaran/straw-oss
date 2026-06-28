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

type Matcher struct {
	repo           domain.RoutingRuleRepository
	cache          *RuleCache
	rules          []domain.RoutingRule
	currentVersion int64
	mu             sync.RWMutex
	lastUpdate     time.Time

	cacheHits   int64
	cacheMisses int64
}

func NewMatcher(repo domain.RoutingRuleRepository, cache *RuleCache) *Matcher {
	return &Matcher{
		repo:  repo,
		cache: cache,
	}
}

func (m *Matcher) LoadRules(ctx context.Context) error {
	if m.cache == nil {
		return m.loadFromDB(ctx)
	}

	version, err := m.cache.GetRulesVersion(ctx)
	if err != nil {
		log.Printf("Failed to get rules version from cache, falling back to DB: %v", err)

		return m.loadFromDB(ctx)
	}

	m.mu.RLock()
	if m.currentVersion == version && version > 0 && len(m.rules) > 0 {
		m.mu.RUnlock()

		return nil
	}
	m.mu.RUnlock()

	if version > 0 {
		rules, err := m.cache.GetRulesByVersion(ctx, version)
		if err != nil {
			log.Printf("Failed to get rules (v%d) from cache: %v", version, err)
		} else if rules != nil {
			m.updateRules(rules, version)
			atomic.AddInt64(&m.cacheHits, 1)
			log.Printf("Routing rules loaded from cache (v%d): count=%d", version, len(rules))

			return nil
		}
	}

	atomic.AddInt64(&m.cacheMisses, 1)
	log.Printf("Loading rules from DB (cache miss/error, version=%d)", version)
	rules, err := m.repo.GetActiveRules(ctx)
	if err != nil {
		return fmt.Errorf("failed to load rules from repo: %w", err)
	}

	if version > 0 {
		go func(v int64, r []domain.RoutingRule) {
			ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
			defer cancel()
			err := m.cache.SetRulesByVersion(ctx, v, r)
			if err != nil {
				log.Printf("Failed to update rules cache (v%d): %v", v, err)
			}
		}(version, rules)
	}

	m.updateRules(rules, version)
	log.Printf("Routing rules loaded from DB: count=%d, version=%d", len(rules), version)

	return nil
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

func (m *Matcher) GetStats() (hits, misses int64) {
	return atomic.LoadInt64(&m.cacheHits), atomic.LoadInt64(&m.cacheMisses)
}
