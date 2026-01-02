package integration

import (
	"context"
	"fmt"
	"log"
	"os"
	"sync"
	"testing"
	"time"
)

// TestSuite holds the shared test infrastructure.
type TestSuite struct {
	Postgres *PostgresContainer
	Redis    *RedisContainer
	RabbitMQ *RabbitMQContainer

	mu sync.Mutex
}

var (
	suite     *TestSuite
	suiteOnce sync.Once
)

// GetSuite returns the shared test suite instance.
// It initializes the suite lazily on first call.
func GetSuite(t *testing.T) *TestSuite {
	t.Helper()

	suiteOnce.Do(func() {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
		defer cancel()

		s, err := SetupSuite(ctx)
		if err != nil {
			log.Fatalf("Failed to setup test suite: %v", err)
		}
		suite = s
	})

	if suite == nil {
		t.Fatal("Test suite not initialized")
	}

	return suite
}

// SetupSuite initializes all test containers.
func SetupSuite(ctx context.Context) (*TestSuite, error) {
	s := &TestSuite{}

	// Start containers in parallel for faster setup
	var wg sync.WaitGroup
	var pgErr, redisErr, rabbitErr error

	wg.Add(3)

	go func() {
		defer wg.Done()
		var err error
		s.Postgres, err = NewPostgresContainer(ctx)
		if err != nil {
			pgErr = fmt.Errorf("postgres: %w", err)
		}
	}()

	go func() {
		defer wg.Done()
		var err error
		s.Redis, err = NewRedisContainer(ctx)
		if err != nil {
			redisErr = fmt.Errorf("redis: %w", err)
		}
	}()

	go func() {
		defer wg.Done()
		var err error
		s.RabbitMQ, err = NewRabbitMQContainer(ctx)
		if err != nil {
			rabbitErr = fmt.Errorf("rabbitmq: %w", err)
		}
	}()

	wg.Wait()

	// Check for errors
	if pgErr != nil || redisErr != nil || rabbitErr != nil {
		// Cleanup any started containers
		s.Teardown(ctx)
		return nil, fmt.Errorf("failed to start containers: postgres=%v, redis=%v, rabbitmq=%v", pgErr, redisErr, rabbitErr)
	}

	// Run database migrations
	if err := s.Postgres.RunMigrations(); err != nil {
		s.Teardown(ctx)
		return nil, fmt.Errorf("failed to run migrations: %w", err)
	}

	log.Printf("Test suite initialized successfully")
	log.Printf("  PostgreSQL: %s", s.Postgres.DSN())
	log.Printf("  Redis: %s", s.Redis.Addr())
	log.Printf("  RabbitMQ: %s", s.RabbitMQ.URL())

	return s, nil
}

// Teardown stops and removes all containers.
func (s *TestSuite) Teardown(ctx context.Context) {
	s.mu.Lock()
	defer s.mu.Unlock()

	if s.Postgres != nil {
		if err := s.Postgres.Terminate(ctx); err != nil {
			log.Printf("Warning: failed to terminate postgres: %v", err)
		}
	}

	if s.Redis != nil {
		if err := s.Redis.Terminate(ctx); err != nil {
			log.Printf("Warning: failed to terminate redis: %v", err)
		}
	}

	if s.RabbitMQ != nil {
		if err := s.RabbitMQ.Terminate(ctx); err != nil {
			log.Printf("Warning: failed to terminate rabbitmq: %v", err)
		}
	}

	log.Println("Test suite cleaned up")
}

// CleanupForTest registers a cleanup function for per-test isolation.
// It truncates all tables to ensure each test starts with a clean state.
func (s *TestSuite) CleanupForTest(t *testing.T) {
	t.Helper()

	ctx := context.Background()
	if err := CleanDatabase(ctx, s.Postgres.DSN()); err != nil {
		t.Logf("Warning: failed to clean database: %v", err)
	}

	// Register cleanup to run after the test
	t.Cleanup(func() {
		if err := CleanDatabase(ctx, s.Postgres.DSN()); err != nil {
			t.Logf("Warning: failed to clean database after test: %v", err)
		}
	})
}

// PostgresDSN returns the PostgreSQL connection string.
func (s *TestSuite) PostgresDSN() string {
	if s.Postgres == nil {
		return ""
	}
	return s.Postgres.DSN()
}

// RedisAddr returns the Redis address.
func (s *TestSuite) RedisAddr() string {
	if s.Redis == nil {
		return ""
	}
	return s.Redis.Addr()
}

// RabbitMQURL returns the RabbitMQ AMQP URL.
func (s *TestSuite) RabbitMQURL() string {
	if s.RabbitMQ == nil {
		return ""
	}
	return s.RabbitMQ.URL()
}

// TestMain provides a standard TestMain implementation for integration tests.
// Usage in your _test.go file:
//
//	func TestMain(m *testing.M) {
//	    integration.RunTestMain(m)
//	}
func RunTestMain(m *testing.M) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
	defer cancel()

	// Setup
	// Setup
	suiteOnce.Do(func() {
		var err error
		suite, err = SetupSuite(ctx)
		if err != nil {
			log.Fatalf("Failed to setup test suite: %v", err)
		}
	})

	// Run tests
	code := m.Run()

	// Teardown
	teardownCtx, teardownCancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer teardownCancel()
	suite.Teardown(teardownCtx)

	os.Exit(code)
}
