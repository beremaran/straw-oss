package domain

import (
	"context"
	"slices"
	"time"
)

// DesiredState represents the desired operational state of an endpoint.
type DesiredState string

const (
	// DesiredStateActive indicates the endpoint should be serving traffic.
	DesiredStateActive DesiredState = "active"
	// DesiredStateDraining indicates the endpoint is finishing in-flight requests.
	DesiredStateDraining DesiredState = "draining"
	// DesiredStateDisabled indicates the endpoint should not receive traffic.
	DesiredStateDisabled DesiredState = "disabled"
	// DesiredStateDeleted indicates the endpoint is scheduled for removal.
	DesiredStateDeleted DesiredState = "deleted"
)

// Endpoint represents a registered endpoint that can process requests.
type Endpoint struct {
	ID            string           `json:"id"`
	Tags          []string         `json:"tags"`
	LastHeartbeat time.Time        `json:"last_heartbeat"`
	IsHealthy     bool             `json:"is_healthy"`
	Metadata      EndpointMetadata `json:"metadata"`
	DesiredState  DesiredState     `json:"desired_state"`
	IsRegistered  bool             `json:"is_registered"`
	DeletedAt     *time.Time       `json:"deleted_at,omitempty"`
	CreatedAt     time.Time        `json:"created_at"`
	UpdatedAt     time.Time        `json:"updated_at"`
}

// EndpointMetadata holds supplementary information about an endpoint.
type EndpointMetadata struct {
	Version        string `json:"version,omitempty"`
	IP             string `json:"ip,omitempty"`
	ActiveTasks    int    `json:"active_tasks"`
	MaxConcurrency int    `json:"max_concurrency,omitempty"`
	Provider       string `json:"provider,omitempty"`
}

// CommandStatus represents the lifecycle state of an endpoint command.
type CommandStatus string

const (
	// CommandStatusAccepted indicates the command has been received.
	CommandStatusAccepted CommandStatus = "accepted"
	// CommandStatusAcknowledged indicates the command has been acknowledged.
	CommandStatusAcknowledged CommandStatus = "acknowledged"
	// CommandStatusRunning indicates the command is executing.
	CommandStatusRunning CommandStatus = "running"
	// CommandStatusSucceeded indicates the command completed successfully.
	CommandStatusSucceeded CommandStatus = "succeeded"
	// CommandStatusFailed indicates the command failed.
	CommandStatusFailed CommandStatus = "failed"
	// CommandStatusTimedOut indicates the command timed out.
	CommandStatusTimedOut CommandStatus = "timed_out"
)

// EndpointCommand represents a command sent to an endpoint.
type EndpointCommand struct {
	ID          string         `json:"id"`
	EndpointID  string         `json:"endpoint_id"`
	Command     string         `json:"command"`
	Status      CommandStatus  `json:"status"`
	Payload     map[string]any `json:"payload"`
	RequestedBy *string        `json:"requested_by,omitempty"`
	RequestedAt time.Time      `json:"requested_at"`
	AcceptedAt  *time.Time     `json:"accepted_at,omitempty"`
	CompletedAt *time.Time     `json:"completed_at,omitempty"`
	Error       *string        `json:"error,omitempty"`
}

// EndpointLogEntry represents a log entry from an endpoint.
type EndpointLogEntry struct {
	ID         int64          `json:"id"`
	EndpointID string         `json:"endpoint_id"`
	ObservedAt time.Time      `json:"observed_at"`
	Level      string         `json:"level"`
	Message    string         `json:"message"`
	Attrs      map[string]any `json:"attrs"`
	TraceID    *string        `json:"trace_id,omitempty"`
	RequestID  *string        `json:"request_id,omitempty"`
}

// DefaultHeartbeatInterval is the default interval for endpoint heartbeats.
const DefaultHeartbeatInterval = 10 * time.Second

// DefaultHealthThreshold is the default time before an endpoint is considered stale.
const DefaultHealthThreshold = 30 * time.Second

// NewEndpoint creates a new active, healthy Endpoint with the given id and tags.
func NewEndpoint(id string, tags []string) *Endpoint {
	now := time.Now()

	return &Endpoint{
		ID:            id,
		Tags:          tags,
		LastHeartbeat: now,
		IsHealthy:     true,
		DesiredState:  DesiredStateActive,
		IsRegistered:  true,
		CreatedAt:     now,
		UpdatedAt:     now,
	}
}

// IsStale reports whether the Endpoint's last heartbeat is older than the threshold.
func (e *Endpoint) IsStale(threshold time.Duration) bool {
	return time.Since(e.LastHeartbeat) > threshold
}

// UpdateHeartbeat records the current time as the last heartbeat and marks the endpoint healthy.
func (e *Endpoint) UpdateHeartbeat() {
	e.LastHeartbeat = time.Now()
	e.IsHealthy = true
}

// MarkUnhealthy marks the endpoint as unhealthy.
func (e *Endpoint) MarkUnhealthy() {
	e.IsHealthy = false
}

// HasTag reports whether the Endpoint has the given tag.
func (e *Endpoint) HasTag(tag string) bool {
	return slices.Contains(e.Tags, tag)
}

// MatchesTags reports whether the Endpoint has all the required tags.
func (e *Endpoint) MatchesTags(requiredTags []string) bool {
	for _, required := range requiredTags {
		if !e.HasTag(required) {
			return false
		}
	}

	return true
}

// EndpointRepository provides persistence operations for Endpoint entities.
type EndpointRepository interface {
	Create(ctx context.Context, endpoint *Endpoint) error
	GetByID(ctx context.Context, id string) (*Endpoint, error)
	Update(ctx context.Context, endpoint *Endpoint) error
	Delete(ctx context.Context, id string) error
	List(ctx context.Context, limit, offset int, includeDeleted bool) ([]Endpoint, int, error)
}

// EndpointCommandRepository provides persistence operations for EndpointCommand entities.
type EndpointCommandRepository interface {
	Create(ctx context.Context, cmd *EndpointCommand) error
	GetByID(ctx context.Context, id string) (*EndpointCommand, error)
	Update(ctx context.Context, cmd *EndpointCommand) error
	ListByEndpointID(ctx context.Context, endpointID string, limit, offset int) ([]EndpointCommand, int, error)
	ListPending(ctx context.Context, before time.Time) ([]EndpointCommand, error)
}

// LogFilter constrains a query for endpoint log entries.
type LogFilter struct {
	Start     *time.Time
	End       *time.Time
	Level     string
	Q         string
	TraceID   string
	RequestID string
	Cursor    int64
	Limit     int
}

// EndpointLogRepository provides persistence operations for EndpointLogEntry entities.
type EndpointLogRepository interface {
	Create(ctx context.Context, entry *EndpointLogEntry) error
	ListByEndpointID(ctx context.Context, endpointID string, beforeID int64, limit int) ([]EndpointLogEntry, error)
	Query(ctx context.Context, endpointID string, filter LogFilter) ([]EndpointLogEntry, error)
	Cleanup(ctx context.Context, maxAge time.Duration, maxSizeBytes int64) error
}
