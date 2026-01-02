package domain

import "time"

// Endpoint represents a distributed worker node in the Straw Proxy system.
// Endpoints consume tasks from RabbitMQ and execute HTTP requests.
type Endpoint struct {
	// ID is the unique identifier for this endpoint.
	// Example: "endpoint-us-res-001"
	ID string `json:"id"`

	// Tags describe the endpoint's capabilities and properties.
	// Example: ["type:residential", "region:us", "provider:luminati"]
	Tags []string `json:"tags"`

	// LastHeartbeat is the timestamp of the last heartbeat received.
	LastHeartbeat time.Time `json:"last_heartbeat"`

	// IsHealthy indicates whether the endpoint is considered healthy.
	IsHealthy bool `json:"is_healthy"`

	// Metadata contains additional endpoint information.
	Metadata EndpointMetadata `json:"metadata"`

	// CreatedAt is when the endpoint was first registered.
	CreatedAt time.Time `json:"created_at"`
}

// EndpointMetadata contains additional information about an endpoint.
type EndpointMetadata struct {
	// Version is the endpoint software version.
	Version string `json:"version,omitempty"`

	// IP is the endpoint's public IP address.
	IP string `json:"ip,omitempty"`

	// ActiveTasks is the number of currently processing tasks.
	ActiveTasks int `json:"active_tasks"`

	// MaxConcurrency is the maximum concurrent requests this endpoint supports.
	MaxConcurrency int `json:"max_concurrency,omitempty"`

	// Provider identifies the proxy provider (if applicable).
	Provider string `json:"provider,omitempty"`
}

// DefaultHeartbeatInterval is the expected interval between heartbeats.
const DefaultHeartbeatInterval = 10 * time.Second

// DefaultHealthThreshold is the time after which an endpoint is considered unhealthy.
const DefaultHealthThreshold = 30 * time.Second

// IsStale checks if the endpoint's heartbeat is older than the threshold.
// Stale endpoints should be marked as unhealthy.
func (e *Endpoint) IsStale(threshold time.Duration) bool {
	return time.Since(e.LastHeartbeat) > threshold
}

// UpdateHeartbeat updates the last heartbeat time and marks the endpoint healthy.
func (e *Endpoint) UpdateHeartbeat() {
	e.LastHeartbeat = time.Now()
	e.IsHealthy = true
}

// MarkUnhealthy marks the endpoint as unhealthy.
func (e *Endpoint) MarkUnhealthy() {
	e.IsHealthy = false
}

// HasTag checks if the endpoint has a specific tag.
func (e *Endpoint) HasTag(tag string) bool {
	for _, t := range e.Tags {
		if t == tag {
			return true
		}
	}
	return false
}

// MatchesTags checks if the endpoint has all the required tags.
func (e *Endpoint) MatchesTags(requiredTags []string) bool {
	for _, required := range requiredTags {
		if !e.HasTag(required) {
			return false
		}
	}
	return true
}

// NewEndpoint creates a new endpoint with the given ID and tags.
func NewEndpoint(id string, tags []string) *Endpoint {
	now := time.Now()
	return &Endpoint{
		ID:            id,
		Tags:          tags,
		LastHeartbeat: now,
		IsHealthy:     true,
		CreatedAt:     now,
	}
}
