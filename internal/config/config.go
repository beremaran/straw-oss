// Package config provides environment-backed configuration.
package config

import (
	"flag"
	"fmt"
	"os"
	"strconv"
	"strings"
	"time"
)

const (
	defaultHTTPPort              = 8080
	defaultShutdownTimeout       = 30 * time.Second
	defaultResultTimeout         = 30 * time.Second
	defaultStreamTimeout         = 10 * time.Second
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
	StreamTimeout         time.Duration
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
		StreamTimeout:         getEnvDuration("NATS_STREAM_TIMEOUT", defaultStreamTimeout),
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

// LoadControlConfigWithFlags loads control configuration from CLI flags,
// falling back to environment variables, then defaults.
func LoadControlConfigWithFlags(fs *flag.FlagSet) (*ControlConfig, error) {
	natsURL := newOptString(getEnv("NATS_URL", "nats://localhost:4222"))
	natsToken := newOptString(getEnv("NATS_TOKEN", ""))

	httpPort := newOptInt(getEnvInt("HTTP_PORT", defaultHTTPPort))
	egressID := newOptString(getEnv("CONTROL_EGRESS_ID", getEnv("EGRESS_ID", "")))
	shutdownTimeout := newOptDuration(getEnvDuration("SHUTDOWN_TIMEOUT", defaultShutdownTimeout))
	resultTimeout := newOptDuration(getEnvDuration("RESULT_TIMEOUT", defaultResultTimeout))
	streamTimeout := newOptDuration(getEnvDuration("NATS_STREAM_TIMEOUT", defaultStreamTimeout))
	maxBodySize := newOptString(getEnv("MAX_BODY_SIZE", "2M"))
	maxConcurrentRequests := newOptInt(getEnvInt("MAX_CONCURRENT_REQUESTS", defaultMaxConcurrentRequests))
	allowPrivateIPs := newOptBool(getEnvBool("ALLOW_PRIVATE_IPS", false))

	tlsCertFile := newOptString(getEnv("TLS_CERT_FILE", ""))
	tlsKeyFile := newOptString(getEnv("TLS_KEY_FILE", ""))

	fs.Var(natsURL, "nats-url", "NATS connection URL")
	fs.Var(natsToken, "nats-token", "NATS auth token")
	fs.Var(httpPort, "http-port", "HTTP listen port")
	fs.Var(egressID, "egress-id", "Egress worker ID")
	fs.Var(shutdownTimeout, "shutdown-timeout", "Shutdown timeout duration")
	fs.Var(resultTimeout, "result-timeout", "Result read timeout duration")
	fs.Var(streamTimeout, "stream-timeout", "NATS stream declaration timeout")
	fs.Var(maxBodySize, "max-body-size", "Maximum request body size (e.g. 2M, 10K)")
	fs.Var(maxConcurrentRequests, "max-concurrent-requests", "Maximum concurrent requests")
	fs.Var(allowPrivateIPs, "allow-private-ips", "Allow requests to private IP addresses")
	fs.Var(tlsCertFile, "tls-cert-file", "TLS certificate file path")
	fs.Var(tlsKeyFile, "tls-key-file", "TLS key file path")

	err := fs.Parse(os.Args[1:])
	if err != nil {
		return nil, fmt.Errorf("parse flags: %w", err)
	}

	cfg := &ControlConfig{
		NATS: NATSConfig{
			URL:   natsURL.Resolve("NATS_URL", "nats://localhost:4222"),
			Token: natsToken.Resolve("NATS_TOKEN", ""),
		},
		Security: SecurityConfig{
			TLSCertFile: tlsCertFile.Resolve("TLS_CERT_FILE", ""),
			TLSKeyFile:  tlsKeyFile.Resolve("TLS_KEY_FILE", ""),
		},
		HTTPPort:              httpPort.Resolve("HTTP_PORT", defaultHTTPPort),
		EgressID:              egressID.Resolve("CONTROL_EGRESS_ID", getEnv("EGRESS_ID", "")),
		ShutdownTimeout:       shutdownTimeout.Resolve("SHUTDOWN_TIMEOUT", defaultShutdownTimeout),
		ResultTimeout:         resultTimeout.Resolve("RESULT_TIMEOUT", defaultResultTimeout),
		StreamTimeout:         streamTimeout.Resolve("NATS_STREAM_TIMEOUT", defaultStreamTimeout),
		MaxBodySize:           maxBodySize.Resolve("MAX_BODY_SIZE", "2M"),
		MaxConcurrentRequests: maxConcurrentRequests.Resolve("MAX_CONCURRENT_REQUESTS", defaultMaxConcurrentRequests),
		AllowPrivateIPs:       allowPrivateIPs.Resolve("ALLOW_PRIVATE_IPS", false),
	}

	err = validateControlConfig(cfg)
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

// LoadEgressConfigWithFlags loads egress configuration from CLI flags,
// falling back to environment variables, then defaults.
func LoadEgressConfigWithFlags(fs *flag.FlagSet) (*EgressConfig, error) {
	natsURL := newOptString(getEnv("NATS_URL", "nats://localhost:4222"))
	natsToken := newOptString(getEnv("NATS_TOKEN", ""))

	egressID := newOptString(getEnv("EGRESS_ID", ""))
	concurrencyLimit := newOptInt(getEnvInt("CONCURRENCY_LIMIT", defaultConcurrencyLimit))

	tlsCertFile := newOptString(getEnv("TLS_CERT_FILE", ""))
	tlsKeyFile := newOptString(getEnv("TLS_KEY_FILE", ""))

	fs.Var(natsURL, "nats-url", "NATS connection URL")
	fs.Var(natsToken, "nats-token", "NATS auth token")
	fs.Var(egressID, "egress-id", "Egress worker ID")
	fs.Var(concurrencyLimit, "concurrency-limit", "Maximum concurrent requests per worker")
	fs.Var(tlsCertFile, "tls-cert-file", "TLS certificate file path")
	fs.Var(tlsKeyFile, "tls-key-file", "TLS key file path")

	err := fs.Parse(os.Args[1:])
	if err != nil {
		return nil, fmt.Errorf("parse flags: %w", err)
	}

	cfg := &EgressConfig{
		NATS: NATSConfig{
			URL:   natsURL.Resolve("NATS_URL", "nats://localhost:4222"),
			Token: natsToken.Resolve("NATS_TOKEN", ""),
		},
		Security: SecurityConfig{
			TLSCertFile: tlsCertFile.Resolve("TLS_CERT_FILE", ""),
			TLSKeyFile:  tlsKeyFile.Resolve("TLS_KEY_FILE", ""),
		},
		ID:               egressID.Resolve("EGRESS_ID", ""),
		ConcurrencyLimit: concurrencyLimit.Resolve("CONCURRENCY_LIMIT", defaultConcurrencyLimit),
	}

	err = validateEgressConfig(cfg)
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

// optString is a flag.Value that tracks whether it was explicitly set.
type optString struct {
	set    bool
	val    string
	defVal string
}

func newOptString(defaultVal string) *optString {
	return &optString{defVal: defaultVal}
}

func (o *optString) Set(s string) error {
	o.val = s

	o.set = true

	return nil
}

func (o *optString) String() string {
	if o.set {
		return o.val
	}

	return o.defVal
}

// Resolve returns the CLI value if set, otherwise the env var, then the default.
func (o *optString) Resolve(envKey, defaultVal string) string {
	if o.set {
		return o.val
	}

	return getEnv(envKey, defaultVal)
}

// optInt is a flag.Value that tracks whether it was explicitly set.
type optInt struct {
	set    bool
	val    int
	defVal int
}

func newOptInt(defaultVal int) *optInt {
	return &optInt{defVal: defaultVal}
}

func (o *optInt) Set(s string) error {
	v, err := strconv.Atoi(s)
	if err != nil {
		return fmt.Errorf("parse int: %w", err)
	}

	o.val = v

	o.set = true

	return nil
}

func (o *optInt) String() string {
	return strconv.Itoa(o.val)
}

// Resolve returns the CLI value if set, otherwise the env var, then the default.
func (o *optInt) Resolve(envKey string, defaultVal int) int {
	if o.set {
		return o.val
	}

	return getEnvInt(envKey, defaultVal)
}

// optDuration is a flag.Value that tracks whether it was explicitly set.
type optDuration struct {
	set    bool
	val    time.Duration
	defVal time.Duration
}

func newOptDuration(defaultVal time.Duration) *optDuration {
	return &optDuration{defVal: defaultVal}
}

func (o *optDuration) Set(s string) error {
	v, err := time.ParseDuration(s)
	if err != nil {
		return fmt.Errorf("parse duration: %w", err)
	}

	o.val = v

	o.set = true

	return nil
}

func (o *optDuration) String() string {
	return o.val.String()
}

// Resolve returns the CLI value if set, otherwise the env var, then the default.
func (o *optDuration) Resolve(envKey string, defaultVal time.Duration) time.Duration {
	if o.set {
		return o.val
	}

	return getEnvDuration(envKey, defaultVal)
}

// optBool is a flag.Value that tracks whether it was explicitly set.
type optBool struct {
	set    bool
	val    bool
	defVal bool
}

func newOptBool(defaultVal bool) *optBool {
	return &optBool{defVal: defaultVal}
}

func (o *optBool) Set(s string) error {
	v, err := strconv.ParseBool(s)
	if err != nil {
		return fmt.Errorf("parse bool: %w", err)
	}

	o.val = v

	o.set = true

	return nil
}

func (o *optBool) String() string {
	return strconv.FormatBool(o.val)
}

// Resolve returns the CLI value if set, otherwise the env var, then the default.
func (o *optBool) Resolve(envKey string, defaultVal bool) bool {
	if o.set {
		return o.val
	}

	return getEnvBool(envKey, defaultVal)
}

// ValidationError represents a configuration validation error.
type ValidationError struct {
	Errors []string
}

func (e *ValidationError) Error() string {
	return "configuration validation failed: " + strings.Join(e.Errors, "; ")
}
