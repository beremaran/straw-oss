package redis_test

import (
	"context"
	"errors"
	"os"
	"testing"
	"time"

	"github.com/beremaran/straw/internal/config"
	"github.com/beremaran/straw/internal/infra/redis"
)

func TestRedisIntegration(t *testing.T) {
	addr := os.Getenv("REDIS_ADDR")
	if addr == "" {
		addr = "localhost:6379"
	}

	client, err := redis.NewClient(config.RedisConfig{Addr: addr}, nil)
	if err != nil {
		t.Skipf("Skipping integration test: %v", err)
	}
	defer client.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	err = client.HealthCheck(ctx)
	if err != nil {
		t.Fatalf("HealthCheck failed: %v", err)
	}

	key := "test_key"
	value := map[string]string{"foo": "bar"}

	err = client.Set(ctx, key, value, 10*time.Second)
	if err != nil {
		t.Fatalf("Set failed: %v", err)
	}

	var result map[string]string
	err = client.Get(ctx, key, &result)
	if err != nil {
		t.Fatalf("Get failed: %v", err)
	}

	if result["foo"] != "bar" {
		t.Errorf("Expected bar, got %s", result["foo"])
	}

	err = client.Delete(ctx, key)
	if err != nil {
		t.Fatalf("Delete failed: %v", err)
	}

	err = client.Get(ctx, key, &result)
	if !errors.Is(err, redis.ErrCacheMiss) {
		t.Errorf("Expected ErrCacheMiss, got %v", err)
	}
}
