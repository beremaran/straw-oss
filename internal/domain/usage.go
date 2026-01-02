package domain

import (
	"context"
	"encoding/json"
	"time"

	"github.com/google/uuid"
)

// UsageDailySummary represents a daily summary record from the database.
type UsageDailySummary struct {
	ID            int64           `json:"id"`
	ApiKeyID      uuid.UUID       `json:"api_key_id"`
	Date          time.Time       `json:"date"`
	TotalRequests int64           `json:"total_requests"`
	TotalBytes    int64           `json:"total_bytes"`
	CostUnits     float64         `json:"cost_units"`
	Breakdown     json.RawMessage `json:"breakdown"` // Stored as JSONB in DB
}

// UsageSummary is the DTO for usage summary responses.
type UsageSummary struct {
	Date          string           `json:"date"`
	TotalRequests int64            `json:"total_requests"`
	TotalBytes    int64            `json:"total_bytes"`
	CostUnits     float64          `json:"cost_units"`
	Breakdown     map[string]int64 `json:"breakdown"`
}

// UsageRepository defines the interface for usage data access.
type UsageRepository interface {
	GetDailySummaries(ctx context.Context, apiKeyID string, start, end time.Time) ([]UsageSummary, error)
}
