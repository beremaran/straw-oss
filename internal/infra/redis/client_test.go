package redis_test

import (
	"context"
	"os"
	"testing"
	"time"

	"github.com/kwilabs/straw-proxy-server/internal/infra/redis"
)

func TestRedisIntegration(t *testing.T) {
	addr := os.Getenv("REDIS_ADDR")
	if addr == "" {
		addr = "localhost:6379"
	}

	client, err := redis.NewClient(addr, nil)
	if err != nil {
		t.Skipf("Skipping integration test: %v", err)
	}
	defer client.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	if err := client.HealthCheck(ctx); err != nil {
		t.Fatalf("HealthCheck failed: %v", err)
	}

	// Cache operations test
	key := "test_key"
	value := map[string]string{"foo": "bar"}

	if err := client.Set(ctx, key, value, 10*time.Second); err != nil {
		t.Fatalf("Set failed: %v", err)
	}

	var result map[string]string
	if err := client.Get(ctx, key, &result); err != nil {
		t.Fatalf("Get failed: %v", err)
	}

	if result["foo"] != "bar" {
		t.Errorf("Expected bar, got %s", result["foo"])
	}

	if err := client.Delete(ctx, key); err != nil {
		t.Fatalf("Delete failed: %v", err)
	}

	err = client.Get(ctx, key, &result)
	if err != redis.ErrCacheMiss {
		t.Errorf("Expected ErrCacheMiss, got %v", err)
	}
}
