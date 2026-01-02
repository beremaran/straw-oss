package postgres

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/kwilabs/straw-proxy-server/internal/domain"
)

// UsageRepository implements usage data access.
type UsageRepository struct {
	client *Client
}

// NewUsageRepository creates a new UsageRepository.
func NewUsageRepository(client *Client) *UsageRepository {
	return &UsageRepository{client: client}
}

// GetDailySummaries returns usage summaries for a given API key within a date range.
// If apiKeyID is empty, it returns summaries for all API keys (aggregated by date? No, the requirement implies per key or specific key helper).
// Let's implement filtering.
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
		var date time.Time
		var totalRequests int64
		var totalBytes int64
		var costUnits float64
		var breakdownRaw []byte

		if err := rows.Scan(&date, &totalRequests, &totalBytes, &costUnits, &breakdownRaw); err != nil {
			return nil, fmt.Errorf("failed to scan usage row: %w", err)
		}

		var breakdown map[string]int64
		if len(breakdownRaw) > 0 {
			if err := json.Unmarshal(breakdownRaw, &breakdown); err != nil {
				// Log error but continue? Or fail? Let's fail safe returning empty map
				breakdown = make(map[string]int64)
			}
		}

		summaries = append(summaries, domain.UsageSummary{
			Date:          date.Format("2006-01-02"),
			TotalRequests: totalRequests,
			TotalBytes:    totalBytes,
			CostUnits:     costUnits,
			Breakdown:     breakdown,
		})
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("error iterating usage rows: %w", err)
	}

	return summaries, nil
}
