package router

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/alicebob/miniredis/v2"
	"github.com/beremaran/straw/internal/domain"
	"github.com/redis/go-redis/v9"
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

func (m *mockRepo) CreateRule(ctx context.Context, rule *domain.RoutingRule) error { return nil }
func (m *mockRepo) GetRuleByID(ctx context.Context, id string) (*domain.RoutingRule, error) {
	return nil, nil
}
func (m *mockRepo) UpdateRule(ctx context.Context, rule *domain.RoutingRule) error { return nil }
func (m *mockRepo) DeleteRule(ctx context.Context, id string) error                { return nil }
func (m *mockRepo) ListRules(ctx context.Context, limit, offset int) ([]domain.RoutingRule, int, error) {
	return nil, 0, nil
}

func TestMatcher_LoadRules(t *testing.T) {
	ctx := context.Background()

	t.Run("Load from Cache", func(t *testing.T) {
		client := newTestRedis(t)
		cache := NewRuleCache(client, time.Minute)
		repo := &mockRepo{}

		expectedRules := []domain.RoutingRule{{ID: "rule-1"}}
		_, err := cache.IncrementRulesVersion(ctx)
		require.NoError(t, err)
		err = cache.SetRulesByVersion(ctx, 1, expectedRules)
		require.NoError(t, err)

		matcher := NewMatcher(repo, cache)

		err = matcher.LoadRules(ctx)
		assert.NoError(t, err)
		assert.Equal(t, expectedRules, matcher.rules)
		assert.Equal(t, int64(1), matcher.currentVersion)
	})

	t.Run("Load from Repo on Cache Miss", func(t *testing.T) {
		client := newTestRedis(t)
		cache := NewRuleCache(client, time.Minute)

		expectedRules := []domain.RoutingRule{{ID: "rule-1"}}
		repo := &mockRepo{rules: expectedRules}
		matcher := NewMatcher(repo, cache)

		err := matcher.LoadRules(ctx)
		assert.NoError(t, err)
		assert.Equal(t, expectedRules, matcher.rules)
	})

	t.Run("Cache Version Error - Fallback to DB", func(t *testing.T) {
		mr, err := miniredis.Run()
		require.NoError(t, err)
		client := redisClientFromAddr(mr.Addr())
		cache := NewRuleCache(client, time.Minute)

		expectedRules := []domain.RoutingRule{{ID: "rule-db"}}
		repo := &mockRepo{rules: expectedRules}
		matcher := NewMatcher(repo, cache)

		mr.Close()

		err = matcher.LoadRules(ctx)
		assert.NoError(t, err)
		assert.Equal(t, expectedRules, matcher.rules)
	})

	t.Run("DB Error", func(t *testing.T) {
		client := newTestRedis(t)
		cache := NewRuleCache(client, time.Minute)
		repo := &mockRepo{err: errors.New("db error")}
		matcher := NewMatcher(repo, cache)

		err := matcher.LoadRules(ctx)
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "failed to load rules from repo")
	})

	t.Run("Already Up To Date", func(t *testing.T) {
		client := newTestRedis(t)
		cache := NewRuleCache(client, time.Minute)
		expectedRules := []domain.RoutingRule{{ID: "rule-1"}}
		repo := &mockRepo{rules: expectedRules}

		_, err := cache.IncrementRulesVersion(ctx)
		require.NoError(t, err)
		err = cache.SetRulesByVersion(ctx, 1, expectedRules)
		require.NoError(t, err)

		matcher := NewMatcher(repo, cache)

		err = matcher.LoadRules(ctx)
		require.NoError(t, err)
		assert.Equal(t, expectedRules, matcher.rules)

		err = matcher.LoadRules(ctx)
		assert.NoError(t, err)
		assert.Equal(t, expectedRules, matcher.rules)
	})
}

func TestMatcher_GetStats(t *testing.T) {
	ctx := context.Background()
	client := newTestRedis(t)
	cache := NewRuleCache(client, time.Minute)
	expectedRules := []domain.RoutingRule{{ID: "rule-1"}}
	repo := &mockRepo{rules: expectedRules}

	_, err := cache.IncrementRulesVersion(ctx)
	require.NoError(t, err)
	err = cache.SetRulesByVersion(ctx, 1, expectedRules)
	require.NoError(t, err)

	matcher := NewMatcher(repo, cache)

	err = matcher.LoadRules(ctx)
	require.NoError(t, err)

	hits, misses := matcher.GetStats()
	assert.Equal(t, int64(1), hits)
	assert.Equal(t, int64(0), misses)

	err = matcher.LoadRules(ctx)
	require.NoError(t, err)

	hits, misses = matcher.GetStats()
	assert.Equal(t, int64(1), hits)
	assert.Equal(t, int64(0), misses)

	cache2 := NewRuleCache(newTestRedis(t), time.Minute)
	matcher2 := NewMatcher(repo, cache2)
	err = matcher2.LoadRules(ctx)
	require.NoError(t, err)

	hits, misses = matcher2.GetStats()
	assert.Equal(t, int64(0), hits)
	assert.Equal(t, int64(1), misses)
}

func TestMatcher_StartAutoRefresh(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	client := newTestRedis(t)
	cache := NewRuleCache(client, time.Minute)
	expectedRules := []domain.RoutingRule{
		{ID: "rule-1", RequiredTags: []string{"target:test"}},
	}
	_, err := cache.IncrementRulesVersion(ctx)
	require.NoError(t, err)
	err = cache.SetRulesByVersion(ctx, 1, expectedRules)
	require.NoError(t, err)

	repo := &mockRepo{}
	matcher := NewMatcher(repo, cache)

	matcher.StartAutoRefresh(ctx, 10*time.Millisecond)

	time.Sleep(50 * time.Millisecond)

	matched := matcher.Match([]domain.Tag{{Key: "target", Value: "test"}})
	assert.NotNil(t, matched)
	assert.Equal(t, "rule-1", matched.ID)
}

func TestMatcher_loadFromDB(t *testing.T) {
	ctx := context.Background()

	t.Run("Successful Load", func(t *testing.T) {
		client := newTestRedis(t)
		cache := NewRuleCache(client, time.Minute)
		expectedRules := []domain.RoutingRule{{ID: "rule-db"}}
		repo := &mockRepo{rules: expectedRules}
		matcher := NewMatcher(repo, cache)

		err := matcher.loadFromDB(ctx)
		assert.NoError(t, err)
		assert.Equal(t, expectedRules, matcher.rules)
	})

	t.Run("DB Error", func(t *testing.T) {
		client := newTestRedis(t)
		cache := NewRuleCache(client, time.Minute)
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
			Priority:     200,
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
		want string
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
			want: "rule-high",
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

func redisClientFromAddr(addr string) *redis.Client {
	rdb := redis.NewClient(&redis.Options{
		Addr: addr,
	})

	return rdb
}
