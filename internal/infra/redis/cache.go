// Package redis provides a Redis client with caching and endpoint health storage.
package redis

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/redis/go-redis/v9"
)

// ErrCacheMiss is returned when a key is not found in the cache.
var ErrCacheMiss = errors.New("cache miss")

// Get retrieves a value from the cache, unmarshals it into v, and returns any error.
func (c *Client) Get(ctx context.Context, key string, v any) error {
	val, err := c.Client.Get(ctx, key).Bytes()
	if err != nil {
		if errors.Is(err, redis.Nil) {
			return ErrCacheMiss
		}

		return fmt.Errorf("redis get: %w", err)
	}

	err = json.Unmarshal(val, v)
	if err != nil {
		return fmt.Errorf("unmarshal cache value: %w", err)
	}

	return nil
}

// Set stores a value in the cache, marshaling v to JSON, with the given expiration.
func (c *Client) Set(ctx context.Context, key string, v any, expiration time.Duration) error {
	val, err := json.Marshal(v)
	if err != nil {
		return fmt.Errorf("marshal cache value: %w", err)
	}

	err = c.Client.Set(ctx, key, val, expiration).Err()
	if err != nil {
		return fmt.Errorf("redis set: %w", err)
	}

	return nil
}

// Delete removes a key from the cache.
func (c *Client) Delete(ctx context.Context, key string) error {
	err := c.Client.Del(ctx, key).Err()
	if err != nil {
		return fmt.Errorf("redis delete: %w", err)
	}

	return nil
}

// Increment atomically increments the integer value stored at key by one.
func (c *Client) Increment(ctx context.Context, key string) (int64, error) {
	result, err := c.Client.Incr(ctx, key).Result()
	if err != nil {
		return 0, fmt.Errorf("redis incr: %w", err)
	}

	return result, nil
}

// Expire sets the TTL for a key in the cache.
func (c *Client) Expire(ctx context.Context, key string, expiration time.Duration) (bool, error) {
	result, err := c.Client.Expire(ctx, key, expiration).Result()
	if err != nil {
		return false, fmt.Errorf("redis expire: %w", err)
	}

	return result, nil
}
