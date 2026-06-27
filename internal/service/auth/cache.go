package auth

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/beremaran/straw/internal/domain"
	"github.com/beremaran/straw/internal/infra/redis"
)

type Cache struct {
	client *redis.Client
	ttl    time.Duration
}

func NewAuthCache(client *redis.Client, ttl time.Duration) *Cache {
	return &Cache{
		client: client,
		ttl:    ttl,
	}
}

func (c *Cache) GetKey(ctx context.Context, keyHash string) (*domain.ApiKey, error) {
	key := fmt.Sprintf("auth:valid:%s", keyHash)
	var apiKey domain.ApiKey

	err := c.client.Get(ctx, key, &apiKey)
	if err != nil {
		if errors.Is(err, redis.ErrCacheMiss) {
			return nil, nil
		}
		return nil, err
	}

	return &apiKey, nil
}

func (c *Cache) SetKey(ctx context.Context, keyHash string, apiKey *domain.ApiKey) error {
	key := fmt.Sprintf("auth:valid:%s", keyHash)
	return c.client.Set(ctx, key, apiKey, c.ttl)
}

func (c *Cache) InvalidateKey(ctx context.Context, keyHash string) error {
	key := fmt.Sprintf("auth:valid:%s", keyHash)
	return c.client.Delete(ctx, key)
}
