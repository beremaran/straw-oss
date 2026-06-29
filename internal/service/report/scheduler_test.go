package report

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/beremaran/straw/internal/domain"
)

type fakeScheduleRepo struct {
	claimed []domain.ReportSchedule
	marked  bool
}

func (f *fakeScheduleRepo) Create(context.Context, *domain.ReportSchedule) error {
	return nil
}

func (f *fakeScheduleRepo) Update(context.Context, *domain.ReportSchedule) error {
	return nil
}

func (f *fakeScheduleRepo) Disable(context.Context, string) error {
	return nil
}

func (f *fakeScheduleRepo) GetByID(context.Context, string) (*domain.ReportSchedule, error) {
	return nil, nil
}

func (f *fakeScheduleRepo) List(context.Context, int, int) ([]domain.ReportSchedule, int, error) {
	return nil, 0, nil
}

func (f *fakeScheduleRepo) ClaimDue(context.Context, time.Time, int) ([]domain.ReportSchedule, error) {
	return f.claimed, nil
}

func (f *fakeScheduleRepo) MarkRun(context.Context, string, time.Time, time.Time) error {
	f.marked = true

	return nil
}

type fakeRunner struct {
	reportID   string
	scheduleID string
}

func (f *fakeRunner) RunScheduledReport(_ context.Context, reportID string, scheduleID string) (*domain.ReportRun, error) {
	f.reportID = reportID
	f.scheduleID = scheduleID

	return &domain.ReportRun{ID: "run-1", Status: domain.ReportRunStatusSucceeded}, nil
}

func TestScheduler_RunDueOnce(t *testing.T) {
	repo := &fakeScheduleRepo{claimed: []domain.ReportSchedule{
		{ID: "schedule-1", ReportID: "report-1", Cron: "*/5 * * * *"},
	}}
	runner := &fakeRunner{}
	scheduler := NewScheduler(repo, runner, time.Minute)

	err := scheduler.RunDueOnce(context.Background())
	require.NoError(t, err)

	assert.Equal(t, "report-1", runner.reportID)
	assert.Equal(t, "schedule-1", runner.scheduleID)
	assert.True(t, repo.marked)
}
