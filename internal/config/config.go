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

// defaultEgressHealthPort is used when egress.health_port is unset.
const defaultEgressHealthPort = 8090

var (
	errMissingControlSection    = errors.New("missing control section")
	errMissingEgressSection     = errors.New("missing egress section")
	errUnexpectedTrailingJSON   = errors.New("unexpected trailing JSON data")
	errServerHostRequired       = errors.New("server.host is required")
	errWorkerIDRequired         = errors.New("worker_id is required")
	errCredentialIDRequired     = errors.New("credential_id is required")
	errPrivateKeyEnvRequired    = errors.New("private_key_ed25519_env is required")
	errInvalidConfigVersion     = errors.New("invalid config_version")
	errInvalidServerAPIPort     = errors.New("server.api_port must be between 1 and 65535")
	errInvalidServerMetricsPort = errors.New("server.metrics_port must be between 1 and 65535")
	errInvalidServerProxyPort   = errors.New("server.proxy_port must be 8081 when proxy_enabled is true")
	errInvalidEgressHealthPort  = errors.New("health_port must be between 1 and 65535")
	errEgressPoolRefIncomplete  = errors.New("allowed_pools entries require both tenant_id and pool_id")
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
	Worker    ControlWorkerConfig    `json:"worker"`
	NATS      NATSConfig             `json:"nats"`
	Database  DatabaseConfig         `json:"database"`
}

// ControlWorkerConfig configures worker registration replay protection
// (docs/planning/27-security-controls.md "Worker Credential Signing").
type ControlWorkerConfig struct {
	// RegistrationNonceTTLMS bounds how long a consumed registration nonce is
	// remembered in Redis before it may be reused. Must comfortably exceed
	// RegistrationClockSkewMS*2 so a nonce cannot expire and become
	// replayable while still inside the accepted issued-at window.
	RegistrationNonceTTLMS int `json:"registration_nonce_ttl_ms"`
	// RegistrationClockSkewMS is the maximum allowed difference between a
	// worker's issued-at timestamp and Control's receive time.
	RegistrationClockSkewMS int `json:"registration_clock_skew_ms"`
	// RegistrationFailOpenOnRedisOutage allows registration to proceed
	// without nonce replay protection when Redis is unavailable. Disabled by
	// default: docs/planning/27 requires registration to fail closed unless a
	// deployment explicitly opts in.
	RegistrationFailOpenOnRedisOutage bool `json:"registration_fail_open_on_redis_outage"`
}

// ControlServerConfig configures the control HTTP server.
type ControlServerConfig struct {
	Host         string `json:"host"`
	APIPort      int    `json:"api_port"`
	MetricsPort  int    `json:"metrics_port"`
	ProxyEnabled bool   `json:"proxy_enabled,omitempty"`
	ProxyPort    int    `json:"proxy_port,omitempty"`
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

// RedisConfig configures the Redis client connection used for rate limits,
// quotas, sticky sessions, and config invalidation (docs/planning/21).
type RedisConfig struct {
	URLEnv         string `json:"url_env"`
	DialTimeoutMS  int    `json:"dial_timeout_ms"`
	ReadTimeoutMS  int    `json:"read_timeout_ms"`
	WriteTimeoutMS int    `json:"write_timeout_ms"`
}

// ClickHouseConfig configures the async request-metadata write path
// (docs/planning/22). It is optional: when Endpoint is empty, Control does not
// write telemetry (the recorder stays a no-op) and the request transport is
// unaffected.
type ClickHouseConfig struct {
	Endpoint        string `json:"endpoint"`
	Database        string `json:"database"`
	UserEnv         string `json:"user_env"`
	PasswordEnv     string `json:"password_env"`
	MaxQueueEntries int    `json:"max_queue_entries"`
	BatchSize       int    `json:"batch_size"`
	FlushIntervalMS int    `json:"flush_interval_ms"`
}

// DatabaseConfig configures all database connections for control.
type DatabaseConfig struct {
	Postgres   PostgresConfig   `json:"postgres"`
	Redis      RedisConfig      `json:"redis"`
	ClickHouse ClickHouseConfig `json:"clickhouse"`
}

// EgressConfig is the egress-worker config block.
type EgressConfig struct {
	WorkerID string `json:"worker_id"`
	// CredentialID identifies the worker_credentials row Control verifies
	// PrivateKeyEd25519Env's matching public key against.
	CredentialID string `json:"credential_id"`
	// PrivateKeyEd25519Env is the name of an environment variable holding
	// the worker's persistent ed25519 private key, base64-standard-encoded
	// (32-byte seed or the full 64-byte private key). Secrets are never
	// stored directly in the config file, matching every other credential
	// field in this package (e.g. RedisConfig.URLEnv).
	PrivateKeyEd25519Env string `json:"private_key_ed25519_env"`
	HeartbeatIntervalMs  int    `json:"heartbeat_interval_ms"`
	// HealthPort serves local /healthz and /readyz (docs/planning/23:
	// "P0 should prefer direct local /healthz and /readyz" for egress).
	HealthPort int        `json:"health_port"`
	NATS       NATSConfig `json:"nats"`
	// AllowedPools are the (tenant, pool) memberships the worker declares at
	// registration. Control only routes a request to a worker whose declared
	// pools include the request's target pool (worker_registry.CandidatesForPool),
	// so a worker with no pools is never a dispatch candidate.
	AllowedPools []EgressPoolRef `json:"allowed_pools,omitempty"`
}

// EgressPoolRef is one (tenant, pool) membership an egress worker declares.
type EgressPoolRef struct {
	TenantID string `json:"tenant_id"`
	PoolID   string `json:"pool_id"`
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
	c.Server.applyDefaults()

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

	c.Worker.applyDefaults()
	c.NATS.applyDefaults()
	c.Database.applyDefaults()
}

func (s *ControlServerConfig) applyDefaults() {
	if s.ProxyEnabled && s.ProxyPort == 0 {
		s.ProxyPort = 8081
	}
}

func (w *ControlWorkerConfig) applyDefaults() {
	if w.RegistrationClockSkewMS == 0 {
		w.RegistrationClockSkewMS = 60_000
	}

	if w.RegistrationNonceTTLMS == 0 {
		w.RegistrationNonceTTLMS = 300_000
	}
}

func (d *DatabaseConfig) applyDefaults() {
	d.Postgres.applyDefaults()
	d.Redis.applyDefaults()
	d.ClickHouse.applyDefaults()
}

func (c *ClickHouseConfig) applyDefaults() {
	if c.Endpoint == "" {
		return
	}

	if c.Database == "" {
		c.Database = "straw"
	}

	if c.UserEnv == "" {
		c.UserEnv = "STRAW_CLICKHOUSE_USER"
	}

	if c.PasswordEnv == "" {
		c.PasswordEnv = "STRAW_CLICKHOUSE_PASSWORD"
	}

	if c.MaxQueueEntries == 0 {
		c.MaxQueueEntries = 10_000
	}

	if c.BatchSize == 0 {
		c.BatchSize = 500
	}

	if c.FlushIntervalMS == 0 {
		c.FlushIntervalMS = 1000
	}
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

func (r *RedisConfig) applyDefaults() {
	if r.URLEnv == "" {
		r.URLEnv = "STRAW_REDIS_URL"
	}

	if r.DialTimeoutMS == 0 {
		r.DialTimeoutMS = 2000
	}

	if r.ReadTimeoutMS == 0 {
		r.ReadTimeoutMS = 500
	}

	if r.WriteTimeoutMS == 0 {
		r.WriteTimeoutMS = 500
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

	if s.ProxyEnabled && s.ProxyPort != 8081 {
		return fmt.Errorf("%w: %d", errInvalidServerProxyPort, s.ProxyPort)
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

	if e.PrivateKeyEd25519Env == "" {
		return errPrivateKeyEnvRequired
	}

	if e.HeartbeatIntervalMs <= 0 {
		e.HeartbeatIntervalMs = 5000
	}

	if e.HealthPort == 0 {
		e.HealthPort = defaultEgressHealthPort
	}

	if e.HealthPort < 1 || e.HealthPort > 65535 {
		return fmt.Errorf("%w: %d", errInvalidEgressHealthPort, e.HealthPort)
	}

	err := validateEgressPoolRefs(e.AllowedPools)
	if err != nil {
		return err
	}

	e.NATS.applyDefaults()

	return nil
}

func validateEgressPoolRefs(refs []EgressPoolRef) error {
	for _, p := range refs {
		if p.TenantID == "" || p.PoolID == "" {
			return errEgressPoolRefIncomplete
		}
	}

	return nil
}
