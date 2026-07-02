package config

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
)

const Version = "v1"

type File struct {
	ConfigVersion string         `json:"config_version"`
	Control       *ControlConfig `json:"control,omitempty"`
	Egress        *EgressConfig  `json:"egress,omitempty"`
}

type ControlConfig struct {
	Server    ControlServerConfig    `json:"server"`
	Request   ControlRequestConfig   `json:"request"`
	Transport ControlTransportConfig `json:"transport"`
	NATS      NATSConfig             `json:"nats"`
}

type ControlServerConfig struct {
	Host        string `json:"host"`
	APIPort     int    `json:"api_port"`
	MetricsPort int    `json:"metrics_port"`
}

type ControlRequestConfig struct {
	MaxInlineRequestBodyBytes  uint64 `json:"max_inline_request_body_bytes"`
	MaxInlineResponseBodyBytes uint64 `json:"max_inline_response_body_bytes"`
	MaxTimeoutMs               uint64 `json:"max_timeout_ms"`
}

type ControlTransportConfig struct {
	MaxFrameDataBytes uint64 `json:"max_frame_data_bytes"`
}

type NATSConfig struct {
	Servers             []string `json:"servers"`
	UserCredentialsFile string   `json:"user_credentials_file"`
	ReconnectAttempts   int      `json:"reconnect_attempts"`
	ReconnectWaitMS     int      `json:"reconnect_wait_ms"`
	PingIntervalMS      int      `json:"ping_interval_ms"`
	MaxPingFailures     int      `json:"max_ping_failures"`
	MaxPayloadBytes     *uint64  `json:"max_payload_bytes"`
}

type EgressConfig struct {
	WorkerID string     `json:"worker_id"`
	NATS     NATSConfig `json:"nats"`
}

func LoadControl(path string) (ControlConfig, error) {
	file, err := loadFile(path)
	if err != nil {
		return ControlConfig{}, err
	}

	if file.Control == nil {
		return ControlConfig{}, errors.New("missing control section")
	}

	if err := file.Control.validate(); err != nil {
		return ControlConfig{}, err
	}

	return *file.Control, nil
}

func LoadEgress(path string) (EgressConfig, error) {
	file, err := loadFile(path)
	if err != nil {
		return EgressConfig{}, err
	}

	if file.Egress == nil {
		return EgressConfig{}, errors.New("missing egress section")
	}

	if err := file.Egress.validate(); err != nil {
		return EgressConfig{}, err
	}

	return *file.Egress, nil
}

func loadFile(path string) (File, error) {
	raw, err := os.ReadFile(path)
	if err != nil {
		return File{}, err
	}

	dec := json.NewDecoder(bytes.NewReader(raw))
	dec.DisallowUnknownFields()

	var file File
	if err := dec.Decode(&file); err != nil {
		return File{}, err
	}

	if err := rejectTrailingJSON(dec); err != nil {
		return File{}, err
	}

	if err := file.validateVersion(); err != nil {
		return File{}, err
	}

	return file, nil
}

func rejectTrailingJSON(dec *json.Decoder) error {
	var extra any

	err := dec.Decode(&extra)
	if err == nil {
		return errors.New("unexpected trailing JSON data")
	}

	if errors.Is(err, io.EOF) {
		return nil
	}

	return err
}

func (f File) validateVersion() error {
	if f.ConfigVersion != Version {
		return fmt.Errorf("config_version must be %q", Version)
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
		return errors.New("server.host is required")
	}

	if s.APIPort < 1 || s.APIPort > 65535 {
		return fmt.Errorf("server.api_port must be between 1 and 65535: %d", s.APIPort)
	}

	if s.MetricsPort < 1 || s.MetricsPort > 65535 {
		return fmt.Errorf("server.metrics_port must be between 1 and 65535: %d", s.MetricsPort)
	}

	return nil
}

func (e *EgressConfig) validate() error {
	if e.WorkerID == "" {
		return errors.New("worker_id is required")
	}

	e.NATS.applyDefaults()

	return nil
}
