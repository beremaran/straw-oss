package ratelimit

import (
	"context"
	"fmt"
	"time"

	"github.com/beremaran/straw/internal/infra/redis"
)

type Result struct {
	Allowed   bool
	Remaining int
	Limit     int
	Reset     time.Duration
}

type RateLimiter struct {
	redis *redis.Client
}

func NewRateLimiter(redis *redis.Client) *RateLimiter {
	return &RateLimiter{
		redis: redis,
	}
}

func (l *RateLimiter) Allow(ctx context.Context, quotaKey string, limitPerSecond, limitPerMinute int) (bool, Result, error) {
	now := time.Now()
	var secResult Result
	secResult.Allowed = true

	if limitPerSecond > 0 {
		secKey := fmt.Sprintf("quota:%s:sec:%d", quotaKey, now.Unix())

		pipe := l.redis.Client.Pipeline()
		incr := pipe.Incr(ctx, secKey)
		pipe.Expire(ctx, secKey, 2*time.Second)
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

	if limitPerMinute > 0 {
		minKey := fmt.Sprintf("quota:%s:min:%d", quotaKey, now.Unix()/60)

		pipe := l.redis.Client.Pipeline()
		incr := pipe.Incr(ctx, minKey)
		pipe.Expire(ctx, minKey, 2*time.Minute)
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

		return true, Result{
			Allowed:   true,
			Limit:     limitPerMinute,
			Remaining: int(remaining),
			Reset:     time.Until(now.Truncate(time.Minute).Add(time.Minute)),
		}, nil
	}

	if limitPerSecond > 0 {
		return true, secResult, nil
	}

	return true, Result{Allowed: true, Remaining: -1}, nil
}
