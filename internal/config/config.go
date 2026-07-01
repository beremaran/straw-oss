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
	defaultMaxPoolHosts          = 1000
	defaultIdleConnsPerHost      = 10
	defaultIdleConnTimeout       = 90 * time.Second
	defaultMaxConcurrentRequests = 50
)

// NATSConfig holds NATS connection settings.
type NATSConfig struct {
	URL   string
	Token string
}

// SecurityConfig holds shared signing and optional relay TLS settings.
type SecurityConfig struct {
	HMACSecret  string
	TLSCertFile string
	TLSKeyFile  string
}

// ServerConfig holds relay configuration.
type ServerConfig struct {
	NATS                  NATSConfig
	Security              SecurityConfig
	HTTPPort              int
	EndpointID            string
	ShutdownTimeout       time.Duration
	ResultTimeout         time.Duration
	MaxBodySize           string
	MaxConcurrentRequests int
	AllowPrivateIPs       bool
}

// EndpointConfig holds endpoint worker configuration.
type EndpointConfig struct {
	NATS             NATSConfig
	Security         SecurityConfig
	ID               string
	ConcurrencyLimit int
	MaxPoolHosts     int
	IdleConnsPerHost int
	IdleConnTimeout  time.Duration
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

// LoadServerConfig loads relay configuration from environment variables.
func LoadServerConfig() (*ServerConfig, error) {
	cfg := &ServerConfig{
		NATS:                  LoadNATSConfig(),
		Security:              LoadSecurityConfig(),
		HTTPPort:              getEnvInt("HTTP_PORT", defaultHTTPPort),
		EndpointID:            getEnv("RELAY_ENDPOINT_ID", getEnv("ENDPOINT_ID", "")),
		ShutdownTimeout:       getEnvDuration("SHUTDOWN_TIMEOUT", defaultShutdownTimeout),
		ResultTimeout:         getEnvDuration("RESULT_TIMEOUT", defaultResultTimeout),
		MaxBodySize:           getEnv("MAX_BODY_SIZE", "2M"),
		MaxConcurrentRequests: getEnvInt("MAX_CONCURRENT_REQUESTS", defaultMaxConcurrentRequests),
		AllowPrivateIPs:       getEnvBool("ALLOW_PRIVATE_IPS", false),
	}

	err := validateServerConfig(cfg)
	if err != nil {
		return nil, err
	}

	return cfg, nil
}

// LoadSecurityConfig loads shared security configuration from environment variables.
func LoadSecurityConfig() SecurityConfig {
	return SecurityConfig{
		HMACSecret:  getEnv("HMAC_SECRET", ""),
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

// LoadEndpointConfig loads endpoint configuration from environment variables.
func LoadEndpointConfig() (*EndpointConfig, error) {
	cfg := &EndpointConfig{
		NATS:             LoadNATSConfig(),
		Security:         LoadSecurityConfig(),
		ID:               getEnv("ENDPOINT_ID", ""),
		ConcurrencyLimit: getEnvInt("CONCURRENCY_LIMIT", defaultConcurrencyLimit),
		MaxPoolHosts:     getEnvInt("MAX_POOL_HOSTS", defaultMaxPoolHosts),
		IdleConnsPerHost: getEnvInt("IDLE_CONNS_PER_HOST", defaultIdleConnsPerHost),
		IdleConnTimeout:  getEnvDuration("IDLE_CONN_TIMEOUT", defaultIdleConnTimeout),
	}

	err := validateEndpointConfig(cfg)
	if err != nil {
		return nil, err
	}

	return cfg, nil
}

func validateServerConfig(cfg *ServerConfig) error {
	var errs []string

	if cfg.EndpointID == "" {
		errs = append(errs, "RELAY_ENDPOINT_ID or ENDPOINT_ID is required")
	}

	if cfg.NATS.URL == "" {
		errs = append(errs, "NATS_URL is required")
	}

	if cfg.Security.HMACSecret == "" {
		errs = append(errs, "HMAC_SECRET is required")
	}

	if cfg.MaxConcurrentRequests <= 0 {
		errs = append(errs, "MAX_CONCURRENT_REQUESTS must be positive")
	}

	if len(errs) > 0 {
		return &ValidationError{Errors: errs}
	}

	return nil
}

func validateEndpointConfig(cfg *EndpointConfig) error {
	var errs []string

	if cfg.ID == "" {
		errs = append(errs, "ENDPOINT_ID is required")
	}

	if cfg.NATS.URL == "" {
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

// ValidationError represents a configuration validation error.
type ValidationError struct {
	Errors []string
}

func (e *ValidationError) Error() string {
	return "configuration validation failed: " + strings.Join(e.Errors, "; ")
}
