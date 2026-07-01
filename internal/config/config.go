// Package config provides environment-backed configuration.
package config

import (
	"os"
	"strconv"
	"strings"
	"time"
)

const (
	defaultHTTPPort              = 8080
	defaultShutdownTimeout       = 30 * time.Second
	defaultResultTimeout         = 30 * time.Second
	defaultConcurrencyLimit      = 25
	defaultMaxConcurrentRequests = 50
)

// NATSConfig holds NATS connection settings.
type NATSConfig struct {
	URL   string
	Token string
}

// SecurityConfig holds optional control TLS settings.
type SecurityConfig struct {
	TLSCertFile string
	TLSKeyFile  string
}

// ControlConfig holds control configuration.
type ControlConfig struct {
	NATS                  NATSConfig
	Security              SecurityConfig
	HTTPPort              int
	EgressID              string
	ShutdownTimeout       time.Duration
	ResultTimeout         time.Duration
	MaxBodySize           string
	MaxConcurrentRequests int
	AllowPrivateIPs       bool
}

// EgressConfig holds egress worker configuration.
type EgressConfig struct {
	NATS             NATSConfig
	Security         SecurityConfig
	ID               string
	ConcurrencyLimit int
}

func getEnv(key, defaultVal string) string {
	val := os.Getenv(key)
	if val == "" {
		return defaultVal
	}

	return val
}

func getEnvBool(key string, defaultVal bool) bool {
	val := os.Getenv(key)
	if val == "" {
		return defaultVal
	}

	b, err := strconv.ParseBool(val)
	if err != nil {
		return defaultVal
	}

	return b
}

func getEnvInt(key string, defaultVal int) int {
	val := os.Getenv(key)
	if val == "" {
		return defaultVal
	}

	n, err := strconv.Atoi(val)
	if err != nil {
		return defaultVal
	}

	return n
}

func getEnvDuration(key string, defaultVal time.Duration) time.Duration {
	val := os.Getenv(key)
	if val == "" {
		return defaultVal
	}

	d, err := time.ParseDuration(val)
	if err != nil {
		return defaultVal
	}

	return d
}

// LoadControlConfig loads control configuration from environment variables.
func LoadControlConfig() (*ControlConfig, error) {
	cfg := &ControlConfig{
		NATS:                  LoadNATSConfig(),
		Security:              LoadSecurityConfig(),
		HTTPPort:              getEnvInt("HTTP_PORT", defaultHTTPPort),
		EgressID:              getEnv("CONTROL_EGRESS_ID", getEnv("EGRESS_ID", "")),
		ShutdownTimeout:       getEnvDuration("SHUTDOWN_TIMEOUT", defaultShutdownTimeout),
		ResultTimeout:         getEnvDuration("RESULT_TIMEOUT", defaultResultTimeout),
		MaxBodySize:           getEnv("MAX_BODY_SIZE", "2M"),
		MaxConcurrentRequests: getEnvInt("MAX_CONCURRENT_REQUESTS", defaultMaxConcurrentRequests),
		AllowPrivateIPs:       getEnvBool("ALLOW_PRIVATE_IPS", false),
	}

	err := validateControlConfig(cfg)
	if err != nil {
		return nil, err
	}

	return cfg, nil
}

// LoadSecurityConfig loads shared security configuration from environment variables.
func LoadSecurityConfig() SecurityConfig {
	return SecurityConfig{
		TLSCertFile: getEnv("TLS_CERT_FILE", ""),
		TLSKeyFile:  getEnv("TLS_KEY_FILE", ""),
	}
}

// LoadNATSConfig loads NATS configuration from environment variables.
func LoadNATSConfig() NATSConfig {
	return NATSConfig{
		URL:   getEnv("NATS_URL", "nats://localhost:4222"),
		Token: getEnv("NATS_TOKEN", ""),
	}
}

// LoadEgressConfig loads egress configuration from environment variables.
func LoadEgressConfig() (*EgressConfig, error) {
	cfg := &EgressConfig{
		NATS:             LoadNATSConfig(),
		Security:         LoadSecurityConfig(),
		ID:               getEnv("EGRESS_ID", ""),
		ConcurrencyLimit: getEnvInt("CONCURRENCY_LIMIT", defaultConcurrencyLimit),
	}

	err := validateEgressConfig(cfg)
	if err != nil {
		return nil, err
	}

	return cfg, nil
}

func validateControlConfig(cfg *ControlConfig) error {
	var errs []string

	if cfg.EgressID == "" {
		errs = append(errs, "CONTROL_EGRESS_ID or EGRESS_ID is required")
	}

	if cfg.NATS.URL == "" {
		errs = append(errs, "NATS_URL is required")
	}

	if cfg.MaxConcurrentRequests <= 0 {
		errs = append(errs, "MAX_CONCURRENT_REQUESTS must be positive")
	}

	if len(errs) > 0 {
		return &ValidationError{Errors: errs}
	}

	return nil
}

func validateEgressConfig(cfg *EgressConfig) error {
	var errs []string

	if cfg.ID == "" {
		errs = append(errs, "EGRESS_ID is required")
	}

	if cfg.NATS.URL == "" {
		errs = append(errs, "NATS_URL is required")
	}

	if cfg.ConcurrencyLimit <= 0 {
		errs = append(errs, "CONCURRENCY_LIMIT must be positive")
	}

	if len(errs) > 0 {
		return &ValidationError{Errors: errs}
	}

	return nil
}

// ValidationError represents a configuration validation error.
type ValidationError struct {
	Errors []string
}

func (e *ValidationError) Error() string {
	return "configuration validation failed: " + strings.Join(e.Errors, "; ")
}
