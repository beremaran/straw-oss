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

// Version is the canonical control-plane config file version.
const Version = "v1"

var (
	errMissingControlSection    = errors.New("missing control section")
	errMissingEgressSection     = errors.New("missing egress section")
	errUnexpectedTrailingJSON   = errors.New("unexpected trailing JSON data")
	errServerHostRequired       = errors.New("server.host is required")
	errWorkerIDRequired         = errors.New("worker_id is required")
	errCredentialIDRequired     = errors.New("credential_id is required")
	errInvalidConfigVersion     = errors.New("invalid config_version")
	errInvalidServerAPIPort     = errors.New("server.api_port must be between 1 and 65535")
	errInvalidServerMetricsPort = errors.New("server.metrics_port must be between 1 and 65535")
)

// File is the top-level JSON envelope for config files.
type File struct {
	ConfigVersion string         `json:"config_version"`
	Control       *ControlConfig `json:"control,omitempty"`
	Egress        *EgressConfig  `json:"egress,omitempty"`
}

// ControlConfig is the control-service config block.
type ControlConfig struct {
	Server    ControlServerConfig    `json:"server"`
	Request   ControlRequestConfig   `json:"request"`
	Transport ControlTransportConfig `json:"transport"`
	NATS      NATSConfig             `json:"nats"`
	Database  DatabaseConfig         `json:"database"`
}

// ControlServerConfig configures the control HTTP server.
type ControlServerConfig struct {
	Host        string `json:"host"`
	APIPort     int    `json:"api_port"`
	MetricsPort int    `json:"metrics_port"`
}

// ControlRequestConfig configures request body and timeout limits.
type ControlRequestConfig struct {
	MaxInlineRequestBodyBytes  uint64 `json:"max_inline_request_body_bytes"`
	MaxInlineResponseBodyBytes uint64 `json:"max_inline_response_body_bytes"`
	MaxTimeoutMs               uint64 `json:"max_timeout_ms"`
}

// ControlTransportConfig configures the control transport limits.
type ControlTransportConfig struct {
	MaxFrameDataBytes uint64 `json:"max_frame_data_bytes"`
}

// NATSConfig configures the NATS client connection.
type NATSConfig struct {
	Servers             []string `json:"servers"`
	UserCredentialsFile string   `json:"user_credentials_file"`
	ReconnectAttempts   int      `json:"reconnect_attempts"`
	ReconnectWaitMS     int      `json:"reconnect_wait_ms"`
	PingIntervalMS      int      `json:"ping_interval_ms"`
	MaxPingFailures     int      `json:"max_ping_failures"`
	MaxPayloadBytes     *uint64  `json:"max_payload_bytes"`
}

// PostgresConfig configures the Postgres connection pool.
type PostgresConfig struct {
	DSNEnv            string `json:"dsn_env"`
	MaxOpenConns      int    `json:"max_open_conns"`
	MaxIdleConns      int    `json:"max_idle_conns"`
	ConnMaxLifetimeMS int    `json:"conn_max_lifetime_ms"`
}

// DatabaseConfig configures all database connections for control.
type DatabaseConfig struct {
	Postgres PostgresConfig `json:"postgres"`
}

// EgressConfig is the egress-worker config block.
type EgressConfig struct {
	WorkerID            string     `json:"worker_id"`
	CredentialID        string     `json:"credential_id"`
	HeartbeatIntervalMs int        `json:"heartbeat_interval_ms"`
	NATS                NATSConfig `json:"nats"`
}

// LoadControl reads and validates a control config file.
func LoadControl(path string) (ControlConfig, error) {
	file, err := loadFile(path)
	if err != nil {
		return ControlConfig{}, err
	}

	if file.Control == nil {
		return ControlConfig{}, errMissingControlSection
	}

	err = file.Control.validate()
	if err != nil {
		return ControlConfig{}, err
	}

	return *file.Control, nil
}

// LoadEgress reads and validates an egress config file.
func LoadEgress(path string) (EgressConfig, error) {
	file, err := loadFile(path)
	if err != nil {
		return EgressConfig{}, err
	}

	if file.Egress == nil {
		return EgressConfig{}, errMissingEgressSection
	}

	err = file.Egress.validate()
	if err != nil {
		return EgressConfig{}, err
	}

	return *file.Egress, nil
}

func loadFile(path string) (File, error) {
	fd, err := syscall.Open(path, syscall.O_RDONLY, 0)
	if err != nil {
		return File{}, fmt.Errorf("read config file: %w", err)
	}

	f := os.NewFile(uintptr(fd), path)
	if f == nil {
		_ = syscall.Close(fd)

		return File{}, fmt.Errorf("read config file: %w", err)
	}

	defer func() {
		_ = f.Close()
	}()

	raw, err := io.ReadAll(f)
	if err != nil {
		return File{}, fmt.Errorf("read config file: %w", err)
	}

	dec := json.NewDecoder(bytes.NewReader(raw))
	dec.DisallowUnknownFields()

	var file File

	err = dec.Decode(&file)
	if err != nil {
		return File{}, fmt.Errorf("decode config file: %w", err)
	}

	err = rejectTrailingJSON(dec)
	if err != nil {
		return File{}, fmt.Errorf("reject trailing json: %w", err)
	}

	err = file.validateVersion()
	if err != nil {
		return File{}, err
	}

	return file, nil
}

func rejectTrailingJSON(dec *json.Decoder) error {
	var extra any

	err := dec.Decode(&extra)
	if err == nil {
		return errUnexpectedTrailingJSON
	}

	if errors.Is(err, io.EOF) {
		return nil
	}

	return fmt.Errorf("decode trailing json: %w", err)
}

func (f File) validateVersion() error {
	if f.ConfigVersion != Version {
		return fmt.Errorf("%w: must be %q", errInvalidConfigVersion, Version)
	}

	return nil
}

func (c *ControlConfig) validate() error {
	err := c.Server.validate()
	if err != nil {
		return err
	}

	c.applyDefaults()

	return nil
}

func (c *ControlConfig) applyDefaults() {
	if c.Request.MaxInlineRequestBodyBytes == 0 {
		c.Request.MaxInlineRequestBodyBytes = 1_048_576
	}

	if c.Request.MaxInlineResponseBodyBytes == 0 {
		c.Request.MaxInlineResponseBodyBytes = 1_048_576
	}

	if c.Transport.MaxFrameDataBytes == 0 {
		c.Transport.MaxFrameDataBytes = 1_048_576
	}

	if c.Request.MaxTimeoutMs == 0 {
		c.Request.MaxTimeoutMs = 120_000
	}

	c.NATS.applyDefaults()
	c.Database.applyDefaults()
}

func (d *DatabaseConfig) applyDefaults() {
	d.Postgres.applyDefaults()
}

func (p *PostgresConfig) applyDefaults() {
	if p.MaxOpenConns == 0 {
		p.MaxOpenConns = 20
	}

	if p.MaxIdleConns == 0 {
		p.MaxIdleConns = 5
	}

	if p.ConnMaxLifetimeMS == 0 {
		p.ConnMaxLifetimeMS = 1_800_000 // 30 minutes
	}
}

func (n *NATSConfig) applyDefaults() {
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

func (s ControlServerConfig) validate() error {
	if s.Host == "" {
		return errServerHostRequired
	}

	if s.APIPort < 1 || s.APIPort > 65535 {
		return fmt.Errorf("%w: %d", errInvalidServerAPIPort, s.APIPort)
	}

	if s.MetricsPort < 1 || s.MetricsPort > 65535 {
		return fmt.Errorf("%w: %d", errInvalidServerMetricsPort, s.MetricsPort)
	}

	return nil
}

func (e *EgressConfig) validate() error {
	if e.WorkerID == "" {
		return errWorkerIDRequired
	}

	if e.CredentialID == "" {
		return errCredentialIDRequired
	}

	if e.HeartbeatIntervalMs <= 0 {
		e.HeartbeatIntervalMs = 5000
	}

	e.NATS.applyDefaults()

	return nil
}
