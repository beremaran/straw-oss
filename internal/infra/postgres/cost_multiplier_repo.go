package postgres

import (
	"context"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5"

	"github.com/beremaran/straw/internal/domain"
)

// CostMultiplierRepository persists cost multiplier configuration.
type CostMultiplierRepository struct {
	client *Client
}

// NewCostMultiplierRepository creates a PostgreSQL cost multiplier repository.
func NewCostMultiplierRepository(client *Client) *CostMultiplierRepository {
	return &CostMultiplierRepository{client: client}
}

// List returns cost multipliers in deterministic order.
func (r *CostMultiplierRepository) List(ctx context.Context, limit, offset int) ([]domain.CostMultiplier, int, error) {
	var total int

	err := r.client.Execute(func() error {
		return r.client.Pool.QueryRow(ctx, `SELECT COUNT(*) FROM cost_multipliers`).Scan(&total)
	})
	if err != nil {
		return nil, 0, fmt.Errorf("failed to count cost multipliers: %w", err)
	}

	query := `
		SELECT id, endpoint_tag, multiplier, COALESCE(description, ''), is_active, version, created_at, updated_at
		FROM cost_multipliers
		ORDER BY endpoint_tag ASC
		LIMIT $1 OFFSET $2
	`

	var rows pgx.Rows

	err = r.client.Execute(func() error {
		var queryErr error

		rows, queryErr = r.client.Pool.Query(ctx, query, limit, offset)
		if queryErr != nil {
			return fmt.Errorf("failed to execute query: %w", queryErr)
		}

		return nil
	})
	if err != nil {
		return nil, 0, fmt.Errorf("failed to list cost multipliers: %w", err)
	}
	defer rows.Close()

	multipliers, err := scanCostMultipliers(rows)
	if err != nil {
		return nil, 0, err
	}

	return multipliers, total, nil
}

// ListActive returns active cost multipliers.
func (r *CostMultiplierRepository) ListActive(ctx context.Context) ([]domain.CostMultiplier, error) {
	query := `
		SELECT id, endpoint_tag, multiplier, COALESCE(description, ''), is_active, version, created_at, updated_at
		FROM cost_multipliers
		WHERE is_active = true
		ORDER BY endpoint_tag ASC
	`

	var rows pgx.Rows

	err := r.client.Execute(func() error {
		var queryErr error

		rows, queryErr = r.client.Pool.Query(ctx, query)
		if queryErr != nil {
			return fmt.Errorf("failed to execute query: %w", queryErr)
		}

		return nil
	})
	if err != nil {
		return nil, fmt.Errorf("failed to list active cost multipliers: %w", err)
	}
	defer rows.Close()

	return scanCostMultipliers(rows)
}

// GetByID returns a cost multiplier by ID.
func (r *CostMultiplierRepository) GetByID(ctx context.Context, id string) (*domain.CostMultiplier, error) {
	query := `
		SELECT id, endpoint_tag, multiplier, COALESCE(description, ''), is_active, version, created_at, updated_at
		FROM cost_multipliers
		WHERE id = $1
	`

	var multiplier domain.CostMultiplier

	err := r.client.Execute(func() error {
		return r.client.Pool.QueryRow(ctx, query, id).Scan(
			&multiplier.ID,
			&multiplier.EndpointTag,
			&multiplier.Multiplier,
			&multiplier.Description,
			&multiplier.IsActive,
			&multiplier.Version,
			&multiplier.CreatedAt,
			&multiplier.UpdatedAt,
		)
	})
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, nil
		}

		return nil, fmt.Errorf("failed to get cost multiplier: %w", err)
	}

	return &multiplier, nil
}

// Create inserts a cost multiplier.
func (r *CostMultiplierRepository) Create(ctx context.Context, multiplier *domain.CostMultiplier) error {
	query := `
		INSERT INTO cost_multipliers (
			id, endpoint_tag, multiplier, description, is_active, created_at, updated_at, version
		) VALUES (
			$1, $2, $3, $4, $5, $6, $7, $8
		)
	`

	err := r.client.Execute(func() error {
		_, execErr := r.client.Pool.Exec(ctx, query,
			multiplier.ID,
			multiplier.EndpointTag,
			multiplier.Multiplier,
			multiplier.Description,
			multiplier.IsActive,
			multiplier.CreatedAt,
			multiplier.UpdatedAt,
			multiplier.Version,
		)
		if execErr != nil {
			return fmt.Errorf("failed to insert cost multiplier: %w", execErr)
		}

		return nil
	})
	if err != nil {
		return fmt.Errorf("failed to create cost multiplier: %w", err)
	}

	return nil
}

// Update updates a cost multiplier when the supplied version matches.
func (r *CostMultiplierRepository) Update(ctx context.Context, multiplier *domain.CostMultiplier) error {
	query := `
		UPDATE cost_multipliers
		SET endpoint_tag = $2,
			multiplier = $3,
			description = $4,
			is_active = $5,
			updated_at = $6,
			version = version + 1
		WHERE id = $1 AND version = $7
		RETURNING version
	`

	err := r.client.Execute(func() error {
		return r.client.Pool.QueryRow(ctx, query,
			multiplier.ID,
			multiplier.EndpointTag,
			multiplier.Multiplier,
			multiplier.Description,
			multiplier.IsActive,
			multiplier.UpdatedAt,
			multiplier.Version,
		).Scan(&multiplier.Version)
	})
	if err == nil {
		return nil
	}

	if !errors.Is(err, pgx.ErrNoRows) {
		return fmt.Errorf("failed to update cost multiplier: %w", err)
	}

	exists, existsErr := r.exists(ctx, multiplier.ID)
	if existsErr != nil {
		return existsErr
	}

	if exists {
		return domain.ErrCostMultiplierVersionConflict
	}

	return domain.ErrCostMultiplierNotFound
}

// Deactivate soft-deletes a cost multiplier.
func (r *CostMultiplierRepository) Deactivate(ctx context.Context, id string) (*domain.CostMultiplier, error) {
	query := `
		UPDATE cost_multipliers
		SET is_active = false,
			updated_at = now(),
			version = version + 1
		WHERE id = $1
		RETURNING id, endpoint_tag, multiplier, COALESCE(description, ''), is_active, version, created_at, updated_at
	`

	var multiplier domain.CostMultiplier

	err := r.client.Execute(func() error {
		return r.client.Pool.QueryRow(ctx, query, id).Scan(
			&multiplier.ID,
			&multiplier.EndpointTag,
			&multiplier.Multiplier,
			&multiplier.Description,
			&multiplier.IsActive,
			&multiplier.Version,
			&multiplier.CreatedAt,
			&multiplier.UpdatedAt,
		)
	})
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, domain.ErrCostMultiplierNotFound
		}

		return nil, fmt.Errorf("failed to deactivate cost multiplier: %w", err)
	}

	return &multiplier, nil
}

func (r *CostMultiplierRepository) exists(ctx context.Context, id string) (bool, error) {
	var exists bool

	err := r.client.Execute(func() error {
		return r.client.Pool.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM cost_multipliers WHERE id = $1)`, id).Scan(&exists)
	})
	if err != nil {
		return false, fmt.Errorf("failed to check cost multiplier existence: %w", err)
	}

	return exists, nil
}

func scanCostMultipliers(rows pgx.Rows) ([]domain.CostMultiplier, error) {
	var multipliers []domain.CostMultiplier

	for rows.Next() {
		var multiplier domain.CostMultiplier

		err := rows.Scan(
			&multiplier.ID,
			&multiplier.EndpointTag,
			&multiplier.Multiplier,
			&multiplier.Description,
			&multiplier.IsActive,
			&multiplier.Version,
			&multiplier.CreatedAt,
			&multiplier.UpdatedAt,
		)
		if err != nil {
			return nil, fmt.Errorf("failed to scan cost multiplier: %w", err)
		}

		multipliers = append(multipliers, multiplier)
	}

	err := rows.Err()
	if err != nil {
		return nil, fmt.Errorf("error iterating cost multipliers: %w", err)
	}

	return multipliers, nil
}
