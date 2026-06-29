package dto

import (
	"time"

	"github.com/beremaran/straw/internal/domain"
)

// CreateReportRequest creates a saved report.
type CreateReportRequest struct {
	Name        string           `json:"name"`
	Description string           `json:"description,omitempty"`
	Type        string           `json:"type"`
	Filters     domain.ConfigMap `json:"filters,omitempty"`
	Format      string           `json:"format,omitempty"`
}

// UpdateReportRequest updates a saved report.
type UpdateReportRequest struct {
	Name        *string           `json:"name,omitempty"`
	Description *string           `json:"description,omitempty"`
	Type        *string           `json:"type,omitempty"`
	Filters     *domain.ConfigMap `json:"filters,omitempty"`
	Format      *string           `json:"format,omitempty"`
}

// ReportResponse represents a saved report.
type ReportResponse struct {
	ID          string           `json:"id"`
	Name        string           `json:"name"`
	Description string           `json:"description,omitempty"`
	Type        string           `json:"type"`
	Filters     domain.ConfigMap `json:"filters"`
	Format      string           `json:"format"`
	CreatedBy   string           `json:"created_by,omitempty"`
	CreatedAt   time.Time        `json:"created_at"`
	UpdatedAt   time.Time        `json:"updated_at"`
}

// ReportRunResponse represents a report run.
type ReportRunResponse struct {
	ID          string     `json:"id"`
	ReportID    string     `json:"report_id"`
	ScheduleID  string     `json:"schedule_id,omitempty"`
	Status      string     `json:"status"`
	StartedAt   time.Time  `json:"started_at"`
	CompletedAt *time.Time `json:"completed_at,omitempty"`
	ArtifactURL string     `json:"artifact_url,omitempty"`
	Error       string     `json:"error,omitempty"`
}

// ListReportsResponse is a paginated list of saved reports.
type ListReportsResponse = PaginatedResponse[ReportResponse]

// ListReportRunsResponse is a paginated list of report runs.
type ListReportRunsResponse = PaginatedResponse[ReportRunResponse]

// FromReport converts a domain.SavedReport to a ReportResponse.
func FromReport(report *domain.SavedReport) *ReportResponse {
	if report == nil {
		return nil
	}

	return &ReportResponse{
		ID:          report.ID,
		Name:        report.Name,
		Description: report.Description,
		Type:        report.Type,
		Filters:     report.Filters,
		Format:      report.Format,
		CreatedBy:   report.CreatedBy,
		CreatedAt:   report.CreatedAt,
		UpdatedAt:   report.UpdatedAt,
	}
}

// FromReports converts saved reports to report responses.
func FromReports(reports []domain.SavedReport) []ReportResponse {
	result := make([]ReportResponse, len(reports))
	for i, report := range reports {
		resp := FromReport(&report)
		if resp != nil {
			result[i] = *resp
		}
	}

	return result
}

// FromReportRun converts a domain.ReportRun to a ReportRunResponse.
func FromReportRun(run *domain.ReportRun) *ReportRunResponse {
	if run == nil {
		return nil
	}

	return &ReportRunResponse{
		ID:          run.ID,
		ReportID:    run.ReportID,
		ScheduleID:  run.ScheduleID,
		Status:      run.Status,
		StartedAt:   run.StartedAt,
		CompletedAt: run.CompletedAt,
		ArtifactURL: run.ArtifactURL,
		Error:       run.Error,
	}
}

// FromReportRuns converts report runs to report run responses.
func FromReportRuns(runs []domain.ReportRun) []ReportRunResponse {
	result := make([]ReportRunResponse, len(runs))
	for i, run := range runs {
		resp := FromReportRun(&run)
		if resp != nil {
			result[i] = *resp
		}
	}

	return result
}
