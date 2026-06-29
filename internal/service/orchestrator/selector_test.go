package orchestrator

import (
	"context"
	"testing"
	"time"

	"github.com/alicebob/miniredis/v2"
	goredis "github.com/redis/go-redis/v9"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/beremaran/straw/internal/domain"
	"github.com/beremaran/straw/internal/infra/redis"
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

	ep1 := &redis.EndpointHealth{EndpointID: ep1ID, State: redis.HealthStateHealthy, Tags: []string{regionUSTag}}
	ep2 := &redis.EndpointHealth{EndpointID: ep2ID, State: redis.HealthStateHealthy, Tags: []string{"region:eu"}}
	ep3 := &redis.EndpointHealth{EndpointID: ep3ID, State: redis.HealthStateUnhealthy, Tags: []string{regionUSTag}}

	ep1.LastSeen = time.Now()
	ep2.LastSeen = time.Now()
	ep3.LastSeen = time.Now()

	require.NoError(t, store.UpdateHealth(ctx, ep1))
	require.NoError(t, store.UpdateHealth(ctx, ep2))
	require.NoError(t, store.UpdateHealth(ctx, ep3))

	t.Run("Select Success", func(t *testing.T) {
		rule := &domain.RoutingRule{
			RequiredTags: []string{regionUSTag},
			EndpointPools: []domain.EndpointPool{
				{Tier: 1},
			},
		}

		epID, err := selector.Select(ctx, rule)
		require.NoError(t, err)
		assert.Equal(t, ep1ID, epID)
	})

	t.Run("No Healthy Endpoints", func(t *testing.T) {
		rule := &domain.RoutingRule{
			RequiredTags: []string{"region:asia"},
			EndpointPools: []domain.EndpointPool{
				{Tier: 1},
			},
		}

		epID, err := selector.Select(ctx, rule)
		require.Error(t, err)
		assert.Empty(t, epID)
	})

	t.Run("Pool Tier Not Configured", func(t *testing.T) {
		rule := &domain.RoutingRule{
			RequiredTags:  []string{regionUSTag},
			EndpointPools: []domain.EndpointPool{},
		}

		epID, err := selector.GetEndpointFromPool(context.Background(), rule, 2, nil)
		require.Error(t, err)
		assert.Empty(t, epID)
	})

	t.Run("Exclusion", func(t *testing.T) {
		rule := &domain.RoutingRule{
			RequiredTags: []string{regionUSTag},
			EndpointPools: []domain.EndpointPool{
				{Tier: 1},
			},
		}

		epID, err := selector.GetEndpointFromPool(context.Background(), rule, 1, []string{ep1ID})
		require.Error(t, err)
		assert.Empty(t, epID)
	})
}

func TestSimpleEndpointSelector_SelectWithSession(t *testing.T) {
	store := newTestHealthStore(t)
	selector := NewSimpleEndpointSelector(store)

	_, err := selector.SelectWithSession(context.Background(), "sess1")
	require.Error(t, err)
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

	ep1 := &redis.EndpointHealth{EndpointID: ep1ID, State: redis.HealthStateHealthy, Tags: []string{regionUSTag}}
	ep2 := &redis.EndpointHealth{EndpointID: ep2ID, State: redis.HealthStateHealthy, Tags: []string{regionUSTag}}
	ep3 := &redis.EndpointHealth{EndpointID: ep3ID, State: redis.HealthStateHealthy, Tags: []string{regionUSTag}}

	ep1.LastSeen = time.Now()
	ep2.LastSeen = time.Now()
	ep3.LastSeen = time.Now()

	require.NoError(t, store.UpdateHealth(context.Background(), ep1))
	require.NoError(t, store.UpdateHealth(context.Background(), ep2))
	require.NoError(t, store.UpdateHealth(context.Background(), ep3))

	rule := &domain.RoutingRule{
		RequiredTags: []string{regionUSTag},
	}

	epID, err := selector.GetEndpointFromPool(context.Background(), rule, 1, nil)
	require.NoError(t, err)
	assert.Contains(t, []string{ep1ID, ep2ID, ep3ID}, epID)
}

func TestSimpleEndpointSelector_GetEndpointFromPool_AllExcluded(t *testing.T) {
	store := newTestHealthStore(t)
	selector := NewSimpleEndpointSelector(store)

	ep1 := &redis.EndpointHealth{EndpointID: ep1ID, State: redis.HealthStateHealthy, Tags: []string{regionUSTag}}
	ep1.LastSeen = time.Now()
	require.NoError(t, store.UpdateHealth(context.Background(), ep1))

	rule := &domain.RoutingRule{
		RequiredTags: []string{regionUSTag},
	}

	epID, err := selector.GetEndpointFromPool(context.Background(), rule, 1, []string{ep1ID})
	require.Error(t, err)
	assert.Empty(t, epID)
	assert.Contains(t, err.Error(), "no available endpoints")
}
