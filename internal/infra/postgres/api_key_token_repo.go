package postgres

import (
	"context"
	"errors"
	"fmt"

	"github.com/beremaran/straw/internal/domain"
	"github.com/jackc/pgx/v5"
)

var ErrApiKeyTokenNotFound = errors.New("api key token not found")

type ApiKeyTokenRepository struct {
	client *Client
}

func NewApiKeyTokenRepository(client *Client) *ApiKeyTokenRepository {
	return &ApiKeyTokenRepository{client: client}
}

func (r *ApiKeyTokenRepository) GetByTokenHash(ctx context.Context, tokenHash string) (*domain.ApiKeyToken, error) {
	query := `
		SELECT id, api_key_id, token_hash, status, expires_at, created_at
		FROM api_key_tokens
		WHERE token_hash = $1
	`

	var t domain.ApiKeyToken
	err := r.client.Execute(func() error {
		return r.client.Pool.QueryRow(ctx, query, tokenHash).Scan(
			&t.ID,
			&t.ApiKeyID,
			&t.TokenHash,
			&t.Status,
			&t.ExpiresAt,
			&t.CreatedAt,
		)
	})

	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, nil
		}

		return nil, fmt.Errorf("failed to get api key token: %w", err)
	}

	return &t, nil
}

func (r *ApiKeyTokenRepository) Create(ctx context.Context, token *domain.ApiKeyToken) error {
	query := `
		INSERT INTO api_key_tokens (id, api_key_id, token_hash, status, expires_at, created_at)
		VALUES ($1, $2, $3, $4, $5, $6)
	`
	err := r.client.Execute(func() error {
		_, err := r.client.Pool.Exec(ctx, query,
			token.ID,
			token.ApiKeyID,
			token.TokenHash,
			token.Status,
			token.ExpiresAt,
			token.CreatedAt,
		)

		return err
	})
	if err != nil {
		return fmt.Errorf("failed to create api key token: %w", err)
	}

	return nil
}

func (r *ApiKeyTokenRepository) ListByApiKeyID(ctx context.Context, apiKeyID string) ([]domain.ApiKeyToken, error) {
	query := `
		SELECT id, api_key_id, token_hash, status, expires_at, created_at
		FROM api_key_tokens
		WHERE api_key_id = $1
		ORDER BY created_at DESC
	`
	rows, err := r.client.Pool.Query(ctx, query, apiKeyID)
	if err != nil {
		return nil, fmt.Errorf("failed to list api key tokens: %w", err)
	}
	defer rows.Close()

	var tokens []domain.ApiKeyToken
	for rows.Next() {
		var t domain.ApiKeyToken
		err := rows.Scan(&t.ID, &t.ApiKeyID, &t.TokenHash, &t.Status, &t.ExpiresAt, &t.CreatedAt)
		if err != nil {
			return nil, fmt.Errorf("failed to scan api key token: %w", err)
		}
		tokens = append(tokens, t)
	}
	err = rows.Err()
	if err != nil {
		return nil, fmt.Errorf("error iterating api key tokens: %w", err)
	}

	return tokens, nil
}

func (r *ApiKeyTokenRepository) UpdateStatus(ctx context.Context, id string, status domain.TokenStatus) error {
	query := `
		UPDATE api_key_tokens
		SET status = $1
		WHERE id = $2
	`
	res, err := r.client.Pool.Exec(ctx, query, status, id)
	if err != nil {
		return fmt.Errorf("failed to update token status: %w", err)
	}
	if res.RowsAffected() == 0 {
		return ErrApiKeyTokenNotFound
	}

	return nil
}
