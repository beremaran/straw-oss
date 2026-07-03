package control

import (
	"context"
	"testing"
	"time"

	"github.com/redis/go-redis/v9"

	"github.com/beremaran/straw/v2/internal/redisx"
)

type discardLogger struct{}

func (discardLogger) Printf(context.Context, string, ...any) {}

func init() {
	// The go-redis pool logs a warning on every failed dial attempt; the
	// deliberately-unreachable-Redis fail-policy tests trigger this
	// intentionally and repeatedly, so silence it to keep test output
	// readable.
	redis.SetLogger(discardLogger{})
}

// testRedisAddr is the local Redis instance used by focused Redis tests
// (docker-compose.yml runs redis:7-alpine on this address). Tests skip
// gracefully when it is unreachable rather than failing make check in an
// environment with no Redis.
const testRedisAddr = "127.0.0.1:6379"

// testUnreachableRedisAddr is a loopback address nothing listens on, used by
// Redis-failure-policy tests to force fast, deterministic command errors.
const testUnreachableRedisAddr = "127.0.0.1:1"

// newTestRedisClient connects to testRedisAddr, skipping the test if Redis
// is unreachable, and registers cleanup that flushes the test keyspace.
func newTestRedisClient(t *testing.T) *redis.Client {
	t.Helper()

	client := redisx.NewClient(redisx.Config{Addr: testRedisAddr})

	ctx, cancel := context.WithTimeout(context.Background(), 500*time.Millisecond)
	defer cancel()

	err := client.Ping(ctx).Err()
	if err != nil {
		_ = client.Close()
		t.Skipf("redis unreachable at %s, skipping: %v", testRedisAddr, err)
	}

	t.Cleanup(func() {
		flushCtx, flushCancel := context.WithTimeout(context.Background(), time.Second)
		defer flushCancel()
		_ = client.FlushDB(flushCtx).Err()
		_ = client.Close()
	})

	return client
}
