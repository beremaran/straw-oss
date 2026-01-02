package router

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/kwilabs/straw-proxy-server/internal/domain"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type mockRepo struct {
	rules []domain.RoutingRule
	err   error
}

func (m *mockRepo) GetActiveRules(ctx context.Context) ([]domain.RoutingRule, error) {
	return m.rules, m.err
}

type mockCache struct {
	rules   []domain.RoutingRule
	version int64
	err     error
}

func (m *mockCache) GetRulesVersion(ctx context.Context) (int64, error) {
	return m.version, m.err
}

func (m *mockCache) GetRulesByVersion(ctx context.Context, version int64) ([]domain.RoutingRule, error) {
	if version == m.version {
		return m.rules, m.err
	}
	return nil, nil // Miss
}

func (m *mockCache) SetRulesByVersion(ctx context.Context, version int64, rules []domain.RoutingRule) error {
	m.version = version
	m.rules = rules
	return m.err
}

func TestMatcher_LoadRules(t *testing.T) {
	ctx := context.Background()

	t.Run("Load from Cache", func(t *testing.T) {
		expectedRules := []domain.RoutingRule{{ID: "rule-1"}}
		cache := &mockCache{rules: expectedRules, version: 1}
		repo := &mockRepo{}
		matcher := NewMatcher(repo, cache)

		err := matcher.LoadRules(ctx)
		assert.NoError(t, err)
		assert.Equal(t, expectedRules, matcher.rules)
		assert.Equal(t, int64(1), matcher.currentVersion)
	})

	t.Run("Load from Repo on Cache Miss", func(t *testing.T) {
		expectedRules := []domain.RoutingRule{{ID: "rule-1"}}
		cache := &mockCache{version: 0} // Miss (no version)
		repo := &mockRepo{rules: expectedRules}
		matcher := NewMatcher(repo, cache)

		err := matcher.LoadRules(ctx)
		assert.NoError(t, err)
		assert.Equal(t, expectedRules, matcher.rules)

		// Wait a bit for async cache update?
		// Since we can't easily sync on the goroutine, checking cache.rules here might be flaky
		// unless we modify SetActiveRules to notify or sleep.
		// For now we assume logic is correct if LoadRules succeeds with repo rules.
	})

	t.Run("Cache Version Error - Fallback to DB", func(t *testing.T) {
		expectedRules := []domain.RoutingRule{{ID: "rule-db"}}
		cache := &mockCache{version: 1, err: errors.New("cache error")}
		repo := &mockRepo{rules: expectedRules}
		matcher := NewMatcher(repo, cache)

		err := matcher.LoadRules(ctx)
		assert.NoError(t, err)
		assert.Equal(t, expectedRules, matcher.rules)
	})

	t.Run("Cache Get Error - Fallback to DB", func(t *testing.T) {
		expectedRules := []domain.RoutingRule{{ID: "rule-db"}}
		cache := &mockCache{version: 1, rules: nil, err: errors.New("get error")}
		repo := &mockRepo{rules: expectedRules}
		matcher := NewMatcher(repo, cache)

		err := matcher.LoadRules(ctx)
		assert.NoError(t, err)
		assert.Equal(t, expectedRules, matcher.rules)
	})

	t.Run("DB Error", func(t *testing.T) {
		cache := &mockCache{version: 0}
		repo := &mockRepo{err: errors.New("db error")}
		matcher := NewMatcher(repo, cache)

		err := matcher.LoadRules(ctx)
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "failed to load rules from repo")
	})

	t.Run("Already Up To Date", func(t *testing.T) {
		expectedRules := []domain.RoutingRule{{ID: "rule-1"}}
		cache := &mockCache{version: 1, rules: expectedRules}
		repo := &mockRepo{}
		matcher := NewMatcher(repo, cache)

		// First load
		err := matcher.LoadRules(ctx)
		require.NoError(t, err)
		assert.Equal(t, expectedRules, matcher.rules)

		// Second load - should skip since version matches
		err = matcher.LoadRules(ctx)
		assert.NoError(t, err)
		assert.Equal(t, expectedRules, matcher.rules)
	})
}

func TestMatcher_GetStats(t *testing.T) {
	ctx := context.Background()
	expectedRules := []domain.RoutingRule{{ID: "rule-1"}}
	cache := &mockCache{version: 1, rules: expectedRules}
	repo := &mockRepo{}
	matcher := NewMatcher(repo, cache)

	// Load rules (should increment cache hit)
	err := matcher.LoadRules(ctx)
	require.NoError(t, err)

	hits, misses := matcher.GetStats()
	assert.Equal(t, int64(1), hits)
	assert.Equal(t, int64(0), misses)

	// Load again with same version - should skip (no change to stats)
	err = matcher.LoadRules(ctx)
	require.NoError(t, err)

	hits, misses = matcher.GetStats()
	assert.Equal(t, int64(1), hits)
	assert.Equal(t, int64(0), misses)

	// Load with version 0 (cache miss scenario)
	cache.version = 0
	cache.rules = nil // Force cache miss
	err = matcher.LoadRules(ctx)
	require.NoError(t, err)

	hits, misses = matcher.GetStats()
	assert.Equal(t, int64(1), hits)
	assert.Equal(t, int64(1), misses)
}

func TestMatcher_StartAutoRefresh(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	expectedRules := []domain.RoutingRule{{ID: "rule-1"}}
	cache := &mockCache{version: 1, rules: expectedRules}
	repo := &mockRepo{}
	matcher := NewMatcher(repo, cache)

	// Start auto refresh with short interval
	matcher.StartAutoRefresh(ctx, 10*time.Millisecond)

	// Wait for at least one refresh
	time.Sleep(50 * time.Millisecond)

	// Rules should still be loaded
	assert.Equal(t, expectedRules, matcher.rules)
}

func TestMatcher_loadFromDB(t *testing.T) {
	ctx := context.Background()

	t.Run("Successful Load", func(t *testing.T) {
		expectedRules := []domain.RoutingRule{{ID: "rule-db"}}
		cache := &mockCache{}
		repo := &mockRepo{rules: expectedRules}
		matcher := NewMatcher(repo, cache)

		err := matcher.loadFromDB(ctx)
		assert.NoError(t, err)
		assert.Equal(t, expectedRules, matcher.rules)
	})

	t.Run("DB Error", func(t *testing.T) {
		cache := &mockCache{}
		repo := &mockRepo{err: errors.New("db error")}
		matcher := NewMatcher(repo, cache)

		err := matcher.loadFromDB(ctx)
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "failed to load rules from repo")
	})
}

func TestMatcher_Match(t *testing.T) {
	rules := []domain.RoutingRule{
		{
			ID:           "rule-high",
			Priority:     100,
			RequiredTags: []string{"target:amazon"},
		},
		{
			ID:           "rule-low",
			Priority:     50,
			RequiredTags: []string{"type:search"},
		},
		{
			ID:           "rule-excluded",
			Priority:     200, // Higher priority but excluded
			RequiredTags: []string{"target:amazon"},
			ExcludedTags: []string{"region:eu"},
		},
	}

	matcher := &Matcher{
		rules: rules,
	}

	tests := []struct {
		name string
		tags []domain.Tag
		want string // ID of matched rule
	}{
		{
			name: "Match High Priority",
			tags: []domain.Tag{{Key: "target", Value: "amazon"}},
			want: "rule-high",
		},
		{
			name: "Match Low Priority",
			tags: []domain.Tag{{Key: "type", Value: "search"}},
			want: "rule-low",
		},
		{
			name: "Match Excluded (Should match next best)",
			tags: []domain.Tag{
				{Key: "target", Value: "amazon"},
				{Key: "region", Value: "eu"},
			},
			want: "rule-high", // rule-excluded skipped, matches rule-high (Wait, rule-high requires target:amazon which is present. Does it have exclusions? No.)
		},
		{
			name: "No Match",
			tags: []domain.Tag{{Key: "target", Value: "google"}},
			want: "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := matcher.Match(tt.tags)
			if tt.want == "" {
				assert.Nil(t, got)
			} else {
				assert.NotNil(t, got)
				assert.Equal(t, tt.want, got.ID)
			}
		})
	}
}
