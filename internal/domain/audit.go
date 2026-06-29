package domain

import (
	"context"
	"time"
)

const (
	ActionCreate   = "create"
	ActionUpdate   = "update"
	ActionDelete   = "delete"
	ActionRevoke   = "revoke"
	ActionDrain    = "drain"
	ActionUndrain  = "undrain"
	ActionPurge    = "purge"
	ActionReorder  = "reorder"
)

type ManagementAuditEvent struct {
	ID           int64       `json:"id"`
	OccurredAt   time.Time   `json:"occurred_at"`
	ActorType    string      `json:"actor_type"`
	ActorID      string      `json:"actor_id"`
	ActorDisplay string      `json:"actor_display"`
	Action       string      `json:"action"`
	EntityType   string      `json:"entity_type"`
	EntityID     string      `json:"entity_id"`
	OldValue     interface{} `json:"old_value,omitempty"`
	NewValue     interface{} `json:"new_value,omitempty"`
	RequestID    string      `json:"request_id,omitempty"`
	TraceID      string      `json:"trace_id,omitempty"`
	IP           string      `json:"ip,omitempty"`
	UserAgent    string      `json:"user_agent,omitempty"`
}

type AuditEventFilter struct {
	StartDate *time.Time
	EndDate   *time.Time
	ActorID   *string
	Action    *string
	Limit     int
	Offset    int
}

type ManagementAuditRequest struct {
	ID               int64     `json:"id"`
	Timestamp        time.Time `json:"timestamp"`
	Method           string    `json:"method"`
	Path             string    `json:"path"`
	Query            string    `json:"query"`
	Body             string    `json:"body,omitempty"`
	IP               string    `json:"ip"`
	UserAgent        string    `json:"user_agent"`
	Status           int       `json:"status"`
	Error            string    `json:"error"`
	ActorType        string    `json:"actor_type"`
	ActorID          string    `json:"actor_id"`
	ActorDisplayName string    `json:"actor_display_name"`
	SessionID        string    `json:"session_id"`
	RequestID        string    `json:"request_id"`
	TraceID          string    `json:"trace_id"`
}

type ManagementAuditRepository interface {
	Create(ctx context.Context, event *ManagementAuditEvent) error
	GetEventByID(ctx context.Context, id int64) (*ManagementAuditEvent, error)
	ListEvents(ctx context.Context, filter AuditEventFilter) ([]*ManagementAuditEvent, int, error)
	ListRequests(ctx context.Context, filter AuditEventFilter, includeBody bool) ([]*ManagementAuditRequest, int, error)
}
