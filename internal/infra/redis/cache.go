package redis

import (
	"context"
	"encoding/json"
	"errors"
	"time"

	"github.com/redis/go-redis/v9"
)

var ErrCacheMiss = errors.New("cache miss")

func (c *Client) Get(ctx context.Context, key string, v interface{}) error {
	val, err := c.Client.Get(ctx, key).Bytes()
	if err != nil {
		if errors.Is(err, redis.Nil) {
			return ErrCacheMiss
		}

		return err
	}

	if err := json.Unmarshal(val, v); err != nil {
		return err
	}

	return nil
}

func (c *Client) Set(ctx context.Context, key string, v interface{}, expiration time.Duration) error {
	val, err := json.Marshal(v)
	if err != nil {
		return err
	}

	return c.Client.Set(ctx, key, val, expiration).Err()
}

func (c *Client) Delete(ctx context.Context, key string) error {
	return c.Client.Del(ctx, key).Err()
}

func (c *Client) Increment(ctx context.Context, key string) (int64, error) {
	return c.Client.Incr(ctx, key).Result()
}

func (c *Client) Expire(ctx context.Context, key string, expiration time.Duration) (bool, error) {
	return c.Client.Expire(ctx, key, expiration).Result()
}
