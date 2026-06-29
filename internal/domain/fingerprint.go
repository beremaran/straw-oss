package domain

import (
	"context"
	"time"
)

// FingerprintPreset represents a named collection of fingerprinting configuration.
type FingerprintPreset struct {
	ID        string    `json:"id"`
	Name      string    `json:"name"`
	Config    ConfigMap `json:"config"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}

// ConfigMap is a map of configuration keys to arbitrary values.
type ConfigMap map[string]any

// FingerprintRepository provides persistence operations for FingerprintPreset entities.
type FingerprintRepository interface {
	ListPresets(ctx context.Context) ([]FingerprintPreset, error)

	GetPreset(ctx context.Context, id string) (*FingerprintPreset, error)

	CreatePreset(ctx context.Context, preset *FingerprintPreset) error

	UpdatePreset(ctx context.Context, preset *FingerprintPreset) error

	DeletePreset(ctx context.Context, id string) error
}
