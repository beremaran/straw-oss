package domain

import (
	"context"
	"time"
)

type DesiredState string

const (
	DesiredStateActive   DesiredState = "active"
	DesiredStateDraining DesiredState = "draining"
	DesiredStateDisabled DesiredState = "disabled"
	DesiredStateDeleted  DesiredState = "deleted"
)

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

type EndpointMetadata struct {
	Version        string `json:"version,omitempty"`
	IP             string `json:"ip,omitempty"`
	ActiveTasks    int    `json:"active_tasks"`
	MaxConcurrency int    `json:"max_concurrency,omitempty"`
	Provider       string `json:"provider,omitempty"`
}

type CommandStatus string

const (
	CommandStatusAccepted     CommandStatus = "accepted"
	CommandStatusAcknowledged CommandStatus = "acknowledged"
	CommandStatusRunning      CommandStatus = "running"
	CommandStatusSucceeded    CommandStatus = "succeeded"
	CommandStatusFailed       CommandStatus = "failed"
	CommandStatusTimedOut     CommandStatus = "timed_out"
)

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

const DefaultHeartbeatInterval = 10 * time.Second

const DefaultHealthThreshold = 30 * time.Second

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

type EndpointRepository interface {
	Create(ctx context.Context, endpoint *Endpoint) error
	GetByID(ctx context.Context, id string) (*Endpoint, error)
	Update(ctx context.Context, endpoint *Endpoint) error
	Delete(ctx context.Context, id string) error
	List(ctx context.Context, limit, offset int, includeDeleted bool) ([]Endpoint, int, error)
}

type EndpointCommandRepository interface {
	Create(ctx context.Context, cmd *EndpointCommand) error
	GetByID(ctx context.Context, id string) (*EndpointCommand, error)
	Update(ctx context.Context, cmd *EndpointCommand) error
	ListByEndpointID(ctx context.Context, endpointID string, limit, offset int) ([]EndpointCommand, int, error)
}

type EndpointLogRepository interface {
	Create(ctx context.Context, entry *EndpointLogEntry) error
	ListByEndpointID(ctx context.Context, endpointID string, beforeID int64, limit int) ([]EndpointLogEntry, error)
}
