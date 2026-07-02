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
	Server ControlServerConfig `json:"server"`
}

type ControlServerConfig struct {
	Host        string `json:"host"`
	APIPort     int    `json:"api_port"`
	MetricsPort int    `json:"metrics_port"`
}

type EgressConfig struct {
	WorkerID string `json:"worker_id"`
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

func (c ControlConfig) validate() error {
	if err := c.Server.validate(); err != nil {
		return err
	}
	return nil
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

func (e EgressConfig) validate() error {
	if e.WorkerID == "" {
		return errors.New("worker_id is required")
	}
	return nil
}
