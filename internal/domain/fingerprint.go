package domain

import (
	"context"
	"time"
)

type FingerprintPreset struct {
	ID        string    `json:"id"`
	Name      string    `json:"name"`
	Config    ConfigMap `json:"config"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}

type ConfigMap map[string]interface{}

type FingerprintRepository interface {
	ListPresets(ctx context.Context) ([]FingerprintPreset, error)

	GetPreset(ctx context.Context, id string) (*FingerprintPreset, error)

	CreatePreset(ctx context.Context, preset *FingerprintPreset) error

	UpdatePreset(ctx context.Context, preset *FingerprintPreset) error

	DeletePreset(ctx context.Context, id string) error
}
