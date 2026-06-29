package handlers

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"

	"github.com/beremaran/straw/internal/domain"
)

const (
	testReportID         = "report-1"
	testReportRunID      = "run-1"
	testReportScheduleID = "schedule-1"
	testReportCron       = "*/5 * * * *"
)

type MockSavedReportRepo struct {
	mock.Mock
}

func (m *MockSavedReportRepo) Create(ctx context.Context, report *domain.SavedReport) error {
	args := m.Called(ctx, report)

	return args.Error(0)
}

func (m *MockSavedReportRepo) Update(ctx context.Context, report *domain.SavedReport) error {
	args := m.Called(ctx, report)

	return args.Error(0)
}

func (m *MockSavedReportRepo) Delete(ctx context.Context, id string) error {
	args := m.Called(ctx, id)

	return args.Error(0)
}

func (m *MockSavedReportRepo) GetByID(ctx context.Context, id string) (*domain.SavedReport, error) {
	args := m.Called(ctx, id)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}

	return args.Get(0).(*domain.SavedReport), args.Error(1)
}

func (m *MockSavedReportRepo) List(ctx context.Context, limit, offset int) ([]domain.SavedReport, int, error) {
	args := m.Called(ctx, limit, offset)
	if args.Get(0) == nil {
		return nil, 0, args.Error(2)
	}

	return args.Get(0).([]domain.SavedReport), args.Int(1), args.Error(2)
}

type MockReportRunRepo struct {
	mock.Mock
}

func (m *MockReportRunRepo) Create(ctx context.Context, run *domain.ReportRun) error {
	args := m.Called(ctx, run)

	return args.Error(0)
}

func (m *MockReportRunRepo) Update(ctx context.Context, run *domain.ReportRun) error {
	args := m.Called(ctx, run)

	return args.Error(0)
}

func (m *MockReportRunRepo) GetByID(ctx context.Context, id string) (*domain.ReportRun, error) {
	args := m.Called(ctx, id)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}

	return args.Get(0).(*domain.ReportRun), args.Error(1)
}

func (m *MockReportRunRepo) ListByReportID(ctx context.Context, reportID string, limit, offset int) ([]domain.ReportRun, int, error) {
	args := m.Called(ctx, reportID, limit, offset)
	if args.Get(0) == nil {
		return nil, 0, args.Error(2)
	}

	return args.Get(0).([]domain.ReportRun), args.Int(1), args.Error(2)
}

type MockReportScheduleRepo struct {
	mock.Mock
}

func (m *MockReportScheduleRepo) Create(ctx context.Context, schedule *domain.ReportSchedule) error {
	args := m.Called(ctx, schedule)

	return args.Error(0)
}

func (m *MockReportScheduleRepo) Update(ctx context.Context, schedule *domain.ReportSchedule) error {
	args := m.Called(ctx, schedule)

	return args.Error(0)
}

func (m *MockReportScheduleRepo) Disable(ctx context.Context, id string) error {
	args := m.Called(ctx, id)

	return args.Error(0)
}

func (m *MockReportScheduleRepo) GetByID(ctx context.Context, id string) (*domain.ReportSchedule, error) {
	args := m.Called(ctx, id)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}

	return args.Get(0).(*domain.ReportSchedule), args.Error(1)
}

func (m *MockReportScheduleRepo) List(ctx context.Context, limit, offset int) ([]domain.ReportSchedule, int, error) {
	args := m.Called(ctx, limit, offset)
	if args.Get(0) == nil {
		return nil, 0, args.Error(2)
	}

	return args.Get(0).([]domain.ReportSchedule), args.Int(1), args.Error(2)
}

func (m *MockReportScheduleRepo) ClaimDue(ctx context.Context, now time.Time, limit int) ([]domain.ReportSchedule, error) {
	args := m.Called(ctx, now, limit)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}

	return args.Get(0).([]domain.ReportSchedule), args.Error(1)
}

func (m *MockReportScheduleRepo) MarkRun(ctx context.Context, id string, lastRunAt time.Time, nextRunAt time.Time) error {
	args := m.Called(ctx, id, lastRunAt, nextRunAt)

	return args.Error(0)
}

func TestReportHandler_HandleCreateReport(t *testing.T) {
	reportRepo := new(MockSavedReportRepo)
	runRepo := new(MockReportRunRepo)
	auditRepo := new(MockManagementAuditRepo)
	handler := NewReportHandler(reportRepo, runRepo, nil, nil, nil, auditRepo, nil, t.TempDir())

	req := httptest.NewRequestWithContext(context.Background(), http.MethodPost, "/management/reports", bytes.NewBufferString(`{
		"name": "Usage",
		"type": "usage_summary",
		"filters": {"start": "2023-01-01", "end": "2023-01-02"}
	}`))
	rec := httptest.NewRecorder()

	reportRepo.On("Create", mock.Anything, mock.MatchedBy(func(report *domain.SavedReport) bool {
		return report.Name == "Usage" && report.Type == domain.ReportTypeUsageSummary && report.Format == domain.ReportFormatCSV
	})).Return(nil).Once()
	auditRepo.On("Create", mock.Anything, mock.MatchedBy(func(event *domain.ManagementAuditEvent) bool {
		return event.Action == domain.ActionCreate && event.EntityType == "report"
	})).Return(nil).Once()

	handler.HandleCreateReport(rec, req)

	assert.Equal(t, http.StatusCreated, rec.Code)
	reportRepo.AssertExpectations(t)
	auditRepo.AssertExpectations(t)
}

func TestReportHandler_HandleRunReportCreatesArtifactAndDownload(t *testing.T) {
	reportRepo := new(MockSavedReportRepo)
	runRepo := new(MockReportRunRepo)
	usageRepo := new(MockUsageRepo)
	auditRepo := new(MockManagementAuditRepo)
	handler := NewReportHandler(reportRepo, runRepo, usageRepo, nil, nil, auditRepo, nil, t.TempDir())

	report := testUsageReport()
	var savedRun domain.ReportRun

	req := httptest.NewRequestWithContext(context.Background(), http.MethodPost, "/management/reports/report-1/run", nil)
	req.SetPathValue("id", testReportID)
	rec := httptest.NewRecorder()

	reportRepo.On("GetByID", mock.Anything, testReportID).Return(report, nil).Once()
	runRepo.On("Create", mock.Anything, mock.MatchedBy(func(run *domain.ReportRun) bool {
		return run.ReportID == testReportID && run.Status == domain.ReportRunStatusRunning
	})).Return(nil).Once()
	usageRepo.On("GetDailySummaries", mock.Anything, "", mock.AnythingOfType("time.Time"), mock.AnythingOfType("time.Time")).Return([]domain.UsageSummary{
		{Date: usageTestDate, TotalRequests: 3, TotalBytes: 30, CostUnits: 1.5},
	}, nil).Once()
	runRepo.On("Update", mock.Anything, mock.MatchedBy(func(run *domain.ReportRun) bool {
		savedRun = *run

		return run.Status == domain.ReportRunStatusSucceeded && run.ArtifactURL != ""
	})).Return(nil).Once()
	auditRepo.On("Create", mock.Anything, mock.MatchedBy(func(event *domain.ManagementAuditEvent) bool {
		return event.Action == domain.ActionRun && event.EntityType == "report_run"
	})).Return(nil).Once()

	handler.HandleRunReport(rec, req)

	assert.Equal(t, http.StatusOK, rec.Code)
	assert.NotEmpty(t, savedRun.ArtifactURL)

	downloadReq := httptest.NewRequestWithContext(context.Background(), http.MethodGet, "/management/report-runs/run-1/download", nil)
	downloadReq.SetPathValue("run_id", testReportRunID)
	downloadRec := httptest.NewRecorder()

	savedRun.ID = testReportRunID
	runRepo.On("GetByID", mock.Anything, testReportRunID).Return(&savedRun, nil).Once()

	handler.HandleDownloadReportRun(downloadRec, downloadReq)

	require.Equal(t, http.StatusOK, downloadRec.Code)
	assert.Contains(t, downloadRec.Body.String(), "date,total_requests,total_bytes,cost_units")
	reportRepo.AssertExpectations(t)
	runRepo.AssertExpectations(t)
	usageRepo.AssertExpectations(t)
	auditRepo.AssertExpectations(t)
}

func TestReportHandler_HandleReportCRUDHistoryAndDetail(t *testing.T) {
	reportRepo := new(MockSavedReportRepo)
	runRepo := new(MockReportRunRepo)
	handler := NewReportHandler(reportRepo, runRepo, nil, nil, nil, nil, nil, t.TempDir())
	report := testUsageReport()
	now := time.Now().UTC()
	run := domain.ReportRun{ID: testReportRunID, ReportID: testReportID, Status: domain.ReportRunStatusSucceeded, StartedAt: now}

	reportRepo.On("List", mock.Anything, defaultReportLimit, 0).Return([]domain.SavedReport{*report}, 1, nil).Once()
	handler.HandleListReports(httptest.NewRecorder(), httptest.NewRequestWithContext(context.Background(), http.MethodGet, "/management/reports", nil))

	getReq := httptest.NewRequestWithContext(context.Background(), http.MethodGet, "/management/reports/report-1", nil)
	getReq.SetPathValue("id", testReportID)
	reportRepo.On("GetByID", mock.Anything, testReportID).Return(report, nil).Once()
	handler.HandleGetReport(httptest.NewRecorder(), getReq)

	updateReq := httptest.NewRequestWithContext(context.Background(), http.MethodPatch, "/management/reports/report-1", bytes.NewBufferString(`{"description":"Updated"}`))
	updateReq.SetPathValue("id", testReportID)
	reportRepo.On("GetByID", mock.Anything, testReportID).Return(report, nil).Once()
	reportRepo.On("Update", mock.Anything, mock.MatchedBy(func(report *domain.SavedReport) bool {
		return report.Description == "Updated"
	})).Return(nil).Once()
	handler.HandleUpdateReport(httptest.NewRecorder(), updateReq)

	runsReq := httptest.NewRequestWithContext(context.Background(), http.MethodGet, "/management/reports/report-1/runs", nil)
	runsReq.SetPathValue("id", testReportID)
	reportRepo.On("GetByID", mock.Anything, testReportID).Return(report, nil).Once()
	runRepo.On("ListByReportID", mock.Anything, testReportID, defaultReportLimit, 0).Return([]domain.ReportRun{run}, 1, nil).Once()
	handler.HandleListReportRuns(httptest.NewRecorder(), runsReq)

	runReq := httptest.NewRequestWithContext(context.Background(), http.MethodGet, "/management/report-runs/run-1", nil)
	runReq.SetPathValue("run_id", testReportRunID)
	runRepo.On("GetByID", mock.Anything, testReportRunID).Return(&run, nil).Once()
	handler.HandleGetReportRun(httptest.NewRecorder(), runReq)

	deleteReq := httptest.NewRequestWithContext(context.Background(), http.MethodDelete, "/management/reports/report-1", nil)
	deleteReq.SetPathValue("id", testReportID)
	reportRepo.On("GetByID", mock.Anything, testReportID).Return(report, nil).Once()
	reportRepo.On("Delete", mock.Anything, testReportID).Return(nil).Once()
	handler.HandleDeleteReport(httptest.NewRecorder(), deleteReq)

	reportRepo.AssertExpectations(t)
	runRepo.AssertExpectations(t)
}

func TestReportHandler_HandleReportSchedules(t *testing.T) {
	reportRepo := new(MockSavedReportRepo)
	runRepo := new(MockReportRunRepo)
	scheduleRepo := new(MockReportScheduleRepo)
	handler := NewReportHandler(reportRepo, runRepo, nil, nil, nil, nil, nil, t.TempDir(), scheduleRepo)
	report := testUsageReport()
	schedule := testReportSchedule()

	createReq := httptest.NewRequestWithContext(context.Background(), http.MethodPost, "/management/report-schedules", bytes.NewBufferString(`{
		"report_id": "report-1",
		"cron": "*/5 * * * *",
		"timezone": "UTC"
	}`))
	createRec := httptest.NewRecorder()
	reportRepo.On("GetByID", mock.Anything, testReportID).Return(report, nil).Once()
	scheduleRepo.On("Create", mock.Anything, mock.MatchedBy(func(schedule *domain.ReportSchedule) bool {
		return schedule.ReportID == testReportID && schedule.Cron == testReportCron && schedule.IsActive
	})).Return(nil).Once()
	handler.HandleCreateReportSchedule(createRec, createReq)
	assert.Equal(t, http.StatusCreated, createRec.Code)

	listReq := httptest.NewRequestWithContext(context.Background(), http.MethodGet, "/management/report-schedules", nil)
	listRec := httptest.NewRecorder()
	scheduleRepo.On("List", mock.Anything, defaultReportLimit, 0).Return([]domain.ReportSchedule{*schedule}, 1, nil).Once()
	handler.HandleListReportSchedules(listRec, listReq)
	assert.Equal(t, http.StatusOK, listRec.Code)

	updateReq := httptest.NewRequestWithContext(context.Background(), http.MethodPatch, "/management/report-schedules/schedule-1", bytes.NewBufferString(`{"is_active":false}`))
	updateReq.SetPathValue("id", testReportScheduleID)
	updateRec := httptest.NewRecorder()
	scheduleRepo.On("GetByID", mock.Anything, testReportScheduleID).Return(schedule, nil).Once()
	scheduleRepo.On("Update", mock.Anything, mock.MatchedBy(func(schedule *domain.ReportSchedule) bool {
		return !schedule.IsActive
	})).Return(nil).Once()
	handler.HandleUpdateReportSchedule(updateRec, updateReq)
	assert.Equal(t, http.StatusOK, updateRec.Code)

	deleteReq := httptest.NewRequestWithContext(context.Background(), http.MethodDelete, "/management/report-schedules/schedule-1", nil)
	deleteReq.SetPathValue("id", testReportScheduleID)
	deleteRec := httptest.NewRecorder()
	scheduleRepo.On("GetByID", mock.Anything, testReportScheduleID).Return(schedule, nil).Once()
	scheduleRepo.On("Disable", mock.Anything, testReportScheduleID).Return(nil).Once()
	handler.HandleDeleteReportSchedule(deleteRec, deleteReq)
	assert.Equal(t, http.StatusNoContent, deleteRec.Code)

	reportRepo.AssertExpectations(t)
	scheduleRepo.AssertExpectations(t)
}

func TestReportHandler_HandleCreateReportScheduleRejectsBadCron(t *testing.T) {
	reportRepo := new(MockSavedReportRepo)
	runRepo := new(MockReportRunRepo)
	scheduleRepo := new(MockReportScheduleRepo)
	handler := NewReportHandler(reportRepo, runRepo, nil, nil, nil, nil, nil, t.TempDir(), scheduleRepo)

	req := httptest.NewRequestWithContext(context.Background(), http.MethodPost, "/management/report-schedules", bytes.NewBufferString(`{
		"report_id": "report-1",
		"cron": "bad",
		"timezone": "UTC"
	}`))
	rec := httptest.NewRecorder()

	reportRepo.On("GetByID", mock.Anything, testReportID).Return(testUsageReport(), nil).Once()

	handler.HandleCreateReportSchedule(rec, req)

	assert.Equal(t, http.StatusBadRequest, rec.Code)
	reportRepo.AssertExpectations(t)
	scheduleRepo.AssertNotCalled(t, "Create")
}

func TestReportHandler_HandleRunReportRejectsExcessiveDateRange(t *testing.T) {
	reportRepo := new(MockSavedReportRepo)
	runRepo := new(MockReportRunRepo)
	usageRepo := new(MockUsageRepo)
	handler := NewReportHandler(reportRepo, runRepo, usageRepo, nil, nil, nil, nil, t.TempDir())

	report := testUsageReport()
	report.Filters = domain.ConfigMap{reportFilterStart: usageTestDate, reportFilterEnd: "2023-03-01"}

	req := httptest.NewRequestWithContext(context.Background(), http.MethodPost, "/management/reports/report-1/run", nil)
	req.SetPathValue("id", testReportID)
	rec := httptest.NewRecorder()

	reportRepo.On("GetByID", mock.Anything, testReportID).Return(report, nil).Once()
	runRepo.On("Create", mock.Anything, mock.Anything).Return(nil).Once()
	runRepo.On("Update", mock.Anything, mock.MatchedBy(func(run *domain.ReportRun) bool {
		return run.Status == domain.ReportRunStatusFailed
	})).Return(nil).Once()

	handler.HandleRunReport(rec, req)

	assert.Equal(t, http.StatusBadRequest, rec.Code)

	var resp map[string]any
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &resp))
	assert.Equal(t, domain.ReportRunStatusFailed, resp["status"])
	reportRepo.AssertExpectations(t)
	runRepo.AssertExpectations(t)
	usageRepo.AssertNotCalled(t, "GetDailySummaries")
}

func testUsageReport() *domain.SavedReport {
	now := time.Now().UTC()

	return &domain.SavedReport{
		ID:        testReportID,
		Name:      "Usage",
		Type:      domain.ReportTypeUsageSummary,
		Filters:   domain.ConfigMap{reportFilterStart: usageTestDate, reportFilterEnd: "2023-01-02"},
		Format:    domain.ReportFormatCSV,
		CreatedAt: now,
		UpdatedAt: now,
	}
}

func testReportSchedule() *domain.ReportSchedule {
	now := time.Now().UTC()
	next := now.Add(time.Hour)

	return &domain.ReportSchedule{
		ID:        testReportScheduleID,
		ReportID:  testReportID,
		Cron:      testReportCron,
		Timezone:  "UTC",
		IsActive:  true,
		NextRunAt: &next,
		CreatedAt: now,
		UpdatedAt: now,
	}
}
