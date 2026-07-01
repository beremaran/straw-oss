package config

import (
	"errors"
	"os"
	"slices"
	"testing"
)

const (
	envNatsURL        = "NATS_URL"
	envEgressID       = "EGRESS_ID"
	envControlEgress  = "CONTROL_EGRESS_ID"
	envControlRoutes  = "CONTROL_ROUTES"
	natsLocalURL      = "nats://localhost:4222"
	testEgressID      = "egress-001"
	testControlEgress = "control-egress"
	testAuthToken     = "secret"
)

func TestLoadControlConfig_Defaults(t *testing.T) {
	setEnvVars(t, map[string]string{
		envControlEgress: testControlEgress,
		envNatsURL:       natsLocalURL,
	})

	cfg, err := LoadControlConfig()
	if err != nil {
		t.Fatalf("LoadControlConfig() error = %v", err)
	}

	if cfg.HTTPPort != 8080 {
		t.Errorf("HTTPPort = %v, want 8080", cfg.HTTPPort)
	}
	if cfg.EgressID != testControlEgress {
		t.Errorf("EgressID = %v, want %v", cfg.EgressID, testControlEgress)
	}
	if cfg.ResultTimeout.String() != "30s" {
		t.Errorf("ResultTimeout = %v, want 30s", cfg.ResultTimeout)
	}
	if cfg.StreamTimeout.String() != "10s" {
		t.Errorf("StreamTimeout = %v, want 10s", cfg.StreamTimeout)
	}
}

func TestLoadControlConfig_UsesEgressIDFallback(t *testing.T) {
	setEnvVars(t, map[string]string{
		envEgressID: testEgressID,
		envNatsURL:  natsLocalURL,
	})

	cfg, err := LoadControlConfig()
	if err != nil {
		t.Fatalf("LoadControlConfig() error = %v", err)
	}

	if cfg.EgressID != testEgressID {
		t.Errorf("EgressID = %v, want %v", cfg.EgressID, testEgressID)
	}
}

func TestLoadControlConfig_StreamTimeout(t *testing.T) {
	setEnvVars(t, map[string]string{
		envControlEgress:      testControlEgress,
		envNatsURL:            natsLocalURL,
		"NATS_STREAM_TIMEOUT": "5s",
	})

	cfg, err := LoadControlConfig()
	if err != nil {
		t.Fatalf("LoadControlConfig() error = %v", err)
	}

	if cfg.StreamTimeout.String() != "5s" {
		t.Errorf("StreamTimeout = %v, want 5s", cfg.StreamTimeout)
	}
}

func TestLoadControlConfig_AuthAndRoutes(t *testing.T) {
	setEnvVars(t, map[string]string{
		envControlEgress:     testControlEgress,
		envNatsURL:           natsLocalURL,
		"CONTROL_AUTH_TOKEN": testAuthToken,
		envControlRoutes:     `[{"egress_id":"us-res-1","country":"US","ip_type":"residential"}]`,
	})

	cfg, err := LoadControlConfig()
	if err != nil {
		t.Fatalf("LoadControlConfig() error = %v", err)
	}

	if cfg.AuthToken != testAuthToken {
		t.Fatalf("AuthToken = %q, want secret", cfg.AuthToken)
	}
	if len(cfg.Routes) != 1 {
		t.Fatalf("routes len = %d, want 1", len(cfg.Routes))
	}
	if cfg.Routes[0].EgressID != "us-res-1" {
		t.Fatalf("route egress_id = %q, want us-res-1", cfg.Routes[0].EgressID)
	}
}

func TestLoadControlConfig_InvalidRoutes(t *testing.T) {
	setEnvVars(t, map[string]string{
		envControlEgress: testControlEgress,
		envNatsURL:       natsLocalURL,
		envControlRoutes: `not-json`,
	})

	_, err := LoadControlConfig()
	if err == nil {
		t.Fatal("LoadControlConfig() expected error, got nil")
	}
}

func TestLoadControlConfig_ProxyRequiresAuth(t *testing.T) {
	setEnvVars(t, map[string]string{
		envControlEgress: testControlEgress,
		envNatsURL:       natsLocalURL,
		"PROXY_PORT":     "8081",
	})

	_, err := LoadControlConfig()
	if err == nil {
		t.Fatal("LoadControlConfig() expected error, got nil")
	}

	assertValidationError(t, err, "CONTROL_AUTH_TOKEN is required when PROXY_PORT is set")
}

func TestLoadControlConfig_MissingRequired(t *testing.T) {
	setEnvVars(t, map[string]string{
		envNatsURL: natsLocalURL,
	})

	_, err := LoadControlConfig()
	if err == nil {
		t.Fatal("LoadControlConfig() expected error, got nil")
	}

	assertValidationError(t, err, "CONTROL_EGRESS_ID or EGRESS_ID is required")
}

func TestLoadEgressConfig_Defaults(t *testing.T) {
	setEnvVars(t, map[string]string{
		envEgressID: testEgressID,
		envNatsURL:  natsLocalURL,
	})

	cfg, err := LoadEgressConfig()
	if err != nil {
		t.Fatalf("LoadEgressConfig() error = %v", err)
	}

	if cfg.ConcurrencyLimit != 25 {
		t.Errorf("ConcurrencyLimit = %v, want 25", cfg.ConcurrencyLimit)
	}
	if cfg.ID != testEgressID {
		t.Errorf("ID = %v, want %v", cfg.ID, testEgressID)
	}
	if cfg.TunnelChunkSize != defaultTunnelChunkSize {
		t.Errorf("TunnelChunkSize = %v, want %v", cfg.TunnelChunkSize, defaultTunnelChunkSize)
	}
}

func TestLoadEgressConfig_MissingRequired(t *testing.T) {
	setEnvVars(t, map[string]string{
		envNatsURL: natsLocalURL,
	})

	_, err := LoadEgressConfig()
	if err == nil {
		t.Fatal("LoadEgressConfig() expected error, got nil")
	}

	assertValidationError(t, err, "EGRESS_ID is required")
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
		envEgressID,
		envControlEgress,
		"HTTP_PORT",
		"RESULT_TIMEOUT",
		"NATS_STREAM_TIMEOUT",
		"MAX_CONCURRENT_REQUESTS",
		"CONCURRENCY_LIMIT",
		"CONTROL_AUTH_TOKEN",
		envControlRoutes,
		"PROXY_PORT",
		"MAX_CONCURRENT_TUNNELS",
		"TUNNEL_CHUNK_SIZE",
	} {
		t.Setenv(key, "")
		_ = os.Unsetenv(key)
	}

	for key, val := range vars {
		t.Setenv(key, val)
	}
}

func TestGetEnvBool(t *testing.T) {
	t.Setenv("TEST_BOOL_TRUE", "true")
	t.Setenv("TEST_BOOL_FALSE", "false")
	t.Setenv("TEST_BOOL_EMPTY", "")
	t.Setenv("TEST_BOOL_INVALID", "notabool")

	if !getEnvBool("TEST_BOOL_TRUE", false) {
		t.Error("expected true for 'true'")
	}
	if getEnvBool("TEST_BOOL_FALSE", true) {
		t.Error("expected false for 'false'")
	}
	if getEnvBool("TEST_BOOL_EMPTY", false) {
		t.Error("expected default false for empty string")
	}
	if getEnvBool("TEST_BOOL_INVALID", false) {
		t.Error("expected default false for invalid value")
	}
}

func TestGetEnvInt(t *testing.T) {
	t.Setenv("TEST_INT_42", "42")
	t.Setenv("TEST_INT_0", "0")
	t.Setenv("TEST_INT_EMPTY", "")
	t.Setenv("TEST_INT_INVALID", "notanint")

	if getEnvInt("TEST_INT_42", 0) != 42 {
		t.Error("expected 42 for '42'")
	}
	if getEnvInt("TEST_INT_0", 10) != 0 {
		t.Error("expected 0 for '0'")
	}
	if getEnvInt("TEST_INT_EMPTY", 10) != 10 {
		t.Error("expected default 10 for empty string")
	}
	if getEnvInt("TEST_INT_INVALID", 10) != 10 {
		t.Error("expected default 10 for invalid value")
	}
}

func TestValidationError(t *testing.T) {
	err := &ValidationError{
		Errors: []string{"error 1", "error 2"},
	}
	expected := "configuration validation failed: error 1; error 2"
	if err.Error() != expected {
		t.Errorf("Error() = %q, want %q", err.Error(), expected)
	}
}

func TestLoadNATSConfig(t *testing.T) {
	t.Setenv(envNatsURL, "nats://custom:4222")
	t.Setenv("NATS_TOKEN", "secret")

	cfg := LoadNATSConfig()
	if cfg.URL != "nats://custom:4222" {
		t.Errorf("URL = %q, want nats://custom:4222", cfg.URL)
	}
	if cfg.Token != "secret" {
		t.Errorf("Token = %q, want secret", cfg.Token)
	}
}

func TestLoadSecurityConfig(t *testing.T) {
	t.Setenv("TLS_CERT_FILE", "/path/to/cert")
	t.Setenv("TLS_KEY_FILE", "/path/to/key")

	cfg := LoadSecurityConfig()
	if cfg.TLSCertFile != "/path/to/cert" {
		t.Errorf("TLSCertFile = %q, want /path/to/cert", cfg.TLSCertFile)
	}
	if cfg.TLSKeyFile != "/path/to/key" {
		t.Errorf("TLSKeyFile = %q, want /path/to/key", cfg.TLSKeyFile)
	}
}
