package postgres

import (
	"context"
	"os"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/beremaran/straw/internal/domain"
)

func TestCostMultiplierRepository(t *testing.T) {
	dsn := os.Getenv("TEST_DB_DSN")
	if dsn == "" {
		t.Skip("Skipping integration test: TEST_DB_DSN not set")
	}

	ctx := context.Background()
	pool, err := pgxpool.New(ctx, dsn)
	require.NoError(t, err)
	defer pool.Close()

	_, err = pool.Exec(ctx, "TRUNCATE cost_multipliers CASCADE")
	require.NoError(t, err)

	repo := NewCostMultiplierRepository(&Client{Pool: pool})
	now := time.Now().UTC()
	multiplier := &domain.CostMultiplier{
		ID:          uuid.New().String(),
		EndpointTag: "type:residential",
		Multiplier:  10,
		Description: "Residential",
		IsActive:    true,
		Version:     1,
		CreatedAt:   now,
		UpdatedAt:   now,
	}

	require.NoError(t, repo.Create(ctx, multiplier))

	loaded, err := repo.GetByID(ctx, multiplier.ID)
	require.NoError(t, err)
	require.NotNil(t, loaded)
	assert.Equal(t, "type:residential", loaded.EndpointTag)

	loaded.Multiplier = 11
	require.NoError(t, repo.Update(ctx, loaded))
	assert.Equal(t, 2, loaded.Version)

	stale := *loaded
	stale.Version = 1
	err = repo.Update(ctx, &stale)
	require.ErrorIs(t, err, domain.ErrCostMultiplierVersionConflict)

	deactivated, err := repo.Deactivate(ctx, multiplier.ID)
	require.NoError(t, err)
	assert.False(t, deactivated.IsActive)

	active, err := repo.ListActive(ctx)
	require.NoError(t, err)
	assert.Empty(t, active)
}
