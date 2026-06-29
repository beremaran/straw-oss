package postgres

import (
	"context"
	"os"
	"testing"
	"time"

	"github.com/beremaran/straw/internal/domain"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestAPIKeyRepositories(t *testing.T) {
	dsn := os.Getenv("TEST_DB_DSN")
	if dsn == "" {
		t.Skip("Skipping integration test: TEST_DB_DSN not set")
	}

	ctx := context.Background()
	require.NoError(t, RunEmbeddedMigrations(ctx, dsn))

	pool, err := pgxpool.New(ctx, dsn)
	require.NoError(t, err)
	defer pool.Close()

	_, err = pool.Exec(ctx, "TRUNCATE api_key_tokens, api_keys CASCADE")
	require.NoError(t, err)

	client := &Client{Pool: pool}
	keyRepo := NewApiKeyRepository(client)
	tokenRepo := NewApiKeyTokenRepository(client)

	rateLimit := 10
	key := &domain.ApiKey{
		ID:                uuid.New().String(),
		TokenHash:         "hash-1",
		Name:              "Initial Key",
		Scopes:            []string{"target:*"},
		RateLimitOverride: &rateLimit,
		IsActive:          true,
		CreatedAt:         time.Now().UTC(),
	}

	require.NoError(t, keyRepo.Create(ctx, key))

	gotKey, err := keyRepo.GetByID(ctx, key.ID)
	require.NoError(t, err)
	require.NotNil(t, gotKey)
	assert.Equal(t, "Initial Key", gotKey.Name)

	tokens, err := tokenRepo.ListByApiKeyID(ctx, key.ID)
	require.NoError(t, err)
	require.Len(t, tokens, 1)
	assert.Equal(t, domain.TokenStatusActive, tokens[0].Status)

	updatedRateLimit := 25
	expiresAt := time.Now().UTC().Add(24 * time.Hour).Truncate(time.Second)
	key.Name = "Updated Key"
	key.RateLimitOverride = &updatedRateLimit
	key.ExpiresAt = &expiresAt

	require.NoError(t, keyRepo.Update(ctx, key))

	gotKey, err = keyRepo.GetByID(ctx, key.ID)
	require.NoError(t, err)
	require.NotNil(t, gotKey)
	assert.Equal(t, "Updated Key", gotKey.Name)
	assert.Equal(t, &updatedRateLimit, gotKey.RateLimitOverride)
	require.NotNil(t, gotKey.ExpiresAt)
	assert.WithinDuration(t, expiresAt, *gotKey.ExpiresAt, time.Second)

	graceUntil := time.Now().UTC().Add(time.Hour).Truncate(time.Second)
	rotatedToken := &domain.ApiKeyToken{
		ID:        uuid.New().String(),
		ApiKeyID:  key.ID,
		TokenHash: "hash-2",
		Status:    domain.TokenStatusActive,
		CreatedAt: time.Now().UTC(),
	}

	require.NoError(t, tokenRepo.Rotate(ctx, key.ID, rotatedToken, &graceUntil, false))

	gotKey, err = keyRepo.GetByTokenHash(ctx, rotatedToken.TokenHash)
	require.NoError(t, err)
	require.NotNil(t, gotKey)
	assert.Equal(t, key.ID, gotKey.ID)

	tokens, err = tokenRepo.ListByApiKeyID(ctx, key.ID)
	require.NoError(t, err)
	require.Len(t, tokens, 2)

	var activeToken *domain.ApiKeyToken
	var graceToken *domain.ApiKeyToken
	for i := range tokens {
		token := tokens[i]
		switch token.ID {
		case rotatedToken.ID:
			activeToken = &token
		default:
			graceToken = &token
		}
	}

	require.NotNil(t, activeToken)
	require.NotNil(t, graceToken)
	assert.Equal(t, domain.TokenStatusActive, activeToken.Status)
	assert.Equal(t, domain.TokenStatusGrace, graceToken.Status)
	require.NotNil(t, graceToken.ExpiresAt)
	assert.WithinDuration(t, graceUntil, *graceToken.ExpiresAt, time.Second)

	require.NoError(t, keyRepo.Revoke(ctx, key.ID))

	gotKey, err = keyRepo.GetByID(ctx, key.ID)
	require.NoError(t, err)
	require.NotNil(t, gotKey)
	assert.False(t, gotKey.IsActive)

	tokens, err = tokenRepo.ListByApiKeyID(ctx, key.ID)
	require.NoError(t, err)
	require.Len(t, tokens, 2)
	for _, token := range tokens {
		assert.Equal(t, domain.TokenStatusRevoked, token.Status)
		require.NotNil(t, token.ExpiresAt)
	}
}
