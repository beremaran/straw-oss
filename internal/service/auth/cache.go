package auth

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/beremaran/straw/internal/domain"
	"github.com/beremaran/straw/internal/infra/redis"
)

// Cache provides a Redis-backed cache for API keys.
type Cache struct {
	client *redis.Client
	ttl    time.Duration
}

// NewAuthCache creates a new Cache with the given Redis client and TTL.
func NewAuthCache(client *redis.Client, ttl time.Duration) *Cache {
	return &Cache{
		client: client,
		ttl:    ttl,
	}
}

// GetKey retrieves a cached API key by its hash.
func (c *Cache) GetKey(ctx context.Context, keyHash string) (*domain.APIKey, error) {
	key := "auth:valid:" + keyHash

	var apiKey domain.APIKey

	err := c.client.Get(ctx, key, &apiKey)
	if err != nil {
		if errors.Is(err, redis.ErrCacheMiss) {
			return nil, nil
		}

		return nil, fmt.Errorf("failed to get cache entry: %w", err)
	}

	return &apiKey, nil
}

// SetKey stores an API key in the cache.
func (c *Cache) SetKey(ctx context.Context, keyHash string, apiKey *domain.APIKey) error {
	key := "auth:valid:" + keyHash

	err := c.client.Set(ctx, key, apiKey, c.ttl)
	if err != nil {
		return fmt.Errorf("failed to set cache entry: %w", err)
	}

	return nil
}

// InvalidateKey removes a cached API key by its hash.
func (c *Cache) InvalidateKey(ctx context.Context, keyHash string) error {
	key := "auth:valid:" + keyHash

	err := c.client.Delete(ctx, key)
	if err != nil {
		return fmt.Errorf("failed to delete cache entry: %w", err)
	}

	return nil
}
