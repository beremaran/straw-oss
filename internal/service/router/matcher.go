package router

import (
	"context"
	"fmt"
	"log"
	"sync"
	"sync/atomic"
	"time"

	"github.com/kwilabs/straw-proxy-server/internal/domain"
)

// RuleRepository defines the interface for fetching routing rules.
type RuleRepository interface {
	GetActiveRules(ctx context.Context) ([]domain.RoutingRule, error)
}

// RuleCacheInterface defines the interface for caching routing rules.
type RuleCacheInterface interface {
	GetRulesVersion(ctx context.Context) (int64, error)
	GetRulesByVersion(ctx context.Context, version int64) ([]domain.RoutingRule, error)
	SetRulesByVersion(ctx context.Context, version int64, rules []domain.RoutingRule) error
}

// Matcher handles matching request tags to routing rules.
type Matcher struct {
	repo           RuleRepository
	cache          RuleCacheInterface
	rules          []domain.RoutingRule
	currentVersion int64
	mu             sync.RWMutex
	lastUpdate     time.Time

	// Stats
	cacheHits   int64
	cacheMisses int64
}

// NewMatcher creates a new Matcher.
func NewMatcher(repo RuleRepository, cache RuleCacheInterface) *Matcher {
	return &Matcher{
		repo:  repo,
		cache: cache,
	}
}

// LoadRules reloads the routing rules from cache or database.
func (m *Matcher) LoadRules(ctx context.Context) error {
	// 1. Check current version in Redis
	version, err := m.cache.GetRulesVersion(ctx)
	if err != nil {
		log.Printf("Failed to get rules version from cache, falling back to DB: %v", err)
		return m.loadFromDB(ctx)
	}

	// If version is 0 (key missing) or same as current, we might still want to check
	// if we have rules loaded. If we have rules and version matches, skip.
	m.mu.RLock()
	if m.currentVersion == version && version > 0 && len(m.rules) > 0 {
		m.mu.RUnlock()
		return nil // Up to date
	}
	m.mu.RUnlock()

	// 2. Try to get rules for this version from cache
	if version > 0 {
		rules, err := m.cache.GetRulesByVersion(ctx, version)
		if err != nil {
			log.Printf("Failed to get rules (v%d) from cache: %v", version, err)
			// Fallback to loading from DB and populating this version
		} else if rules != nil {
			// Cache Hit
			m.updateRules(rules, version)
			atomic.AddInt64(&m.cacheHits, 1)
			log.Printf("Routing rules loaded from cache (v%d): count=%d", version, len(rules))
			return nil
		}
	}

	// 3. Fallback to DB (Cache Miss or No Version or Redis Error)
	atomic.AddInt64(&m.cacheMisses, 1)
	log.Printf("Loading rules from DB (cache miss/error, version=%d)", version)
	rules, err := m.repo.GetActiveRules(ctx)
	if err != nil {
		return fmt.Errorf("failed to load rules from repo: %w", err)
	}

	// 4. Update Cache (if we have a valid version)
	// If version was 0, we can't really cache it effectively under a version mechanism
	// unless we knew what the next version should be.
	// In a real scenario, we might want to "Fix" the version if it's missing,
	// but for now we just load local.
	if version > 0 {
		go func(v int64, r []domain.RoutingRule) {
			ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
			defer cancel()
			if err := m.cache.SetRulesByVersion(ctx, v, r); err != nil {
				log.Printf("Failed to update rules cache (v%d): %v", v, err)
			}
		}(version, rules)
	}

	m.updateRules(rules, version)
	log.Printf("Routing rules loaded from DB: count=%d, version=%d", len(rules), version)
	return nil
}

// loadFromDB forces loading from DB, ignoring cache versions logic temporarily
// used when cache is completely unreachable or reporting errors.
func (m *Matcher) loadFromDB(ctx context.Context) error {
	rules, err := m.repo.GetActiveRules(ctx)
	if err != nil {
		return fmt.Errorf("failed to load rules from repo (fallback): %w", err)
	}
	// We preserve the current version since we don't know the new one
	// Or maybe we treat it as "unknown" (0). Let's keep existing version to avoid thrashing?
	// Actually, if we can't reach Redis, we can't validly say we are at version X.
	// But resetting to 0 might cause re-fetch loops if Redis flickers.
	// Let's just update rules.
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

// Match finds the best matching routing rule for the given tags.
// Returns nil if no rule matches.
func (m *Matcher) Match(tags []domain.Tag) *domain.RoutingRule {
	m.mu.RLock()
	defer m.mu.RUnlock()

	// Rules are assumed to be sorted by priority (descending) from the source
	for i := range m.rules {
		rule := &m.rules[i]
		if rule.MatchesTags(tags) {
			return rule
		}
	}

	return nil
}

// StartAutoRefresh starts a background goroutine to refresh rules periodically.
func (m *Matcher) StartAutoRefresh(ctx context.Context, interval time.Duration) {
	ticker := time.NewTicker(interval)
	go func() {
		defer ticker.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				if err := m.LoadRules(ctx); err != nil {
					log.Printf("Failed to auto-refresh routing rules: %v", err)
				}
			}
		}
	}()
}

// GetStats returns the cache hits and misses.
func (m *Matcher) GetStats() (hits, misses int64) {
	return atomic.LoadInt64(&m.cacheHits), atomic.LoadInt64(&m.cacheMisses)
}
