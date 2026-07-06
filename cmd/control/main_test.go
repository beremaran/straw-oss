package main

import (
	"context"
	"testing"
	"time"

	"github.com/beremaran/straw/v2/internal/config"
)

func TestOpenRedisMissingURLEnvFails(t *testing.T) {
	t.Setenv("STRAW_TEST_MAIN_REDIS_URL_UNSET", "")

	_, err := openRedis(config.RedisConfig{URLEnv: "STRAW_TEST_MAIN_REDIS_URL_UNSET"})
	if err == nil {
		t.Fatal("openRedis() error = nil, want error for unset STRAW_REDIS_URL")
	}
}

func TestOpenRedisInvalidURLFails(t *testing.T) {
	t.Setenv("STRAW_TEST_MAIN_REDIS_URL", "not-a-valid-redis-url")

	_, err := openRedis(config.RedisConfig{URLEnv: "STRAW_TEST_MAIN_REDIS_URL"})
	if err == nil {
		t.Fatal("openRedis() error = nil, want error for malformed url")
	}
}

// TestOpenRedisUnreachableStillReturnsClient proves a configured-but-down
// Redis does not fail Control startup (docs/planning/29 "Redis unavailable:
// Apply configured fail policy"); only a bad/missing URL does.
func TestOpenRedisUnreachableStillReturnsClient(t *testing.T) {
	t.Setenv("STRAW_TEST_MAIN_REDIS_URL", "redis://127.0.0.1:1/0")

	client, err := openRedis(config.RedisConfig{URLEnv: "STRAW_TEST_MAIN_REDIS_URL", DialTimeoutMS: 50})
	if err != nil {
		t.Fatalf("openRedis() error = %v, want nil for an unreachable-but-configured redis", err)
	}
	defer func() { _ = client.Close() }()

	pingCtx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()

	pingErr := client.Ping(pingCtx).Err()
	if pingErr == nil {
		t.Fatal("client.Ping() error = nil, want error against an unreachable address")
	}
}

func TestBuildProxyHandlerOnlyWhenEnabled(t *testing.T) {
	t.Parallel()

	cfg := config.ControlConfig{
		Server: config.ControlServerConfig{Host: "127.0.0.1", APIPort: 8080, MetricsPort: 9090},
	}
	if got := buildProxyHandler(cfg, nil, nil, nil); got != nil {
		t.Fatal("buildProxyHandler disabled = non-nil, want nil")
	}

	cfg.Server.ProxyEnabled = true
	cfg.Server.ProxyPort = 8081
	if got := buildProxyHandler(cfg, nil, nil, nil); got == nil {
		t.Fatal("buildProxyHandler enabled = nil, want handler")
	}
}

func TestBuildConnectHandlerOnlyWhenEnabled(t *testing.T) {
	t.Parallel()

	cfg := config.ControlConfig{
		Server: config.ControlServerConfig{Host: "127.0.0.1", APIPort: 8080, MetricsPort: 9090},
	}
	if got := buildConnectHandler(cfg, nil, nil); got != nil {
		t.Fatal("buildConnectHandler disabled = non-nil, want nil")
	}

	cfg.Server.ConnectEnabled = true
	cfg.Server.ConnectPort = 8082
	if got := buildConnectHandler(cfg, nil, nil); got == nil {
		t.Fatal("buildConnectHandler enabled = nil, want handler")
	}
}
