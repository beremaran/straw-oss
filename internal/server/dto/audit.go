package dto

import (
	"time"

	"github.com/beremaran/straw/internal/domain"
)

// AuditEvent represents an audit event in the system.
type AuditEvent struct {
	ID           int64     `json:"id"`
	OccurredAt   time.Time `json:"occurred_at"`
	ActorType    string    `json:"actor_type"`
	ActorID      string    `json:"actor_id"`
	ActorDisplay string    `json:"actor_display"`
	Action       string    `json:"action"`
	EntityType   string    `json:"entity_type"`
	EntityID     string    `json:"entity_id"`
	OldValue     any       `json:"old_value,omitempty"`
	NewValue     any       `json:"new_value,omitempty"`
	RequestID    string    `json:"request_id,omitempty"`
	TraceID      string    `json:"trace_id,omitempty"`
	IP           string    `json:"ip,omitempty"`
	UserAgent    string    `json:"user_agent,omitempty"`
}

// ListAuditEventsResponse is a paginated list of audit events.
type ListAuditEventsResponse struct {
	Data  []*AuditEvent `json:"data"`
	Total int           `json:"total"`
	Page  int           `json:"page"`
	Limit int           `json:"limit"`
}

// AuditRequest represents a recorded HTTP request for audit purposes.
type AuditRequest struct {
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

// ListAuditRequestsResponse is a paginated list of audit requests.
type ListAuditRequestsResponse struct {
	Data  []*AuditRequest `json:"data"`
	Total int             `json:"total"`
	Page  int             `json:"page"`
	Limit int             `json:"limit"`
}

// FromAuditEvent converts a domain audit event to a DTO.
func FromAuditEvent(e *domain.ManagementAuditEvent, redactBody bool) *AuditEvent {
	if e == nil {
		return nil
	}

	dtoEvent := &AuditEvent{
		ID:           e.ID,
		OccurredAt:   e.OccurredAt,
		ActorType:    e.ActorType,
		ActorID:      e.ActorID,
		ActorDisplay: e.ActorDisplay,
		Action:       e.Action,
		EntityType:   e.EntityType,
		EntityID:     e.EntityID,
		RequestID:    e.RequestID,
		TraceID:      e.TraceID,
		IP:           e.IP,
		UserAgent:    e.UserAgent,
	}
	if !redactBody {
		dtoEvent.OldValue = e.OldValue
		dtoEvent.NewValue = e.NewValue
	}

	return dtoEvent
}

// FromAuditEvents converts a slice of domain audit events to DTOs.
func FromAuditEvents(events []*domain.ManagementAuditEvent, redactBody bool) []*AuditEvent {
	dtos := make([]*AuditEvent, len(events))
	for i, e := range events {
		dtos[i] = FromAuditEvent(e, redactBody)
	}

	return dtos
}

// FromAuditRequest converts a domain audit request to a DTO.
func FromAuditRequest(r *domain.ManagementAuditRequest) *AuditRequest {
	if r == nil {
		return nil
	}

	return &AuditRequest{
		ID:               r.ID,
		Timestamp:        r.Timestamp,
		Method:           r.Method,
		Path:             r.Path,
		Query:            r.Query,
		Body:             r.Body,
		IP:               r.IP,
		UserAgent:        r.UserAgent,
		Status:           r.Status,
		Error:            r.Error,
		ActorType:        r.ActorType,
		ActorID:          r.ActorID,
		ActorDisplayName: r.ActorDisplayName,
		SessionID:        r.SessionID,
		RequestID:        r.RequestID,
		TraceID:          r.TraceID,
	}
}

// FromAuditRequests converts a slice of domain audit requests to DTOs.
func FromAuditRequests(requests []*domain.ManagementAuditRequest) []*AuditRequest {
	dtos := make([]*AuditRequest, len(requests))
	for i, r := range requests {
		dtos[i] = FromAuditRequest(r)
	}

	return dtos
}
