package postgres

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/beremaran/straw/internal/domain"
)

// UsageRepository persists and retrieves usage summaries.
type UsageRepository struct {
	client *Client
}

// NewUsageRepository creates a new UsageRepository backed by the given client.
func NewUsageRepository(client *Client) *UsageRepository {
	return &UsageRepository{client: client}
}

// GetDailySummaries returns daily usage summaries between start and end dates.
func (r *UsageRepository) GetDailySummaries(ctx context.Context, apiKeyID string, start, end time.Time) ([]domain.UsageSummary, error) {
	sql := `
		SELECT date, total_requests, total_bytes, cost_units, breakdown
		FROM usage_daily_summary
		WHERE date >= $1 AND date <= $2
	`
	args := []any{start, end}

	if apiKeyID != "" {
		sql += ` AND api_key_id = $3`

		args = append(args, apiKeyID)
	}

	sql += ` ORDER BY date DESC`

	var (
		rows pgx.Rows
		err  error
	)

	err = r.client.Execute(func() error {
		var queryErr error

		rows, queryErr = r.client.Pool.Query(ctx, sql, args...)
		if queryErr != nil {
			return fmt.Errorf("failed to execute query: %w", queryErr)
		}

		return nil
	})
	if err != nil {
		return nil, fmt.Errorf("failed to query usage summaries: %w", err)
	}

	defer rows.Close()

	var summaries []domain.UsageSummary

	for rows.Next() {
		summary, err := scanUsageSummary(rows)
		if err != nil {
			return nil, err
		}

		summaries = append(summaries, summary)
	}

	err = rows.Err()
	if err != nil {
		return nil, fmt.Errorf("error iterating usage rows: %w", err)
	}

	return summaries, nil
}

func scanUsageSummary(rows pgx.Rows) (domain.UsageSummary, error) {
	var (
		date          time.Time
		totalRequests int64
		totalBytes    int64
		costUnits     float64
		breakdownRaw  []byte
	)

	err := rows.Scan(&date, &totalRequests, &totalBytes, &costUnits, &breakdownRaw)
	if err != nil {
		return domain.UsageSummary{}, fmt.Errorf("failed to scan usage row: %w", err)
	}

	return domain.UsageSummary{
		Date:          date.Format("2006-01-02"),
		TotalRequests: totalRequests,
		TotalBytes:    totalBytes,
		CostUnits:     costUnits,
		Breakdown:     usageBreakdown(breakdownRaw),
	}, nil
}

func usageBreakdown(raw []byte) map[string]int64 {
	var breakdown map[string]int64
	if len(raw) > 0 {
		err := json.Unmarshal(raw, &breakdown)
		if err != nil {
			return make(map[string]int64)
		}
	}

	return breakdown
}
