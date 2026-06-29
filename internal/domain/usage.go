package domain

import (
	"context"
	"encoding/json"
	"time"

	"github.com/google/uuid"
)

// UsageDailySummary represents usage data aggregated for a single day.
type UsageDailySummary struct {
	ID            int64           `json:"id"`
	APIKeyID      uuid.UUID       `json:"api_key_id"`
	Date          time.Time       `json:"date"`
	TotalRequests int64           `json:"total_requests"`
	TotalBytes    int64           `json:"total_bytes"`
	CostUnits     float64         `json:"cost_units"`
	Breakdown     json.RawMessage `json:"breakdown"`
}

// UsageSummary represents a high-level usage summary for a date.
type UsageSummary struct {
	Date          string           `json:"date"`
	TotalRequests int64            `json:"total_requests"`
	TotalBytes    int64            `json:"total_bytes"`
	CostUnits     float64          `json:"cost_units"`
	Breakdown     map[string]int64 `json:"breakdown"`
}

// UsageRepository provides persistence operations for usage data.
type UsageRepository interface {
	GetDailySummaries(ctx context.Context, apiKeyID string, start, end time.Time) ([]UsageSummary, error)
}
