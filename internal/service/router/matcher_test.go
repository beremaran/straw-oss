package router

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/alicebob/miniredis/v2"
	"github.com/redis/go-redis/v9"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/beremaran/straw/internal/domain"
)

const (
	testMatcherRuleID = "rule-1"
	testRuleHigh      = "rule-high"
	testTarget        = "target"
	testTargetVal     = "amazon"
	testSearchVal     = "search"
	testRegionKey     = "region"
	testRegionVal     = "eu"
	testTypeKey       = "type"
)

type mockRepo struct {
	rules []domain.RoutingRule
	err   error
}

func (m *mockRepo) GetActiveRules(context.Context) ([]domain.RoutingRule, error) {
	return m.rules, m.err
}

func (m *mockRepo) CreateRule(context.Context, *domain.RoutingRule) error { return nil }
func (m *mockRepo) GetRuleByID(context.Context, string) (*domain.RoutingRule, error) {
	return nil, nil
}
func (m *mockRepo) UpdateRule(context.Context, *domain.RoutingRule) error { return nil }
func (m *mockRepo) DeleteRule(context.Context, string) error              { return nil }
func (m *mockRepo) ListRules(context.Context, int, int) ([]domain.RoutingRule, int, error) {
	return nil, 0, nil
}

func TestMatcher_LoadRules(t *testing.T) {
	ctx := context.Background()

	t.Run("Load from Cache", func(t *testing.T) {
		client := newTestRedis(t)
		cache := NewRuleCache(client, time.Minute)
		repo := &mockRepo{}

		expectedRules := []domain.RoutingRule{{ID: testMatcherRuleID}}
		_, err := cache.IncrementRulesVersion(ctx)
		require.NoError(t, err)
		err = cache.SetRulesByVersion(ctx, 1, expectedRules)
		require.NoError(t, err)

		matcher := NewMatcher(repo, cache)

		err = matcher.LoadRules(ctx)
		require.NoError(t, err)
		assert.Equal(t, expectedRules, matcher.rules)
		assert.Equal(t, int64(1), matcher.currentVersion)
	})

	t.Run("Load from Repo on Cache Miss", func(t *testing.T) {
		client := newTestRedis(t)
		cache := NewRuleCache(client, time.Minute)

		expectedRules := []domain.RoutingRule{{ID: testMatcherRuleID}}
		repo := &mockRepo{rules: expectedRules}
		matcher := NewMatcher(repo, cache)

		err := matcher.LoadRules(ctx)
		require.NoError(t, err)
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
		require.NoError(t, err)
		assert.Equal(t, expectedRules, matcher.rules)
	})

	t.Run("DB Error", func(t *testing.T) {
		client := newTestRedis(t)
		cache := NewRuleCache(client, time.Minute)
		repo := &mockRepo{err: errors.New("db error")}
		matcher := NewMatcher(repo, cache)

		err := matcher.LoadRules(ctx)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "failed to load rules from repo")
	})

	t.Run("Already Up To Date", func(t *testing.T) {
		client := newTestRedis(t)
		cache := NewRuleCache(client, time.Minute)
		expectedRules := []domain.RoutingRule{{ID: testMatcherRuleID}}
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
		require.NoError(t, err)
		assert.Equal(t, expectedRules, matcher.rules)
	})
}

func TestMatcher_GetStats(t *testing.T) {
	ctx := context.Background()
	client := newTestRedis(t)
	cache := NewRuleCache(client, time.Minute)
	expectedRules := []domain.RoutingRule{{ID: testMatcherRuleID}}
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
	ctx := t.Context()

	client := newTestRedis(t)
	cache := NewRuleCache(client, time.Minute)
	expectedRules := []domain.RoutingRule{
		{ID: testMatcherRuleID, RequiredTags: []string{"target:test"}},
	}
	_, err := cache.IncrementRulesVersion(ctx)
	require.NoError(t, err)
	err = cache.SetRulesByVersion(ctx, 1, expectedRules)
	require.NoError(t, err)

	repo := &mockRepo{}
	matcher := NewMatcher(repo, cache)

	matcher.StartAutoRefresh(ctx, 10*time.Millisecond)

	time.Sleep(50 * time.Millisecond)

	matched := matcher.Match([]domain.Tag{{Key: testTarget, Value: "test"}})
	assert.NotNil(t, matched)
	assert.Equal(t, testMatcherRuleID, matched.ID)
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
		require.NoError(t, err)
		assert.Equal(t, expectedRules, matcher.rules)
	})

	t.Run("DB Error", func(t *testing.T) {
		client := newTestRedis(t)
		cache := NewRuleCache(client, time.Minute)
		repo := &mockRepo{err: errors.New("db error")}
		matcher := NewMatcher(repo, cache)

		err := matcher.loadFromDB(ctx)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "failed to load rules from repo")
	})
}

func TestMatcher_Match(t *testing.T) {
	rules := []domain.RoutingRule{
		{
			ID:           testRuleHigh,
			Priority:     100,
			RequiredTags: []string{testTarget + ":" + testTargetVal},
		},
		{
			ID:           "rule-low",
			Priority:     50,
			RequiredTags: []string{testTypeKey + ":" + testSearchVal},
		},
		{
			ID:           "rule-excluded",
			Priority:     200,
			RequiredTags: []string{testTarget + ":" + testTargetVal},
			ExcludedTags: []string{testRegionKey + ":" + testRegionVal},
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
			tags: []domain.Tag{{Key: testTarget, Value: testTargetVal}},
			want: testRuleHigh,
		},
		{
			name: "Match Low Priority",
			tags: []domain.Tag{{Key: testTypeKey, Value: testSearchVal}},
			want: "rule-low",
		},
		{
			name: "Match Excluded (Should match next best)",
			tags: []domain.Tag{
				{Key: testTarget, Value: testTargetVal},
				{Key: testRegionKey, Value: testRegionVal},
			},
			want: testRuleHigh,
		},
		{
			name: "No Match",
			tags: []domain.Tag{{Key: testTarget, Value: "google"}},
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
