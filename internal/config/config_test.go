package config

import (
	"errors"
	"os"
	"slices"
	"testing"
)

const (
	envPostgresDSN = "POSTGRES_DSN"
	envNatsURL     = "NATS_URL"
	envHmacSecret  = "HMAC_SECRET"
	envEndpointID  = "ENDPOINT_ID"
	natsLocalURL   = "nats://localhost:4222"
	testSecret     = "test-secret"
	testEndpointID = "endpoint-001"
)

func TestLoadServerConfig_Defaults(t *testing.T) {
	setEnvVars(t, map[string]string{
		envPostgresDSN: "postgres://localhost/test",
		envNatsURL:     natsLocalURL,
		envHmacSecret:  testSecret,
	})

	cfg, err := LoadServerConfig()
	if err != nil {
		t.Fatalf("LoadServerConfig() error = %v", err)
	}

	if cfg.Redis.Addr != "localhost:6379" {
		t.Errorf("RedisAddr = %v, want localhost:6379", cfg.Redis.Addr)
	}
	if cfg.Observability.LogLevel != "info" {
		t.Errorf("LogLevel = %v, want info", cfg.Observability.LogLevel)
	}
	if cfg.Observability.LogFormat != "json" {
		t.Errorf("LogFormat = %v, want json", cfg.Observability.LogFormat)
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
	if cfg.ManagementPort != 8081 {
		t.Errorf("ManagementPort = %v, want 8081", cfg.ManagementPort)
	}
	if cfg.ManagementLegacyTokenDisabled {
		t.Error("ManagementLegacyTokenDisabled should be false by default")
	}
}

func TestLoadServerConfig_EnvOverride(t *testing.T) {
	setEnvVars(t, map[string]string{
		envPostgresDSN:                    "postgres://custom/db",
		envNatsURL:                        "nats://custom:4222",
		envHmacSecret:                     "custom-secret",
		"REDIS_ADDR":                      "redis.example.com:6380",
		"LOG_LEVEL":                       "debug",
		"METRICS_PORT":                    "9091",
		"HTTP_PORT":                       "3000",
		"MANAGEMENT_LEGACY_TOKEN_ENABLED": "false",
	})

	cfg, err := LoadServerConfig()
	if err != nil {
		t.Fatalf("LoadServerConfig() error = %v", err)
	}

	if cfg.Database.DSN != "postgres://custom/db" {
		t.Errorf("PostgresDSN = %v, want postgres://custom/db", cfg.Database.DSN)
	}
	if cfg.Redis.Addr != "redis.example.com:6380" {
		t.Errorf("RedisAddr = %v, want redis.example.com:6380", cfg.Redis.Addr)
	}
	if cfg.Observability.LogLevel != "debug" {
		t.Errorf("LogLevel = %v, want debug", cfg.Observability.LogLevel)
	}
	if cfg.Observability.MetricsPort != 9091 {
		t.Errorf("MetricsPort = %v, want 9091", cfg.Observability.MetricsPort)
	}
	if cfg.HTTPPort != 3000 {
		t.Errorf("HTTPPort = %v, want 3000", cfg.HTTPPort)
	}
	if !cfg.ManagementLegacyTokenDisabled {
		t.Error("ManagementLegacyTokenDisabled should be true")
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
				envNatsURL:    natsLocalURL,
				envHmacSecret: "secret",
			},
			wantErr: "POSTGRES_DSN is required",
		},
		{
			name: "missing HMAC_SECRET",
			envVars: map[string]string{
				envPostgresDSN: "postgres://localhost/test",
				envNatsURL:     natsLocalURL,
			},
			wantErr: "HMAC_SECRET is required",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			setEnvVars(t, tt.envVars)

			_, err := LoadServerConfig()
			if err == nil {
				t.Fatal("LoadServerConfig() expected error, got nil")
			}

			var validationErr *ValidationError
			ok := errors.As(err, &validationErr)
			if !ok {
				t.Fatalf("expected ValidationError, got %T", err)
			}

			found := slices.Contains(validationErr.Errors, tt.wantErr)
			if !found {
				t.Errorf("expected error containing %q, got %v", tt.wantErr, validationErr.Errors)
			}
		})
	}
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
		t.Errorf("ID = %v, want endpoint-001", cfg.ID)
	}
}

func TestLoadEndpointConfig_TagsParsing(t *testing.T) {
	setEnvVars(t, map[string]string{
		envEndpointID:   testEndpointID,
		envNatsURL:      natsLocalURL,
		envHmacSecret:   testSecret,
		"ENDPOINT_TAGS": "type:residential, region:us, capability:stealth",
	})

	cfg, err := LoadEndpointConfig()
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
				envNatsURL:    natsLocalURL,
				envHmacSecret: "secret",
			},
			wantErr: "ENDPOINT_ID is required",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			setEnvVars(t, tt.envVars)

			_, err := LoadEndpointConfig()
			if err == nil {
				t.Fatal("LoadEndpointConfig() expected error, got nil")
			}

			var validationErr *ValidationError
			ok := errors.As(err, &validationErr)
			if !ok {
				t.Fatalf("expected ValidationError, got %T", err)
			}

			found := slices.Contains(validationErr.Errors, tt.wantErr)
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

func setEnvVars(t *testing.T, vars map[string]string) {
	t.Helper()

	allVars := []string{
		envPostgresDSN, "REDIS_ADDR", envNatsURL, "LOG_LEVEL", "LOG_FORMAT",
		envHmacSecret, "TLS_CERT_FILE", "TLS_KEY_FILE", "VAULT_ADDR",
		"OTEL_EXPORTER_OTLP_ENDPOINT", "METRICS_ENABLED", "METRICS_PORT",
		"HTTP_PORT", "MANAGEMENT_PORT", "SHUTDOWN_TIMEOUT", "MANAGEMENT_API_KEY",
		"MANAGEMENT_LEGACY_TOKEN_ENABLED",
		envEndpointID, "ENDPOINT_TAGS", "CONCURRENCY_LIMIT", "SELF_UPDATE_URL",
	}
	for _, v := range allVars {
		_ = os.Unsetenv(v)
	}

	for k, v := range vars {
		_ = os.Setenv(k, v)
	}

	t.Cleanup(func() {
		for k := range vars {
			_ = os.Unsetenv(k)
		}
	})
}
