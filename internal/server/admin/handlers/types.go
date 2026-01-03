package handlers

import (
	"time"

	"github.com/kwilabs/straw-proxy-server/internal/domain"
)

// ListApiKeysResponse represents the response for listing API keys
type ListApiKeysResponse struct {
	Data  []domain.ApiKey `json:"data"`
	Total int             `json:"total"`
	Page  int             `json:"page"`
	Limit int             `json:"limit"`
}

// CreateApiKeyResponse represents the response for creating an API key
type CreateApiKeyResponse struct {
	*domain.ApiKey
	RawKey string `json:"raw_key"`
}

// UsageSummaryResponse represents the usage summary response
type UsageSummaryResponse struct {
	Data  []domain.UsageSummary `json:"data"`
	Start string                `json:"start"` // YYYY-MM-DD
	End   string                `json:"end"`   // YYYY-MM-DD
}

// BillingEstimateResponse represents the billing estimate
type BillingEstimateResponse struct {
	TotalCostUnits float64 `json:"total_cost_units"`
	EstimatedUSD   float64 `json:"estimated_usd"`
	Currency       string  `json:"currency"`
	Start          string  `json:"start"`
	End            string  `json:"end"`
}

// ClearCacheResponse represents the response for clearing cache
type ClearCacheResponse struct {
	Message string `json:"message"`
	Pattern string `json:"pattern"`
	Deleted int    `json:"deleted"`
}

// CacheStatsResponse represents the cache stats response
type CacheStatsResponse struct {
	Info string `json:"info"`
}

// EndpointHealthResponse represents the health state of an endpoint
type EndpointHealthResponse struct {
	EndpointID  string    `json:"endpoint_id"`
	State       string    `json:"state"`
	Tags        []string  `json:"tags,omitempty"`
	Version     string    `json:"version,omitempty"`
	ActiveTasks int       `json:"active_tasks"`
	LastSeen    time.Time `json:"last_seen"`
}
