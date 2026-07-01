package config

import (
	"errors"
	"os"
	"slices"
	"testing"
)

const (
	envNatsURL        = "NATS_URL"
	envHmacSecret     = "HMAC_SECRET"
	envEndpointID     = "ENDPOINT_ID"
	envRelayEndpoint  = "RELAY_ENDPOINT_ID"
	natsLocalURL      = "nats://localhost:4222"
	testSecret        = "test-secret"
	testEndpointID    = "endpoint-001"
	testRelayEndpoint = "endpoint-relay"
)

func TestLoadServerConfig_Defaults(t *testing.T) {
	setEnvVars(t, map[string]string{
		envRelayEndpoint: testRelayEndpoint,
		envNatsURL:       natsLocalURL,
		envHmacSecret:    testSecret,
	})

	cfg, err := LoadServerConfig()
	if err != nil {
		t.Fatalf("LoadServerConfig() error = %v", err)
	}

	if cfg.HTTPPort != 8080 {
		t.Errorf("HTTPPort = %v, want 8080", cfg.HTTPPort)
	}
	if cfg.EndpointID != testRelayEndpoint {
		t.Errorf("EndpointID = %v, want %v", cfg.EndpointID, testRelayEndpoint)
	}
	if cfg.ResultTimeout.String() != "30s" {
		t.Errorf("ResultTimeout = %v, want 30s", cfg.ResultTimeout)
	}
}

func TestLoadServerConfig_UsesEndpointIDFallback(t *testing.T) {
	setEnvVars(t, map[string]string{
		envEndpointID: testEndpointID,
		envNatsURL:    natsLocalURL,
		envHmacSecret: testSecret,
	})

	cfg, err := LoadServerConfig()
	if err != nil {
		t.Fatalf("LoadServerConfig() error = %v", err)
	}

	if cfg.EndpointID != testEndpointID {
		t.Errorf("EndpointID = %v, want %v", cfg.EndpointID, testEndpointID)
	}
}

func TestLoadServerConfig_MissingRequired(t *testing.T) {
	setEnvVars(t, map[string]string{
		envNatsURL:    natsLocalURL,
		envHmacSecret: testSecret,
	})

	_, err := LoadServerConfig()
	if err == nil {
		t.Fatal("LoadServerConfig() expected error, got nil")
	}

	assertValidationError(t, err, "RELAY_ENDPOINT_ID or ENDPOINT_ID is required")
}

func TestLoadEndpointConfig_Defaults(t *testing.T) {
	setEnvVars(t, map[string]string{
		envEndpointID: testEndpointID,
		envNatsURL:    natsLocalURL,
		envHmacSecret: testSecret,
	})

	cfg, err := LoadEndpointConfig()
	if err != nil {
		t.Fatalf("LoadEndpointConfig() error = %v", err)
	}

	if cfg.ConcurrencyLimit != 25 {
		t.Errorf("ConcurrencyLimit = %v, want 25", cfg.ConcurrencyLimit)
	}
	if cfg.ID != testEndpointID {
		t.Errorf("ID = %v, want %v", cfg.ID, testEndpointID)
	}
}

func TestLoadEndpointConfig_MissingRequired(t *testing.T) {
	setEnvVars(t, map[string]string{
		envNatsURL:    natsLocalURL,
		envHmacSecret: testSecret,
	})

	_, err := LoadEndpointConfig()
	if err == nil {
		t.Fatal("LoadEndpointConfig() expected error, got nil")
	}

	assertValidationError(t, err, "ENDPOINT_ID is required")
}

func assertValidationError(t *testing.T, err error, want string) {
	t.Helper()

	var validationErr *ValidationError
	if !errors.As(err, &validationErr) {
		t.Fatalf("expected ValidationError, got %T", err)
	}

	if !slices.Contains(validationErr.Errors, want) {
		t.Errorf("expected error containing %q, got %v", want, validationErr.Errors)
	}
}

func setEnvVars(t *testing.T, vars map[string]string) {
	t.Helper()

	for _, key := range []string{
		envNatsURL,
		envHmacSecret,
		envEndpointID,
		envRelayEndpoint,
		"HTTP_PORT",
		"RESULT_TIMEOUT",
		"MAX_CONCURRENT_REQUESTS",
		"CONCURRENCY_LIMIT",
	} {
		t.Setenv(key, "")
		_ = os.Unsetenv(key)
	}

	for key, val := range vars {
		t.Setenv(key, val)
	}
}
