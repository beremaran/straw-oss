package router

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"github.com/alicebob/miniredis/v2"
	"github.com/redis/go-redis/v9"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/beremaran/straw/internal/domain"
)

const testCacheRuleID = "rule1"

func newTestRedis(t *testing.T) *redis.Client {
	mr, err := miniredis.Run()
	require.NoError(t, err)
	t.Cleanup(func() {
		mr.Close()
	})

	return redis.NewClient(&redis.Options{
		Addr: mr.Addr(),
	})
}

func TestRuleCache_RulesVersion(t *testing.T) {
	client := newTestRedis(t)
	cache := NewRuleCache(client, time.Minute)
	ctx := context.Background()

	ver, err := cache.GetRulesVersion(ctx)
	require.NoError(t, err)
	assert.Equal(t, int64(0), ver)

	newVer, err := cache.IncrementRulesVersion(ctx)
	require.NoError(t, err)
	assert.Equal(t, int64(1), newVer)

	ver, err = cache.GetRulesVersion(ctx)
	require.NoError(t, err)
	assert.Equal(t, int64(1), ver)
}

func TestRuleCache_RulesByVersion(t *testing.T) {
	client := newTestRedis(t)
	cache := NewRuleCache(client, time.Minute)
	ctx := context.Background()
	version := int64(1)

	rules := []domain.RoutingRule{
		{ID: testCacheRuleID, Priority: 10},
		{ID: "rule2", Priority: 5},
	}

	cachedRules, err := cache.GetRulesByVersion(ctx, version)
	require.NoError(t, err)
	assert.Nil(t, cachedRules)

	err = cache.SetRulesByVersion(ctx, version, rules)
	require.NoError(t, err)

	cachedRules, err = cache.GetRulesByVersion(ctx, version)
	require.NoError(t, err)
	assert.Equal(t, rules, cachedRules)

	key := ActiveRulesKeyPrefix + "1"
	val, err := client.Get(ctx, key).Bytes()
	require.NoError(t, err)
	var fromRedis []domain.RoutingRule
	err = json.Unmarshal(val, &fromRedis)
	require.NoError(t, err)
	assert.Equal(t, rules, fromRedis)
}

func TestRuleCache_Invalidate(t *testing.T) {
	client := newTestRedis(t)
	cache := NewRuleCache(client, time.Minute)
	ctx := context.Background()
	version := int64(1)

	rules := []domain.RoutingRule{{ID: testCacheRuleID}}
	err := cache.SetRulesByVersion(ctx, version, rules)
	require.NoError(t, err)

	err = cache.Invalidate(ctx, version)
	require.NoError(t, err)

	cachedRules, err := cache.GetRulesByVersion(ctx, version)
	require.NoError(t, err)
	assert.Nil(t, cachedRules)
}

func TestRuleCache_NewRuleCache(t *testing.T) {
	client := newTestRedis(t)

	t.Run("With Custom TTL", func(t *testing.T) {
		customTTL := 5 * time.Minute
		cache := NewRuleCache(client, customTTL)
		assert.Equal(t, customTTL, cache.ttl)
	})

	t.Run("With Zero TTL (Uses Default)", func(t *testing.T) {
		cache := NewRuleCache(client, 0)
		assert.Equal(t, DefaultCacheTTL, cache.ttl)
	})
}

func TestRuleCache_ErrorCases(t *testing.T) {
	client := newTestRedis(t)
	cache := NewRuleCache(client, time.Minute)
	ctx := context.Background()

	t.Run("GetRulesVersion - Redis Error", func(t *testing.T) {
		_ = client.Close()

		ver, err := cache.GetRulesVersion(ctx)
		require.Error(t, err)
		assert.Equal(t, int64(0), ver)
		assert.Contains(t, err.Error(), "failed to get rules version")
	})

	t.Run("GetRulesByVersion - Redis Error", func(t *testing.T) {
		newClient := newTestRedis(t)
		newCache := NewRuleCache(newClient, time.Minute)
		_ = newClient.Close()

		rules, err := newCache.GetRulesByVersion(ctx, 1)
		require.Error(t, err)
		assert.Nil(t, rules)
		assert.Contains(t, err.Error(), "failed to get rules from cache")
	})

	t.Run("SetRulesByVersion - JSON Marshal Error", func(t *testing.T) {
		newClient := newTestRedis(t)
		newCache := NewRuleCache(newClient, time.Minute)
		_ = newClient.Close()

		rules := []domain.RoutingRule{{ID: testCacheRuleID}}
		err := newCache.SetRulesByVersion(ctx, 1, rules)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "failed to set rules in cache")
	})
}
