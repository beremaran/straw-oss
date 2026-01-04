package dto

// UsageSummaryDTO represents daily usage data.
//
//	@Description Daily usage summary
type UsageSummaryDTO struct {
	// Date in YYYY-MM-DD format
	Date string `json:"date"`

	// TotalRequests is the number of requests made
	TotalRequests int64 `json:"total_requests"`

	// TotalBytes is the total data transferred
	TotalBytes int64 `json:"total_bytes"`

	// CostUnits is the calculated cost units
	CostUnits float64 `json:"cost_units"`

	// Breakdown contains per-endpoint or per-rule breakdown
	Breakdown map[string]int64 `json:"breakdown"`
}

// UsageSummaryResponse is the response for usage summaries.
//
//	@Description Response containing usage summary data for a date range
type UsageSummaryResponse struct {
	// Data contains the daily usage summaries
	Data []UsageSummaryDTO `json:"data"`

	// Start date in YYYY-MM-DD format
	Start string `json:"start"`

	// End date in YYYY-MM-DD format
	End string `json:"end"`
}

// BillingEstimateResponse is the response for billing estimates.
//
//	@Description Response containing billing estimate for a date range
type BillingEstimateResponse struct {
	// TotalCostUnits is the sum of all cost units
	TotalCostUnits float64 `json:"total_cost_units"`

	// EstimatedUSD is the estimated cost in USD
	EstimatedUSD float64 `json:"estimated_usd"`

	// Currency is the currency code (e.g., "USD")
	Currency string `json:"currency"`

	// Start date in YYYY-MM-DD format
	Start string `json:"start"`

	// End date in YYYY-MM-DD format
	End string `json:"end"`
}

// ClearCacheResponse represents the response for clearing cache.
//
//	@Description Response after clearing cache keys
type ClearCacheResponse struct {
	// Message describes what was done
	Message string `json:"message"`

	// Pattern is the key pattern that was matched
	Pattern string `json:"pattern"`

	// Deleted is the number of keys removed
	Deleted int `json:"deleted"`
}

// CacheStatsResponse represents the cache stats response.
//
//	@Description Response containing Redis cache statistics
type CacheStatsResponse struct {
	// Info contains Redis server information
	Info string `json:"info"`
}

// EndpointHealthResponse represents the health state of an endpoint.
//
//	@Description Health information for a registered endpoint
type EndpointHealthResponse struct {
	// EndpointID is the unique identifier
	EndpointID string `json:"endpoint_id"`

	// State is the current health state
	State string `json:"state"`

	// Tags are the endpoint's capability tags
	Tags []string `json:"tags,omitempty"`

	// Version is the endpoint software version
	Version string `json:"version,omitempty"`

	// ActiveTasks is the number of in-flight requests
	ActiveTasks int `json:"active_tasks"`

	// LastSeen is when the endpoint last reported in
	LastSeen string `json:"last_seen"`
}
