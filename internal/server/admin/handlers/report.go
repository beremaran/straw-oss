package handlers

import (
	"bytes"
	"context"
	"encoding/csv"
	"errors"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/google/uuid"

	"github.com/beremaran/straw/internal/domain"
	"github.com/beremaran/straw/internal/server/admin/middleware"
	"github.com/beremaran/straw/internal/server/dto"
	"github.com/beremaran/straw/internal/server/helper"
)

const (
	maxReportLimit     = 100
	defaultReportLimit = 20
	maxReportRows      = 500
	reportDateLayout   = "2006-01-02"
	reportFilterStart  = "start"
	reportFilterEnd    = "end"
	reportDirPerm      = 0o700
	reportFilePerm     = 0o600
)

const maxReportDateRange = 31 * 24 * time.Hour

var (
	errReportNameRequired      = errors.New("name is required")
	errReportTypeUnsupported   = errors.New("unsupported report type")
	errReportFormatUnsupported = errors.New("unsupported report format")
	errReportDateRange         = errors.New("report date range cannot exceed 31 days")
	errReportDateOrder         = errors.New("start must be before end")
	errReportTooManyRows       = errors.New("report row count exceeds limit")
	errReportSourceUnavailable = errors.New("report source is unavailable")
)

// ReportHandler manages saved reports and report runs.
type ReportHandler struct {
	reportRepo         domain.SavedReportRepository
	runRepo            domain.ReportRunRepository
	usageRepo          domain.UsageRepository
	apiKeyRepo         domain.APIKeyRepository
	endpointRepo       domain.EndpointRepository
	auditRepo          domain.ManagementAuditRepository
	costMultiplierRepo domain.CostMultiplierRepository
	artifactDir        string
}

// NewReportHandler creates a new ReportHandler.
func NewReportHandler(
	reportRepo domain.SavedReportRepository,
	runRepo domain.ReportRunRepository,
	usageRepo domain.UsageRepository,
	apiKeyRepo domain.APIKeyRepository,
	endpointRepo domain.EndpointRepository,
	auditRepo domain.ManagementAuditRepository,
	costMultiplierRepo domain.CostMultiplierRepository,
	artifactDir string,
) *ReportHandler {
	return &ReportHandler{
		reportRepo:         reportRepo,
		runRepo:            runRepo,
		usageRepo:          usageRepo,
		apiKeyRepo:         apiKeyRepo,
		endpointRepo:       endpointRepo,
		auditRepo:          auditRepo,
		costMultiplierRepo: costMultiplierRepo,
		artifactDir:        artifactDir,
	}
}

// HandleListReports lists saved reports.
func (h *ReportHandler) HandleListReports(w http.ResponseWriter, r *http.Request) {
	page, limit := reportPageLimit(r)

	reports, total, err := h.reportRepo.List(r.Context(), limit, (page-1)*limit)
	if err != nil {
		helper.WriteError(w, http.StatusInternalServerError, "failed to list reports")

		return
	}

	helper.WriteJSON(w, http.StatusOK, dto.ListReportsResponse{
		Data:  dto.FromReports(reports),
		Total: total,
		Page:  page,
		Limit: limit,
	})
}

// HandleCreateReport creates a saved report.
func (h *ReportHandler) HandleCreateReport(w http.ResponseWriter, r *http.Request) {
	var req dto.CreateReportRequest

	err := helper.ReadJSON(r, &req)
	if err != nil {
		helper.WriteError(w, http.StatusBadRequest, "invalid request body")

		return
	}

	report := reportFromCreateRequest(req, createdByFromContext(r.Context()))

	err = validateReport(report)
	if err != nil {
		helper.WriteError(w, http.StatusBadRequest, err.Error())

		return
	}

	err = h.reportRepo.Create(r.Context(), report)
	if err != nil {
		helper.WriteError(w, http.StatusInternalServerError, "failed to create report")

		return
	}

	resp := dto.FromReport(report)
	h.audit(r, domain.ActionCreate, "report", report.ID, nil, resp)

	helper.WriteJSON(w, http.StatusCreated, resp)
}

// HandleGetReport returns a saved report.
func (h *ReportHandler) HandleGetReport(w http.ResponseWriter, r *http.Request) {
	report := h.loadReport(w, r.PathValue("id"), r)
	if report == nil {
		return
	}

	helper.WriteJSON(w, http.StatusOK, dto.FromReport(report))
}

// HandleUpdateReport updates a saved report.
func (h *ReportHandler) HandleUpdateReport(w http.ResponseWriter, r *http.Request) {
	report := h.loadReport(w, r.PathValue("id"), r)
	if report == nil {
		return
	}

	var req dto.UpdateReportRequest

	err := helper.ReadJSON(r, &req)
	if err != nil {
		helper.WriteError(w, http.StatusBadRequest, "invalid request body")

		return
	}

	oldReport := *report
	applyReportUpdate(report, req)

	err = validateReport(report)
	if err != nil {
		helper.WriteError(w, http.StatusBadRequest, err.Error())

		return
	}

	report.UpdatedAt = time.Now().UTC()

	err = h.reportRepo.Update(r.Context(), report)
	if err != nil {
		helper.WriteError(w, http.StatusInternalServerError, "failed to update report")

		return
	}

	resp := dto.FromReport(report)
	h.audit(r, domain.ActionUpdate, "report", report.ID, dto.FromReport(&oldReport), resp)

	helper.WriteJSON(w, http.StatusOK, resp)
}

// HandleDeleteReport deletes a saved report.
func (h *ReportHandler) HandleDeleteReport(w http.ResponseWriter, r *http.Request) {
	report := h.loadReport(w, r.PathValue("id"), r)
	if report == nil {
		return
	}

	err := h.reportRepo.Delete(r.Context(), report.ID)
	if err != nil {
		helper.WriteError(w, http.StatusInternalServerError, "failed to delete report")

		return
	}

	h.audit(r, domain.ActionDelete, "report", report.ID, dto.FromReport(report), nil)
	w.WriteHeader(http.StatusNoContent)
}

// HandleRunReport runs a saved report immediately.
func (h *ReportHandler) HandleRunReport(w http.ResponseWriter, r *http.Request) {
	report := h.loadReport(w, r.PathValue("id"), r)
	if report == nil {
		return
	}

	run := &domain.ReportRun{
		ID:        uuid.New().String(),
		ReportID:  report.ID,
		Status:    domain.ReportRunStatusRunning,
		StartedAt: time.Now().UTC(),
	}

	err := h.runRepo.Create(r.Context(), run)
	if err != nil {
		helper.WriteError(w, http.StatusInternalServerError, "failed to create report run")

		return
	}

	artifactURL, runErr := h.runReport(r.Context(), report, run.ID)
	completedAt := time.Now().UTC()

	run.CompletedAt = &completedAt
	if runErr != nil {
		run.Status = domain.ReportRunStatusFailed
		run.Error = runErr.Error()
	} else {
		run.Status = domain.ReportRunStatusSucceeded
		run.ArtifactURL = artifactURL
	}

	err = h.runRepo.Update(r.Context(), run)
	if err != nil {
		helper.WriteError(w, http.StatusInternalServerError, "failed to update report run")

		return
	}

	resp := dto.FromReportRun(run)
	h.audit(r, domain.ActionRun, "report_run", run.ID, nil, resp)

	if runErr != nil {
		helper.WriteJSON(w, reportRunErrorStatus(runErr), resp)

		return
	}

	helper.WriteJSON(w, http.StatusOK, resp)
}

// HandleListReportRuns lists runs for a report.
func (h *ReportHandler) HandleListReportRuns(w http.ResponseWriter, r *http.Request) {
	report := h.loadReport(w, r.PathValue("id"), r)
	if report == nil {
		return
	}

	page, limit := reportPageLimit(r)

	runs, total, err := h.runRepo.ListByReportID(r.Context(), report.ID, limit, (page-1)*limit)
	if err != nil {
		helper.WriteError(w, http.StatusInternalServerError, "failed to list report runs")

		return
	}

	helper.WriteJSON(w, http.StatusOK, dto.ListReportRunsResponse{
		Data:  dto.FromReportRuns(runs),
		Total: total,
		Page:  page,
		Limit: limit,
	})
}

// HandleGetReportRun returns a report run.
func (h *ReportHandler) HandleGetReportRun(w http.ResponseWriter, r *http.Request) {
	run := h.loadReportRun(w, r.PathValue("run_id"), r)
	if run == nil {
		return
	}

	helper.WriteJSON(w, http.StatusOK, dto.FromReportRun(run))
}

// HandleDownloadReportRun downloads a report artifact.
func (h *ReportHandler) HandleDownloadReportRun(w http.ResponseWriter, r *http.Request) {
	run := h.loadReportRun(w, r.PathValue("run_id"), r)
	if run == nil {
		return
	}

	if run.ArtifactURL == "" || run.Status != domain.ReportRunStatusSucceeded {
		helper.WriteError(w, http.StatusNotFound, "report artifact not found")

		return
	}

	w.Header().Set("Content-Type", "text/csv")
	w.Header().Set("Content-Disposition", fmt.Sprintf(`attachment; filename="%s.csv"`, run.ID))
	http.ServeFile(w, r, run.ArtifactURL)
}

func (h *ReportHandler) runReport(ctx context.Context, report *domain.SavedReport, runID string) (string, error) {
	rows, err := h.generateRows(ctx, report)
	if err != nil {
		return "", err
	}

	content, err := csvContent(rows)
	if err != nil {
		return "", err
	}

	return h.writeArtifact(runID, content)
}

func (h *ReportHandler) generateRows(ctx context.Context, report *domain.SavedReport) ([][]string, error) {
	switch report.Type {
	case domain.ReportTypeUsageSummary:
		return h.usageSummaryRows(ctx, report.Filters)
	case domain.ReportTypeBillingEstimate:
		return h.billingEstimateRows(ctx, report.Filters)
	case domain.ReportTypeAPIKeyInventory:
		return h.apiKeyInventoryRows(ctx)
	case domain.ReportTypeEndpointHealth:
		return h.endpointHealthRows(ctx)
	case domain.ReportTypeAuditEvents:
		return h.auditEventRows(ctx, report.Filters)
	default:
		return nil, errReportTypeUnsupported
	}
}

func (h *ReportHandler) usageSummaryRows(ctx context.Context, filters domain.ConfigMap) ([][]string, error) {
	if h.usageRepo == nil {
		return nil, errReportSourceUnavailable
	}

	start, end, err := reportDateRange(filters)
	if err != nil {
		return nil, err
	}

	summaries, err := h.usageRepo.GetDailySummaries(ctx, filterString(filters, "api_key_id"), start, end)
	if err != nil {
		return nil, fmt.Errorf("get usage summaries: %w", err)
	}

	if len(summaries) > maxReportRows {
		return nil, errReportTooManyRows
	}

	rows := [][]string{{"date", "total_requests", "total_bytes", "cost_units"}}
	for _, summary := range summaries {
		rows = append(rows, []string{
			summary.Date,
			strconv.FormatInt(summary.TotalRequests, 10),
			strconv.FormatInt(summary.TotalBytes, 10),
			strconv.FormatFloat(summary.CostUnits, 'f', -1, 64),
		})
	}

	return rows, nil
}

func (h *ReportHandler) billingEstimateRows(ctx context.Context, filters domain.ConfigMap) ([][]string, error) {
	if h.usageRepo == nil {
		return nil, errReportSourceUnavailable
	}

	start, end, err := reportDateRange(filters)
	if err != nil {
		return nil, err
	}

	summaries, err := h.usageRepo.GetDailySummaries(ctx, filterString(filters, "api_key_id"), start, end)
	if err != nil {
		return nil, fmt.Errorf("get billing summaries: %w", err)
	}

	if len(summaries) > maxReportRows {
		return nil, errReportTooManyRows
	}

	multipliers, err := h.activeReportMultipliers(ctx)
	if err != nil {
		return nil, err
	}

	totalCost := billingCostUnits(summaries, multipliers)
	estimatedUSD := totalCost * costPerUnitUSD

	return [][]string{
		{reportFilterStart, reportFilterEnd, "total_cost_units", "estimated_usd", "currency", "pricing_version"},
		{
			start.Format(reportDateLayout),
			end.Format(reportDateLayout),
			strconv.FormatFloat(totalCost, 'f', -1, 64),
			strconv.FormatFloat(estimatedUSD, 'f', -1, 64),
			"USD",
			pricingVersion(multipliers),
		},
	}, nil
}

func (h *ReportHandler) apiKeyInventoryRows(ctx context.Context) ([][]string, error) {
	if h.apiKeyRepo == nil {
		return nil, errReportSourceUnavailable
	}

	keys, total, err := h.apiKeyRepo.List(ctx, maxReportRows+1, 0)
	if err != nil {
		return nil, fmt.Errorf("list api keys: %w", err)
	}

	if total > maxReportRows || len(keys) > maxReportRows {
		return nil, errReportTooManyRows
	}

	rows := [][]string{{"id", "name", "is_active", "created_at", "expires_at"}}

	for _, key := range keys {
		expiresAt := ""
		if key.ExpiresAt != nil {
			expiresAt = key.ExpiresAt.Format(time.RFC3339)
		}

		rows = append(rows, []string{
			key.ID,
			key.Name,
			strconv.FormatBool(key.IsActive),
			key.CreatedAt.Format(time.RFC3339),
			expiresAt,
		})
	}

	return rows, nil
}

func (h *ReportHandler) endpointHealthRows(ctx context.Context) ([][]string, error) {
	if h.endpointRepo == nil {
		return nil, errReportSourceUnavailable
	}

	endpoints, total, err := h.endpointRepo.List(ctx, maxReportRows+1, 0, true)
	if err != nil {
		return nil, fmt.Errorf("list endpoints: %w", err)
	}

	if total > maxReportRows || len(endpoints) > maxReportRows {
		return nil, errReportTooManyRows
	}

	rows := [][]string{{"id", "tags", "is_healthy", "desired_state", "is_registered", "last_heartbeat"}}
	for _, endpoint := range endpoints {
		rows = append(rows, []string{
			endpoint.ID,
			strings.Join(endpoint.Tags, ","),
			strconv.FormatBool(endpoint.IsHealthy),
			string(endpoint.DesiredState),
			strconv.FormatBool(endpoint.IsRegistered),
			endpoint.LastHeartbeat.Format(time.RFC3339),
		})
	}

	return rows, nil
}

func (h *ReportHandler) auditEventRows(ctx context.Context, filters domain.ConfigMap) ([][]string, error) {
	if h.auditRepo == nil {
		return nil, errReportSourceUnavailable
	}

	start, end, err := reportDateRange(filters)
	if err != nil {
		return nil, err
	}

	events, total, err := h.auditRepo.ListEvents(ctx, domain.AuditEventFilter{
		StartDate: &start,
		EndDate:   &end,
		Limit:     maxReportRows + 1,
	})
	if err != nil {
		return nil, fmt.Errorf("list audit events: %w", err)
	}

	if total > maxReportRows || len(events) > maxReportRows {
		return nil, errReportTooManyRows
	}

	rows := [][]string{{"id", "occurred_at", "actor_id", "action", "entity_type", "entity_id"}}
	for _, event := range events {
		rows = append(rows, []string{
			strconv.FormatInt(event.ID, 10),
			event.OccurredAt.Format(time.RFC3339),
			event.ActorID,
			event.Action,
			event.EntityType,
			event.EntityID,
		})
	}

	return rows, nil
}

func (h *ReportHandler) activeReportMultipliers(ctx context.Context) ([]domain.CostMultiplier, error) {
	if h.costMultiplierRepo == nil {
		return nil, nil
	}

	multipliers, err := h.costMultiplierRepo.ListActive(ctx)
	if err != nil {
		return nil, fmt.Errorf("list active cost multipliers: %w", err)
	}

	return multipliers, nil
}

func (h *ReportHandler) writeArtifact(runID string, content []byte) (string, error) {
	err := os.MkdirAll(h.artifactDir, reportDirPerm)
	if err != nil {
		return "", fmt.Errorf("create report artifact dir: %w", err)
	}

	path := filepath.Join(h.artifactDir, runID+".csv")

	err = os.WriteFile(path, content, reportFilePerm)
	if err != nil {
		return "", fmt.Errorf("write report artifact: %w", err)
	}

	return path, nil
}

func (h *ReportHandler) loadReport(w http.ResponseWriter, id string, r *http.Request) *domain.SavedReport {
	report, err := h.reportRepo.GetByID(r.Context(), id)
	if err != nil {
		helper.WriteError(w, http.StatusInternalServerError, "failed to get report")

		return nil
	}

	if report == nil {
		helper.WriteError(w, http.StatusNotFound, "report not found")

		return nil
	}

	return report
}

func (h *ReportHandler) loadReportRun(w http.ResponseWriter, id string, r *http.Request) *domain.ReportRun {
	run, err := h.runRepo.GetByID(r.Context(), id)
	if err != nil {
		helper.WriteError(w, http.StatusInternalServerError, "failed to get report run")

		return nil
	}

	if run == nil {
		helper.WriteError(w, http.StatusNotFound, "report run not found")

		return nil
	}

	return run
}

func (h *ReportHandler) audit(r *http.Request, action, entityType, id string, oldValue, newValue any) {
	if h.auditRepo == nil {
		return
	}

	event := middleware.NewAuditEvent(r, action, entityType, id, oldValue, newValue)
	_ = h.auditRepo.Create(r.Context(), event)
}

func reportFromCreateRequest(req dto.CreateReportRequest, createdBy string) *domain.SavedReport {
	now := time.Now().UTC()

	return &domain.SavedReport{
		ID:          uuid.New().String(),
		Name:        strings.TrimSpace(req.Name),
		Description: strings.TrimSpace(req.Description),
		Type:        strings.TrimSpace(req.Type),
		Filters:     normalizeFilters(req.Filters),
		Format:      reportFormat(req.Format),
		CreatedBy:   createdBy,
		CreatedAt:   now,
		UpdatedAt:   now,
	}
}

func applyReportUpdate(report *domain.SavedReport, req dto.UpdateReportRequest) {
	if req.Name != nil {
		report.Name = strings.TrimSpace(*req.Name)
	}

	if req.Description != nil {
		report.Description = strings.TrimSpace(*req.Description)
	}

	if req.Type != nil {
		report.Type = strings.TrimSpace(*req.Type)
	}

	if req.Filters != nil {
		report.Filters = normalizeFilters(*req.Filters)
	}

	if req.Format != nil {
		report.Format = reportFormat(*req.Format)
	}
}

func validateReport(report *domain.SavedReport) error {
	if report.Name == "" {
		return errReportNameRequired
	}

	if !supportedReportType(report.Type) {
		return errReportTypeUnsupported
	}

	if report.Format != domain.ReportFormatCSV {
		return errReportFormatUnsupported
	}

	return nil
}

func supportedReportType(reportType string) bool {
	switch reportType {
	case domain.ReportTypeUsageSummary,
		domain.ReportTypeBillingEstimate,
		domain.ReportTypeAPIKeyInventory,
		domain.ReportTypeEndpointHealth,
		domain.ReportTypeAuditEvents:
		return true
	default:
		return false
	}
}

func reportFormat(format string) string {
	format = strings.TrimSpace(format)
	if format == "" {
		return domain.ReportFormatCSV
	}

	return format
}

func normalizeFilters(filters domain.ConfigMap) domain.ConfigMap {
	if filters == nil {
		return domain.ConfigMap{}
	}

	return filters
}

func createdByFromContext(ctx context.Context) string {
	actor, ok := middleware.ActorFromContext(ctx)
	if !ok {
		return ""
	}

	_, err := uuid.Parse(actor.ID)
	if err != nil {
		return ""
	}

	return actor.ID
}

func reportPageLimit(r *http.Request) (int, int) {
	page, _ := strconv.Atoi(r.URL.Query().Get("page"))
	if page < 1 {
		page = 1
	}

	limit, _ := strconv.Atoi(r.URL.Query().Get("limit"))
	if limit < 1 || limit > maxReportLimit {
		limit = defaultReportLimit
	}

	return page, limit
}

func reportDateRange(filters domain.ConfigMap) (time.Time, time.Time, error) {
	now := time.Now().UTC()
	start := now.AddDate(0, 0, -30)
	end := now

	var err error

	if startStr := filterString(filters, reportFilterStart); startStr != "" {
		start, err = time.Parse(reportDateLayout, startStr)
		if err != nil {
			return time.Time{}, time.Time{}, fmt.Errorf("parse report start date: %w", err)
		}
	}

	if endStr := filterString(filters, reportFilterEnd); endStr != "" {
		end, err = time.Parse(reportDateLayout, endStr)
		if err != nil {
			return time.Time{}, time.Time{}, fmt.Errorf("parse report end date: %w", err)
		}
	}

	if end.Before(start) {
		return time.Time{}, time.Time{}, errReportDateOrder
	}

	if end.Sub(start) > maxReportDateRange {
		return time.Time{}, time.Time{}, errReportDateRange
	}

	return start, end, nil
}

func filterString(filters domain.ConfigMap, key string) string {
	value, ok := filters[key]
	if !ok || value == nil {
		return ""
	}

	switch v := value.(type) {
	case string:
		return strings.TrimSpace(v)
	default:
		return strings.TrimSpace(fmt.Sprint(v))
	}
}

func csvContent(rows [][]string) ([]byte, error) {
	var buf bytes.Buffer

	writer := csv.NewWriter(&buf)

	err := writer.WriteAll(rows)
	if err != nil {
		return nil, fmt.Errorf("write csv: %w", err)
	}

	return buf.Bytes(), nil
}

func reportRunErrorStatus(err error) int {
	if errors.Is(err, errReportDateRange) ||
		errors.Is(err, errReportDateOrder) ||
		errors.Is(err, errReportTooManyRows) ||
		errors.Is(err, errReportTypeUnsupported) ||
		errors.Is(err, errReportSourceUnavailable) {
		return http.StatusBadRequest
	}

	return http.StatusInternalServerError
}
