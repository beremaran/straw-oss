package redis

import (
	"context"
	"fmt"
	"time"

	"github.com/kwilabs/straw-proxy-server/internal/infra/circuitbreaker"
	"github.com/redis/go-redis/v9"
)

// Client wraps the redis.Client to provide a unified interface for cache interactions.
type Client struct {
	Client  *redis.Client
	breaker *circuitbreaker.CircuitBreaker
}

// NewClient creates a new Redis client.
func NewClient(addr string, breaker *circuitbreaker.CircuitBreaker) (*Client, error) {
	opts := &redis.Options{
		Addr:     addr,
		Password: "", // no password set
		DB:       0,  // use default DB
	}

	client := redis.NewClient(opts)

	// Verify connection
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	if err := client.Ping(ctx).Err(); err != nil {
		return nil, fmt.Errorf("failed to connect to redis: %w", err)
	}

	return &Client{Client: client, breaker: breaker}, nil
}

// Execute runs the given function within the circuit breaker if one is configured.
func (c *Client) Execute(fn func() error) error {
	if c.breaker != nil {
		return c.breaker.Execute(fn)
	}
	return fn()
}

// Close closes the Redis client.
func (c *Client) Close() error {
	return c.Client.Close()
}

// HealthCheck checks the Redis connection.
func (c *Client) HealthCheck(ctx context.Context) error {
	return c.Client.Ping(ctx).Err()
}
