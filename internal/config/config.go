package config

import (
	"fmt"
	"os"
	"strconv"
	"strings"
	"time"
)

type NATSConfig struct {
	URL   string
	Token string
}

type DatabaseConfig struct {
	DSN         string
	AutoMigrate bool
}

type RedisConfig struct {
	Addr         string
	PoolSize     int
	MinIdleConns int
}

type SecurityConfig struct {
	HMACSecret  string
	TLSCertFile string
	TLSKeyFile  string
	VaultAddr   string
}

type ObservabilityConfig struct {
	LogLevel  string
	LogFormat string

	MetricsEnabled bool
	MetricsPort    int
	OTELEndpoint   string
}

type ServerConfig struct {
	NATS          NATSConfig
	Database      DatabaseConfig
	Redis         RedisConfig
	Security      SecurityConfig
	Observability ObservabilityConfig

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

	AllowPrivateIPs bool
}

type EndpointConfig struct {
	NATS          NATSConfig
	Security      SecurityConfig
	Observability ObservabilityConfig

	ID                 string
	Tags               []string
	ConcurrencyLimit   int
	SelfUpdateURL      string
	SelfUpdateInterval time.Duration
	SelfUpdateEnabled  bool

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

func LoadServerConfig() (*ServerConfig, error) {
	cfg := &ServerConfig{
		Database:                      LoadDatabaseConfig(),
		Redis:                         LoadRedisConfig(),
		NATS:                          LoadNATSConfig(),
		Security:                      LoadSecurityConfig(),
		Observability:                 LoadObservabilityConfig(),
		HTTPPort:                      getEnvInt("HTTP_PORT", 8080),
		ManagementPort:                getEnvInt("MANAGEMENT_PORT", 8081),
		ShutdownTimeout:               getEnvDuration("SHUTDOWN_TIMEOUT", 30*time.Second),
		ManagementAPIKey:              getEnv("MANAGEMENT_API_KEY", ""),
		ManagementLegacyTokenDisabled: !getEnvBool("MANAGEMENT_LEGACY_TOKEN_ENABLED", true),
		ManagementAccessTokenTTL:      getEnvDuration("MANAGEMENT_ACCESS_TOKEN_TTL", 15*time.Minute),
		ManagementRefreshTokenTTL:     getEnvDuration("MANAGEMENT_REFRESH_TOKEN_TTL", 7*24*time.Hour),
		ResultTimeout:                 getEnvDuration("RESULT_TIMEOUT", 30*time.Second),
		MaxBodySize:                   getEnv("MAX_BODY_SIZE", "2M"),
		MaxConcurrentRequests:         getEnvInt("MAX_CONCURRENT_REQUESTS", 50),
		AllowPrivateIPs:               getEnvBool("ALLOW_PRIVATE_IPS", false),
	}

	err := validateServerConfig(cfg)
	if err != nil {
		return nil, err
	}

	return cfg, nil
}

func LoadRedisConfig() RedisConfig {
	return RedisConfig{
		Addr:         getEnv("REDIS_ADDR", "localhost:6379"),
		PoolSize:     getEnvInt("REDIS_POOL_SIZE", 100),
		MinIdleConns: getEnvInt("REDIS_MIN_IDLE_CONNS", 10),
	}
}

func LoadDatabaseConfig() DatabaseConfig {
	return DatabaseConfig{
		DSN:         getEnv("POSTGRES_DSN", ""),
		AutoMigrate: getEnvBool("DB_AUTO_MIGRATE", false),
	}
}

func LoadObservabilityConfig() ObservabilityConfig {
	return ObservabilityConfig{
		LogLevel:       getEnv("LOG_LEVEL", "info"),
		LogFormat:      getEnv("LOG_FORMAT", "json"),
		OTELEndpoint:   getEnv("OTEL_EXPORTER_OTLP_ENDPOINT", ""),
		MetricsEnabled: getEnvBool("METRICS_ENABLED", true),
		MetricsPort:    getEnvInt("METRICS_PORT", 9090),
	}
}

func LoadSecurityConfig() SecurityConfig {
	return SecurityConfig{
		HMACSecret:  getEnv("HMAC_SECRET", ""),
		TLSCertFile: getEnv("TLS_CERT_FILE", ""),
		TLSKeyFile:  getEnv("TLS_KEY_FILE", ""),
		VaultAddr:   getEnv("VAULT_ADDR", ""),
	}
}

func LoadNATSConfig() NATSConfig {
	return NATSConfig{
		URL:   getEnv("NATS_URL", "nats://localhost:4222"),
		Token: getEnv("NATS_TOKEN", ""),
	}
}

func LoadEndpointConfig() (*EndpointConfig, error) {
	tagsStr := getEnv("ENDPOINT_TAGS", "")

	cfg := &EndpointConfig{
		NATS:               LoadNATSConfig(),
		Security:           LoadSecurityConfig(),
		Observability:      LoadObservabilityConfig(),
		ID:                 getEnv("ENDPOINT_ID", ""),
		Tags:               parseCommaSeparated(tagsStr),
		ConcurrencyLimit:   getEnvInt("CONCURRENCY_LIMIT", 25),
		SelfUpdateURL:      getEnv("SELF_UPDATE_URL", ""),
		SelfUpdateInterval: getEnvDuration("SELF_UPDATE_INTERVAL", 5*time.Minute),
		SelfUpdateEnabled:  getEnvBool("SELF_UPDATE_ENABLED", true),
		MaxPoolHosts:       getEnvInt("MAX_POOL_HOSTS", 1000),
		IdleConnsPerHost:   getEnvInt("IDLE_CONNS_PER_HOST", 10),
		IdleConnTimeout:    getEnvDuration("IDLE_CONN_TIMEOUT", 90*time.Second),
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

type ValidationError struct {
	Errors []string
}

func (e *ValidationError) Error() string {
	return fmt.Sprintf("configuration validation failed: %s", strings.Join(e.Errors, "; "))
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
