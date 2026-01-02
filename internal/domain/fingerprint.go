package domain

import (
	"context"
	"time"
)

// FingerprintPreset represents a pre-configured TLS fingerprint.
type FingerprintPreset struct {
	// ID is the unique identifier for this preset (e.g., "chrome-130").
	ID string `json:"id"`

	// Name is a human-readable name for the preset.
	Name string `json:"name"`

	// Config is the detailed configuration of the fingerprint.
	// This mirrors the structure expected by the fingerprinting engine.
	Config ConfigMap `json:"config"`

	// CreatedAt is when the preset was created.
	CreatedAt time.Time `json:"created_at"`

	// UpdatedAt is when the preset was last modified.
	UpdatedAt time.Time `json:"updated_at"`
}

// ConfigMap is a flexible map for storing configuration.
type ConfigMap map[string]interface{}

// FingerprintRepository defines the interface for fingerprint preset storage.
type FingerprintRepository interface {
	// ListPresets returns all fingerprint presets.
	ListPresets(ctx context.Context) ([]FingerprintPreset, error)

	// GetPreset retrieves a preset by ID.
	GetPreset(ctx context.Context, id string) (*FingerprintPreset, error)

	// CreatePreset creates a new fingerprint preset.
	CreatePreset(ctx context.Context, preset *FingerprintPreset) error

	// UpdatePreset updates an existing fingerprint preset.
	UpdatePreset(ctx context.Context, preset *FingerprintPreset) error

	// DeletePreset deletes a fingerprint preset by ID.
	DeletePreset(ctx context.Context, id string) error
}
