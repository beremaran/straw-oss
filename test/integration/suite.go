package integration

import (
	"context"
	"flag"
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
	Nats     *NatsContainer

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
	var pgErr, redisErr, natsErr error

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
		s.Nats, err = NewNatsContainer(ctx)
		if err != nil {
			natsErr = fmt.Errorf("nats: %w", err)
		}
	}()

	wg.Wait()

	// Check for errors
	if pgErr != nil || redisErr != nil || natsErr != nil {
		// Cleanup any started containers
		s.Teardown(ctx)
		return nil, fmt.Errorf("failed to start containers: postgres=%v, redis=%v, nats=%v", pgErr, redisErr, natsErr)
	}

	// Run database migrations
	if err := s.Postgres.RunMigrations(); err != nil {
		s.Teardown(ctx)
		return nil, fmt.Errorf("failed to run migrations: %w", err)
	}

	log.Printf("Test suite initialized successfully")
	log.Printf("  PostgreSQL: %s", s.Postgres.DSN())
	log.Printf("  Redis: %s", s.Redis.Addr())
	log.Printf("  NATS: %s", s.Nats.URL())

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

	if s.Nats != nil {
		if err := s.Nats.Terminate(ctx); err != nil {
			log.Printf("Warning: failed to terminate nats: %v", err)
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

// NatsURL returns the NATS URL.
func (s *TestSuite) NatsURL() string {
	if s.Nats == nil {
		return ""
	}
	return s.Nats.URL()
}

// TestMain provides a standard TestMain implementation for integration tests.
// Usage in your _test.go file:
//
//	func TestMain(m *testing.M) {
//	    integration.RunTestMain(m)
//	}
func RunTestMain(m *testing.M) {
	// Parse flags to check for -short
	flag.Parse()

	// Skip integration tests in short mode (unit tests only)
	if testing.Short() {
		log.Println("Skipping integration tests in short mode")
		os.Exit(0)
	}

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
