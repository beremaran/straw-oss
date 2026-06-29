package postgres

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5"

	"github.com/beremaran/straw/internal/domain"
)

// SavedReportRepository persists saved report definitions.
type SavedReportRepository struct {
	client *Client
}

// NewSavedReportRepository creates a new SavedReportRepository.
func NewSavedReportRepository(client *Client) *SavedReportRepository {
	return &SavedReportRepository{client: client}
}

// Create inserts a saved report.
func (r *SavedReportRepository) Create(ctx context.Context, report *domain.SavedReport) error {
	filters, err := json.Marshal(report.Filters)
	if err != nil {
		return fmt.Errorf("marshal report filters: %w", err)
	}

	query := `
		INSERT INTO saved_reports (id, name, description, type, filters, format, created_by, created_at, updated_at)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9)
	`

	err = r.client.Execute(func() error {
		_, execErr := r.client.Pool.Exec(ctx, query,
			report.ID,
			report.Name,
			report.Description,
			report.Type,
			filters,
			report.Format,
			nullableString(report.CreatedBy),
			report.CreatedAt,
			report.UpdatedAt,
		)
		if execErr != nil {
			return fmt.Errorf("insert saved report: %w", execErr)
		}

		return nil
	})
	if err != nil {
		return fmt.Errorf("create saved report: %w", err)
	}

	return nil
}

// Update modifies a saved report.
func (r *SavedReportRepository) Update(ctx context.Context, report *domain.SavedReport) error {
	filters, err := json.Marshal(report.Filters)
	if err != nil {
		return fmt.Errorf("marshal report filters: %w", err)
	}

	query := `
		UPDATE saved_reports
		SET name = $2,
			description = $3,
			type = $4,
			filters = $5,
			format = $6,
			updated_at = $7
		WHERE id = $1
	`

	var rows int64

	err = r.client.Execute(func() error {
		res, execErr := r.client.Pool.Exec(ctx, query,
			report.ID,
			report.Name,
			report.Description,
			report.Type,
			filters,
			report.Format,
			report.UpdatedAt,
		)
		if execErr != nil {
			return fmt.Errorf("update saved report: %w", execErr)
		}

		rows = res.RowsAffected()

		return nil
	})
	if err != nil {
		return fmt.Errorf("update saved report: %w", err)
	}

	if rows == 0 {
		return domain.ErrReportNotFound
	}

	return nil
}

// Delete removes a saved report and cascades schedules/runs.
func (r *SavedReportRepository) Delete(ctx context.Context, id string) error {
	var rows int64

	err := r.client.Execute(func() error {
		res, execErr := r.client.Pool.Exec(ctx, `DELETE FROM saved_reports WHERE id = $1`, id)
		if execErr != nil {
			return fmt.Errorf("delete saved report: %w", execErr)
		}

		rows = res.RowsAffected()

		return nil
	})
	if err != nil {
		return fmt.Errorf("delete saved report: %w", err)
	}

	if rows == 0 {
		return domain.ErrReportNotFound
	}

	return nil
}

// GetByID returns a saved report by ID.
func (r *SavedReportRepository) GetByID(ctx context.Context, id string) (*domain.SavedReport, error) {
	query := `
		SELECT id, name, COALESCE(description, ''), type, filters, format, COALESCE(created_by::text, ''), created_at, updated_at
		FROM saved_reports
		WHERE id = $1
	`

	var report domain.SavedReport

	err := r.client.Execute(func() error {
		return scanSavedReport(r.client.Pool.QueryRow(ctx, query, id), &report)
	})
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, nil
		}

		return nil, fmt.Errorf("get saved report: %w", err)
	}

	return &report, nil
}

// List returns saved reports.
func (r *SavedReportRepository) List(ctx context.Context, limit, offset int) ([]domain.SavedReport, int, error) {
	var total int

	err := r.client.Execute(func() error {
		return r.client.Pool.QueryRow(ctx, `SELECT COUNT(*) FROM saved_reports`).Scan(&total)
	})
	if err != nil {
		return nil, 0, fmt.Errorf("count saved reports: %w", err)
	}

	query := `
		SELECT id, name, COALESCE(description, ''), type, filters, format, COALESCE(created_by::text, ''), created_at, updated_at
		FROM saved_reports
		ORDER BY created_at DESC
		LIMIT $1 OFFSET $2
	`

	var rows pgx.Rows

	err = r.client.Execute(func() error {
		var queryErr error

		rows, queryErr = r.client.Pool.Query(ctx, query, limit, offset)
		if queryErr != nil {
			return fmt.Errorf("query saved reports: %w", queryErr)
		}

		return nil
	})
	if err != nil {
		return nil, 0, fmt.Errorf("list saved reports: %w", err)
	}
	defer rows.Close()

	var reports []domain.SavedReport

	for rows.Next() {
		var report domain.SavedReport

		err = scanSavedReport(rows, &report)
		if err != nil {
			return nil, 0, err
		}

		reports = append(reports, report)
	}

	err = rows.Err()
	if err != nil {
		return nil, 0, fmt.Errorf("iterate saved reports: %w", err)
	}

	return reports, total, nil
}

// ReportRunRepository persists report runs.
type ReportRunRepository struct {
	client *Client
}

// NewReportRunRepository creates a new ReportRunRepository.
func NewReportRunRepository(client *Client) *ReportRunRepository {
	return &ReportRunRepository{client: client}
}

// Create inserts a report run.
func (r *ReportRunRepository) Create(ctx context.Context, run *domain.ReportRun) error {
	query := `
		INSERT INTO report_runs (id, report_id, schedule_id, status, started_at, completed_at, artifact_url, error)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8)
	`

	err := r.client.Execute(func() error {
		_, execErr := r.client.Pool.Exec(ctx, query,
			run.ID,
			run.ReportID,
			nullableString(run.ScheduleID),
			run.Status,
			run.StartedAt,
			run.CompletedAt,
			nullableString(run.ArtifactURL),
			nullableString(run.Error),
		)
		if execErr != nil {
			return fmt.Errorf("insert report run: %w", execErr)
		}

		return nil
	})
	if err != nil {
		return fmt.Errorf("create report run: %w", err)
	}

	return nil
}

// Update modifies a report run.
func (r *ReportRunRepository) Update(ctx context.Context, run *domain.ReportRun) error {
	query := `
		UPDATE report_runs
		SET status = $2,
			completed_at = $3,
			artifact_url = $4,
			error = $5
		WHERE id = $1
	`

	var rows int64

	err := r.client.Execute(func() error {
		res, execErr := r.client.Pool.Exec(ctx, query,
			run.ID,
			run.Status,
			run.CompletedAt,
			nullableString(run.ArtifactURL),
			nullableString(run.Error),
		)
		if execErr != nil {
			return fmt.Errorf("update report run: %w", execErr)
		}

		rows = res.RowsAffected()

		return nil
	})
	if err != nil {
		return fmt.Errorf("update report run: %w", err)
	}

	if rows == 0 {
		return domain.ErrReportRunNotFound
	}

	return nil
}

// GetByID returns a report run by ID.
func (r *ReportRunRepository) GetByID(ctx context.Context, id string) (*domain.ReportRun, error) {
	query := `
		SELECT id, report_id, COALESCE(schedule_id::text, ''), status, started_at, completed_at,
		       COALESCE(artifact_url, ''), COALESCE(error, '')
		FROM report_runs
		WHERE id = $1
	`

	var run domain.ReportRun

	err := r.client.Execute(func() error {
		return scanReportRun(r.client.Pool.QueryRow(ctx, query, id), &run)
	})
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, nil
		}

		return nil, fmt.Errorf("get report run: %w", err)
	}

	return &run, nil
}

// ListByReportID returns report runs for a saved report.
func (r *ReportRunRepository) ListByReportID(ctx context.Context, reportID string, limit, offset int) ([]domain.ReportRun, int, error) {
	var total int

	err := r.client.Execute(func() error {
		return r.client.Pool.QueryRow(ctx, `SELECT COUNT(*) FROM report_runs WHERE report_id = $1`, reportID).Scan(&total)
	})
	if err != nil {
		return nil, 0, fmt.Errorf("count report runs: %w", err)
	}

	query := `
		SELECT id, report_id, COALESCE(schedule_id::text, ''), status, started_at, completed_at,
		       COALESCE(artifact_url, ''), COALESCE(error, '')
		FROM report_runs
		WHERE report_id = $1
		ORDER BY started_at DESC
		LIMIT $2 OFFSET $3
	`

	var rows pgx.Rows

	err = r.client.Execute(func() error {
		var queryErr error

		rows, queryErr = r.client.Pool.Query(ctx, query, reportID, limit, offset)
		if queryErr != nil {
			return fmt.Errorf("query report runs: %w", queryErr)
		}

		return nil
	})
	if err != nil {
		return nil, 0, fmt.Errorf("list report runs: %w", err)
	}
	defer rows.Close()

	var runs []domain.ReportRun

	for rows.Next() {
		var run domain.ReportRun

		err = scanReportRun(rows, &run)
		if err != nil {
			return nil, 0, err
		}

		runs = append(runs, run)
	}

	err = rows.Err()
	if err != nil {
		return nil, 0, fmt.Errorf("iterate report runs: %w", err)
	}

	return runs, total, nil
}

type scanner interface {
	Scan(dest ...any) error
}

func scanSavedReport(row scanner, report *domain.SavedReport) error {
	var filters []byte

	err := row.Scan(
		&report.ID,
		&report.Name,
		&report.Description,
		&report.Type,
		&filters,
		&report.Format,
		&report.CreatedBy,
		&report.CreatedAt,
		&report.UpdatedAt,
	)
	if err != nil {
		return fmt.Errorf("scan saved report: %w", err)
	}

	if len(filters) > 0 {
		err = json.Unmarshal(filters, &report.Filters)
		if err != nil {
			return fmt.Errorf("unmarshal report filters: %w", err)
		}
	}

	if report.Filters == nil {
		report.Filters = domain.ConfigMap{}
	}

	return nil
}

func scanReportRun(row scanner, run *domain.ReportRun) error {
	var completedAt sql.NullTime

	err := row.Scan(
		&run.ID,
		&run.ReportID,
		&run.ScheduleID,
		&run.Status,
		&run.StartedAt,
		&completedAt,
		&run.ArtifactURL,
		&run.Error,
	)
	if err != nil {
		return fmt.Errorf("scan report run: %w", err)
	}

	if completedAt.Valid {
		run.CompletedAt = &completedAt.Time
	}

	return nil
}

func nullableString(value string) any {
	if value == "" {
		return nil
	}

	return value
}
