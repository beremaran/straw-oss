//nolint:errcheck
package ratelimit_test

import (
	"context"
	"testing"
	"time"

	"github.com/alicebob/miniredis/v2"
	"github.com/beremaran/straw/internal/config"
	"github.com/beremaran/straw/internal/infra/redis"
	"github.com/beremaran/straw/internal/service/ratelimit"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestRateLimiter_Allow(t *testing.T) {
	ctx := context.Background()
	s, err := miniredis.Run()
	require.NoError(t, err)
	defer s.Close()

	client, err := redis.NewClient(ctx, config.RedisConfig{Addr: s.Addr()}, nil)
	require.NoError(t, err)
	defer client.Close()

	limiter := ratelimit.NewRateLimiter(client)
	quotaKey := "test_key"

	t.Run("Approves request within limits", func(t *testing.T) {
		s.FlushAll()
		allowed, res, err := limiter.Allow(ctx, quotaKey, 10, 60)
		require.NoError(t, err)
		assert.True(t, allowed)
		assert.Equal(t, 60, res.Limit)
		assert.Equal(t, 59, res.Remaining)
	})

	t.Run("Blocks request exceeding second limit", func(t *testing.T) {
		s.FlushAll()

		allowed, _, err := limiter.Allow(ctx, quotaKey, 1, 60)
		require.NoError(t, err)
		assert.True(t, allowed)

		allowed, res, err := limiter.Allow(ctx, quotaKey, 1, 60)
		require.NoError(t, err)
		assert.False(t, allowed)
		assert.Equal(t, 1, res.Limit)
		assert.Equal(t, 0, res.Remaining)
		assert.Greater(t, res.Reset, time.Duration(0))
	})

	t.Run("Blocks request exceeding minute limit", func(t *testing.T) {
		s.FlushAll()

		for i := 0; i < 2; i++ {
			allowed, _, err := limiter.Allow(ctx, quotaKey, 100, 2)
			require.NoError(t, err)
			assert.True(t, allowed)
		}

		allowed, res, err := limiter.Allow(ctx, quotaKey, 100, 2)
		require.NoError(t, err)
		assert.False(t, allowed)
		assert.Equal(t, 2, res.Limit)
		assert.Equal(t, 0, res.Remaining)
	})

	t.Run("Handles no limits", func(t *testing.T) {
		s.FlushAll()
		allowed, res, err := limiter.Allow(ctx, quotaKey, 0, 0)
		require.NoError(t, err)
		assert.True(t, allowed)
		assert.Equal(t, -1, res.Remaining)
	})

	t.Run("Only per-second limit (no minute limit)", func(t *testing.T) {
		s.FlushAll()

		for i := 0; i < 5; i++ {
			allowed, res, err := limiter.Allow(ctx, quotaKey, 5, 0)
			require.NoError(t, err)
			assert.True(t, allowed)
			assert.Equal(t, 5-(i+1), res.Remaining)
		}

		allowed, res, err := limiter.Allow(ctx, quotaKey, 5, 0)
		require.NoError(t, err)
		assert.False(t, allowed)
		assert.Equal(t, 5, res.Limit)
		assert.Equal(t, 0, res.Remaining)
		assert.Greater(t, res.Reset, time.Duration(0))
	})

	t.Run("Only per-minute limit (no second limit)", func(t *testing.T) {
		s.FlushAll()

		for i := 0; i < 3; i++ {
			allowed, res, err := limiter.Allow(ctx, quotaKey, 0, 3)
			require.NoError(t, err)
			assert.True(t, allowed)
			assert.Equal(t, 3, res.Limit)
			assert.Equal(t, 2-i, res.Remaining)
		}

		allowed, res, err := limiter.Allow(ctx, quotaKey, 0, 3)
		require.NoError(t, err)
		assert.False(t, allowed)
		assert.Equal(t, 3, res.Limit)
		assert.Equal(t, 0, res.Remaining)
		assert.Greater(t, res.Reset, time.Duration(0))
	})

	t.Run("Returns correct remaining count for minute limit", func(t *testing.T) {
		s.FlushAll()

		allowed, res, err := limiter.Allow(ctx, quotaKey, 100, 10)
		require.NoError(t, err)
		assert.True(t, allowed)
		assert.Equal(t, 10, res.Limit)
		assert.Equal(t, 9, res.Remaining)

		allowed, res, err = limiter.Allow(ctx, quotaKey, 100, 10)
		require.NoError(t, err)
		assert.True(t, allowed)
		assert.Equal(t, 8, res.Remaining)
	})

	t.Run("Handles per-second limit with high minute limit", func(t *testing.T) {
		s.FlushAll()

		allowed, res, err := limiter.Allow(ctx, quotaKey, 2, 1000)
		require.NoError(t, err)
		assert.True(t, allowed)
		assert.Equal(t, 2, res.Limit)
		assert.Equal(t, 1, res.Remaining)

		allowed, res, err = limiter.Allow(ctx, quotaKey, 2, 1000)
		require.NoError(t, err)
		assert.True(t, allowed)
		assert.Equal(t, 2, res.Limit)
		assert.Equal(t, 0, res.Remaining)

		allowed, res, err = limiter.Allow(ctx, quotaKey, 2, 1000)
		require.NoError(t, err)
		assert.False(t, allowed)
		assert.Equal(t, 2, res.Limit)
		assert.Equal(t, 0, res.Remaining)
	})
}

func TestRateLimiter_RedisErrors(t *testing.T) {
	ctx := context.Background()
	s, err := miniredis.Run()
	require.NoError(t, err)
	defer s.Close()

	client, err := redis.NewClient(ctx, config.RedisConfig{Addr: s.Addr()}, nil)
	require.NoError(t, err)
	defer client.Close()

	limiter := ratelimit.NewRateLimiter(client)
	quotaKey := "test_key"

	t.Run("Returns error on Redis failure for per-second limit", func(t *testing.T) {
		s.Close()

		allowed, res, err := limiter.Allow(ctx, quotaKey, 10, 60)
		require.Error(t, err)
		assert.False(t, allowed)
		assert.Contains(t, err.Error(), "redis error (sec)")
		assert.Equal(t, ratelimit.Result{}, res)

		s, err = miniredis.Run()
		require.NoError(t, err)
		defer s.Close()
		client, err = redis.NewClient(ctx, config.RedisConfig{Addr: s.Addr()}, nil)
		require.NoError(t, err)
		defer client.Close()
		limiter = ratelimit.NewRateLimiter(client)
	})

	t.Run("Returns error on Redis failure for per-minute limit", func(t *testing.T) {
		s.FlushAll()
		s.Close()

		allowed, res, err := limiter.Allow(ctx, quotaKey, 0, 60)
		require.Error(t, err)
		assert.False(t, allowed)
		assert.Contains(t, err.Error(), "redis error (min)")
		assert.Equal(t, ratelimit.Result{}, res)
	})
}

func TestRateLimiter_ResetCalculation(t *testing.T) {
	ctx := context.Background()
	s, err := miniredis.Run()
	require.NoError(t, err)
	defer s.Close()

	client, err := redis.NewClient(ctx, config.RedisConfig{Addr: s.Addr()}, nil)
	require.NoError(t, err)
	defer client.Close()

	limiter := ratelimit.NewRateLimiter(client)
	quotaKey := "test_key"

	t.Run("Reset duration is positive when limit exceeded", func(t *testing.T) {
		s.FlushAll()

		allowed, _, err := limiter.Allow(ctx, quotaKey, 1, 60)
		require.NoError(t, err)
		assert.True(t, allowed)

		allowed, res, err := limiter.Allow(ctx, quotaKey, 1, 60)
		require.NoError(t, err)
		assert.False(t, allowed)
		assert.Greater(t, res.Reset, time.Duration(0))
		assert.LessOrEqual(t, res.Reset, time.Second)
	})

	t.Run("Reset duration for minute limit is less than or equal to one minute", func(t *testing.T) {
		s.FlushAll()

		for i := 0; i < 2; i++ {
			allowed, _, err := limiter.Allow(ctx, quotaKey, 100, 2)
			require.NoError(t, err)
			assert.True(t, allowed)
		}

		allowed, res, err := limiter.Allow(ctx, quotaKey, 100, 2)
		require.NoError(t, err)
		assert.False(t, allowed)
		assert.Greater(t, res.Reset, time.Duration(0))
		assert.LessOrEqual(t, res.Reset, time.Minute)
	})
}

func TestNewRateLimiter(t *testing.T) {
	t.Run("Creates non-nil RateLimiter", func(t *testing.T) {
		ctx := context.Background()
		s, err := miniredis.Run()
		require.NoError(t, err)
		defer s.Close()

		client, err := redis.NewClient(ctx, config.RedisConfig{Addr: s.Addr()}, nil)
		require.NoError(t, err)
		defer client.Close()

		limiter := ratelimit.NewRateLimiter(client)
		assert.NotNil(t, limiter)
	})

	t.Run("Creates RateLimiter with nil Redis client", func(t *testing.T) {
		limiter := ratelimit.NewRateLimiter(nil)
		assert.NotNil(t, limiter)
	})
}
