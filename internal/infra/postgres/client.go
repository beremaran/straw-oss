package postgres

import (
	"context"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/kwilabs/straw-proxy-server/internal/infra/circuitbreaker"
)

// Client wraps the pgxpool.Pool to provide a unified interface for database interactions.
type Client struct {
	Pool    *pgxpool.Pool
	breaker *circuitbreaker.CircuitBreaker
}

// NewClient creates a new PostgreSQL client.
func NewClient(ctx context.Context, dsn string, breaker *circuitbreaker.CircuitBreaker) (*Client, error) {
	config, err := pgxpool.ParseConfig(dsn)
	if err != nil {
		return nil, fmt.Errorf("failed to parse postgres dsn: %w", err)
	}

	// Set some reasonable defaults if not provided in DSN
	if config.MaxConns == 0 {
		config.MaxConns = 25
	}
	if config.MinConns == 0 {
		config.MinConns = 2
	}
	if config.MaxConnLifetime == 0 {
		config.MaxConnLifetime = 1 * time.Hour
	}
	if config.MaxConnIdleTime == 0 {
		config.MaxConnIdleTime = 30 * time.Minute
	}

	pool, err := pgxpool.NewWithConfig(ctx, config)
	if err != nil {
		return nil, fmt.Errorf("failed to create postgres pool: %w", err)
	}

	// Verify connection
	if err := pool.Ping(ctx); err != nil {
		pool.Close()
		return nil, fmt.Errorf("failed to connect to postgres: %w", err)
	}

	return &Client{Pool: pool, breaker: breaker}, nil
}

// Close closes the database connection pool.
func (c *Client) Close() {
	c.Pool.Close()
}

// HealthCheck checks the database connection.
func (c *Client) HealthCheck(ctx context.Context) error {
	return c.Pool.Ping(ctx)
}

// Execute runs the given function within the circuit breaker if one is configured.
func (c *Client) Execute(fn func() error) error {
	if c.breaker != nil {
		return c.breaker.Execute(fn)
	}
	return fn()
}
