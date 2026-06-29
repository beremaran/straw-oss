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

type ManagementAuditRepository interface {
	Create(ctx context.Context, event *ManagementAuditEvent) error
}
