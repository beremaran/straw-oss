// Package integration provides test helpers for spinning up integration
// test containers (Postgres, Redis, NATS).
package integration

import (
	"context"
	"fmt"
	"time"

	"github.com/testcontainers/testcontainers-go"
	"github.com/testcontainers/testcontainers-go/modules/nats"
	"github.com/testcontainers/testcontainers-go/modules/postgres"
	"github.com/testcontainers/testcontainers-go/modules/redis"
	"github.com/testcontainers/testcontainers-go/wait"

	pginfra "github.com/beremaran/straw/internal/infra/postgres"
)

const (
	postgresLogOccurrence = 2
	postgresStartup       = 60 * time.Second
	redisStartup          = 30 * time.Second
)

// PostgresContainer wraps a running Postgres testcontainer and exposes its
// DSN for application tests.
type PostgresContainer struct {
	container *postgres.PostgresContainer
	dsn       string
}

// NewPostgresContainer starts a Postgres 17 container and returns it
// ready for use.
func NewPostgresContainer(ctx context.Context) (*PostgresContainer, error) {
	container, err := postgres.Run(ctx,
		"postgres:17-alpine",
		postgres.WithDatabase("straw_test"),
		postgres.WithUsername("straw"),
		postgres.WithPassword("straw"),
		testcontainers.WithWaitStrategy(
			wait.ForLog("database system is ready to accept connections").
				WithOccurrence(postgresLogOccurrence).
				WithStartupTimeout(postgresStartup),
		),
	)
	if err != nil {
		return nil, fmt.Errorf("failed to start postgres container: %w", err)
	}

	dsn, err := container.ConnectionString(ctx, "sslmode=disable")
	if err != nil {
		_ = container.Terminate(ctx)

		return nil, fmt.Errorf("failed to get postgres connection string: %w", err)
	}

	return &PostgresContainer{
		container: container,
		dsn:       dsn,
	}, nil
}

// DSN returns the Postgres connection string for the running container.
func (c *PostgresContainer) DSN() string {
	return c.dsn
}

// RunMigrations applies the embedded migrations to the container.
func (c *PostgresContainer) RunMigrations(ctx context.Context) error {
	err := pginfra.RunEmbeddedMigrations(ctx, c.dsn)
	if err != nil {
		return fmt.Errorf("run migrations: %w", err)
	}

	return nil
}

// Terminate stops and removes the container.
func (c *PostgresContainer) Terminate(ctx context.Context) error {
	if c.container != nil {
		err := c.container.Terminate(ctx)
		if err != nil {
			return fmt.Errorf("terminate postgres container: %w", err)
		}
	}

	return nil
}

// RedisContainer wraps a running Redis testcontainer and exposes its
// address for application tests.
type RedisContainer struct {
	container *redis.RedisContainer
	addr      string
}

// NewRedisContainer starts a Redis 7 container and returns it ready for use.
func NewRedisContainer(ctx context.Context) (*RedisContainer, error) {
	container, err := redis.Run(ctx,
		"redis:7-alpine",
		testcontainers.WithWaitStrategy(
			wait.ForLog("Ready to accept connections").
				WithStartupTimeout(redisStartup),
		),
	)
	if err != nil {
		return nil, fmt.Errorf("failed to start redis container: %w", err)
	}

	host, err := container.Host(ctx)
	if err != nil {
		_ = container.Terminate(ctx)

		return nil, fmt.Errorf("failed to get redis host: %w", err)
	}

	port, err := container.MappedPort(ctx, "6379")
	if err != nil {
		_ = container.Terminate(ctx)

		return nil, fmt.Errorf("failed to get redis port: %w", err)
	}

	return &RedisContainer{
		container: container,
		addr:      fmt.Sprintf("%s:%s", host, port.Port()),
	}, nil
}

// Addr returns the Redis address for the running container.
func (c *RedisContainer) Addr() string {
	return c.addr
}

// Terminate stops and removes the container.
func (c *RedisContainer) Terminate(ctx context.Context) error {
	if c.container != nil {
		err := c.container.Terminate(ctx)
		if err != nil {
			return fmt.Errorf("terminate redis container: %w", err)
		}
	}

	return nil
}

// NatsContainer wraps a running NATS testcontainer and exposes its
// URL for application tests.
type NatsContainer struct {
	container *nats.NATSContainer
	url       string
}

// NewNatsContainer starts a NATS container with JetStream enabled and
// returns it ready for use.
func NewNatsContainer(ctx context.Context) (*NatsContainer, error) {
	container, err := nats.Run(ctx,
		"nats:latest",
		testcontainers.WithCmd("-js"),
	)
	if err != nil {
		return nil, fmt.Errorf("failed to start nats container: %w", err)
	}

	url, err := container.ConnectionString(ctx)
	if err != nil {
		_ = container.Terminate(ctx)

		return nil, fmt.Errorf("failed to get nats connection string: %w", err)
	}

	return &NatsContainer{
		container: container,
		url:       url,
	}, nil
}

// URL returns the NATS connection URL for the running container.
func (c *NatsContainer) URL() string {
	return c.url
}

// Terminate stops and removes the container.
func (c *NatsContainer) Terminate(ctx context.Context) error {
	if c.container != nil {
		err := c.container.Terminate(ctx)
		if err != nil {
			return fmt.Errorf("terminate nats container: %w", err)
		}
	}

	return nil
}
