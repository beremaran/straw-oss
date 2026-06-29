package postgres

import (
	"context"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/beremaran/straw/internal/infra/circuitbreaker"
)

// Client is a PostgreSQL connection pool with optional circuit breaker support.
type Client struct {
	Pool    *pgxpool.Pool
	breaker *circuitbreaker.CircuitBreaker
}

// NewClient creates a new PostgreSQL client with the given DSN and optional circuit breaker.
func NewClient(ctx context.Context, dsn string, breaker *circuitbreaker.CircuitBreaker) (*Client, error) {
	config, err := pgxpool.ParseConfig(dsn)
	if err != nil {
		return nil, fmt.Errorf("failed to parse postgres dsn: %w", err)
	}

	if config.MaxConns == 0 {
		config.MaxConns = 25
	}

	if config.MinConns == 0 {
		config.MinConns = 2
	}

	const defaultMaxConnIdleTime = 30 * time.Minute

	if config.MaxConnLifetime == 0 {
		config.MaxConnLifetime = 1 * time.Hour
	}

	if config.MaxConnIdleTime == 0 {
		config.MaxConnIdleTime = defaultMaxConnIdleTime
	}

	pool, err := pgxpool.NewWithConfig(ctx, config)
	if err != nil {
		return nil, fmt.Errorf("failed to create postgres pool: %w", err)
	}

	err = pool.Ping(ctx)
	if err != nil {
		pool.Close()

		return nil, fmt.Errorf("failed to connect to postgres: %w", err)
	}

	return &Client{Pool: pool, breaker: breaker}, nil
}

// Close releases all connections in the pool.
func (c *Client) Close() {
	c.Pool.Close()
}

// HealthCheck verifies the database is reachable.
func (c *Client) HealthCheck(ctx context.Context) error {
	err := c.Pool.Ping(ctx)
	if err != nil {
		return fmt.Errorf("health check failed: %w", err)
	}

	return nil
}

// Execute runs the given function, optionally routing it through a circuit breaker.
func (c *Client) Execute(fn func() error) error {
	if c.breaker != nil {
		err := c.breaker.Execute(fn)
		if err != nil {
			return fmt.Errorf("execute via circuit breaker: %w", err)
		}

		return nil
	}

	return fn()
}
