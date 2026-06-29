// Package ratelimit provides sliding-window rate limiting backed by Redis.
package ratelimit

import (
	"context"
	"fmt"
	"time"

	"github.com/beremaran/straw/internal/infra/redis"
)

const (
	secondsPerMinute = 60
	keyTTL           = 2
)

// Result describes the outcome of a rate limit check.
type Result struct {
	Allowed   bool
	Remaining int
	Limit     int
	Reset     time.Duration
}

// RateLimiter enforces per-second and per-minute rate limits using Redis.
type RateLimiter struct {
	redis *redis.Client
}

// NewRateLimiter creates a new RateLimiter using the given Redis client.
func NewRateLimiter(redis *redis.Client) *RateLimiter {
	return &RateLimiter{
		redis: redis,
	}
}

// Allow checks whether the request is within both the per-second and
// per-minute rate limits for the given quotaKey.
func (l *RateLimiter) Allow(ctx context.Context, quotaKey string, limitPerSecond, limitPerMinute int) (bool, Result, error) {
	now := time.Now()

	secResult, blocked, err := l.checkOptionalSecondLimit(ctx, quotaKey, limitPerSecond, now)
	if err != nil || blocked {
		return false, secResult, err
	}

	minResult, blocked, err := l.checkOptionalMinuteLimit(ctx, quotaKey, limitPerMinute, now)
	if err != nil || blocked {
		return false, minResult, err
	}

	return true, allowedResult(limitPerSecond, limitPerMinute, secResult, minResult), nil
}

func (l *RateLimiter) checkOptionalSecondLimit(
	ctx context.Context,
	quotaKey string,
	limitPerSecond int,
	now time.Time,
) (Result, bool, error) {
	if limitPerSecond <= 0 {
		return Result{}, false, nil
	}

	allowed, res, err := l.checkSecondLimit(ctx, quotaKey, limitPerSecond, now)

	return res, !allowed, err
}

func (l *RateLimiter) checkOptionalMinuteLimit(
	ctx context.Context,
	quotaKey string,
	limitPerMinute int,
	now time.Time,
) (Result, bool, error) {
	if limitPerMinute <= 0 {
		return Result{}, false, nil
	}

	allowed, res, err := l.checkMinuteLimit(ctx, quotaKey, limitPerMinute, now)

	return res, !allowed, err
}

func allowedResult(limitPerSecond, limitPerMinute int, secResult, minResult Result) Result {
	if limitPerSecond > 0 && limitPerMinute > 0 {
		if limitPerSecond*60 < limitPerMinute {
			return secResult
		}

		return minResult
	}

	if limitPerSecond > 0 {
		return secResult
	}

	if limitPerMinute > 0 {
		return minResult
	}

	return Result{Allowed: true, Remaining: -1}
}

func (l *RateLimiter) checkSecondLimit(ctx context.Context, quotaKey string, limitPerSecond int, now time.Time) (bool, Result, error) {
	secKey := fmt.Sprintf("quota:%s:sec:%d", quotaKey, now.Unix())

	pipe := l.redis.Client.Pipeline()
	incr := pipe.Incr(ctx, secKey)
	pipe.Expire(ctx, secKey, time.Duration(keyTTL)*time.Second)

	_, err := pipe.Exec(ctx)
	if err != nil {
		return false, Result{}, fmt.Errorf("redis error (sec): %w", err)
	}

	count := incr.Val()

	remaining := max(int64(limitPerSecond)-count, 0)

	if count > int64(limitPerSecond) {
		reset := max(time.Until(now.Truncate(time.Second).Add(time.Second)), 0)

		return false, Result{
			Allowed:   false,
			Limit:     limitPerSecond,
			Remaining: 0,
			Reset:     reset,
		}, nil
	}

	return true, Result{
		Allowed:   true,
		Limit:     limitPerSecond,
		Remaining: int(remaining),
		Reset:     time.Until(now.Truncate(time.Second).Add(time.Second)),
	}, nil
}

func (l *RateLimiter) checkMinuteLimit(ctx context.Context, quotaKey string, limitPerMinute int, now time.Time) (bool, Result, error) {
	minKey := fmt.Sprintf("quota:%s:min:%d", quotaKey, now.Unix()/secondsPerMinute)

	pipe := l.redis.Client.Pipeline()
	incr := pipe.Incr(ctx, minKey)
	pipe.Expire(ctx, minKey, time.Duration(keyTTL)*time.Minute)

	_, err := pipe.Exec(ctx)
	if err != nil {
		return false, Result{}, fmt.Errorf("redis error (min): %w", err)
	}

	count := incr.Val()

	remaining := max(int64(limitPerMinute)-count, 0)

	if count > int64(limitPerMinute) {
		reset := max(time.Until(now.Truncate(time.Minute).Add(time.Minute)), 0)

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
