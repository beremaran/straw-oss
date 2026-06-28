package postgres

import (
	"context"
	"os"
	"testing"
	"time"

	"github.com/beremaran/straw/internal/domain"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestRoutingRuleRepository(t *testing.T) {
	dsn := os.Getenv("TEST_DB_DSN")
	if dsn == "" {
		t.Skip("Skipping integration test: TEST_DB_DSN not set")
	}

	ctx := context.Background()
	pool, err := pgxpool.New(ctx, dsn)
	require.NoError(t, err)
	defer pool.Close()

	_, err = pool.Exec(ctx, "TRUNCATE routing_rules CASCADE")
	require.NoError(t, err)

	client := &Client{Pool: pool}
	repo := NewRoutingRuleRepository(client)

	t.Run("CreateRule", func(t *testing.T) {
		rule := &domain.RoutingRule{
			ID:                 "rule-001",
			Name:               "Test Rule 1",
			Priority:           100,
			RequiredTags:       []string{"target:test"},
			ExcludedTags:       []string{},
			RateLimitPerMinute: 60,
			IsActive:           true,
			Version:            1,
			CreatedAt:          time.Now(),
			UpdatedAt:          time.Now(),
		}

		err := repo.CreateRule(ctx, rule)
		assert.NoError(t, err)
	})

	t.Run("GetActiveRules", func(t *testing.T) {
		inactiveRule := &domain.RoutingRule{
			ID:           "rule-002",
			Name:         "Inactive Rule",
			Priority:     50,
			RequiredTags: []string{"target:inactive"},
			IsActive:     false,
			Version:      1,
			CreatedAt:    time.Now(),
			UpdatedAt:    time.Now(),
		}
		require.NoError(t, repo.CreateRule(ctx, inactiveRule))

		rules, err := repo.GetActiveRules(ctx)
		assert.NoError(t, err)
		assert.Len(t, rules, 1)
		assert.Equal(t, "rule-001", rules[0].ID)
		assert.Equal(t, "Test Rule 1", rules[0].Name)
		assert.Equal(t, 100, rules[0].Priority)
		assert.Equal(t, 60, rules[0].RateLimitPerMinute)
	})

	t.Run("GetRuleByID", func(t *testing.T) {
		rule, err := repo.GetRuleByID(ctx, "rule-001")
		assert.NoError(t, err)
		assert.NotNil(t, rule)
		assert.Equal(t, "rule-001", rule.ID)

		rule, err = repo.GetRuleByID(ctx, "non-existent")
		assert.NoError(t, err)
		assert.Nil(t, rule)
	})
}
