package config

import (
	"errors"
	"os"
	"testing"
)

func TestLoadServerConfig_Defaults(t *testing.T) {
	// Set required env vars
	setEnvVars(t, map[string]string{
		"POSTGRES_DSN": "postgres://localhost/test",
		"NATS_URL":     "nats://localhost:4222",
		"HMAC_SECRET":  "test-secret",
	})

	cfg, err := LoadServerConfig("")
	if err != nil {
		t.Fatalf("LoadServerConfig() error = %v", err)
	}

	// Check defaults applied
	if cfg.Core.RedisAddr != "localhost:6379" {
		t.Errorf("RedisAddr = %v, want localhost:6379", cfg.Core.RedisAddr)
	}
	if cfg.Core.LogLevel != "info" {
		t.Errorf("LogLevel = %v, want info", cfg.Core.LogLevel)
	}
	if cfg.Core.LogFormat != "json" {
		t.Errorf("LogFormat = %v, want json", cfg.Core.LogFormat)
	}
	if !cfg.Observability.MetricsEnabled {
		t.Error("MetricsEnabled should be true by default")
	}
	if cfg.Observability.MetricsPort != 9090 {
		t.Errorf("MetricsPort = %v, want 9090", cfg.Observability.MetricsPort)
	}
	if cfg.HTTPPort != 8080 {
		t.Errorf("HTTPPort = %v, want 8080", cfg.HTTPPort)
	}
	if cfg.AdminPort != 8081 {
		t.Errorf("AdminPort = %v, want 8081", cfg.AdminPort)
	}
}

func TestLoadServerConfig_EnvOverride(t *testing.T) {
	setEnvVars(t, map[string]string{
		"POSTGRES_DSN": "postgres://custom/db",
		"NATS_URL":     "nats://custom:4222",
		"HMAC_SECRET":  "custom-secret",
		"REDIS_ADDR":   "redis.example.com:6380",
		"LOG_LEVEL":    "debug",
		"METRICS_PORT": "9091",
		"HTTP_PORT":    "3000",
	})

	cfg, err := LoadServerConfig("")
	if err != nil {
		t.Fatalf("LoadServerConfig() error = %v", err)
	}

	if cfg.Core.PostgresDSN != "postgres://custom/db" {
		t.Errorf("PostgresDSN = %v, want postgres://custom/db", cfg.Core.PostgresDSN)
	}
	if cfg.Core.RedisAddr != "redis.example.com:6380" {
		t.Errorf("RedisAddr = %v, want redis.example.com:6380", cfg.Core.RedisAddr)
	}
	if cfg.Core.LogLevel != "debug" {
		t.Errorf("LogLevel = %v, want debug", cfg.Core.LogLevel)
	}
	if cfg.Observability.MetricsPort != 9091 {
		t.Errorf("MetricsPort = %v, want 9091", cfg.Observability.MetricsPort)
	}
	if cfg.HTTPPort != 3000 {
		t.Errorf("HTTPPort = %v, want 3000", cfg.HTTPPort)
	}
}

func TestLoadServerConfig_MissingRequired(t *testing.T) {
	tests := []struct {
		name    string
		envVars map[string]string
		wantErr string
	}{
		{
			name: "missing POSTGRES_DSN",
			envVars: map[string]string{
				"NATS_URL":    "nats://localhost:4222",
				"HMAC_SECRET": "secret",
			},
			wantErr: "POSTGRES_DSN is required",
		},
		{
			name: "missing HMAC_SECRET",
			envVars: map[string]string{
				"POSTGRES_DSN": "postgres://localhost/test",
				"NATS_URL":     "nats://localhost:4222",
			},
			wantErr: "HMAC_SECRET is required",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			setEnvVars(t, tt.envVars)

			_, err := LoadServerConfig("")
			if err == nil {
				t.Fatal("LoadServerConfig() expected error, got nil")
			}

			var validationErr *ValidationError
			ok := errors.As(err, &validationErr)
			if !ok {
				t.Fatalf("expected ValidationError, got %T", err)
			}

			found := false
			for _, e := range validationErr.Errors {
				if e == tt.wantErr {
					found = true
					break
				}
			}
			if !found {
				t.Errorf("expected error containing %q, got %v", tt.wantErr, validationErr.Errors)
			}
		})
	}
}

func TestLoadEndpointConfig_Defaults(t *testing.T) {
	setEnvVars(t, map[string]string{
		"ENDPOINT_ID": "endpoint-001",
		"NATS_URL":    "nats://localhost:4222",
		"HMAC_SECRET": "test-secret",
	})

	cfg, err := LoadEndpointConfig("")
	if err != nil {
		t.Fatalf("LoadEndpointConfig() error = %v", err)
	}

	if cfg.ConcurrencyLimit != 25 {
		t.Errorf("ConcurrencyLimit = %v, want 25", cfg.ConcurrencyLimit)
	}
	if cfg.ID != "endpoint-001" {
		t.Errorf("ID = %v, want endpoint-001", cfg.ID)
	}
}

func TestLoadEndpointConfig_TagsParsing(t *testing.T) {
	setEnvVars(t, map[string]string{
		"ENDPOINT_ID":   "endpoint-001",
		"NATS_URL":      "nats://localhost:4222",
		"HMAC_SECRET":   "test-secret",
		"ENDPOINT_TAGS": "type:residential, region:us, capability:stealth",
	})

	cfg, err := LoadEndpointConfig("")
	if err != nil {
		t.Fatalf("LoadEndpointConfig() error = %v", err)
	}

	expectedTags := []string{"type:residential", "region:us", "capability:stealth"}
	if len(cfg.Tags) != len(expectedTags) {
		t.Errorf("Tags length = %v, want %v", len(cfg.Tags), len(expectedTags))
	}
	for i, tag := range expectedTags {
		if i < len(cfg.Tags) && cfg.Tags[i] != tag {
			t.Errorf("Tags[%d] = %v, want %v", i, cfg.Tags[i], tag)
		}
	}
}

func TestLoadEndpointConfig_MissingRequired(t *testing.T) {
	tests := []struct {
		name    string
		envVars map[string]string
		wantErr string
	}{
		{
			name: "missing ENDPOINT_ID",
			envVars: map[string]string{
				"NATS_URL":    "nats://localhost:4222",
				"HMAC_SECRET": "secret",
			},
			wantErr: "ENDPOINT_ID is required",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			setEnvVars(t, tt.envVars)

			_, err := LoadEndpointConfig("")
			if err == nil {
				t.Fatal("LoadEndpointConfig() expected error, got nil")
			}

			var validationErr *ValidationError
			ok := errors.As(err, &validationErr)
			if !ok {
				t.Fatalf("expected ValidationError, got %T", err)
			}

			found := false
			for _, e := range validationErr.Errors {
				if e == tt.wantErr {
					found = true
					break
				}
			}
			if !found {
				t.Errorf("expected error containing %q, got %v", tt.wantErr, validationErr.Errors)
			}
		})
	}
}

func TestParseCommaSeparated(t *testing.T) {
	tests := []struct {
		name  string
		input string
		want  []string
	}{
		{
			name:  "multiple values",
			input: "a, b, c",
			want:  []string{"a", "b", "c"},
		},
		{
			name:  "single value",
			input: "a",
			want:  []string{"a"},
		},
		{
			name:  "empty string",
			input: "",
			want:  nil,
		},
		{
			name:  "whitespace handling",
			input: "  a  ,  b  ,  c  ",
			want:  []string{"a", "b", "c"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := parseCommaSeparated(tt.input)
			if len(got) != len(tt.want) {
				t.Errorf("parseCommaSeparated() = %v, want %v", got, tt.want)
				return
			}
			for i := range got {
				if got[i] != tt.want[i] {
					t.Errorf("parseCommaSeparated()[%d] = %v, want %v", i, got[i], tt.want[i])
				}
			}
		})
	}
}

func TestValidationError_Error(t *testing.T) {
	err := &ValidationError{Errors: []string{"error1", "error2"}}
	want := "configuration validation failed: error1; error2"
	if got := err.Error(); got != want {
		t.Errorf("ValidationError.Error() = %v, want %v", got, want)
	}
}

// setEnvVars is a test helper that sets environment variables and cleans them up after the test.
func setEnvVars(t *testing.T, vars map[string]string) {
	t.Helper()

	// Clear all config-related env vars first
	allVars := []string{
		"POSTGRES_DSN", "REDIS_ADDR", "NATS_URL", "LOG_LEVEL", "LOG_FORMAT",
		"HMAC_SECRET", "TLS_CERT_FILE", "TLS_KEY_FILE", "VAULT_ADDR",
		"OTEL_EXPORTER_OTLP_ENDPOINT", "METRICS_ENABLED", "METRICS_PORT",
		"HTTP_PORT", "ADMIN_PORT", "SHUTDOWN_TIMEOUT", "ADMIN_API_KEY",
		"ENDPOINT_ID", "ENDPOINT_TAGS", "CONCURRENCY_LIMIT", "SELF_UPDATE_URL",
	}
	for _, v := range allVars {
		_ = os.Unsetenv(v)
	}

	// Set the provided vars
	for k, v := range vars {
		_ = os.Setenv(k, v)
	}

	// Cleanup after test
	t.Cleanup(func() {
		for k := range vars {
			_ = os.Unsetenv(k)
		}
	})
}
