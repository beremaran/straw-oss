package config

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"syscall"
)

// Version is the supported configuration format version.
const (
	Version                 = "v1"
	defaultEgressHealthPort = 8090
	defaultNATSServer       = "nats://127.0.0.1:4222"
)

var (
	errMissingControlSection  = errors.New("missing control section")
	errMissingEgressSection   = errors.New("missing egress section")
	errUnexpectedTrailingJSON = errors.New("unexpected trailing JSON data")
	errInvalidConfigVersion   = errors.New("invalid config_version")
	errInvalidAPIPort         = errors.New("server.api_port must be between 1 and 65535")
	errInvalidMetricsPort     = errors.New("server.metrics_port must be between 1 and 65535")
	errInvalidHealthPort      = errors.New("health_port must be between 1 and 65535")
	errInvalidHeartbeat       = errors.New("heartbeat_interval_ms must be positive")
	errOpenConfig             = errors.New("open config file")
)

// File is the versioned top-level JSON configuration envelope.
type File struct {
	ConfigVersion string         `json:"config_version"`
	Control       *ControlConfig `json:"control,omitempty"`
	Egress        *EgressConfig  `json:"egress,omitempty"`
}

// ControlConfig configures the Control service.
type ControlConfig struct {
	Server    ControlServerConfig    `json:"server"`
	Request   ControlRequestConfig   `json:"request"`
	Transport ControlTransportConfig `json:"transport"`
	NATS      NATSConfig             `json:"nats"`
}

// ControlServerConfig configures Control's HTTP listeners.
type ControlServerConfig struct {
	Host        string `json:"host"`
	APIPort     int    `json:"api_port"`
	MetricsPort int    `json:"metrics_port"`
}

// ControlRequestConfig configures request limits.
type ControlRequestConfig struct {
	MaxInlineRequestBodyBytes  uint64 `json:"max_inline_request_body_bytes"`
	MaxInlineResponseBodyBytes uint64 `json:"max_inline_response_body_bytes"`
	MaxTimeoutMs               uint64 `json:"max_timeout_ms"`
}

// ControlTransportConfig configures internal stream frames.
type ControlTransportConfig struct {
	MaxFrameDataBytes uint64 `json:"max_frame_data_bytes"`
}

// NATSConfig configures a NATS connection.
type NATSConfig struct {
	Servers             []string `json:"servers"`
	UserCredentialsFile string   `json:"user_credentials_file,omitempty"`
	UsernameEnv         string   `json:"username_env,omitempty"`
	PasswordEnv         string   `json:"password_env,omitempty"`
	ReconnectAttempts   int      `json:"reconnect_attempts,omitempty"`
	ReconnectWaitMS     int      `json:"reconnect_wait_ms,omitempty"`
	PingIntervalMS      int      `json:"ping_interval_ms,omitempty"`
	MaxPingFailures     int      `json:"max_ping_failures,omitempty"`
	MaxPayloadBytes     *uint64  `json:"max_payload_bytes,omitempty"`
}

// EgressConfig configures an Egress worker.
type EgressConfig struct {
	WorkerID               string                             `json:"worker_id"`
	HeartbeatIntervalMs    int                                `json:"heartbeat_interval_ms"`
	HealthPort             int                                `json:"health_port"`
	NATS                   NATSConfig                         `json:"nats"`
	Capabilities           EgressCapabilities                 `json:"capabilities,omitzero"`
	UpstreamConnectionPool EgressUpstreamConnectionPoolConfig `json:"upstream_connection_pool"`
	HTTP2                  EgressHTTP2Config                  `json:"http2"`
}

// EgressCapabilities describes a worker's routing capabilities.
type EgressCapabilities struct {
	Tags                  []string `json:"tags,omitempty"`
	Countries             []string `json:"countries,omitempty"`
	Regions               []string `json:"regions,omitempty"`
	IPTypes               []string `json:"ip_types,omitempty"`
	SupportedIngressModes []string `json:"supported_ingress_modes,omitempty"`
	MaxConcurrency        uint32   `json:"max_concurrency,omitempty"`
}

// EgressUpstreamConnectionPoolConfig configures optional upstream reuse.
type EgressUpstreamConnectionPoolConfig struct {
	Enabled             bool `json:"enabled"`
	MaxIdleConnsPerHost int  `json:"max_idle_conns_per_host"`
	IdleTimeoutMS       int  `json:"idle_timeout_ms"`
	MaxLifetimeMS       int  `json:"max_lifetime_ms"`
}

// EgressHTTP2Config configures outbound HTTP/2.
type EgressHTTP2Config struct {
	Enabled            bool `json:"enabled"`
	FallbackCacheTTLMS int  `json:"fallback_cache_ttl_ms"`
}

// LoadControl reads a Control JSON configuration file.
func LoadControl(path string) (ControlConfig, error) {
	file, err := loadFile(path)
	if err != nil {
		return ControlConfig{}, err
	}

	if file.Control == nil {
		return ControlConfig{}, errMissingControlSection
	}

	file.Control.applyDefaults()

	err = file.Control.validate()
	if err != nil {
		return ControlConfig{}, err
	}

	return *file.Control, nil
}

// DefaultControl returns the local Control defaults.
func DefaultControl() ControlConfig {
	cfg := ControlConfig{}
	cfg.applyDefaults()

	return cfg
}

// LoadEgress reads an Egress JSON configuration file.
func LoadEgress(path string) (EgressConfig, error) {
	file, err := loadFile(path)
	if err != nil {
		return EgressConfig{}, err
	}

	if file.Egress == nil {
		return EgressConfig{}, errMissingEgressSection
	}

	file.Egress.applyDefaults()

	err = file.Egress.validate()
	if err != nil {
		return EgressConfig{}, err
	}

	return *file.Egress, nil
}

// DefaultEgress returns the local worker defaults.
func DefaultEgress() EgressConfig {
	cfg := EgressConfig{}
	cfg.applyDefaults()

	return cfg
}

func loadFile(path string) (File, error) {
	fd, err := syscall.Open(path, syscall.O_RDONLY, 0)
	if err != nil {
		return File{}, fmt.Errorf("read config file: %w", err)
	}

	fileHandle := os.NewFile(uintptr(fd), path)
	if fileHandle == nil {
		_ = syscall.Close(fd)

		return File{}, errOpenConfig
	}

	defer func() { _ = fileHandle.Close() }()

	raw, err := io.ReadAll(fileHandle)
	if err != nil {
		return File{}, fmt.Errorf("read config file: %w", err)
	}

	decoder := json.NewDecoder(bytes.NewReader(raw))
	decoder.DisallowUnknownFields()

	var file File

	err = decoder.Decode(&file)
	if err != nil {
		return File{}, fmt.Errorf("decode config: %w", err)
	}

	var extra any

	err = decoder.Decode(&extra)
	if err == nil {
		return File{}, errUnexpectedTrailingJSON
	}

	if !errors.Is(err, io.EOF) {
		return File{}, fmt.Errorf("decode trailing config: %w", err)
	}

	if file.ConfigVersion != Version {
		return File{}, fmt.Errorf("%w: must be %q", errInvalidConfigVersion, Version)
	}

	return file, nil
}

func (c *ControlConfig) applyDefaults() {
	if c.Server.Host == "" {
		c.Server.Host = "0.0.0.0"
	}

	if c.Server.APIPort == 0 {
		c.Server.APIPort = 8080
	}

	if c.Server.MetricsPort == 0 {
		c.Server.MetricsPort = 9090
	}

	if c.Request.MaxInlineRequestBodyBytes == 0 {
		c.Request.MaxInlineRequestBodyBytes = 1_048_576
	}

	if c.Request.MaxInlineResponseBodyBytes == 0 {
		c.Request.MaxInlineResponseBodyBytes = 1_048_576
	}

	if c.Request.MaxTimeoutMs == 0 {
		c.Request.MaxTimeoutMs = 120_000
	}

	if c.Transport.MaxFrameDataBytes == 0 {
		c.Transport.MaxFrameDataBytes = 1_048_576
	}

	c.NATS.applyDefaults()
}

func (c ControlConfig) validate() error {
	if c.Server.APIPort < 1 || c.Server.APIPort > 65535 {
		return fmt.Errorf("%w: %d", errInvalidAPIPort, c.Server.APIPort)
	}

	if c.Server.MetricsPort < 1 || c.Server.MetricsPort > 65535 {
		return fmt.Errorf("%w: %d", errInvalidMetricsPort, c.Server.MetricsPort)
	}

	return nil
}

func (e *EgressConfig) applyDefaults() {
	if e.WorkerID == "" {
		e.WorkerID = "egress-1"
	}

	if e.HeartbeatIntervalMs == 0 {
		e.HeartbeatIntervalMs = 5000
	}

	if e.HealthPort == 0 {
		e.HealthPort = defaultEgressHealthPort
	}

	if len(e.Capabilities.SupportedIngressModes) == 0 {
		e.Capabilities.SupportedIngressModes = []string{"rest"}
	}

	if e.HTTP2.FallbackCacheTTLMS == 0 {
		e.HTTP2.FallbackCacheTTLMS = 300_000
	}

	if e.UpstreamConnectionPool.Enabled {
		if e.UpstreamConnectionPool.MaxIdleConnsPerHost == 0 {
			e.UpstreamConnectionPool.MaxIdleConnsPerHost = 8
		}

		if e.UpstreamConnectionPool.IdleTimeoutMS == 0 {
			e.UpstreamConnectionPool.IdleTimeoutMS = 30_000
		}

		if e.UpstreamConnectionPool.MaxLifetimeMS == 0 {
			e.UpstreamConnectionPool.MaxLifetimeMS = 300_000
		}
	}

	e.NATS.applyDefaults()
}

func (e EgressConfig) validate() error {
	if e.HealthPort < 1 || e.HealthPort > 65535 {
		return fmt.Errorf("%w: %d", errInvalidHealthPort, e.HealthPort)
	}

	if e.HeartbeatIntervalMs < 1 {
		return errInvalidHeartbeat
	}

	return nil
}

func (n *NATSConfig) applyDefaults() {
	if len(n.Servers) == 0 {
		n.Servers = []string{defaultNATSServer}
	}

	if n.ReconnectAttempts == 0 {
		n.ReconnectAttempts = 10
	}

	if n.ReconnectWaitMS == 0 {
		n.ReconnectWaitMS = 2000
	}

	if n.PingIntervalMS == 0 {
		n.PingIntervalMS = 30000
	}

	if n.MaxPingFailures == 0 {
		n.MaxPingFailures = 3
	}
}
