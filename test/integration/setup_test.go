package integration

import (
	"context"
	"database/sql"
	"testing"
	"time"

	_ "github.com/jackc/pgx/v5/stdlib"
	"github.com/nats-io/nats.go"
	"github.com/redis/go-redis/v9"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestMain(m *testing.M) {
	RunTestMain(m)
}

func TestContainerSetup(t *testing.T) {
	suite := GetSuite(t)

	t.Run("PostgreSQL is accessible", func(t *testing.T) {
		db, err := sql.Open("pgx", suite.PostgresDSN())
		require.NoError(t, err, "should connect to PostgreSQL")
		defer db.Close()

		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()

		err = db.PingContext(ctx)
		assert.NoError(t, err, "should ping PostgreSQL")
	})

	t.Run("Redis is accessible", func(t *testing.T) {
		client := redis.NewClient(&redis.Options{
			Addr: suite.RedisAddr(),
		})
		defer client.Close()

		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()

		_, err := client.Ping(ctx).Result()
		assert.NoError(t, err, "should ping Redis")
	})

	t.Run("NATS is accessible", func(t *testing.T) {
		nc, err := nats.Connect(suite.NatsURL())
		require.NoError(t, err, "should connect to NATS")
		defer nc.Close()

		require.True(t, nc.IsConnected(), "should be connected")
	})
}

func TestDatabaseMigrations(t *testing.T) {
	suite := GetSuite(t)

	db, err := sql.Open("pgx", suite.PostgresDSN())
	require.NoError(t, err)
	defer db.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	expectedTables := []string{
		"api_keys",
		"routing_rules",
		"endpoints",
		"audit_log",
		"usage_records",
		"cost_multipliers",
		"usage_daily_summary",
		"admin_audit_log",
		"fingerprint_presets",
	}

	for _, tableName := range expectedTables {
		t.Run("table_"+tableName, func(t *testing.T) {
			var exists bool
			query := `SELECT EXISTS (
				SELECT FROM information_schema.tables 
				WHERE table_schema = 'public' AND table_name = $1
			)`
			err := db.QueryRowContext(ctx, query, tableName).Scan(&exists)
			require.NoError(t, err, "should query for table existence")
			assert.True(t, exists, "table %s should exist", tableName)
		})
	}
}

func TestDatabaseCleanup(t *testing.T) {
	suite := GetSuite(t)

	db, err := sql.Open("pgx", suite.PostgresDSN())
	require.NoError(t, err)
	defer db.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	_, err = db.ExecContext(ctx, `
		INSERT INTO api_keys (id, name, token_hash, scopes, is_active)
		VALUES ('a0eebc99-9c0b-4ef8-bb6d-6bb9bd380a11', 'Test Key', 'hash123', '[]', true)
	`)
	require.NoError(t, err, "should insert test data")

	var count int
	err = db.QueryRowContext(ctx, "SELECT COUNT(*) FROM api_keys WHERE id = 'a0eebc99-9c0b-4ef8-bb6d-6bb9bd380a11'").Scan(&count)
	require.NoError(t, err)
	assert.Equal(t, 1, count, "should have inserted data")

	err = CleanDatabase(ctx, suite.PostgresDSN())
	require.NoError(t, err, "should clean database")

	err = db.QueryRowContext(ctx, "SELECT COUNT(*) FROM api_keys WHERE id = 'a0eebc99-9c0b-4ef8-bb6d-6bb9bd380a11'").Scan(&count)
	require.NoError(t, err)
	assert.Equal(t, 0, count, "data should be cleaned up")
}

func TestParallelTestIsolation(t *testing.T) {
	suite := GetSuite(t)

	t.Run("parallel_test_1", func(t *testing.T) {
		t.Parallel()
		suite.CleanupForTest(t)

		db, err := sql.Open("pgx", suite.PostgresDSN())
		require.NoError(t, err)
		defer db.Close()

		ctx := context.Background()
		_, err = db.ExecContext(ctx, `
			INSERT INTO api_keys (id, name, token_hash, scopes, is_active)
			VALUES ('b1eebc99-9c0b-4ef8-bb6d-6bb9bd380a11', 'Parallel Test 1', 'hash1', '[]', true)
		`)
		require.NoError(t, err)

		time.Sleep(100 * time.Millisecond)

		var name string
		err = db.QueryRowContext(ctx, "SELECT name FROM api_keys WHERE id = 'b1eebc99-9c0b-4ef8-bb6d-6bb9bd380a11'").Scan(&name)
		require.NoError(t, err)
		assert.Equal(t, "Parallel Test 1", name)
	})

	t.Run("parallel_test_2", func(t *testing.T) {
		t.Parallel()
		suite.CleanupForTest(t)

		db, err := sql.Open("pgx", suite.PostgresDSN())
		require.NoError(t, err)
		defer db.Close()

		ctx := context.Background()
		_, err = db.ExecContext(ctx, `
			INSERT INTO api_keys (id, name, token_hash, scopes, is_active)
			VALUES ('c2eebc99-9c0b-4ef8-bb6d-6bb9bd380a11', 'Parallel Test 2', 'hash2', '[]', true)
		`)
		require.NoError(t, err)

		time.Sleep(100 * time.Millisecond)

		var name string
		err = db.QueryRowContext(ctx, "SELECT name FROM api_keys WHERE id = 'c2eebc99-9c0b-4ef8-bb6d-6bb9bd380a11'").Scan(&name)
		require.NoError(t, err)
		assert.Equal(t, "Parallel Test 2", name)
	})
}

func TestConfigHelpers(t *testing.T) {
	suite := GetSuite(t)

	t.Run("NewTestServerConfig", func(t *testing.T) {
		cfg := NewTestServerConfig(suite.PostgresDSN(), suite.RedisAddr(), suite.NatsURL())
		assert.Equal(t, suite.PostgresDSN(), cfg.Database.DSN)
		assert.Equal(t, suite.RedisAddr(), cfg.Redis.Addr)
		assert.Equal(t, suite.NatsURL(), cfg.NATS.URL)
		assert.Equal(t, "debug", cfg.Observability.LogLevel)
	})

	t.Run("NewTestEndpointConfig", func(t *testing.T) {
		cfg := NewTestEndpointConfig(suite.PostgresDSN(), suite.RedisAddr(), suite.NatsURL())
		assert.Equal(t, suite.NatsURL(), cfg.NATS.URL)
		assert.Equal(t, "test-endpoint-1", cfg.ID)
	})
}

func TestWaitForHealthy(t *testing.T) {
	t.Run("succeeds when healthy immediately", func(t *testing.T) {
		ctx := context.Background()
		err := WaitForHealthy(ctx, func() error {
			return nil
		}, 100*time.Millisecond, 1*time.Second)
		assert.NoError(t, err)
	})

	t.Run("succeeds after retries", func(t *testing.T) {
		ctx := context.Background()
		attempts := 0
		err := WaitForHealthy(ctx, func() error {
			attempts++
			if attempts < 3 {
				return assert.AnError
			}
			return nil
		}, 50*time.Millisecond, 1*time.Second)
		assert.NoError(t, err)
		assert.GreaterOrEqual(t, attempts, 3)
	})

	t.Run("times out when never healthy", func(t *testing.T) {
		ctx := context.Background()
		err := WaitForHealthy(ctx, func() error {
			return assert.AnError
		}, 50*time.Millisecond, 200*time.Millisecond)
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "timed out")
	})

	t.Run("respects context cancellation", func(t *testing.T) {
		ctx, cancel := context.WithCancel(context.Background())
		go func() {
			time.Sleep(100 * time.Millisecond)
			cancel()
		}()

		err := WaitForHealthy(ctx, func() error {
			return assert.AnError
		}, 50*time.Millisecond, 10*time.Second)
		assert.Error(t, err)
	})
}
