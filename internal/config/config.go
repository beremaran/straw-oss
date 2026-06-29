// Package config provides application configuration loading and validation.
package config

import (
	"os"
	"strconv"
	"strings"
	"time"
)

const (
	defaultHTTPPort                  = 8080
	defaultManagementPort            = 8081
	defaultShutdownTimeout           = 30 * time.Second
	defaultManagementAccessTokenTTL  = 15 * time.Minute
	defaultResultTimeout             = 30 * time.Second
	defaultMaxConcurrentRequests     = 50
	defaultRedisPoolSize             = 100
	defaultRedisMinIdleConns         = 10
	defaultMetricsPort               = 9090
	defaultConcurrencyLimit          = 25
	defaultSelfUpdateInterval        = 5 * time.Minute
	defaultMaxPoolHosts              = 1000
	defaultIdleConnsPerHost          = 10
	defaultIdleConnTimeout           = 90 * time.Second
	defaultManagementRefreshTokenTTL = 7 * 24 * time.Hour
	defaultReportArtifactDir         = "data/reports"
	defaultReportSchedulerInterval   = time.Minute
)

// NATSConfig holds NATS connection settings.
type NATSConfig struct {
	URL   string
	Token string
}

// DatabaseConfig holds database connection settings.
type DatabaseConfig struct {
	DSN         string
	AutoMigrate bool
}

// RedisConfig holds Redis connection settings.
type RedisConfig struct {
	Addr         string
	PoolSize     int
	MinIdleConns int
}

// SecurityConfig holds security-related settings.
type SecurityConfig struct {
	HMACSecret  string
	TLSCertFile string
	TLSKeyFile  string
	VaultAddr   string
}

// ObservabilityConfig holds observability settings (logging, metrics, tracing).
type ObservabilityConfig struct {
	LogLevel       string
	LogFormat      string
	MetricsEnabled bool
	MetricsPort    int
	OTELEndpoint   string
}

// ServerConfig holds server configuration.
type ServerConfig struct {
	NATS                          NATSConfig
	Database                      DatabaseConfig
	Redis                         RedisConfig
	Security                      SecurityConfig
	Observability                 ObservabilityConfig
	HTTPPort                      int
	ManagementPort                int
	ShutdownTimeout               time.Duration
	ManagementAPIKey              string
	ManagementLegacyTokenDisabled bool
	ManagementAccessTokenTTL      time.Duration
	ManagementRefreshTokenTTL     time.Duration
	ResultTimeout                 time.Duration
	MaxBodySize                   string
	MaxConcurrentRequests         int
	AllowPrivateIPs               bool
	ReportArtifactDir             string
	ReportSchedulerInterval       time.Duration
}

// EndpointConfig holds endpoint-specific configuration.
type EndpointConfig struct {
	NATS               NATSConfig
	Security           SecurityConfig
	Observability      ObservabilityConfig
	ID                 string
	Tags               []string
	ConcurrencyLimit   int
	SelfUpdateURL      string
	SelfUpdateInterval time.Duration
	SelfUpdateEnabled  bool
	MaxPoolHosts       int
	IdleConnsPerHost   int
	IdleConnTimeout    time.Duration
	LogStreamEnabled   bool
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

// LoadServerConfig loads server configuration from environment variables.
func LoadServerConfig() (*ServerConfig, error) {
	cfg := &ServerConfig{
		Database:                      LoadDatabaseConfig(),
		Redis:                         LoadRedisConfig(),
		NATS:                          LoadNATSConfig(),
		Security:                      LoadSecurityConfig(),
		Observability:                 LoadObservabilityConfig(),
		HTTPPort:                      getEnvInt("HTTP_PORT", defaultHTTPPort),
		ManagementPort:                getEnvInt("MANAGEMENT_PORT", defaultManagementPort),
		ShutdownTimeout:               getEnvDuration("SHUTDOWN_TIMEOUT", defaultShutdownTimeout),
		ManagementAPIKey:              getEnv("MANAGEMENT_API_KEY", ""),
		ManagementLegacyTokenDisabled: !getEnvBool("MANAGEMENT_LEGACY_TOKEN_ENABLED", true),
		ManagementAccessTokenTTL:      getEnvDuration("MANAGEMENT_ACCESS_TOKEN_TTL", defaultManagementAccessTokenTTL),
		ManagementRefreshTokenTTL:     getEnvDuration("MANAGEMENT_REFRESH_TOKEN_TTL", defaultManagementRefreshTokenTTL),
		ResultTimeout:                 getEnvDuration("RESULT_TIMEOUT", defaultResultTimeout),
		MaxBodySize:                   getEnv("MAX_BODY_SIZE", "2M"),
		MaxConcurrentRequests:         getEnvInt("MAX_CONCURRENT_REQUESTS", defaultMaxConcurrentRequests),
		AllowPrivateIPs:               getEnvBool("ALLOW_PRIVATE_IPS", false),
		ReportArtifactDir:             getEnv("REPORT_ARTIFACT_DIR", defaultReportArtifactDir),
		ReportSchedulerInterval:       getEnvDuration("REPORT_SCHEDULER_INTERVAL", defaultReportSchedulerInterval),
	}

	err := validateServerConfig(cfg)
	if err != nil {
		return nil, err
	}

	return cfg, nil
}

// LoadRedisConfig loads Redis configuration from environment variables.
func LoadRedisConfig() RedisConfig {
	return RedisConfig{
		Addr:         getEnv("REDIS_ADDR", "localhost:6379"),
		PoolSize:     getEnvInt("REDIS_POOL_SIZE", defaultRedisPoolSize),
		MinIdleConns: getEnvInt("REDIS_MIN_IDLE_CONNS", defaultRedisMinIdleConns),
	}
}

// LoadDatabaseConfig loads database configuration from environment variables.
func LoadDatabaseConfig() DatabaseConfig {
	return DatabaseConfig{
		DSN:         getEnv("POSTGRES_DSN", ""),
		AutoMigrate: getEnvBool("DB_AUTO_MIGRATE", false),
	}
}

// LoadObservabilityConfig loads observability configuration from environment variables.
func LoadObservabilityConfig() ObservabilityConfig {
	return ObservabilityConfig{
		LogLevel:       getEnv("LOG_LEVEL", "info"),
		LogFormat:      getEnv("LOG_FORMAT", "json"),
		OTELEndpoint:   getEnv("OTEL_EXPORTER_OTLP_ENDPOINT", ""),
		MetricsEnabled: getEnvBool("METRICS_ENABLED", true),
		MetricsPort:    getEnvInt("METRICS_PORT", defaultMetricsPort),
	}
}

// LoadSecurityConfig loads security configuration from environment variables.
func LoadSecurityConfig() SecurityConfig {
	return SecurityConfig{
		HMACSecret:  getEnv("HMAC_SECRET", ""),
		TLSCertFile: getEnv("TLS_CERT_FILE", ""),
		TLSKeyFile:  getEnv("TLS_KEY_FILE", ""),
		VaultAddr:   getEnv("VAULT_ADDR", ""),
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
	tagsStr := getEnv("ENDPOINT_TAGS", "")

	cfg := &EndpointConfig{
		NATS:               LoadNATSConfig(),
		Security:           LoadSecurityConfig(),
		Observability:      LoadObservabilityConfig(),
		ID:                 getEnv("ENDPOINT_ID", ""),
		Tags:               parseCommaSeparated(tagsStr),
		ConcurrencyLimit:   getEnvInt("CONCURRENCY_LIMIT", defaultConcurrencyLimit),
		SelfUpdateURL:      getEnv("SELF_UPDATE_URL", ""),
		SelfUpdateInterval: getEnvDuration("SELF_UPDATE_INTERVAL", defaultSelfUpdateInterval),
		SelfUpdateEnabled:  getEnvBool("SELF_UPDATE_ENABLED", true),
		MaxPoolHosts:       getEnvInt("MAX_POOL_HOSTS", defaultMaxPoolHosts),
		IdleConnsPerHost:   getEnvInt("IDLE_CONNS_PER_HOST", defaultIdleConnsPerHost),
		IdleConnTimeout:    getEnvDuration("IDLE_CONN_TIMEOUT", defaultIdleConnTimeout),
		LogStreamEnabled:   getEnvBool("ENDPOINT_LOG_STREAM_ENABLED", false),
	}

	err := validateEndpointConfig(cfg)
	if err != nil {
		return nil, err
	}

	return cfg, nil
}

func validateServerConfig(cfg *ServerConfig) error {
	var errs []string

	if cfg.Database.DSN == "" {
		errs = append(errs, "POSTGRES_DSN is required")
	}

	if cfg.NATS.URL == "" {
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
