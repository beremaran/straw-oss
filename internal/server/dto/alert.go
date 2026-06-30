package dto

import (
	"time"

	"github.com/beremaran/straw/internal/domain"
)

// CreateAlertRuleRequest creates an alert rule.
type CreateAlertRuleRequest struct {
	Name                   string           `json:"name"`
	Description            string           `json:"description,omitempty"`
	Metric                 string           `json:"metric"`
	Condition              string           `json:"condition"`
	Threshold              float64          `json:"threshold"`
	Window                 string           `json:"window,omitempty"`
	Filters                domain.ConfigMap `json:"filters,omitempty"`
	Severity               string           `json:"severity,omitempty"`
	IsActive               *bool            `json:"is_active,omitempty"`
	Cooldown               string           `json:"cooldown,omitempty"`
	NotificationChannelIDs []string         `json:"notification_channel_ids,omitempty"`
}

// UpdateAlertRuleRequest updates an alert rule.
type UpdateAlertRuleRequest struct {
	Name                   *string           `json:"name,omitempty"`
	Description            *string           `json:"description,omitempty"`
	Metric                 *string           `json:"metric,omitempty"`
	Condition              *string           `json:"condition,omitempty"`
	Threshold              *float64          `json:"threshold,omitempty"`
	Window                 *string           `json:"window,omitempty"`
	Filters                *domain.ConfigMap `json:"filters,omitempty"`
	Severity               *string           `json:"severity,omitempty"`
	IsActive               *bool             `json:"is_active,omitempty"`
	Cooldown               *string           `json:"cooldown,omitempty"`
	NotificationChannelIDs *[]string         `json:"notification_channel_ids,omitempty"`
}

// AlertRuleResponse represents an alert rule.
type AlertRuleResponse struct {
	ID                     string           `json:"id"`
	Name                   string           `json:"name"`
	Description            string           `json:"description,omitempty"`
	Metric                 string           `json:"metric"`
	Condition              string           `json:"condition"`
	Threshold              float64          `json:"threshold"`
	Window                 string           `json:"window"`
	Filters                domain.ConfigMap `json:"filters"`
	Severity               string           `json:"severity"`
	IsActive               bool             `json:"is_active"`
	Cooldown               string           `json:"cooldown"`
	NotificationChannelIDs []string         `json:"notification_channel_ids"`
	CreatedBy              string           `json:"created_by,omitempty"`
	CreatedAt              time.Time        `json:"created_at"`
	UpdatedAt              time.Time        `json:"updated_at"`
}

// AlertEventResponse represents an alert event.
type AlertEventResponse struct {
	ID             string           `json:"id"`
	AlertRuleID    string           `json:"alert_rule_id"`
	Status         string           `json:"status"`
	Value          float64          `json:"value"`
	StartedAt      time.Time        `json:"started_at"`
	ResolvedAt     *time.Time       `json:"resolved_at,omitempty"`
	LastNotifiedAt *time.Time       `json:"last_notified_at,omitempty"`
	Details        domain.ConfigMap `json:"details"`
}

// ListAlertRulesResponse is a paginated list of alert rules.
type ListAlertRulesResponse = PaginatedResponse[AlertRuleResponse]

// ListAlertEventsResponse is a paginated list of alert events.
type ListAlertEventsResponse = PaginatedResponse[AlertEventResponse]

// FromAlertRule converts a domain.AlertRule to an AlertRuleResponse.
func FromAlertRule(rule *domain.AlertRule) *AlertRuleResponse {
	if rule == nil {
		return nil
	}

	return &AlertRuleResponse{
		ID:                     rule.ID,
		Name:                   rule.Name,
		Description:            rule.Description,
		Metric:                 rule.Metric,
		Condition:              rule.Condition,
		Threshold:              rule.Threshold,
		Window:                 rule.Window,
		Filters:                rule.Filters,
		Severity:               rule.Severity,
		IsActive:               rule.IsActive,
		Cooldown:               rule.Cooldown,
		NotificationChannelIDs: rule.NotificationChannelIDs,
		CreatedBy:              rule.CreatedBy,
		CreatedAt:              rule.CreatedAt,
		UpdatedAt:              rule.UpdatedAt,
	}
}

// FromAlertRules converts alert rules to responses.
func FromAlertRules(rules []domain.AlertRule) []AlertRuleResponse {
	result := make([]AlertRuleResponse, len(rules))
	for i, rule := range rules {
		resp := FromAlertRule(&rule)
		if resp != nil {
			result[i] = *resp
		}
	}

	return result
}

// FromAlertEvent converts a domain.AlertEvent to an AlertEventResponse.
func FromAlertEvent(event *domain.AlertEvent) *AlertEventResponse {
	if event == nil {
		return nil
	}

	return &AlertEventResponse{
		ID:             event.ID,
		AlertRuleID:    event.AlertRuleID,
		Status:         event.Status,
		Value:          event.Value,
		StartedAt:      event.StartedAt,
		ResolvedAt:     event.ResolvedAt,
		LastNotifiedAt: event.LastNotifiedAt,
		Details:        event.Details,
	}
}

// FromAlertEvents converts alert events to responses.
func FromAlertEvents(events []domain.AlertEvent) []AlertEventResponse {
	result := make([]AlertEventResponse, len(events))
	for i, event := range events {
		resp := FromAlertEvent(&event)
		if resp != nil {
			result[i] = *resp
		}
	}

	return result
}
