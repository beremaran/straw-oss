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

func SetupSuite(ctx context.Context) (*TestSuite, error) {
	s := &TestSuite{}

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

	if pgErr != nil || redisErr != nil || natsErr != nil {
		s.Teardown(ctx)

		return nil, fmt.Errorf("failed to start containers: postgres=%w, redis=%w, nats=%w", pgErr, redisErr, natsErr)
	}

	err := s.Postgres.RunMigrations(ctx)
	if err != nil {
		s.Teardown(ctx)

		return nil, fmt.Errorf("failed to run migrations: %w", err)
	}

	log.Printf("Test suite initialized successfully")
	log.Printf("  PostgreSQL: %s", s.Postgres.DSN())
	log.Printf("  Redis: %s", s.Redis.Addr())
	log.Printf("  NATS: %s", s.Nats.URL())

	return s, nil
}

func (s *TestSuite) Teardown(ctx context.Context) {
	s.mu.Lock()
	defer s.mu.Unlock()

	if s.Postgres != nil {
		err := s.Postgres.Terminate(ctx)
		if err != nil {
			log.Printf("Warning: failed to terminate postgres: %v", err)
		}
	}

	if s.Redis != nil {
		err := s.Redis.Terminate(ctx)
		if err != nil {
			log.Printf("Warning: failed to terminate redis: %v", err)
		}
	}

	if s.Nats != nil {
		err := s.Nats.Terminate(ctx)
		if err != nil {
			log.Printf("Warning: failed to terminate nats: %v", err)
		}
	}

	log.Println("Test suite cleaned up")
}

func (s *TestSuite) CleanupForTest(t *testing.T) {
	t.Helper()

	ctx := context.Background()
	err := CleanDatabase(ctx, s.Postgres.DSN())
	if err != nil {
		t.Logf("Warning: failed to clean database: %v", err)
	}

	t.Cleanup(func() {
		err := CleanDatabase(ctx, s.Postgres.DSN())
		if err != nil {
			t.Logf("Warning: failed to clean database after test: %v", err)
		}
	})
}

func (s *TestSuite) PostgresDSN() string {
	if s.Postgres == nil {
		return ""
	}

	return s.Postgres.DSN()
}

func (s *TestSuite) RedisAddr() string {
	if s.Redis == nil {
		return ""
	}

	return s.Redis.Addr()
}

func (s *TestSuite) NatsURL() string {
	if s.Nats == nil {
		return ""
	}

	return s.Nats.URL()
}

func RunTestMain(m *testing.M) {
	flag.Parse()

	if testing.Short() {
		log.Println("Skipping integration tests in short mode")
		os.Exit(0)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
	defer cancel()

	suiteOnce.Do(func() {
		var err error
		suite, err = SetupSuite(ctx)
		if err != nil {
			log.Fatalf("Failed to setup test suite: %v", err)
		}
	})

	code := m.Run()

	teardownCtx, teardownCancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer teardownCancel()
	suite.Teardown(teardownCtx)

	os.Exit(code)
}
