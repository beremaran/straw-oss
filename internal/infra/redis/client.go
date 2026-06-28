package redis

import (
	"context"
	"fmt"
	"time"

	"github.com/beremaran/straw/internal/config"
	"github.com/beremaran/straw/internal/infra/circuitbreaker"
	"github.com/redis/go-redis/v9"
)

type Client struct {
	Client  *redis.Client
	breaker *circuitbreaker.CircuitBreaker
}

func NewClient(ctx context.Context, cfg config.RedisConfig, breaker *circuitbreaker.CircuitBreaker) (*Client, error) {
	opts := &redis.Options{
		Addr:         cfg.Addr,
		Password:     "",
		DB:           0,
		PoolSize:     cfg.PoolSize,
		MinIdleConns: cfg.MinIdleConns,
	}

	client := redis.NewClient(opts)

	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()

	err := client.Ping(ctx).Err()
	if err != nil {
		return nil, fmt.Errorf("failed to connect to redis: %w", err)
	}

	return &Client{Client: client, breaker: breaker}, nil
}

func (c *Client) Close() error {
	return c.Client.Close()
}

func (c *Client) HealthCheck(ctx context.Context) error {
	return c.Client.Ping(ctx).Err()
}
