package postgres

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/beremaran/straw/internal/domain"
	"github.com/jackc/pgx/v5"
)

type UsageRepository struct {
	client *Client
}

func NewUsageRepository(client *Client) *UsageRepository {
	return &UsageRepository{client: client}
}

func (r *UsageRepository) GetDailySummaries(ctx context.Context, apiKeyID string, start, end time.Time) ([]domain.UsageSummary, error) {
	sql := `
		SELECT date, total_requests, total_bytes, cost_units, breakdown
		FROM usage_daily_summary
		WHERE date >= $1 AND date <= $2
	`
	args := []interface{}{start, end}

	if apiKeyID != "" {
		sql += ` AND api_key_id = $3`
		args = append(args, apiKeyID)
	}

	sql += ` ORDER BY date DESC`

	var rows pgx.Rows
	var err error
	err = r.client.Execute(func() error {
		rows, err = r.client.Pool.Query(ctx, sql, args...)

		return err
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
	var date time.Time
	var totalRequests int64
	var totalBytes int64
	var costUnits float64
	var breakdownRaw []byte

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
