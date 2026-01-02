package auth

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/kwilabs/straw-proxy-server/internal/domain"
	"github.com/kwilabs/straw-proxy-server/internal/infra/redis"
)

// AuthCache handles caching of API keys.
type AuthCache struct {
	client *redis.Client
	ttl    time.Duration
}

// NewAuthCache creates a new AuthCache.
func NewAuthCache(client *redis.Client, ttl time.Duration) *AuthCache {
	return &AuthCache{
		client: client,
		ttl:    ttl,
	}
}

// GetKey retrieves a cached API key by the hash of the raw key.
func (c *AuthCache) GetKey(ctx context.Context, keyHash string) (*domain.ApiKey, error) {
	key := fmt.Sprintf("auth:valid:%s", keyHash)
	var apiKey domain.ApiKey

	err := c.client.Get(ctx, key, &apiKey)
	if err != nil {
		if errors.Is(err, redis.ErrCacheMiss) {
			return nil, nil // Cache miss
		}
		return nil, err
	}

	return &apiKey, nil
}

// SetKey caches a validated API key using the hash of the raw key.
func (c *AuthCache) SetKey(ctx context.Context, keyHash string, apiKey *domain.ApiKey) error {
	key := fmt.Sprintf("auth:valid:%s", keyHash)
	return c.client.Set(ctx, key, apiKey, c.ttl)
}

// InvalidateKey removes a cached API key by the hash of the raw key.
// This is called when a key is revoked or its status changes.
func (c *AuthCache) InvalidateKey(ctx context.Context, keyHash string) error {
	key := fmt.Sprintf("auth:valid:%s", keyHash)
	return c.client.Delete(ctx, key)
}
