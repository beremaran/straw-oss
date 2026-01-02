package ratelimit

import (
	"context"
	"fmt"
	"time"

	"github.com/kwilabs/straw-proxy-server/internal/infra/redis"
)

// Result represents the result of a rate limit check.
type Result struct {
	Allowed   bool
	Remaining int
	Limit     int
	Reset     time.Duration // Duration until the limit resets
}

// RateLimiter handles rate limiting logic using Redis.
type RateLimiter struct {
	redis *redis.Client
}

// NewRateLimiter creates a new RateLimiter.
func NewRateLimiter(redis *redis.Client) *RateLimiter {
	return &RateLimiter{
		redis: redis,
	}
}

// Allow checks if the request is allowed based on the provided limits.
// It implements a dual-bucket sliding window (per-second and per-minute).
func (l *RateLimiter) Allow(ctx context.Context, quotaKey string, limitPerSecond, limitPerMinute int) (bool, Result, error) {
	now := time.Now()
	var secResult Result
	secResult.Allowed = true

	// Check per-second limit (if configured)
	if limitPerSecond > 0 {
		secKey := fmt.Sprintf("quota:%s:sec:%d", quotaKey, now.Unix())

		// Use a pipeline to ensure atomicity and reduce RTT
		pipe := l.redis.Client.Pipeline()
		incr := pipe.Incr(ctx, secKey)
		pipe.Expire(ctx, secKey, 2*time.Second) // 2s TTL for margin
		_, err := pipe.Exec(ctx)
		if err != nil {
			return false, Result{}, fmt.Errorf("redis error (sec): %w", err)
		}

		count := incr.Val()
		remaining := int64(limitPerSecond) - count
		if remaining < 0 {
			remaining = 0
		}

		if count > int64(limitPerSecond) {
			reset := time.Until(now.Truncate(time.Second).Add(time.Second))
			if reset < 0 {
				reset = 0
			}
			return false, Result{
				Allowed:   false,
				Limit:     limitPerSecond,
				Remaining: 0,
				Reset:     reset,
			}, nil
		}

		secResult.Limit = limitPerSecond
		secResult.Remaining = int(remaining)
		secResult.Reset = time.Until(now.Truncate(time.Second).Add(time.Second))
	}

	// Check per-minute limit (if configured)
	if limitPerMinute > 0 {
		minKey := fmt.Sprintf("quota:%s:min:%d", quotaKey, now.Unix()/60)

		pipe := l.redis.Client.Pipeline()
		incr := pipe.Incr(ctx, minKey)
		pipe.Expire(ctx, minKey, 2*time.Minute) // 2m TTL for margin
		_, err := pipe.Exec(ctx)
		if err != nil {
			return false, Result{}, fmt.Errorf("redis error (min): %w", err)
		}

		count := incr.Val()
		remaining := int64(limitPerMinute) - count
		if remaining < 0 {
			remaining = 0
		}

		if count > int64(limitPerMinute) {
			reset := time.Until(now.Truncate(time.Minute).Add(time.Minute))
			if reset < 0 {
				reset = 0
			}
			return false, Result{
				Allowed:   false,
				Limit:     limitPerMinute,
				Remaining: 0,
				Reset:     reset,
			}, nil
		}

		// If passed, return based on the longer window (minute) as the primary "Remaining" info
		// defaulting Reset to start of next minute.
		return true, Result{
			Allowed:   true,
			Limit:     limitPerMinute,
			Remaining: int(remaining),
			Reset:     time.Until(now.Truncate(time.Minute).Add(time.Minute)),
		}, nil
	}

	// If no limits or only second limit passed (and no minute limit)
	if limitPerSecond > 0 {
		return true, secResult, nil
	}

	return true, Result{Allowed: true, Remaining: -1}, nil
}
