// Package postgres provides PostgreSQL-backed repository implementations for the straw domain models.
package postgres

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/beremaran/straw/internal/domain"
)

// ErrAPIKeyNotFound is returned when an API key is not found.
var ErrAPIKeyNotFound = errors.New("api key not found")

// APIKeyRepository persists and retrieves API keys.
type APIKeyRepository struct {
	client *Client
}

// NewAPIKeyRepository creates a new APIKeyRepository backed by the given client.
func NewAPIKeyRepository(client *Client) *APIKeyRepository {
	return &APIKeyRepository{client: client}
}

// GetByID returns the API key with the given ID.
func (r *APIKeyRepository) GetByID(ctx context.Context, id string) (*domain.APIKey, error) {
	query := `
		SELECT id, token_hash, name, scopes, rate_limit_override, is_active, created_at, expires_at
		FROM api_keys
		WHERE id = $1
	`

	var k domain.APIKey

	err := r.client.Execute(func() error {
		return r.client.Pool.QueryRow(ctx, query, id).Scan(
			&k.ID,
			&k.TokenHash,
			&k.Name,
			&k.Scopes,
			&k.RateLimitOverride,
			&k.IsActive,
			&k.CreatedAt,
			&k.ExpiresAt,
		)
	})
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, nil
		}

		return nil, fmt.Errorf("failed to get api key by id: %w", err)
	}

	return &k, nil
}

// GetByTokenHash returns the API key with the given token hash.
func (r *APIKeyRepository) GetByTokenHash(ctx context.Context, tokenHash string) (*domain.APIKey, error) {
	query := `
		SELECT k.id, t.token_hash, k.name, k.scopes, k.rate_limit_override, k.is_active, k.created_at, k.expires_at
		FROM api_keys k
		JOIN api_key_tokens t ON t.api_key_id = k.id
		WHERE t.token_hash = $1
	`

	var k domain.APIKey

	err := r.client.Execute(func() error {
		return r.client.Pool.QueryRow(ctx, query, tokenHash).Scan(
			&k.ID,
			&k.TokenHash,
			&k.Name,
			&k.Scopes,
			&k.RateLimitOverride,
			&k.IsActive,
			&k.CreatedAt,
			&k.ExpiresAt,
		)
	})
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, nil
		}

		return nil, fmt.Errorf("failed to get api key by token hash: %w", err)
	}

	return &k, nil
}

// Create inserts a new API key and its initial token.
func (r *APIKeyRepository) Create(ctx context.Context, key *domain.APIKey) error {
	query := `
		INSERT INTO api_keys (
			id, token_hash, name, scopes, rate_limit_override, 
			is_active, created_at, expires_at
		) VALUES (
			$1, $2, $3, $4, $5, $6, $7, $8
		)
	`

	err := r.client.Execute(func() error {
		tx, err := r.client.Pool.Begin(ctx)
		if err != nil {
			return fmt.Errorf("failed to begin transaction: %w", err)
		}
		defer func() { _ = tx.Rollback(ctx) }()

		_, err = tx.Exec(ctx, query,
			key.ID,
			key.TokenHash,
			key.Name,
			key.Scopes,
			key.RateLimitOverride,
			key.IsActive,
			key.CreatedAt,
			key.ExpiresAt,
		)
		if err != nil {
			return fmt.Errorf("failed to insert api key: %w", err)
		}

		tokenQuery := `
				INSERT INTO api_key_tokens (api_key_id, token_hash, status, created_at)
				VALUES ($1, $2, 'active', $3)
			`

		_, err = tx.Exec(ctx, tokenQuery, key.ID, key.TokenHash, key.CreatedAt)
		if err != nil {
			return fmt.Errorf("failed to insert api key token: %w", err)
		}

		return tx.Commit(ctx)
	})
	if err != nil {
		return fmt.Errorf("failed to create api key: %w", err)
	}

	return nil
}

// Update modifies an existing API key.
func (r *APIKeyRepository) Update(ctx context.Context, key *domain.APIKey) error {
	query := `
		UPDATE api_keys
		SET name = $2,
			scopes = $3,
			rate_limit_override = $4,
			is_active = $5,
			expires_at = $6
		WHERE id = $1
	`

	var rows int64

	err := r.client.Execute(func() error {
		_, err := r.client.Pool.Exec(ctx, query,
			key.ID,
			key.Name,
			key.Scopes,
			key.RateLimitOverride,
			key.IsActive,
			key.ExpiresAt,
		)
		if err != nil {
			return fmt.Errorf("failed to execute update: %w", err)
		}

		rows = 1

		return nil
	})
	if err != nil {
		return fmt.Errorf("failed to update api key: %w", err)
	}

	if rows == 0 {
		return ErrAPIKeyNotFound
	}

	return nil
}

// List returns a paginated list of API keys.
func (r *APIKeyRepository) List(ctx context.Context, limit, offset int) ([]domain.APIKey, int, error) {
	var total int

	countQuery := `SELECT COUNT(*) FROM api_keys`

	err := r.client.Pool.QueryRow(ctx, countQuery).Scan(&total)
	if err != nil {
		return nil, 0, fmt.Errorf("failed to count api keys: %w", err)
	}

	query := `
		SELECT id, token_hash, name, scopes, rate_limit_override, is_active, created_at, expires_at
		FROM api_keys
		ORDER BY created_at DESC
		LIMIT $1 OFFSET $2
	`

	rows, err := r.client.Pool.Query(ctx, query, limit, offset)
	if err != nil {
		return nil, 0, fmt.Errorf("failed to list api keys: %w", err)
	}
	defer rows.Close()

	var keys []domain.APIKey

	for rows.Next() {
		var k domain.APIKey

		err := rows.Scan(
			&k.ID,
			&k.TokenHash,
			&k.Name,
			&k.Scopes,
			&k.RateLimitOverride,
			&k.IsActive,
			&k.CreatedAt,
			&k.ExpiresAt,
		)
		if err != nil {
			return nil, 0, fmt.Errorf("failed to scan api key: %w", err)
		}

		keys = append(keys, k)
	}

	err = rows.Err()
	if err != nil {
		return nil, 0, fmt.Errorf("error iterating api keys: %w", err)
	}

	return keys, total, nil
}

// Exists returns whether any API keys exist.
func (r *APIKeyRepository) Exists(ctx context.Context) (bool, error) {
	query := `SELECT EXISTS(SELECT 1 FROM api_keys LIMIT 1)`

	var exists bool

	err := r.client.Pool.QueryRow(ctx, query).Scan(&exists)
	if err != nil {
		return false, fmt.Errorf("failed to check api keys existence: %w", err)
	}

	return exists, nil
}

// Revoke deactivates an API key and its tokens.
func (r *APIKeyRepository) Revoke(ctx context.Context, id string) error {
	now := time.Now().UTC()

	return r.client.Execute(func() error {
		tx, err := r.client.Pool.Begin(ctx)
		if err != nil {
			return fmt.Errorf("failed to begin transaction: %w", err)
		}
		defer func() { _ = tx.Rollback(ctx) }()

		res, err := tx.Exec(ctx, `
			UPDATE api_keys
			SET is_active = false
			WHERE id = $1
		`, id)
		if err != nil {
			return fmt.Errorf("failed to revoke api key: %w", err)
		}

		if res.RowsAffected() == 0 {
			return ErrAPIKeyNotFound
		}

		_, err = tx.Exec(ctx, `
			UPDATE api_key_tokens
			SET status = $2,
				expires_at = $3
			WHERE api_key_id = $1 AND status <> $2
		`, id, domain.TokenStatusRevoked, now)
		if err != nil {
			return fmt.Errorf("failed to revoke api key tokens: %w", err)
		}

		return tx.Commit(ctx)
	})
}
