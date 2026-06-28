package domain

import "time"

type Endpoint struct {
	ID string `json:"id"`

	Tags []string `json:"tags"`

	LastHeartbeat time.Time `json:"last_heartbeat"`

	IsHealthy bool `json:"is_healthy"`

	Metadata EndpointMetadata `json:"metadata"`

	CreatedAt time.Time `json:"created_at"`
}

type EndpointMetadata struct {
	Version string `json:"version,omitempty"`

	IP string `json:"ip,omitempty"`

	ActiveTasks int `json:"active_tasks"`

	MaxConcurrency int `json:"max_concurrency,omitempty"`

	Provider string `json:"provider,omitempty"`
}

const DefaultHeartbeatInterval = 10 * time.Second

const DefaultHealthThreshold = 30 * time.Second

func (e *Endpoint) IsStale(threshold time.Duration) bool {
	return time.Since(e.LastHeartbeat) > threshold
}

func (e *Endpoint) UpdateHeartbeat() {
	e.LastHeartbeat = time.Now()
	e.IsHealthy = true
}

func (e *Endpoint) MarkUnhealthy() {
	e.IsHealthy = false
}

func (e *Endpoint) HasTag(tag string) bool {
	for _, t := range e.Tags {
		if t == tag {
			return true
		}
	}

	return false
}

func (e *Endpoint) MatchesTags(requiredTags []string) bool {
	for _, required := range requiredTags {
		if !e.HasTag(required) {
			return false
		}
	}

	return true
}

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
