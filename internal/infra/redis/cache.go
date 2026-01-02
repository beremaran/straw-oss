package redis

import (
	"context"
	"encoding/json"
	"errors"
	"time"

	"github.com/redis/go-redis/v9"
)

// ErrCacheMiss is returned when a key is not found in the cache.
var ErrCacheMiss = errors.New("cache miss")

// Get retrieves a value from the cache and unmarshals it into the provided v interface.
func (c *Client) Get(ctx context.Context, key string, v interface{}) error {
	val, err := c.Client.Get(ctx, key).Bytes()
	if err != nil {
		if err == redis.Nil {
			return ErrCacheMiss
		}
		return err
	}

	if err := json.Unmarshal(val, v); err != nil {
		return err
	}

	return nil
}

// Set stores a value in the cache with the given expiration.
func (c *Client) Set(ctx context.Context, key string, v interface{}, expiration time.Duration) error {
	val, err := json.Marshal(v)
	if err != nil {
		return err
	}

	return c.Client.Set(ctx, key, val, expiration).Err()
}

// Delete removes a key from the cache.
func (c *Client) Delete(ctx context.Context, key string) error {
	return c.Client.Del(ctx, key).Err()
}

// Increment adds 1 to the value of the key.
func (c *Client) Increment(ctx context.Context, key string) (int64, error) {
	return c.Client.Incr(ctx, key).Result()
}

// Expire sets an expiration time on a key.
func (c *Client) Expire(ctx context.Context, key string, expiration time.Duration) (bool, error) {
	return c.Client.Expire(ctx, key, expiration).Result()
}
