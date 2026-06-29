package domain

import (
	"context"
	"errors"
	"time"
)

const (
	// ReportTypeUsageSummary identifies usage summary reports.
	ReportTypeUsageSummary = "usage_summary"
	// ReportTypeBillingEstimate identifies billing estimate reports.
	ReportTypeBillingEstimate = "billing_estimate"
	// ReportTypeAPIKeyInventory identifies API key inventory reports.
	ReportTypeAPIKeyInventory = "api_key_inventory"
	// ReportTypeEndpointHealth identifies endpoint health reports.
	ReportTypeEndpointHealth = "endpoint_health"
	// ReportTypeAuditEvents identifies audit event reports.
	ReportTypeAuditEvents = "audit_events"

	// ReportFormatCSV is the first-release artifact format.
	ReportFormatCSV = "csv"

	// ReportRunStatusRunning means a report run is still executing.
	ReportRunStatusRunning = "running"
	// ReportRunStatusSucceeded means a report run completed and wrote an artifact.
	ReportRunStatusSucceeded = "succeeded"
	// ReportRunStatusFailed means a report run failed.
	ReportRunStatusFailed = "failed"
)

var (
	// ErrReportNotFound is returned when a saved report does not exist.
	ErrReportNotFound = errors.New("report not found")
	// ErrReportRunNotFound is returned when a report run does not exist.
	ErrReportRunNotFound = errors.New("report run not found")
	// ErrReportScheduleNotFound is returned when a report schedule does not exist.
	ErrReportScheduleNotFound = errors.New("report schedule not found")
)

// SavedReport stores a reusable report definition.
type SavedReport struct {
	ID          string
	Name        string
	Description string
	Type        string
	Filters     ConfigMap
	Format      string
	CreatedBy   string
	CreatedAt   time.Time
	UpdatedAt   time.Time
}

// ReportRun stores one execution of a saved report.
type ReportRun struct {
	ID          string
	ReportID    string
	ScheduleID  string
	Status      string
	StartedAt   time.Time
	CompletedAt *time.Time
	ArtifactURL string
	Error       string
}

// ReportSchedule stores recurring execution settings for a saved report.
type ReportSchedule struct {
	ID                   string
	ReportID             string
	Cron                 string
	Timezone             string
	DestinationChannelID string
	IsActive             bool
	NextRunAt            *time.Time
	LastRunAt            *time.Time
	CreatedAt            time.Time
	UpdatedAt            time.Time
}

// SavedReportRepository provides persistence operations for saved reports.
type SavedReportRepository interface {
	Create(ctx context.Context, report *SavedReport) error
	Update(ctx context.Context, report *SavedReport) error
	Delete(ctx context.Context, id string) error
	GetByID(ctx context.Context, id string) (*SavedReport, error)
	List(ctx context.Context, limit, offset int) ([]SavedReport, int, error)
}

// ReportRunRepository provides persistence operations for report runs.
type ReportRunRepository interface {
	Create(ctx context.Context, run *ReportRun) error
	Update(ctx context.Context, run *ReportRun) error
	GetByID(ctx context.Context, id string) (*ReportRun, error)
	ListByReportID(ctx context.Context, reportID string, limit, offset int) ([]ReportRun, int, error)
}

// ReportScheduleRepository provides persistence operations for report schedules.
type ReportScheduleRepository interface {
	Create(ctx context.Context, schedule *ReportSchedule) error
	Update(ctx context.Context, schedule *ReportSchedule) error
	Disable(ctx context.Context, id string) error
	GetByID(ctx context.Context, id string) (*ReportSchedule, error)
	List(ctx context.Context, limit, offset int) ([]ReportSchedule, int, error)
	ClaimDue(ctx context.Context, now time.Time, limit int) ([]ReportSchedule, error)
	MarkRun(ctx context.Context, id string, lastRunAt time.Time, nextRunAt time.Time) error
}
