// Package config provides configuration management for the Straw Proxy system.
// It uses Viper for loading configuration from environment variables and config files.
package config

import (
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/spf13/viper"
)

// CoreConfig contains core infrastructure settings shared by Server and Endpoint.
type CoreConfig struct {
	PostgresDSN       string `mapstructure:"postgres_dsn"`         // PostgreSQL connection string
	DBAutoMigrate     bool   `mapstructure:"db_auto_migrate"`      // Automatically run migrations on startup
	RedisAddr         string `mapstructure:"redis_addr"`           // Redis address (host:port)
	RedisPoolSize     int    `mapstructure:"redis_pool_size"`      // Max number of socket connections
	RedisMinIdleConns int    `mapstructure:"redis_min_idle_conns"` // Min number of idle connections
	// RabbitMQURL   string `mapstructure:"rabbitmq_url"`    // RabbitMQ connection URL -- DEPRECATED
	NatsURL   string `mapstructure:"nats_url"`   // NATS connection URL
	NatsToken string `mapstructure:"nats_token"` // NATS authentication token
	LogLevel  string `mapstructure:"log_level"`  // Logging level (debug, info, warn, error)
	LogFormat string `mapstructure:"log_format"` // Log format (json, text)
}

// SecurityConfig contains security-related settings.
type SecurityConfig struct {
	HMACSecret  string `mapstructure:"hmac_secret"`   // Shared secret for task signing
	TLSCertFile string `mapstructure:"tls_cert_file"` // Path to TLS certificate
	TLSKeyFile  string `mapstructure:"tls_key_file"`  // Path to TLS private key
	VaultAddr   string `mapstructure:"vault_addr"`    // HashiCorp Vault address (optional)
}

// ObservabilityConfig contains observability settings.
type ObservabilityConfig struct {
	OTELEndpoint   string `mapstructure:"otel_exporter_otlp_endpoint"` // OpenTelemetry collector endpoint
	MetricsEnabled bool   `mapstructure:"metrics_enabled"`             // Enable Prometheus metrics
	MetricsPort    int    `mapstructure:"metrics_port"`                // Prometheus metrics port
}

// ServerConfig is the full configuration for the Relay Server (The Brain).
type ServerConfig struct {
	Core          CoreConfig          `mapstructure:",squash"`
	Security      SecurityConfig      `mapstructure:",squash"`
	Observability ObservabilityConfig `mapstructure:",squash"`

	// Server-specific settings
	HTTPPort              int           `mapstructure:"http_port"`               // HTTP server port
	AdminPort             int           `mapstructure:"admin_port"`              // Admin API port
	ShutdownTimeout       time.Duration `mapstructure:"shutdown_timeout"`        // Graceful shutdown timeout
	AdminAPIKey           string        `mapstructure:"admin_api_key"`           // Admin API authentication key
	ResultTimeout         time.Duration `mapstructure:"result_timeout"`          // Timeout for waiting on endpoint results (default: 30s)
	MaxBodySize           string        `mapstructure:"max_body_size"`           // Max request body size (default: 2M)
	MaxConcurrentRequests int           `mapstructure:"max_concurrent_requests"` // Max concurrent relay requests (default: 50)

	// Testing options
	AllowPrivateIPs bool `mapstructure:"allow_private_ips"` // Allow localhost/private IPs (for testing only)
}

// EndpointConfig is the configuration for the Endpoint Worker (The Muscle).
type EndpointConfig struct {
	Core          CoreConfig          `mapstructure:",squash"`
	Security      SecurityConfig      `mapstructure:",squash"`
	Observability ObservabilityConfig `mapstructure:",squash"`

	// Endpoint-specific settings
	ID                 string        `mapstructure:"endpoint_id"`          // Unique identifier for this endpoint
	Tags               []string      `mapstructure:"endpoint_tags"`        // Capability tags (parsed from comma-separated)
	ConcurrencyLimit   int           `mapstructure:"concurrency_limit"`    // Max concurrent requests
	SelfUpdateURL      string        `mapstructure:"self_update_url"`      // Binary update URL (optional)
	SelfUpdateInterval time.Duration `mapstructure:"self_update_interval"` // Check interval (default: 5m)
	SelfUpdateEnabled  bool          `mapstructure:"self_update_enabled"`  // Enable auto-update (default: true if URL set)

	// Connection pool settings (Section 2.3 of design doc)
	MaxPoolHosts     int           `mapstructure:"max_pool_hosts"`      // Max distinct hosts to pool (default: 1000)
	IdleConnsPerHost int           `mapstructure:"idle_conns_per_host"` // Keep-alive connections per target (default: 10)
	IdleConnTimeout  time.Duration `mapstructure:"idle_conn_timeout"`   // Close idle connections after (default: 90s)
}

// LoadServerConfig loads the server configuration from environment variables and optional config file.
func LoadServerConfig(configPath string) (*ServerConfig, error) {
	v := viper.New()
	setDefaults(v)
	setServerDefaults(v)
	bindEnvVars(v)

	if configPath != "" {
		v.SetConfigFile(configPath)
		if err := v.ReadInConfig(); err != nil {
			// Config file is optional, only error if file was specified but couldn't be read
			var configFileNotFoundError viper.ConfigFileNotFoundError
			if !errors.As(err, &configFileNotFoundError) {
				return nil, fmt.Errorf("failed to read config file: %w", err)
			}
		}
	}

	var cfg ServerConfig
	if err := v.Unmarshal(&cfg); err != nil {
		return nil, fmt.Errorf("failed to unmarshal config: %w", err)
	}

	if err := validateServerConfig(&cfg); err != nil {
		return nil, err
	}

	return &cfg, nil
}

// LoadEndpointConfig loads the endpoint configuration from environment variables and optional config file.
func LoadEndpointConfig(configPath string) (*EndpointConfig, error) {
	v := viper.New()
	setDefaults(v)
	setEndpointDefaults(v)
	bindEnvVars(v)

	if configPath != "" {
		v.SetConfigFile(configPath)
		if err := v.ReadInConfig(); err != nil {
			var configFileNotFoundError viper.ConfigFileNotFoundError
			if !errors.As(err, &configFileNotFoundError) {
				return nil, fmt.Errorf("failed to read config file: %w", err)
			}
		}
	}

	var cfg EndpointConfig
	if err := v.Unmarshal(&cfg); err != nil {
		return nil, fmt.Errorf("failed to unmarshal config: %w", err)
	}

	// Always parse endpoint tags from the raw string to ensure proper trimming
	// Viper may unmarshal comma-separated values into a slice without trimming whitespace
	if tagsStr := v.GetString("endpoint_tags"); tagsStr != "" {
		cfg.Tags = parseCommaSeparated(tagsStr)
	}

	if err := validateEndpointConfig(&cfg); err != nil {
		return nil, err
	}

	return &cfg, nil
}

// setDefaults sets default values shared across configurations.
func setDefaults(v *viper.Viper) {
	// Core defaults
	v.SetDefault("redis_addr", "localhost:6379")
	v.SetDefault("redis_pool_size", 100)
	v.SetDefault("redis_min_idle_conns", 10)
	v.SetDefault("db_auto_migrate", false)
	v.SetDefault("nats_url", "nats://localhost:4222")
	v.SetDefault("nats_token", "")
	v.SetDefault("log_level", "info")
	v.SetDefault("log_format", "json")

	// Observability defaults
	v.SetDefault("metrics_enabled", true)
	v.SetDefault("metrics_port", 9090)
}

// setServerDefaults sets server-specific default values.
func setServerDefaults(v *viper.Viper) {
	v.SetDefault("http_port", 8080)
	v.SetDefault("admin_port", 8081)
	v.SetDefault("shutdown_timeout", 30*time.Second)
	v.SetDefault("result_timeout", 30*time.Second) // Default timeout for waiting on endpoint results
	v.SetDefault("max_body_size", "2M")
	v.SetDefault("max_concurrent_requests", 50) // Default relay server concurrency limit
}

// setEndpointDefaults sets endpoint-specific default values.
func setEndpointDefaults(v *viper.Viper) {
	v.SetDefault("concurrency_limit", 25) // Reduced from 100 for better resource usage
	// Connection pool defaults per design doc Section 2.3
	v.SetDefault("max_pool_hosts", 1000)
	v.SetDefault("idle_conns_per_host", 10)
	v.SetDefault("idle_conn_timeout", 90*time.Second)
	// Self-update defaults
	v.SetDefault("self_update_interval", 5*time.Minute)
	v.SetDefault("self_update_enabled", true)
}

// bindEnvVars binds environment variables to viper keys.
func bindEnvVars(v *viper.Viper) {
	v.AutomaticEnv()
	v.SetEnvKeyReplacer(strings.NewReplacer(".", "_"))

	// Explicit bindings for clarity and to ensure proper mapping
	envBindings := []string{
		"postgres_dsn",
		"db_auto_migrate",
		"redis_addr",
		"redis_pool_size",
		"redis_min_idle_conns",
		"nats_url",
		"nats_token",
		"log_level",
		"log_format",
		"hmac_secret",
		"tls_cert_file",
		"tls_key_file",
		"vault_addr",
		"otel_exporter_otlp_endpoint",
		"metrics_enabled",
		"metrics_port",
		"http_port",
		"admin_port",
		"shutdown_timeout",
		"admin_api_key",
		"result_timeout",
		"endpoint_id",
		"endpoint_tags",
		"concurrency_limit",
		"self_update_url",
		"self_update_interval",
		"self_update_enabled",
		"max_pool_hosts",
		"idle_conns_per_host",
		"idle_conn_timeout",
		"max_body_size",
		"max_concurrent_requests",
		"allow_private_ips",
	}

	for _, key := range envBindings {
		_ = v.BindEnv(key, strings.ToUpper(key))
	}
}

// validateServerConfig validates required fields for server configuration.
func validateServerConfig(cfg *ServerConfig) error {
	var errs []string

	if cfg.Core.PostgresDSN == "" {
		errs = append(errs, "POSTGRES_DSN is required")
	}
	if cfg.Core.NatsURL == "" {
		errs = append(errs, "NATS_URL is required")
	}
	if cfg.Security.HMACSecret == "" {
		errs = append(errs, "HMAC_SECRET is required")
	}

	if len(errs) > 0 {
		return &ValidationError{Errors: errs}
	}
	return nil
}

// validateEndpointConfig validates required fields for endpoint configuration.
func validateEndpointConfig(cfg *EndpointConfig) error {
	var errs []string

	if cfg.ID == "" {
		errs = append(errs, "ENDPOINT_ID is required")
	}
	if cfg.Core.NatsURL == "" {
		errs = append(errs, "NATS_URL is required")
	}
	if cfg.Security.HMACSecret == "" {
		errs = append(errs, "HMAC_SECRET is required")
	}
	if cfg.ConcurrencyLimit <= 0 {
		errs = append(errs, "CONCURRENCY_LIMIT must be positive")
	}

	if len(errs) > 0 {
		return &ValidationError{Errors: errs}
	}
	return nil
}

// ValidationError represents configuration validation errors.
type ValidationError struct {
	Errors []string
}

func (e *ValidationError) Error() string {
	return fmt.Sprintf("configuration validation failed: %s", strings.Join(e.Errors, "; "))
}

// parseCommaSeparated splits a comma-separated string into trimmed parts.
func parseCommaSeparated(s string) []string {
	if s == "" {
		return nil
	}
	parts := strings.Split(s, ",")
	result := make([]string, 0, len(parts))
	for _, part := range parts {
		part = strings.TrimSpace(part)
		if part != "" {
			result = append(result, part)
		}
	}
	return result
}
