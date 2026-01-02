package orchestrator

import (
	"context"
	"testing"
	"time"

	"github.com/alicebob/miniredis/v2"
	"github.com/kwilabs/straw-proxy-server/internal/domain"
	"github.com/kwilabs/straw-proxy-server/internal/infra/redis"
	goredis "github.com/redis/go-redis/v9"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func newTestHealthStore(t *testing.T) *redis.EndpointHealthStore {
	mr, err := miniredis.Run()
	require.NoError(t, err)
	t.Cleanup(func() {
		mr.Close()
	})

	rdb := goredis.NewClient(&goredis.Options{
		Addr: mr.Addr(),
	})

	client := &redis.Client{Client: rdb}
	return redis.NewEndpointHealthStore(client)
}

func TestSimpleEndpointSelector_Select(t *testing.T) {
	ctx := context.Background()
	store := newTestHealthStore(t)
	selector := NewSimpleEndpointSelector(store)

	// Setup endpoints
	ep1 := &redis.EndpointHealth{EndpointID: "ep1", State: redis.HealthStateHealthy, Tags: []string{"region:us"}}
	ep2 := &redis.EndpointHealth{EndpointID: "ep2", State: redis.HealthStateHealthy, Tags: []string{"region:eu"}}
	ep3 := &redis.EndpointHealth{EndpointID: "ep3", State: redis.HealthStateUnhealthy, Tags: []string{"region:us"}}

	// Note: UpdateHealth only updates Redis, LastSeen is handled by caller usually or inside UpdateHealth if passed.
	// We should set LastSeen to avoid instant staleness if checked, but here we just check if it's in list.
	ep1.LastSeen = time.Now()
	ep2.LastSeen = time.Now()
	ep3.LastSeen = time.Now()

	require.NoError(t, store.UpdateHealth(ctx, ep1))
	require.NoError(t, store.UpdateHealth(ctx, ep2))
	require.NoError(t, store.UpdateHealth(ctx, ep3))

	t.Run("Select Success", func(t *testing.T) {
		rule := &domain.RoutingRule{
			RequiredTags: []string{"region:us"},
			EndpointPools: []domain.EndpointPool{
				{Tier: 1}, // Implicit config
			},
		}

		epID, err := selector.Select(ctx, rule)
		assert.NoError(t, err)
		assert.Equal(t, "ep1", epID)
	})

	t.Run("No Healthy Endpoints", func(t *testing.T) {
		rule := &domain.RoutingRule{
			RequiredTags: []string{"region:asia"},
			EndpointPools: []domain.EndpointPool{
				{Tier: 1},
			},
		}

		epID, err := selector.Select(ctx, rule)
		assert.Error(t, err)
		assert.Empty(t, epID)
	})

	t.Run("Pool Tier Not Configured", func(t *testing.T) {
		rule := &domain.RoutingRule{
			RequiredTags:  []string{"region:us"},
			EndpointPools: []domain.EndpointPool{}, // Empty pools
		}

		// Tier 2 requested but not configured
		epID, err := selector.GetEndpointFromPool(context.Background(), rule, 2, nil)
		assert.Error(t, err)
		assert.Empty(t, epID)
	})

	t.Run("Exclusion", func(t *testing.T) {
		rule := &domain.RoutingRule{
			RequiredTags: []string{"region:us"},
			EndpointPools: []domain.EndpointPool{
				{Tier: 1},
			},
		}

		// ep1 matches but is excluded
		epID, err := selector.GetEndpointFromPool(context.Background(), rule, 1, []string{"ep1"})
		assert.Error(t, err)
		assert.Empty(t, epID)
	})
}

func TestSimpleEndpointSelector_SelectWithSession(t *testing.T) {
	store := newTestHealthStore(t)
	selector := NewSimpleEndpointSelector(store)

	_, err := selector.SelectWithSession(context.Background(), "sess1")
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "not implemented")
}

func TestSimpleEndpointSelector_GetPoolConfig(t *testing.T) {
	store := newTestHealthStore(t)
	selector := NewSimpleEndpointSelector(store)

	t.Run("nil rule", func(t *testing.T) {
		config := selector.GetPoolConfig(nil, 1)
		assert.Nil(t, config)
	})

	t.Run("pool tier exists", func(t *testing.T) {
		rule := &domain.RoutingRule{
			EndpointPools: []domain.EndpointPool{
				{Tier: 1, MaxRetries: 2},
				{Tier: 2, MaxRetries: 3},
			},
		}

		config := selector.GetPoolConfig(rule, 1)
		assert.NotNil(t, config)
		assert.Equal(t, 1, config.Tier)
		assert.Equal(t, 2, config.MaxRetries)
	})

	t.Run("pool tier not found", func(t *testing.T) {
		rule := &domain.RoutingRule{
			EndpointPools: []domain.EndpointPool{
				{Tier: 1, MaxRetries: 2},
			},
		}

		config := selector.GetPoolConfig(rule, 2)
		assert.Nil(t, config)
	})

	t.Run("empty pools", func(t *testing.T) {
		rule := &domain.RoutingRule{
			EndpointPools: []domain.EndpointPool{},
		}

		config := selector.GetPoolConfig(rule, 1)
		assert.Nil(t, config)
	})
}

func TestSimpleEndpointSelector_GetEndpointFromPool_MultipleEndpoints(t *testing.T) {
	store := newTestHealthStore(t)
	selector := NewSimpleEndpointSelector(store)

	// Setup multiple endpoints with same tags
	ep1 := &redis.EndpointHealth{EndpointID: "ep1", State: redis.HealthStateHealthy, Tags: []string{"region:us"}}
	ep2 := &redis.EndpointHealth{EndpointID: "ep2", State: redis.HealthStateHealthy, Tags: []string{"region:us"}}
	ep3 := &redis.EndpointHealth{EndpointID: "ep3", State: redis.HealthStateHealthy, Tags: []string{"region:us"}}

	ep1.LastSeen = time.Now()
	ep2.LastSeen = time.Now()
	ep3.LastSeen = time.Now()

	require.NoError(t, store.UpdateHealth(context.Background(), ep1))
	require.NoError(t, store.UpdateHealth(context.Background(), ep2))
	require.NoError(t, store.UpdateHealth(context.Background(), ep3))

	rule := &domain.RoutingRule{
		RequiredTags: []string{"region:us"},
	}

	// Should return one of the endpoints (random selection)
	epID, err := selector.GetEndpointFromPool(context.Background(), rule, 1, nil)
	assert.NoError(t, err)
	assert.Contains(t, []string{"ep1", "ep2", "ep3"}, epID)
}

func TestSimpleEndpointSelector_GetEndpointFromPool_AllExcluded(t *testing.T) {
	store := newTestHealthStore(t)
	selector := NewSimpleEndpointSelector(store)

	// Setup endpoint
	ep1 := &redis.EndpointHealth{EndpointID: "ep1", State: redis.HealthStateHealthy, Tags: []string{"region:us"}}
	ep1.LastSeen = time.Now()
	require.NoError(t, store.UpdateHealth(context.Background(), ep1))

	rule := &domain.RoutingRule{
		RequiredTags: []string{"region:us"},
	}

	// Exclude the only endpoint
	epID, err := selector.GetEndpointFromPool(context.Background(), rule, 1, []string{"ep1"})
	assert.Error(t, err)
	assert.Empty(t, epID)
	assert.Contains(t, err.Error(), "no available endpoints")
}
