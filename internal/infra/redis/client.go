package redis

import (
	"context"
	"fmt"
	"time"

	"github.com/redis/go-redis/v9"

	"github.com/beremaran/straw/internal/config"
	"github.com/beremaran/straw/internal/infra/circuitbreaker"
)

const defaultPingTimeout = 5 * time.Second

// Client is a Redis client with circuit breaker support.
type Client struct {
	Client  *redis.Client
	breaker *circuitbreaker.CircuitBreaker
}

// NewClient creates a new Redis client and verifies connectivity with a ping.
func NewClient(ctx context.Context, cfg config.RedisConfig, breaker *circuitbreaker.CircuitBreaker) (*Client, error) {
	opts := &redis.Options{
		Addr:         cfg.Addr,
		Password:     "",
		DB:           0,
		PoolSize:     cfg.PoolSize,
		MinIdleConns: cfg.MinIdleConns,
	}

	client := redis.NewClient(opts)

	ctx, cancel := context.WithTimeout(ctx, defaultPingTimeout)
	defer cancel()

	err := client.Ping(ctx).Err()
	if err != nil {
		return nil, fmt.Errorf("failed to connect to redis: %w", err)
	}

	return &Client{Client: client, breaker: breaker}, nil
}

// Close closes the underlying Redis connection.
func (c *Client) Close() error {
	err := c.Client.Close()
	if err != nil {
		return fmt.Errorf("redis close: %w", err)
	}

	return nil
}

// HealthCheck pings the Redis server and returns any error.
func (c *Client) HealthCheck(ctx context.Context) error {
	err := c.Client.Ping(ctx).Err()
	if err != nil {
		return fmt.Errorf("redis ping: %w", err)
	}

	return nil
}
