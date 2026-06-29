package integration

import (
	"context"
	"testing"
	"time"

	"github.com/beremaran/straw/internal/domain"
	"github.com/beremaran/straw/internal/infra/postgres"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestEndpointIntegration(t *testing.T) {
	suite := GetSuite(t)
	ctx := context.Background()

	pool, err := pgxpool.New(ctx, suite.PostgresDSN())
	require.NoError(t, err)
	defer pool.Close()

	client := &postgres.Client{Pool: pool}
	epRepo := postgres.NewPostgresEndpointRepository(client)
	cmdRepo := postgres.NewPostgresEndpointCommandRepository(client)
	logRepo := postgres.NewPostgresEndpointLogRepository(client)

	t.Run("Endpoint Registry and Logs Integration", func(t *testing.T) {
		epID := "ep-int-" + uuid.New().String()
		ep := &domain.Endpoint{
			ID:            epID,
			Tags:          []string{"type:residential", "region:us"},
			LastHeartbeat: time.Now().UTC().Truncate(time.Second),
			IsHealthy:     true,
			Metadata: domain.EndpointMetadata{
				Version: "1.0.0",
				IP:      "192.168.1.1",
			},
			DesiredState: domain.DesiredStateActive,
			IsRegistered: true,
			CreatedAt:    time.Now().UTC().Truncate(time.Second),
			UpdatedAt:    time.Now().UTC().Truncate(time.Second),
		}

		// Create
		err = epRepo.Create(ctx, ep)
		require.NoError(t, err)

		// Get
		got, err := epRepo.GetByID(ctx, epID)
		require.NoError(t, err)
		require.NotNil(t, got)
		assert.Equal(t, ep.ID, got.ID)

		// Write a command
		cmdID := uuid.New().String()
		cmd := &domain.EndpointCommand{
			ID:          cmdID,
			EndpointID:  epID,
			Command:     "restart",
			Status:      domain.CommandStatusAccepted,
			Payload:     map[string]any{},
			RequestedAt: time.Now().UTC().Truncate(time.Second),
		}
		err = cmdRepo.Create(ctx, cmd)
		require.NoError(t, err)

		// Get command
		gotCmd, err := cmdRepo.GetByID(ctx, cmdID)
		require.NoError(t, err)
		require.NotNil(t, gotCmd)
		assert.Equal(t, cmdID, gotCmd.ID)

		// Write log entry
		logEntry := &domain.EndpointLogEntry{
			EndpointID: epID,
			ObservedAt: time.Now().UTC().Truncate(time.Second),
			Level:      "info",
			Message:    "Endpoint registered",
			Attrs:      map[string]any{"task": "bootstrap"},
		}
		err = logRepo.Create(ctx, logEntry)
		require.NoError(t, err)
		assert.Greater(t, logEntry.ID, int64(0))

		// Query logs
		logs, err := logRepo.ListByEndpointID(ctx, epID, 0, 10)
		require.NoError(t, err)
		require.Len(t, logs, 1)
		assert.Equal(t, "Endpoint registered", logs[0].Message)
	})
}
