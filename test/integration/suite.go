package integration

import (
	"context"
	"database/sql"
	"flag"
	"fmt"
	"log"
	"os"
	"sync"
	"testing"
	"time"

	_ "github.com/jackc/pgx/v5/stdlib" // register pgx driver
	"github.com/testcontainers/testcontainers-go"
	"github.com/testcontainers/testcontainers-go/modules/nats"
	tcpostgres "github.com/testcontainers/testcontainers-go/modules/postgres"
	tcredis "github.com/testcontainers/testcontainers-go/modules/redis"
	"github.com/testcontainers/testcontainers-go/wait"

	"github.com/beremaran/straw/internal/config"
	pginfra "github.com/beremaran/straw/internal/infra/postgres"
	infraredis "github.com/beremaran/straw/internal/infra/redis"
)

const (
	suiteTimeout    = 5 * time.Minute
	teardownTimeout = 30 * time.Second
	containerCount  = 3
)

// TestSuite holds integration test container fixtures and lifecycle management.
type TestSuite struct {
	postgresContainer *tcpostgres.PostgresContainer
	postgresDSN       string
	redisContainer    *tcredis.RedisContainer
	redisAddr         string
	natsContainer     *nats.NATSContainer
	natsURL           string
	mu                sync.Mutex
}

var (
	suite     *TestSuite
	suiteOnce sync.Once
)

// GetSuite returns the initialized test suite, lazily setting it up on first call.
func GetSuite(t *testing.T) *TestSuite {
	t.Helper()

	suiteOnce.Do(func() {
		ctx, cancel := context.WithTimeout(context.Background(), suiteTimeout)
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

// SetupSuite initializes all test containers and runs database migrations.
func SetupSuite(ctx context.Context) (*TestSuite, error) {
	s := &TestSuite{}

	var (
		wg                       sync.WaitGroup
		pgErr, redisErr, natsErr error
	)

	wg.Add(containerCount)

	go func() {
		defer wg.Done()

		var err error

		s.postgresContainer, s.postgresDSN, err = newPostgresContainer(ctx)
		if err != nil {
			pgErr = fmt.Errorf("postgres: %w", err)
		}
	}()

	go func() {
		defer wg.Done()

		var err error

		s.redisContainer, s.redisAddr, err = newRedisContainer(ctx)
		if err != nil {
			redisErr = fmt.Errorf("redis: %w", err)
		}
	}()

	go func() {
		defer wg.Done()

		var err error

		s.natsContainer, s.natsURL, err = newNatsContainer(ctx)
		if err != nil {
			natsErr = fmt.Errorf("nats: %w", err)
		}
	}()

	wg.Wait()

	if pgErr != nil || redisErr != nil || natsErr != nil {
		s.Teardown(ctx)

		return nil, fmt.Errorf("failed to start containers: postgres=%w, redis=%w, nats=%w", pgErr, redisErr, natsErr)
	}

	err := pginfra.RunEmbeddedMigrations(ctx, s.postgresDSN)
	if err != nil {
		s.Teardown(ctx)

		return nil, fmt.Errorf("failed to run migrations: %w", err)
	}

	logSuite(s)

	return s, nil
}

func logSuite(s *TestSuite) {
	log.Printf("Test suite initialized successfully")
	log.Printf("  PostgreSQL: %s", s.PostgresDSN())
	log.Printf("  Redis: %s", s.RedisAddr())
	log.Printf("  NATS: %s", s.NatsURL())
}

// Teardown terminates all running test containers.
func (s *TestSuite) Teardown(ctx context.Context) {
	s.mu.Lock()
	defer s.mu.Unlock()

	if s.postgresContainer != nil {
		err := s.postgresContainer.Terminate(ctx)
		if err != nil {
			log.Printf("Warning: failed to terminate postgres: %v", err)
		}
	}

	if s.redisContainer != nil {
		err := s.redisContainer.Terminate(ctx)
		if err != nil {
			log.Printf("Warning: failed to terminate redis: %v", err)
		}
	}

	if s.natsContainer != nil {
		err := s.natsContainer.Terminate(ctx)
		if err != nil {
			log.Printf("Warning: failed to terminate nats: %v", err)
		}
	}

	log.Println("Test suite cleaned up")
}

// CleanupForTest clears database and Redis state before and after each test.
func (s *TestSuite) CleanupForTest(t *testing.T) {
	t.Helper()

	ctx := context.Background()

	if dsn := s.PostgresDSN(); dsn != "" {
		err := cleanDatabase(ctx, dsn)
		if err != nil {
			t.Logf("Warning: failed to clean database: %v", err)
		}
	}

	if addr := s.RedisAddr(); addr != "" {
		err := cleanRedis(ctx, addr)
		if err != nil {
			t.Logf("Warning: failed to clean redis: %v", err)
		}
	}

	t.Cleanup(func() {
		if dsn := s.PostgresDSN(); dsn != "" {
			err := cleanDatabase(ctx, dsn)
			if err != nil {
				t.Logf("Warning: failed to clean database after test: %v", err)
			}
		}

		if addr := s.RedisAddr(); addr != "" {
			err := cleanRedis(ctx, addr)
			if err != nil {
				t.Logf("Warning: failed to clean redis after test: %v", err)
			}
		}
	})
}

// PostgresDSN returns the PostgreSQL connection string.
func (s *TestSuite) PostgresDSN() string {
	if s == nil || s.postgresDSN == "" {
		return ""
	}

	return s.postgresDSN
}

// RedisAddr returns the Redis connection address.
func (s *TestSuite) RedisAddr() string {
	if s == nil || s.redisAddr == "" {
		return ""
	}

	return s.redisAddr
}

// NatsURL returns the NATS connection URL.
func (s *TestSuite) NatsURL() string {
	if s == nil || s.natsURL == "" {
		return ""
	}

	return s.natsURL
}

func newPostgresContainer(ctx context.Context) (*tcpostgres.PostgresContainer, string, error) {
	container, err := tcpostgres.Run(ctx,
		"postgres:17-alpine",
		tcpostgres.WithDatabase("straw_test"),
		tcpostgres.WithUsername("straw"),
		tcpostgres.WithPassword("straw"),
		testcontainers.WithWaitStrategy(
			wait.ForLog("database system is ready to accept connections").
				WithOccurrence(postgresLogOccurrence).
				WithStartupTimeout(postgresStartup),
		),
	)
	if err != nil {
		return nil, "", fmt.Errorf("failed to start postgres container: %w", err)
	}

	dsn, err := container.ConnectionString(ctx, "sslmode=disable")
	if err != nil {
		_ = container.Terminate(ctx)

		return nil, "", fmt.Errorf("failed to get postgres connection string: %w", err)
	}

	return container, dsn, nil
}

func newRedisContainer(ctx context.Context) (*tcredis.RedisContainer, string, error) {
	container, err := tcredis.Run(ctx,
		"redis:7-alpine",
		testcontainers.WithWaitStrategy(
			wait.ForLog("Ready to accept connections").
				WithStartupTimeout(redisStartup),
		),
	)
	if err != nil {
		return nil, "", fmt.Errorf("failed to start redis container: %w", err)
	}

	host, err := container.Host(ctx)
	if err != nil {
		_ = container.Terminate(ctx)

		return nil, "", fmt.Errorf("failed to get redis host: %w", err)
	}

	port, err := container.MappedPort(ctx, "6379")
	if err != nil {
		_ = container.Terminate(ctx)

		return nil, "", fmt.Errorf("failed to get redis port: %w", err)
	}

	return container, fmt.Sprintf("%s:%s", host, port.Port()), nil
}

func newNatsContainer(ctx context.Context) (*nats.NATSContainer, string, error) {
	container, err := nats.Run(ctx,
		"nats:latest",
		testcontainers.WithCmd("-js"),
	)
	if err != nil {
		return nil, "", fmt.Errorf("failed to start nats container: %w", err)
	}

	url, err := container.ConnectionString(ctx)
	if err != nil {
		_ = container.Terminate(ctx)

		return nil, "", fmt.Errorf("failed to get nats connection string: %w", err)
	}

	return container, url, nil
}

func cleanDatabase(ctx context.Context, dsn string) error {
	db, err := sql.Open("pgx", dsn)
	if err != nil {
		return fmt.Errorf("failed to open database for cleanup: %w", err)
	}
	defer func() { _ = db.Close() }()

	tables := []string{
		tableUsageDailySummary,
		tableUsageRecords,
		tableCostMultipliers,
		tableAdminAuditLog,
		tableAuditLog,
		tableRoutingRules,
		tableAPIKeys,
		tableEndpoints,
		tableFingerprintPresets,
	}

	for _, table := range tables {
		query := fmt.Sprintf("TRUNCATE TABLE %s CASCADE", table)

		_, err := db.ExecContext(ctx, query)
		if err != nil {
			continue
		}
	}

	return nil
}

func cleanRedis(ctx context.Context, addr string) error {
	client, err := infraredis.NewClient(ctx, config.RedisConfig{Addr: addr}, nil)
	if err != nil {
		return fmt.Errorf("failed to open redis for cleanup: %w", err)
	}
	defer func() { _ = client.Close() }()

	err = client.Client.FlushDB(ctx).Err()
	if err != nil {
		return fmt.Errorf("flush redis: %w", err)
	}

	return nil
}

// RunTestMain initializes the test suite and runs the test main, handling teardown.
func RunTestMain(m *testing.M) {
	flag.Parse()

	if testing.Short() {
		log.Println("Skipping integration tests in short mode")
		os.Exit(0)
	}

	ctx, cancel := context.WithTimeout(context.Background(), suiteTimeout)

	suiteOnce.Do(func() {
		var err error

		suite, err = SetupSuite(ctx)
		if err != nil {
			log.Fatalf("Failed to setup test suite: %v", err)
		}
	})

	code := m.Run()

	teardownCtx, teardownCancel := context.WithTimeout(context.Background(), teardownTimeout)

	suite.Teardown(teardownCtx)

	teardownCancel()
	cancel()
	os.Exit(code)
}
