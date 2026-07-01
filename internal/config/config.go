// Package config provides environment-backed configuration.
package config

import (
	"encoding/json"
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
	defaultTunnelOpenTimeout     = 10 * time.Second
	defaultTunnelIdleTimeout     = 2 * time.Minute
	defaultTunnelMaxLifetime     = 30 * time.Minute
	defaultConcurrencyLimit      = 25
	defaultMaxConcurrentRequests = 50
	defaultMaxConcurrentTunnels  = 100
	defaultTunnelChunkSize       = 16 * 1024
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
	ProxyPort             int
	EgressID              string
	AuthToken             string
	Routes                []Route
	ShutdownTimeout       time.Duration
	ResultTimeout         time.Duration
	StreamTimeout         time.Duration
	MaxBodySize           string
	MaxConcurrentRequests int
	AllowPrivateIPs       bool
	MaxConcurrentTunnels  int
	TunnelOpenTimeout     time.Duration
	TunnelIdleTimeout     time.Duration
	TunnelMaxLifetime     time.Duration
	TunnelChunkSize       int
	MITMCACertFile        string
	MITMCAKeyFile         string
}

// Route is an allowed control-to-egress route.
type Route struct {
	EgressID string `json:"egress_id"`
	Country  string `json:"country"`
	IPType   string `json:"ip_type"`
}

// EgressConfig holds egress worker configuration.
type EgressConfig struct {
	NATS             NATSConfig
	ID               string
	ConcurrencyLimit int
	TunnelChunkSize  int
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
		ProxyPort:             getEnvInt("PROXY_PORT", 0),
		EgressID:              getEnv("CONTROL_EGRESS_ID", getEnv("EGRESS_ID", "")),
		AuthToken:             getEnv("CONTROL_AUTH_TOKEN", ""),
		ShutdownTimeout:       getEnvDuration("SHUTDOWN_TIMEOUT", defaultShutdownTimeout),
		ResultTimeout:         getEnvDuration("RESULT_TIMEOUT", defaultResultTimeout),
		StreamTimeout:         getEnvDuration("NATS_STREAM_TIMEOUT", defaultStreamTimeout),
		MaxBodySize:           getEnv("MAX_BODY_SIZE", "2M"),
		MaxConcurrentRequests: getEnvInt("MAX_CONCURRENT_REQUESTS", defaultMaxConcurrentRequests),
		AllowPrivateIPs:       getEnvBool("ALLOW_PRIVATE_IPS", false),
		MaxConcurrentTunnels:  getEnvInt("MAX_CONCURRENT_TUNNELS", defaultMaxConcurrentTunnels),
		TunnelOpenTimeout:     getEnvDuration("TUNNEL_OPEN_TIMEOUT", defaultTunnelOpenTimeout),
		TunnelIdleTimeout:     getEnvDuration("TUNNEL_IDLE_TIMEOUT", defaultTunnelIdleTimeout),
		TunnelMaxLifetime:     getEnvDuration("TUNNEL_MAX_LIFETIME", defaultTunnelMaxLifetime),
		TunnelChunkSize:       getEnvInt("TUNNEL_CHUNK_SIZE", defaultTunnelChunkSize),
		MITMCACertFile:        getEnv("MITM_CA_CERT_FILE", ""),
		MITMCAKeyFile:         getEnv("MITM_CA_KEY_FILE", ""),
	}

	routes, err := parseRoutes(getEnv("CONTROL_ROUTES", ""))
	if err != nil {
		return nil, err
	}

	cfg.Routes = routes

	err = validateControlConfig(cfg)
	if err != nil {
		return nil, err
	}

	return cfg, nil
}

// LoadControlConfigWithFlags loads control configuration from CLI flags,
// falling back to environment variables, then defaults.
func LoadControlConfigWithFlags(fs *flag.FlagSet) (*ControlConfig, error) {
	natsURL := fs.String("nats-url", getEnv("NATS_URL", "nats://localhost:4222"), "NATS connection URL")
	natsToken := fs.String("nats-token", getEnv("NATS_TOKEN", ""), "NATS auth token")
	httpPort := fs.Int("http-port", getEnvInt("HTTP_PORT", defaultHTTPPort), "HTTP listen port")
	egressID := fs.String("egress-id", getEnv("CONTROL_EGRESS_ID", getEnv("EGRESS_ID", "")), "Egress worker ID")
	shutdownTimeout := fs.Duration("shutdown-timeout", getEnvDuration("SHUTDOWN_TIMEOUT", defaultShutdownTimeout), "Shutdown timeout duration")
	resultTimeout := fs.Duration("result-timeout", getEnvDuration("RESULT_TIMEOUT", defaultResultTimeout), "Result read timeout duration")
	streamTimeout := fs.Duration("stream-timeout", getEnvDuration("NATS_STREAM_TIMEOUT", defaultStreamTimeout), "NATS stream declaration timeout")
	maxBodySize := fs.String("max-body-size", getEnv("MAX_BODY_SIZE", "2M"), "Maximum request body size (e.g. 2M, 10K)")
	maxConcurrentRequests := fs.Int("max-concurrent-requests", getEnvInt("MAX_CONCURRENT_REQUESTS", defaultMaxConcurrentRequests), "Maximum concurrent requests")
	allowPrivateIPs := fs.Bool("allow-private-ips", getEnvBool("ALLOW_PRIVATE_IPS", false), "Allow requests to private IP addresses")
	authToken := fs.String("control-auth-token", getEnv("CONTROL_AUTH_TOKEN", ""), "Control API bearer token")
	routesJSON := fs.String("control-routes", getEnv("CONTROL_ROUTES", ""), "Allowed egress routes JSON")
	proxyFlags := controlProxyFlagsFrom(fs)
	tlsCertFile := fs.String("tls-cert-file", getEnv("TLS_CERT_FILE", ""), "TLS certificate file path")
	tlsKeyFile := fs.String("tls-key-file", getEnv("TLS_KEY_FILE", ""), "TLS key file path")

	err := fs.Parse(os.Args[1:])
	if err != nil {
		return nil, fmt.Errorf("parse flags: %w", err)
	}

	routes, err := parseRoutes(*routesJSON)
	if err != nil {
		return nil, err
	}

	cfg := controlConfigFromFlags(controlFlagValues{
		natsURL, natsToken, httpPort, egressID, shutdownTimeout, resultTimeout,
		streamTimeout, maxBodySize, maxConcurrentRequests, allowPrivateIPs,
		authToken, routes, tlsCertFile, tlsKeyFile, proxyFlags,
	})

	err = validateControlConfig(cfg)
	if err != nil {
		return nil, err
	}

	return cfg, nil
}

type controlFlagValues struct {
	natsURL               *string
	natsToken             *string
	httpPort              *int
	egressID              *string
	shutdownTimeout       *time.Duration
	resultTimeout         *time.Duration
	streamTimeout         *time.Duration
	maxBodySize           *string
	maxConcurrentRequests *int
	allowPrivateIPs       *bool
	authToken             *string
	routes                []Route
	tlsCertFile           *string
	tlsKeyFile            *string
	proxyFlags            controlProxyFlags
}

func controlConfigFromFlags(v controlFlagValues) *ControlConfig {
	return &ControlConfig{
		NATS:                  NATSConfig{URL: *v.natsURL, Token: *v.natsToken},
		Security:              SecurityConfig{TLSCertFile: *v.tlsCertFile, TLSKeyFile: *v.tlsKeyFile},
		HTTPPort:              *v.httpPort,
		ProxyPort:             *v.proxyFlags.proxyPort,
		EgressID:              *v.egressID,
		AuthToken:             *v.authToken,
		Routes:                v.routes,
		ShutdownTimeout:       *v.shutdownTimeout,
		ResultTimeout:         *v.resultTimeout,
		StreamTimeout:         *v.streamTimeout,
		MaxBodySize:           *v.maxBodySize,
		MaxConcurrentRequests: *v.maxConcurrentRequests,
		AllowPrivateIPs:       *v.allowPrivateIPs,
		MaxConcurrentTunnels:  *v.proxyFlags.maxConcurrentTunnels,
		TunnelOpenTimeout:     *v.proxyFlags.tunnelOpenTimeout,
		TunnelIdleTimeout:     *v.proxyFlags.tunnelIdleTimeout,
		TunnelMaxLifetime:     *v.proxyFlags.tunnelMaxLifetime,
		TunnelChunkSize:       *v.proxyFlags.tunnelChunkSize,
		MITMCACertFile:        *v.proxyFlags.mitmCACertFile,
		MITMCAKeyFile:         *v.proxyFlags.mitmCAKeyFile,
	}
}

type controlProxyFlags struct {
	proxyPort            *int
	maxConcurrentTunnels *int
	tunnelOpenTimeout    *time.Duration
	tunnelIdleTimeout    *time.Duration
	tunnelMaxLifetime    *time.Duration
	tunnelChunkSize      *int
	mitmCACertFile       *string
	mitmCAKeyFile        *string
}

func controlProxyFlagsFrom(fs *flag.FlagSet) controlProxyFlags {
	return controlProxyFlags{
		proxyPort:            fs.Int("proxy-port", getEnvInt("PROXY_PORT", 0), "HTTP proxy listen port"),
		maxConcurrentTunnels: fs.Int("max-concurrent-tunnels", getEnvInt("MAX_CONCURRENT_TUNNELS", defaultMaxConcurrentTunnels), "Maximum concurrent proxy tunnels"),
		tunnelOpenTimeout:    fs.Duration("tunnel-open-timeout", getEnvDuration("TUNNEL_OPEN_TIMEOUT", defaultTunnelOpenTimeout), "Tunnel open timeout"),
		tunnelIdleTimeout:    fs.Duration("tunnel-idle-timeout", getEnvDuration("TUNNEL_IDLE_TIMEOUT", defaultTunnelIdleTimeout), "Tunnel idle timeout"),
		tunnelMaxLifetime:    fs.Duration("tunnel-max-lifetime", getEnvDuration("TUNNEL_MAX_LIFETIME", defaultTunnelMaxLifetime), "Tunnel max lifetime"),
		tunnelChunkSize:      fs.Int("tunnel-chunk-size", getEnvInt("TUNNEL_CHUNK_SIZE", defaultTunnelChunkSize), "Tunnel chunk size"),
		mitmCACertFile:       fs.String("mitm-ca-cert-file", getEnv("MITM_CA_CERT_FILE", ""), "MITM CA certificate file"),
		mitmCAKeyFile:        fs.String("mitm-ca-key-file", getEnv("MITM_CA_KEY_FILE", ""), "MITM CA key file"),
	}
}

func parseRoutes(raw string) ([]Route, error) {
	if strings.TrimSpace(raw) == "" {
		return nil, nil
	}

	var routes []Route

	err := json.Unmarshal([]byte(raw), &routes)
	if err != nil {
		return nil, fmt.Errorf("parse CONTROL_ROUTES: %w", err)
	}

	return routes, nil
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
		ID:               getEnv("EGRESS_ID", ""),
		ConcurrencyLimit: getEnvInt("CONCURRENCY_LIMIT", defaultConcurrencyLimit),
		TunnelChunkSize:  getEnvInt("TUNNEL_CHUNK_SIZE", defaultTunnelChunkSize),
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
	natsURL := fs.String("nats-url", getEnv("NATS_URL", "nats://localhost:4222"), "NATS connection URL")
	natsToken := fs.String("nats-token", getEnv("NATS_TOKEN", ""), "NATS auth token")
	egressID := fs.String("egress-id", getEnv("EGRESS_ID", ""), "Egress worker ID")
	concurrencyLimit := fs.Int("concurrency-limit", getEnvInt("CONCURRENCY_LIMIT", defaultConcurrencyLimit), "Maximum concurrent requests per worker")
	tunnelChunkSize := fs.Int("tunnel-chunk-size", getEnvInt("TUNNEL_CHUNK_SIZE", defaultTunnelChunkSize), "Tunnel chunk size")

	err := fs.Parse(os.Args[1:])
	if err != nil {
		return nil, fmt.Errorf("parse flags: %w", err)
	}

	cfg := &EgressConfig{
		NATS: NATSConfig{
			URL:   *natsURL,
			Token: *natsToken,
		},
		ID:               *egressID,
		ConcurrencyLimit: *concurrencyLimit,
		TunnelChunkSize:  *tunnelChunkSize,
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

	for i, route := range cfg.Routes {
		if route.EgressID == "" {
			errs = append(errs, fmt.Sprintf("CONTROL_ROUTES[%d].egress_id is required", i))
		}
	}

	if cfg.MaxConcurrentRequests <= 0 {
		errs = append(errs, "MAX_CONCURRENT_REQUESTS must be positive")
	}

	errs = append(errs, validateControlProxyConfig(cfg)...)

	if len(errs) > 0 {
		return &ValidationError{Errors: errs}
	}

	return nil
}

func validateControlProxyConfig(cfg *ControlConfig) []string {
	var errs []string

	if cfg.ProxyPort != 0 && cfg.AuthToken == "" {
		errs = append(errs, "CONTROL_AUTH_TOKEN is required when PROXY_PORT is set")
	}

	if cfg.MaxConcurrentTunnels <= 0 {
		errs = append(errs, "MAX_CONCURRENT_TUNNELS must be positive")
	}

	if cfg.TunnelChunkSize <= 0 {
		errs = append(errs, "TUNNEL_CHUNK_SIZE must be positive")
	}

	return errs
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

	if cfg.TunnelChunkSize <= 0 {
		errs = append(errs, "TUNNEL_CHUNK_SIZE must be positive")
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
