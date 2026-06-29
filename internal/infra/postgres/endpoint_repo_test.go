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

func TestEndpointRepositories(t *testing.T) {
	dsn := os.Getenv("TEST_DB_DSN")
	if dsn == "" {
		t.Skip("Skipping integration test: TEST_DB_DSN not set")
	}

	ctx := context.Background()
	require.NoError(t, RunEmbeddedMigrations(ctx, dsn))

	pool, err := pgxpool.New(ctx, dsn)
	require.NoError(t, err)
	defer pool.Close()

	// Truncate tables clean state
	_, err = pool.Exec(ctx, "TRUNCATE endpoints, endpoint_commands, endpoint_log_entries CASCADE")
	require.NoError(t, err)

	client := &Client{Pool: pool}
	epRepo := NewPostgresEndpointRepository(client)
	cmdRepo := NewPostgresEndpointCommandRepository(client)
	logRepo := NewPostgresEndpointLogRepository(client)

	t.Run("Endpoint Lifecycle", func(t *testing.T) {
		epID := "ep-" + uuid.New().String()
		ep := &domain.Endpoint{
			ID:            epID,
			Tags:          []string{"type:residential", "region:us"},
			LastHeartbeat: time.Now().UTC().Truncate(time.Microsecond),
			IsHealthy:     true,
			Metadata: domain.EndpointMetadata{
				Version:     "1.0.0",
				IP:          "127.0.0.1",
				ActiveTasks: 2,
				Provider:    "test",
			},
			DesiredState: domain.DesiredStateActive,
			IsRegistered: true,
			CreatedAt:    time.Now().UTC().Truncate(time.Microsecond),
			UpdatedAt:    time.Now().UTC().Truncate(time.Microsecond),
		}

		// Create
		err := epRepo.Create(ctx, ep)
		require.NoError(t, err)

		// GetByID
		got, err := epRepo.GetByID(ctx, epID)
		require.NoError(t, err)
		require.NotNil(t, got)
		assert.Equal(t, ep.ID, got.ID)
		assert.Equal(t, ep.Tags, got.Tags)
		assert.Equal(t, ep.IsHealthy, got.IsHealthy)
		assert.Equal(t, ep.Metadata.Version, got.Metadata.Version)
		assert.Equal(t, ep.DesiredState, got.DesiredState)
		assert.True(t, got.IsRegistered)
		assert.Nil(t, got.DeletedAt)

		// Update
		ep.IsHealthy = false
		ep.DesiredState = domain.DesiredStateDraining
		ep.Metadata.ActiveTasks = 5
		ep.Tags = append(ep.Tags, "draining")
		err = epRepo.Update(ctx, ep)
		require.NoError(t, err)

		got, err = epRepo.GetByID(ctx, epID)
		require.NoError(t, err)
		require.NotNil(t, got)
		assert.False(t, got.IsHealthy)
		assert.Equal(t, domain.DesiredStateDraining, got.DesiredState)
		assert.Equal(t, 5, got.Metadata.ActiveTasks)
		assert.Contains(t, got.Tags, "draining")

		// List (without deleted)
		list, total, err := epRepo.List(ctx, 10, 0, false)
		require.NoError(t, err)
		assert.Equal(t, 1, total)
		assert.Len(t, list, 1)
		assert.Equal(t, epID, list[0].ID)

		// Soft Delete
		err = epRepo.Delete(ctx, epID)
		require.NoError(t, err)

		got, err = epRepo.GetByID(ctx, epID)
		require.NoError(t, err)
		require.NotNil(t, got)
		assert.False(t, got.IsRegistered)
		assert.Equal(t, domain.DesiredStateDeleted, got.DesiredState)
		assert.NotNil(t, got.DeletedAt)

		// List (without deleted - should be empty)
		list, total, err = epRepo.List(ctx, 10, 0, false)
		require.NoError(t, err)
		assert.Equal(t, 0, total)
		assert.Len(t, list, 0)

		// List (with deleted - should return it)
		list, total, err = epRepo.List(ctx, 10, 0, true)
		require.NoError(t, err)
		assert.Equal(t, 1, total)
		assert.Len(t, list, 1)
		assert.Equal(t, epID, list[0].ID)
	})

	t.Run("Command Lifecycle", func(t *testing.T) {
		epID := "ep-cmd-" + uuid.New().String()
		cmdID := uuid.New().String()
		reqBy := "admin"

		cmd := &domain.EndpointCommand{
			ID:          cmdID,
			EndpointID:  epID,
			Command:     "restart",
			Status:      domain.CommandStatusAccepted,
			Payload:     map[string]any{"force": true},
			RequestedBy: &reqBy,
			RequestedAt: time.Now().UTC().Truncate(time.Microsecond),
		}

		// Create
		err := cmdRepo.Create(ctx, cmd)
		require.NoError(t, err)

		// GetByID
		got, err := cmdRepo.GetByID(ctx, cmdID)
		require.NoError(t, err)
		require.NotNil(t, got)
		assert.Equal(t, cmd.ID, got.ID)
		assert.Equal(t, cmd.EndpointID, got.EndpointID)
		assert.Equal(t, cmd.Command, got.Command)
		assert.Equal(t, domain.CommandStatusAccepted, got.Status)
		assert.Equal(t, cmd.Payload, got.Payload)
		assert.Equal(t, *cmd.RequestedBy, *got.RequestedBy)

		// Update (e.g. running, then succeeded)
		now := time.Now().UTC().Truncate(time.Microsecond)
		cmd.Status = domain.CommandStatusRunning
		cmd.AcceptedAt = &now
		err = cmdRepo.Update(ctx, cmd)
		require.NoError(t, err)

		got, err = cmdRepo.GetByID(ctx, cmdID)
		require.NoError(t, err)
		assert.Equal(t, domain.CommandStatusRunning, got.Status)
		require.NotNil(t, got.AcceptedAt)

		completedAt := time.Now().UTC().Truncate(time.Microsecond)
		cmd.Status = domain.CommandStatusSucceeded
		cmd.CompletedAt = &completedAt
		err = cmdRepo.Update(ctx, cmd)
		require.NoError(t, err)

		got, err = cmdRepo.GetByID(ctx, cmdID)
		require.NoError(t, err)
		assert.Equal(t, domain.CommandStatusSucceeded, got.Status)
		require.NotNil(t, got.CompletedAt)

		// List by Endpoint
		list, total, err := cmdRepo.ListByEndpointID(ctx, epID, 10, 0)
		require.NoError(t, err)
		assert.Equal(t, 1, total)
		assert.Len(t, list, 1)
		assert.Equal(t, cmdID, list[0].ID)
	})

	t.Run("Log Entry Queries", func(t *testing.T) {
		epID := "ep-log-" + uuid.New().String()
		trace := "trace-123"
		request := "req-456"

		// Insert some logs
		for i := 1; i <= 5; i++ {
			entry := &domain.EndpointLogEntry{
				EndpointID: epID,
				ObservedAt: time.Now().UTC().Add(time.Duration(i) * time.Second),
				Level:      "info",
				Message:    "Log message",
				Attrs:      map[string]any{"index": i},
				TraceID:    &trace,
				RequestID:  &request,
			}
			err := logRepo.Create(ctx, entry)
			require.NoError(t, err)
			assert.Greater(t, entry.ID, int64(0))
		}

		// Retrieve all logs (latest first, so index 5 should be first)
		logs, err := logRepo.ListByEndpointID(ctx, epID, 0, 10)
		require.NoError(t, err)
		require.Len(t, logs, 5)
		assert.Equal(t, float64(5), logs[0].Attrs["index"].(float64))
		assert.Equal(t, float64(1), logs[4].Attrs["index"].(float64))

		// Retrieve with cursor (before ID of logs[2] which has index 3)
		cursorID := logs[2].ID
		subset, err := logRepo.ListByEndpointID(ctx, epID, cursorID, 10)
		require.NoError(t, err)
		// Should return logs with index 2 and index 1
		require.Len(t, subset, 2)
		assert.Equal(t, float64(2), subset[0].Attrs["index"].(float64))
		assert.Equal(t, float64(1), subset[1].Attrs["index"].(float64))
	})
}
