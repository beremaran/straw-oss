package ratelimit_test

import (
	"context"
	"testing"
	"time"

	"github.com/alicebob/miniredis/v2"
	"github.com/kwilabs/straw-proxy-server/internal/config"
	"github.com/kwilabs/straw-proxy-server/internal/infra/redis"
	"github.com/kwilabs/straw-proxy-server/internal/service/ratelimit"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestRateLimiter_Allow(t *testing.T) {
	s, err := miniredis.Run()
	require.NoError(t, err)
	defer s.Close()

	client, err := redis.NewClient(config.CoreConfig{RedisAddr: s.Addr()}, nil)
	require.NoError(t, err)
	defer client.Close()

	limiter := ratelimit.NewRateLimiter(client)
	ctx := context.Background()
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
		// Limit 1 per second
		allowed, _, err := limiter.Allow(ctx, quotaKey, 1, 60)
		require.NoError(t, err)
		assert.True(t, allowed)

		allowed, res, err := limiter.Allow(ctx, quotaKey, 1, 60)
		require.NoError(t, err)
		assert.False(t, allowed)
		assert.Equal(t, 1, res.Limit) // Reports the limit that was hit
		assert.Equal(t, 0, res.Remaining)
		assert.Greater(t, res.Reset, time.Duration(0))
	})

	t.Run("Blocks request exceeding minute limit", func(t *testing.T) {
		s.FlushAll()
		// Limit 2 per minute, high per second
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
		// Limit 5 per second, 0 per minute
		for i := 0; i < 5; i++ {
			allowed, res, err := limiter.Allow(ctx, quotaKey, 5, 0)
			require.NoError(t, err)
			assert.True(t, allowed)
			assert.Equal(t, 5-(i+1), res.Remaining)
		}

		// 6th request should be blocked
		allowed, res, err := limiter.Allow(ctx, quotaKey, 5, 0)
		require.NoError(t, err)
		assert.False(t, allowed)
		assert.Equal(t, 5, res.Limit)
		assert.Equal(t, 0, res.Remaining)
		assert.Greater(t, res.Reset, time.Duration(0))
	})

	t.Run("Only per-minute limit (no second limit)", func(t *testing.T) {
		s.FlushAll()
		// Limit 0 per second, 3 per minute
		for i := 0; i < 3; i++ {
			allowed, res, err := limiter.Allow(ctx, quotaKey, 0, 3)
			require.NoError(t, err)
			assert.True(t, allowed)
			assert.Equal(t, 3, res.Limit)
			assert.Equal(t, 2-i, res.Remaining)
		}

		// 4th request should be blocked
		allowed, res, err := limiter.Allow(ctx, quotaKey, 0, 3)
		require.NoError(t, err)
		assert.False(t, allowed)
		assert.Equal(t, 3, res.Limit)
		assert.Equal(t, 0, res.Remaining)
		assert.Greater(t, res.Reset, time.Duration(0))
	})

	t.Run("Returns correct remaining count for minute limit", func(t *testing.T) {
		s.FlushAll()
		// Test that remaining count decreases correctly
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
		// Low per-second limit, high per-minute limit
		// This tests the case where second limit is hit first
		allowed, res, err := limiter.Allow(ctx, quotaKey, 2, 1000)
		require.NoError(t, err)
		assert.True(t, allowed)

		allowed, res, err = limiter.Allow(ctx, quotaKey, 2, 1000)
		require.NoError(t, err)
		assert.True(t, allowed)

		// 3rd request should be blocked by second limit
		allowed, res, err = limiter.Allow(ctx, quotaKey, 2, 1000)
		require.NoError(t, err)
		assert.False(t, allowed)
		assert.Equal(t, 2, res.Limit) // Reports the second limit
		assert.Equal(t, 0, res.Remaining)
	})
}

func TestRateLimiter_RedisErrors(t *testing.T) {
	s, err := miniredis.Run()
	require.NoError(t, err)
	defer s.Close()

	client, err := redis.NewClient(config.CoreConfig{RedisAddr: s.Addr()}, nil)
	require.NoError(t, err)
	defer client.Close()

	limiter := ratelimit.NewRateLimiter(client)
	ctx := context.Background()
	quotaKey := "test_key"

	t.Run("Returns error on Redis failure for per-second limit", func(t *testing.T) {
		s.Close() // Close Redis to simulate failure

		allowed, res, err := limiter.Allow(ctx, quotaKey, 10, 60)
		require.Error(t, err)
		assert.False(t, allowed)
		assert.Contains(t, err.Error(), "redis error (sec)")
		assert.Equal(t, ratelimit.Result{}, res)

		// Reopen Redis for cleanup
		s, err = miniredis.Run()
		require.NoError(t, err)
		defer s.Close()
		client, err = redis.NewClient(config.CoreConfig{RedisAddr: s.Addr()}, nil)
		require.NoError(t, err)
		defer client.Close()
		limiter = ratelimit.NewRateLimiter(client)
	})

	t.Run("Returns error on Redis failure for per-minute limit", func(t *testing.T) {
		s.FlushAll()
		s.Close() // Close Redis to simulate failure

		allowed, res, err := limiter.Allow(ctx, quotaKey, 0, 60)
		require.Error(t, err)
		assert.False(t, allowed)
		assert.Contains(t, err.Error(), "redis error (min)")
		assert.Equal(t, ratelimit.Result{}, res)
	})
}

func TestRateLimiter_ResetCalculation(t *testing.T) {
	s, err := miniredis.Run()
	require.NoError(t, err)
	defer s.Close()

	client, err := redis.NewClient(config.CoreConfig{RedisAddr: s.Addr()}, nil)
	require.NoError(t, err)
	defer client.Close()

	limiter := ratelimit.NewRateLimiter(client)
	ctx := context.Background()
	quotaKey := "test_key"

	t.Run("Reset duration is positive when limit exceeded", func(t *testing.T) {
		s.FlushAll()
		// Exhaust the limit
		allowed, _, err := limiter.Allow(ctx, quotaKey, 1, 60)
		require.NoError(t, err)
		assert.True(t, allowed)

		// Next request should be blocked with positive reset time
		allowed, res, err := limiter.Allow(ctx, quotaKey, 1, 60)
		require.NoError(t, err)
		assert.False(t, allowed)
		assert.Greater(t, res.Reset, time.Duration(0))
		assert.LessOrEqual(t, res.Reset, time.Second)
	})

	t.Run("Reset duration for minute limit is less than or equal to one minute", func(t *testing.T) {
		s.FlushAll()
		// Exhaust the minute limit
		for i := 0; i < 2; i++ {
			allowed, _, err := limiter.Allow(ctx, quotaKey, 100, 2)
			require.NoError(t, err)
			assert.True(t, allowed)
		}

		// Next request should be blocked
		allowed, res, err := limiter.Allow(ctx, quotaKey, 100, 2)
		require.NoError(t, err)
		assert.False(t, allowed)
		assert.Greater(t, res.Reset, time.Duration(0))
		assert.LessOrEqual(t, res.Reset, time.Minute)
	})
}

func TestNewRateLimiter(t *testing.T) {
	t.Run("Creates non-nil RateLimiter", func(t *testing.T) {
		s, err := miniredis.Run()
		require.NoError(t, err)
		defer s.Close()

		client, err := redis.NewClient(config.CoreConfig{RedisAddr: s.Addr()}, nil)
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
