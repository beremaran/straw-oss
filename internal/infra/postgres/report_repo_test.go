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

func TestReportScheduleRepository_ClaimDue(t *testing.T) {
	dsn := os.Getenv("TEST_DB_DSN")
	if dsn == "" {
		t.Skip("Skipping integration test: TEST_DB_DSN not set")
	}

	ctx := context.Background()
	pool, err := pgxpool.New(ctx, dsn)
	require.NoError(t, err)
	defer pool.Close()

	_, err = pool.Exec(ctx, "TRUNCATE report_runs, report_schedules, saved_reports CASCADE")
	require.NoError(t, err)

	client := &Client{Pool: pool}
	reportRepo := NewSavedReportRepository(client)
	scheduleRepo := NewReportScheduleRepository(client)
	now := time.Now().UTC()
	report := &domain.SavedReport{
		ID:        uuid.New().String(),
		Name:      "Usage",
		Type:      domain.ReportTypeUsageSummary,
		Filters:   domain.ConfigMap{},
		Format:    domain.ReportFormatCSV,
		CreatedAt: now,
		UpdatedAt: now,
	}
	require.NoError(t, reportRepo.Create(ctx, report))

	due := now.Add(-time.Minute)
	active := reportScheduleForTest(report.ID, due, true)
	inactive := reportScheduleForTest(report.ID, due, false)
	require.NoError(t, scheduleRepo.Create(ctx, active))
	require.NoError(t, scheduleRepo.Create(ctx, inactive))

	claimed, err := scheduleRepo.ClaimDue(ctx, now, 10)
	require.NoError(t, err)
	require.Len(t, claimed, 1)
	assert.Equal(t, active.ID, claimed[0].ID)

	claimed, err = scheduleRepo.ClaimDue(ctx, now, 10)
	require.NoError(t, err)
	assert.Empty(t, claimed)
}

func reportScheduleForTest(reportID string, nextRunAt time.Time, active bool) *domain.ReportSchedule {
	now := time.Now().UTC()

	return &domain.ReportSchedule{
		ID:        uuid.New().String(),
		ReportID:  reportID,
		Cron:      "*/5 * * * *",
		Timezone:  "UTC",
		IsActive:  active,
		NextRunAt: &nextRunAt,
		CreatedAt: now,
		UpdatedAt: now,
	}
}
