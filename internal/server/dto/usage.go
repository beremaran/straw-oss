package dto

// UsageSummaryDTO contains usage summary information.
type UsageSummaryDTO struct {
	Date          string           `json:"date"`
	TotalRequests int64            `json:"total_requests"`
	TotalBytes    int64            `json:"total_bytes"`
	CostUnits     float64          `json:"cost_units"`
	Breakdown     map[string]int64 `json:"breakdown"`
}

// UsageSummaryResponse is the response body for usage summary queries.
type UsageSummaryResponse struct {
	Data  []UsageSummaryDTO `json:"data"`
	Start string            `json:"start"`
	End   string            `json:"end"`
}

// BillingEstimateResponse contains a billing cost estimate.
type BillingEstimateResponse struct {
	TotalCostUnits float64 `json:"total_cost_units"`
	EstimatedUSD   float64 `json:"estimated_usd"`
	Currency       string  `json:"currency"`
	Start          string  `json:"start"`
	End            string  `json:"end"`
}

// ClearCacheResponse is the response body for clearing cache entries.
type ClearCacheResponse struct {
	Message string `json:"message"`
	Pattern string `json:"pattern"`
	Deleted int    `json:"deleted"`
}

// CacheStatsResponse contains cache statistics.
type CacheStatsResponse struct {
	Info string `json:"info"`
}

// EndpointHealthResponse contains health information for an endpoint.
type EndpointHealthResponse struct {
	EndpointID  string   `json:"endpoint_id"`
	State       string   `json:"state"`
	Tags        []string `json:"tags,omitempty"`
	Version     string   `json:"version,omitempty"`
	ActiveTasks int      `json:"active_tasks"`
	LastSeen    string   `json:"last_seen"`
}
