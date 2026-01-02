// Package integration provides testcontainer-based infrastructure for integration testing.
package integration

import (
	"context"
	"fmt"
	"time"

	"github.com/testcontainers/testcontainers-go"
	"github.com/testcontainers/testcontainers-go/modules/postgres"
	"github.com/testcontainers/testcontainers-go/modules/rabbitmq"
	"github.com/testcontainers/testcontainers-go/modules/redis"
	"github.com/testcontainers/testcontainers-go/wait"

	pginfra "github.com/kwilabs/straw-proxy-server/internal/infra/postgres"
)

// PostgresContainer wraps a PostgreSQL testcontainer.
type PostgresContainer struct {
	container *postgres.PostgresContainer
	dsn       string
}

// NewPostgresContainer creates and starts a new PostgreSQL testcontainer.
func NewPostgresContainer(ctx context.Context) (*PostgresContainer, error) {
	container, err := postgres.Run(ctx,
		"postgres:17-alpine",
		postgres.WithDatabase("straw_test"),
		postgres.WithUsername("straw"),
		postgres.WithPassword("straw"),
		testcontainers.WithWaitStrategy(
			wait.ForLog("database system is ready to accept connections").
				WithOccurrence(2).
				WithStartupTimeout(60*time.Second),
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

// DSN returns the PostgreSQL connection string.
func (c *PostgresContainer) DSN() string {
	return c.dsn
}

// RunMigrations runs database migrations using the project's embedded migrations.
func (c *PostgresContainer) RunMigrations() error {
	return pginfra.RunEmbeddedMigrations(c.dsn)
}

// Terminate stops and removes the container.
func (c *PostgresContainer) Terminate(ctx context.Context) error {
	if c.container != nil {
		return c.container.Terminate(ctx)
	}
	return nil
}

// RedisContainer wraps a Redis testcontainer.
type RedisContainer struct {
	container *redis.RedisContainer
	addr      string
}

// NewRedisContainer creates and starts a new Redis testcontainer.
func NewRedisContainer(ctx context.Context) (*RedisContainer, error) {
	container, err := redis.Run(ctx,
		"redis:7-alpine",
		testcontainers.WithWaitStrategy(
			wait.ForLog("Ready to accept connections").
				WithStartupTimeout(30*time.Second),
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

// Addr returns the Redis address (host:port).
func (c *RedisContainer) Addr() string {
	return c.addr
}

// Terminate stops and removes the container.
func (c *RedisContainer) Terminate(ctx context.Context) error {
	if c.container != nil {
		return c.container.Terminate(ctx)
	}
	return nil
}

// RabbitMQContainer wraps a RabbitMQ testcontainer.
type RabbitMQContainer struct {
	container *rabbitmq.RabbitMQContainer
	url       string
}

// NewRabbitMQContainer creates and starts a new RabbitMQ testcontainer.
func NewRabbitMQContainer(ctx context.Context) (*RabbitMQContainer, error) {
	container, err := rabbitmq.Run(ctx,
		"rabbitmq:4-alpine",
		testcontainers.WithWaitStrategy(
			wait.ForLog("started TCP listener").
				WithStartupTimeout(60*time.Second),
		),
	)
	if err != nil {
		return nil, fmt.Errorf("failed to start rabbitmq container: %w", err)
	}

	url, err := container.AmqpURL(ctx)
	if err != nil {
		_ = container.Terminate(ctx)
		return nil, fmt.Errorf("failed to get rabbitmq url: %w", err)
	}

	return &RabbitMQContainer{
		container: container,
		url:       url,
	}, nil
}

// URL returns the RabbitMQ AMQP URL.
func (c *RabbitMQContainer) URL() string {
	return c.url
}

// Terminate stops and removes the container.
func (c *RabbitMQContainer) Terminate(ctx context.Context) error {
	if c.container != nil {
		return c.container.Terminate(ctx)
	}
	return nil
}
