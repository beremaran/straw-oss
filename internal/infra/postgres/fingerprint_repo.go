package postgres

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/beremaran/straw/internal/domain"
	"github.com/jackc/pgx/v5"
)

type FingerprintRepository struct {
	client *Client
}

func NewFingerprintRepository(client *Client) *FingerprintRepository {
	return &FingerprintRepository{client: client}
}

func (r *FingerprintRepository) ListPresets(ctx context.Context) ([]domain.FingerprintPreset, error) {
	query := `
		SELECT id, name, config, created_at, updated_at
		FROM fingerprint_presets
		ORDER BY name ASC
	`

	var rows pgx.Rows
	var err error
	err = r.client.Execute(func() error {
		rows, err = r.client.Pool.Query(ctx, query)
		return err
	})
	if err != nil {
		return nil, fmt.Errorf("failed to query presets: %w", err)
	}
	defer rows.Close()

	var presets []domain.FingerprintPreset
	for rows.Next() {
		var p domain.FingerprintPreset
		var configJSON []byte
		if err := rows.Scan(&p.ID, &p.Name, &configJSON, &p.CreatedAt, &p.UpdatedAt); err != nil {
			return nil, fmt.Errorf("failed to scan preset: %w", err)
		}
		if err := json.Unmarshal(configJSON, &p.Config); err != nil {
			return nil, fmt.Errorf("failed to unmarshal config for preset %s: %w", p.ID, err)
		}
		presets = append(presets, p)
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("rows error: %w", err)
	}

	return presets, nil
}

func (r *FingerprintRepository) GetPreset(ctx context.Context, id string) (*domain.FingerprintPreset, error) {
	query := `
		SELECT id, name, config, created_at, updated_at
		FROM fingerprint_presets
		WHERE id = $1
	`

	var p domain.FingerprintPreset
	var configJSON []byte
	err := r.client.Execute(func() error {
		return r.client.Pool.QueryRow(ctx, query, id).Scan(&p.ID, &p.Name, &configJSON, &p.CreatedAt, &p.UpdatedAt)
	})
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, nil
		}
		return nil, fmt.Errorf("failed to get preset: %w", err)
	}

	if err := json.Unmarshal(configJSON, &p.Config); err != nil {
		return nil, fmt.Errorf("failed to unmarshal config: %w", err)
	}

	return &p, nil
}

func (r *FingerprintRepository) CreatePreset(ctx context.Context, preset *domain.FingerprintPreset) error {
	query := `
		INSERT INTO fingerprint_presets (id, name, config, created_at, updated_at)
		VALUES ($1, $2, $3, $4, $5)
	`

	configJSON, err := json.Marshal(preset.Config)
	if err != nil {
		return fmt.Errorf("failed to marshal config: %w", err)
	}

	now := time.Now()
	if preset.CreatedAt.IsZero() {
		preset.CreatedAt = now
	}
	preset.UpdatedAt = now

	err = r.client.Execute(func() error {
		_, err := r.client.Pool.Exec(ctx, query, preset.ID, preset.Name, configJSON, preset.CreatedAt, preset.UpdatedAt)
		return err
	})
	if err != nil {
		return fmt.Errorf("failed to create preset: %w", err)
	}

	return nil
}

func (r *FingerprintRepository) UpdatePreset(ctx context.Context, preset *domain.FingerprintPreset) error {
	query := `
		UPDATE fingerprint_presets
		SET name = $2, config = $3, updated_at = $4
		WHERE id = $1
	`

	configJSON, err := json.Marshal(preset.Config)
	if err != nil {
		return fmt.Errorf("failed to marshal config: %w", err)
	}

	preset.UpdatedAt = time.Now()

	result, err := r.client.Pool.Exec(ctx, query, preset.ID, preset.Name, configJSON, preset.UpdatedAt)
	if err != nil {
		return fmt.Errorf("failed to update preset: %w", err)
	}

	if result.RowsAffected() == 0 {
		return fmt.Errorf("preset not found")
	}

	return nil
}

func (r *FingerprintRepository) DeletePreset(ctx context.Context, id string) error {
	query := `DELETE FROM fingerprint_presets WHERE id = $1`

	result, err := r.client.Pool.Exec(ctx, query, id)
	if err != nil {
		return fmt.Errorf("failed to delete preset: %w", err)
	}

	if result.RowsAffected() == 0 {
		return fmt.Errorf("preset not found")
	}

	return nil
}
