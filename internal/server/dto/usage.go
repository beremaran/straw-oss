package dto

type UsageSummaryDTO struct {
	Date          string           `json:"date"`
	TotalRequests int64            `json:"total_requests"`
	TotalBytes    int64            `json:"total_bytes"`
	CostUnits     float64          `json:"cost_units"`
	Breakdown     map[string]int64 `json:"breakdown"`
}

type UsageSummaryResponse struct {
	Data  []UsageSummaryDTO `json:"data"`
	Start string            `json:"start"`
	End   string            `json:"end"`
}

type BillingEstimateResponse struct {
	TotalCostUnits float64 `json:"total_cost_units"`
	EstimatedUSD   float64 `json:"estimated_usd"`
	Currency       string  `json:"currency"`
	Start          string  `json:"start"`
	End            string  `json:"end"`
}

type ClearCacheResponse struct {
	Message string `json:"message"`
	Pattern string `json:"pattern"`
	Deleted int    `json:"deleted"`
}

type CacheStatsResponse struct {
	Info string `json:"info"`
}

type EndpointHealthResponse struct {
	EndpointID  string   `json:"endpoint_id"`
	State       string   `json:"state"`
	Tags        []string `json:"tags,omitempty"`
	Version     string   `json:"version,omitempty"`
	ActiveTasks int      `json:"active_tasks"`
	LastSeen    string   `json:"last_seen"`
}
