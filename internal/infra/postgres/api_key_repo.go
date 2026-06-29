package postgres

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/beremaran/straw/internal/domain"
	"github.com/jackc/pgx/v5"
)

var ErrApiKeyNotFound = errors.New("api key not found")

type ApiKeyRepository struct {
	client *Client
}

func NewApiKeyRepository(client *Client) *ApiKeyRepository {
	return &ApiKeyRepository{client: client}
}

func (r *ApiKeyRepository) GetByID(ctx context.Context, id string) (*domain.ApiKey, error) {
	query := `
		SELECT id, token_hash, name, scopes, rate_limit_override, is_active, created_at, expires_at
		FROM api_keys
		WHERE id = $1
	`

	var k domain.ApiKey
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

func (r *ApiKeyRepository) GetByTokenHash(ctx context.Context, tokenHash string) (*domain.ApiKey, error) {
	query := `
		SELECT k.id, t.token_hash, k.name, k.scopes, k.rate_limit_override, k.is_active, k.created_at, k.expires_at
		FROM api_keys k
		JOIN api_key_tokens t ON t.api_key_id = k.id
		WHERE t.token_hash = $1
	`

	var k domain.ApiKey
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

func (r *ApiKeyRepository) Create(ctx context.Context, key *domain.ApiKey) error {
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
			return err
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
			return err
		}

		tokenQuery := `
			INSERT INTO api_key_tokens (api_key_id, token_hash, status, created_at)
			VALUES ($1, $2, 'active', $3)
		`
		_, err = tx.Exec(ctx, tokenQuery, key.ID, key.TokenHash, key.CreatedAt)
		if err != nil {
			return err
		}

		return tx.Commit(ctx)
	})

	if err != nil {
		return fmt.Errorf("failed to create api key: %w", err)
	}

	return nil
}

func (r *ApiKeyRepository) Update(ctx context.Context, key *domain.ApiKey) error {
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
		res, err := r.client.Pool.Exec(ctx, query,
			key.ID,
			key.Name,
			key.Scopes,
			key.RateLimitOverride,
			key.IsActive,
			key.ExpiresAt,
		)
		rows = res.RowsAffected()

		return err
	})
	if err != nil {
		return fmt.Errorf("failed to update api key: %w", err)
	}
	if rows == 0 {
		return ErrApiKeyNotFound
	}

	return nil
}

func (r *ApiKeyRepository) List(ctx context.Context, limit, offset int) ([]domain.ApiKey, int, error) {
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

	var keys []domain.ApiKey
	for rows.Next() {
		var k domain.ApiKey
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

func (r *ApiKeyRepository) Exists(ctx context.Context) (bool, error) {
	query := `SELECT EXISTS(SELECT 1 FROM api_keys LIMIT 1)`
	var exists bool
	err := r.client.Pool.QueryRow(ctx, query).Scan(&exists)
	if err != nil {
		return false, fmt.Errorf("failed to check api keys existence: %w", err)
	}

	return exists, nil
}

func (r *ApiKeyRepository) Revoke(ctx context.Context, id string) error {
	now := time.Now().UTC()

	return r.client.Execute(func() error {
		tx, err := r.client.Pool.Begin(ctx)
		if err != nil {
			return err
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
			return ErrApiKeyNotFound
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
