package config

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/url"
	"os"
	"syscall"
)

// Version is the supported configuration format version.
const (
	Version                         = "v1"
	defaultEgressHealthPort         = 8090
	defaultNATSServer               = "nats://127.0.0.1:4222"
	objectStorageBackendLocal       = "local"
	objectStorageBackendS3          = "s3"
	defaultMaxObjectBytes           = int64(1 << 30)
	defaultMaxPartBytes             = int64(16 << 20)
	defaultReceiptRetentionSeconds  = int64(86400)
	defaultReceiptAssignmentSeconds = int64(300)
	defaultReceiptCleanupSeconds    = int64(3600)
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
	errInvalidRuntimeHistory  = errors.New("runtime_admin.history_limit must be between 1 and 64")
	errInvalidRuntimeState    = errors.New("runtime_state backend must be memory or redis")
	errInvalidRuntimeStateTTL = errors.New("runtime_state TTLs must be positive and request_ttl_ms must exceed max_timeout_ms")
	errInvalidObjectStorage   = errors.New("object_storage configuration is invalid")
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
	Server        ControlServerConfig    `json:"server"`
	Request       ControlRequestConfig   `json:"request"`
	Transport     ControlTransportConfig `json:"transport"`
	NATS          NATSConfig             `json:"nats"`
	RuntimeAdmin  RuntimeAdminConfig     `json:"runtime_admin"`
	RuntimeState  RuntimeStateConfig     `json:"runtime_state"`
	ObjectStorage ObjectStorageConfig    `json:"object_storage"`
}

// ObjectStorageConfig enables durable request and response receipts. Secrets
// are named by environment variable and never stored in the JSON file.
type ObjectStorageConfig struct {
	Enabled                bool   `json:"enabled"`
	Backend                string `json:"backend"`
	LocalDirectory         string `json:"local_directory,omitempty"`
	Endpoint               string `json:"endpoint,omitempty"`
	Bucket                 string `json:"bucket,omitempty"`
	Region                 string `json:"region,omitempty"`
	AccessKeyEnv           string `json:"access_key_env,omitempty"`
	SecretKeyEnv           string `json:"secret_key_env,omitempty"`
	SessionTokenEnv        string `json:"session_token_env,omitempty"`
	SigningKeyEnv          string `json:"signing_key_env,omitempty"`
	DownloadBaseURL        string `json:"download_base_url,omitempty"`
	MaxObjectBytes         int64  `json:"max_object_bytes,omitempty"`
	MaxPartBytes           int64  `json:"max_part_bytes,omitempty"`
	RetentionSeconds       int64  `json:"retention_seconds,omitempty"`
	AssignmentTTLSeconds   int64  `json:"assignment_ttl_seconds,omitempty"`
	CleanupIntervalSeconds int64  `json:"cleanup_interval_seconds,omitempty"`
	ServerSideEncryption   string `json:"server_side_encryption,omitempty"`
	KMSKeyID               string `json:"kms_key_id,omitempty"`
}

// RuntimeStateConfig selects the local development state store or opt-in
// Redis coordination used by interchangeable Control instances.
type RuntimeStateConfig struct {
	Backend            string `json:"backend"`
	RedisURLEnv        string `json:"redis_url_env"`
	KeyPrefix          string `json:"key_prefix"`
	InstanceIDEnv      string `json:"instance_id_env"`
	WorkerTTLMS        int    `json:"worker_ttl_ms"`
	RequestTTLMS       int    `json:"request_ttl_ms"`
	InstanceTTLMS      int    `json:"instance_ttl_ms"`
	OperationTimeoutMS int    `json:"operation_timeout_ms"`
}

// RuntimeAdminConfig enables the optional durable runtime-administration profile.
type RuntimeAdminConfig struct {
	Enabled      bool   `json:"enabled"`
	TokenEnv     string `json:"token_env"`
	Bucket       string `json:"bucket"`
	HistoryLimit uint8  `json:"history_limit"`
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

	c.RuntimeAdmin.applyDefaults()
	c.RuntimeState.applyDefaults()
	c.ObjectStorage.applyDefaults()

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

func (o *ObjectStorageConfig) applyDefaults() {
	o.applyBackendDefaults()
	o.applyEnvironmentDefaults()
	o.applyLimitDefaults()
}

func (o *ObjectStorageConfig) applyBackendDefaults() {
	if o.Backend == "" {
		o.Backend = objectStorageBackendLocal
	}

	if o.LocalDirectory == "" {
		o.LocalDirectory = ".straw/objects"
	}

	if o.Region == "" {
		o.Region = "us-east-1"
	}
}

func (o *ObjectStorageConfig) applyEnvironmentDefaults() {
	if o.AccessKeyEnv == "" {
		o.AccessKeyEnv = "STRAW_S3_ACCESS_KEY"
	}

	if o.SecretKeyEnv == "" {
		o.SecretKeyEnv = "STRAW_S3_SECRET_KEY"
	}

	if o.SessionTokenEnv == "" {
		o.SessionTokenEnv = "STRAW_S3_SESSION_TOKEN"
	}

	if o.SigningKeyEnv == "" {
		o.SigningKeyEnv = "STRAW_RECEIPT_SIGNING_KEY"
	}

	if o.DownloadBaseURL == "" {
		o.DownloadBaseURL = "http://control:8080"
	}
}

func (o *ObjectStorageConfig) applyLimitDefaults() {
	if o.MaxObjectBytes == 0 {
		o.MaxObjectBytes = defaultMaxObjectBytes
	}

	if o.MaxPartBytes == 0 {
		o.MaxPartBytes = defaultMaxPartBytes
	}

	if o.RetentionSeconds == 0 {
		o.RetentionSeconds = defaultReceiptRetentionSeconds
	}

	if o.AssignmentTTLSeconds == 0 {
		o.AssignmentTTLSeconds = defaultReceiptAssignmentSeconds
	}

	if o.CleanupIntervalSeconds == 0 {
		o.CleanupIntervalSeconds = defaultReceiptCleanupSeconds
	}
}

func (r *RuntimeStateConfig) applyDefaults() {
	if r.Backend == "" {
		r.Backend = "memory"
	}

	if r.RedisURLEnv == "" {
		r.RedisURLEnv = "STRAW_REDIS_URL"
	}

	if r.KeyPrefix == "" {
		r.KeyPrefix = "straw"
	}

	if r.InstanceIDEnv == "" {
		r.InstanceIDEnv = "STRAW_CONTROL_INSTANCE_ID"
	}

	if r.WorkerTTLMS == 0 {
		r.WorkerTTLMS = 30000
	}

	if r.RequestTTLMS == 0 {
		r.RequestTTLMS = 130000
	}

	if r.InstanceTTLMS == 0 {
		r.InstanceTTLMS = 15000
	}

	if r.OperationTimeoutMS == 0 {
		r.OperationTimeoutMS = 1000
	}
}

func (r *RuntimeAdminConfig) applyDefaults() {
	if r.TokenEnv == "" {
		r.TokenEnv = "STRAW_ADMIN_TOKEN"
	}

	if r.Bucket == "" {
		r.Bucket = "STRAW_RUNTIME_CONFIG"
	}

	if r.HistoryLimit == 0 {
		r.HistoryLimit = 64
	}
}

func (c ControlConfig) validate() error {
	if c.Server.APIPort < 1 || c.Server.APIPort > 65535 {
		return fmt.Errorf("%w: %d", errInvalidAPIPort, c.Server.APIPort)
	}

	if c.Server.MetricsPort < 1 || c.Server.MetricsPort > 65535 {
		return fmt.Errorf("%w: %d", errInvalidMetricsPort, c.Server.MetricsPort)
	}

	if c.RuntimeAdmin.Enabled && (c.RuntimeAdmin.HistoryLimit < 1 || c.RuntimeAdmin.HistoryLimit > 64) {
		return fmt.Errorf("%w: %d", errInvalidRuntimeHistory, c.RuntimeAdmin.HistoryLimit)
	}

	err := c.RuntimeState.validate(c.Request.MaxTimeoutMs)
	if err != nil {
		return err
	}

	return c.ObjectStorage.validate()
}

func (o ObjectStorageConfig) validate() error {
	if !o.Enabled {
		return nil
	}

	err := o.validateBackend()
	if err != nil {
		return err
	}

	err = o.validateURLs()
	if err != nil {
		return err
	}

	err = o.validateLimits()
	if err != nil {
		return err
	}

	return o.validateEncryption()
}

func (o ObjectStorageConfig) validateBackend() error {
	if o.Backend != objectStorageBackendLocal && o.Backend != objectStorageBackendS3 {
		return fmt.Errorf("%w: backend must be local or s3", errInvalidObjectStorage)
	}

	if o.Backend == objectStorageBackendLocal && o.LocalDirectory == "" {
		return fmt.Errorf("%w: local_directory is required", errInvalidObjectStorage)
	}

	if o.Backend == objectStorageBackendS3 && (o.Endpoint == "" || o.Bucket == "") {
		return fmt.Errorf("%w: S3 endpoint and bucket are required", errInvalidObjectStorage)
	}

	return nil
}

func (o ObjectStorageConfig) validateURLs() error {
	parsed, err := url.Parse(o.DownloadBaseURL)
	if err != nil || parsed.Scheme == "" || parsed.Host == "" {
		return fmt.Errorf("%w: download_base_url must be absolute", errInvalidObjectStorage)
	}

	if o.Backend == objectStorageBackendS3 {
		parsed, err = url.Parse(o.Endpoint)
		if err != nil || parsed.Scheme == "" || parsed.Host == "" {
			return fmt.Errorf("%w: endpoint must be absolute", errInvalidObjectStorage)
		}
	}

	return nil
}

func (o ObjectStorageConfig) validateLimits() error {
	if o.MaxObjectBytes < 1 || o.MaxPartBytes < 1 || o.MaxPartBytes > o.MaxObjectBytes {
		return errInvalidObjectStorage
	}

	if o.RetentionSeconds < 1 || o.AssignmentTTLSeconds < 1 || o.CleanupIntervalSeconds < 1 {
		return errInvalidObjectStorage
	}

	return nil
}

func (o ObjectStorageConfig) validateEncryption() error {
	if o.ServerSideEncryption != "" && o.ServerSideEncryption != "AES256" && o.ServerSideEncryption != "aws:kms" {
		return fmt.Errorf("%w: server_side_encryption must be AES256 or aws:kms", errInvalidObjectStorage)
	}

	if o.ServerSideEncryption == "aws:kms" && o.KMSKeyID == "" {
		return fmt.Errorf("%w: kms_key_id is required for aws:kms", errInvalidObjectStorage)
	}

	return nil
}

func (r RuntimeStateConfig) validate(maxTimeoutMS uint64) error {
	if r.Backend != "memory" && r.Backend != "redis" {
		return fmt.Errorf("%w: %q", errInvalidRuntimeState, r.Backend)
	}

	if r.WorkerTTLMS < 1 || r.RequestTTLMS < 1 || r.InstanceTTLMS < 1 || r.OperationTimeoutMS < 1 {
		return errInvalidRuntimeStateTTL
	}

	if uint64(r.RequestTTLMS) <= maxTimeoutMS {
		return errInvalidRuntimeStateTTL
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
