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
	natsLocalURL      = "nats://localhost:4222"
	testEgressID      = "egress-001"
	testControlEgress = "control-egress"
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
