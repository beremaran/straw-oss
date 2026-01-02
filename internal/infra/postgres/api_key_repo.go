package postgres

import (
	"context"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5"
	"github.com/kwilabs/straw-proxy-server/internal/domain"
)

// ApiKeyRepository implements the storage for API keys.
type ApiKeyRepository struct {
	client *Client
}

// NewApiKeyRepository creates a new ApiKeyRepository.
func NewApiKeyRepository(client *Client) *ApiKeyRepository {
	return &ApiKeyRepository{client: client}
}

// GetByID retrieves an API key by its ID.
func (r *ApiKeyRepository) GetByID(ctx context.Context, id string) (*domain.ApiKey, error) {
	query := `
		SELECT id, key_hash, name, scopes, rate_limit_override, is_active, created_at, expires_at
		FROM api_keys
		WHERE id = $1
	`

	var k domain.ApiKey
	err := r.client.Execute(func() error {
		return r.client.Pool.QueryRow(ctx, query, id).Scan(
			&k.ID,
			&k.KeyHash,
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
			return nil, nil // Not found
		}
		return nil, fmt.Errorf("failed to get api key by id: %w", err)
	}

	return &k, nil
}

// Create stores a new API key in the database.
func (r *ApiKeyRepository) Create(ctx context.Context, key *domain.ApiKey) error {
	query := `
		INSERT INTO api_keys (
			id, key_hash, name, scopes, rate_limit_override, 
			is_active, created_at, expires_at
		) VALUES (
			$1, $2, $3, $4, $5, $6, $7, $8
		)
	`

	err := r.client.Execute(func() error {
		_, err := r.client.Pool.Exec(ctx, query,
			key.ID,
			key.KeyHash,
			key.Name,
			key.Scopes,
			key.RateLimitOverride,
			key.IsActive,
			key.CreatedAt,
			key.ExpiresAt,
		)
		return err
	})

	if err != nil {
		return fmt.Errorf("failed to create api key: %w", err)
	}

	return nil
}

// List retrieves a paginated list of API keys.
func (r *ApiKeyRepository) List(ctx context.Context, limit, offset int) ([]domain.ApiKey, int, error) {
	// Get total count
	var total int
	countQuery := `SELECT COUNT(*) FROM api_keys`
	err := r.client.Pool.QueryRow(ctx, countQuery).Scan(&total)
	if err != nil {
		return nil, 0, fmt.Errorf("failed to count api keys: %w", err)
	}

	// Get keys
	query := `
		SELECT id, key_hash, name, scopes, rate_limit_override, is_active, created_at, expires_at
		FROM api_keys
		ORDER BY created_at DESC
		LIMIT $1 OFFSET $2
	`

	rows, err := r.client.Pool.Query(ctx, query, limit, offset)
	if err != nil {
		return nil, 0, fmt.Errorf("failed to list api keys: %w", err)
	}
	defer rows.Close()

	var keys []domain.ApiKey
	for rows.Next() {
		var k domain.ApiKey
		if err := rows.Scan(
			&k.ID,
			&k.KeyHash,
			&k.Name,
			&k.Scopes,
			&k.RateLimitOverride,
			&k.IsActive,
			&k.CreatedAt,
			&k.ExpiresAt,
		); err != nil {
			return nil, 0, fmt.Errorf("failed to scan api key: %w", err)
		}
		keys = append(keys, k)
	}

	if err := rows.Err(); err != nil {
		return nil, 0, fmt.Errorf("error iterating api keys: %w", err)
	}

	return keys, total, nil
}

// Revoke effectively deletes an API key by setting is_active to false.
// This is preferred over hard deletion to keep historical records.
func (r *ApiKeyRepository) Revoke(ctx context.Context, id string) error {
	query := `
		UPDATE api_keys 
		SET is_active = false 
		WHERE id = $1
	`

	res, err := r.client.Pool.Exec(ctx, query, id)
	if err != nil {
		return fmt.Errorf("failed to revoke api key: %w", err)
	}

	if res.RowsAffected() == 0 {
		return errors.New("api key not found")
	}

	return nil
}
