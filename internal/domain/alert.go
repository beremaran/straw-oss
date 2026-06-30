package domain

import (
	"context"
	"errors"
	"time"
)

const (
	// AlertMetricEndpointUnhealthyCount counts unhealthy endpoints.
	AlertMetricEndpointUnhealthyCount = "endpoint_unhealthy_count"
	// AlertMetricEndpointDrainingCount counts draining endpoints.
	AlertMetricEndpointDrainingCount = "endpoint_draining_count"
	// AlertMetricEndpointActiveTasks sums active endpoint tasks.
	AlertMetricEndpointActiveTasks = "endpoint_active_tasks"
	// AlertMetricUsageRequests sums usage requests over the rule window.
	AlertMetricUsageRequests = "usage_requests"
	// AlertMetricUsageBytes sums usage bytes over the rule window.
	AlertMetricUsageBytes = "usage_bytes"
	// AlertMetricBillingEstimatedUSD estimates billing over the rule window.
	AlertMetricBillingEstimatedUSD = "billing_estimated_usd"
	// AlertMetricCacheErrorRate reports cache error rate.
	AlertMetricCacheErrorRate = "cache_error_rate"

	// AlertConditionGreaterThan fires when value is greater than threshold.
	AlertConditionGreaterThan = "greater_than"
	// AlertConditionGreaterThanOrEqual fires when value is at least threshold.
	AlertConditionGreaterThanOrEqual = "greater_than_or_equal"
	// AlertConditionLessThan fires when value is below threshold.
	AlertConditionLessThan = "less_than"
	// AlertConditionLessThanOrEqual fires when value is at most threshold.
	AlertConditionLessThanOrEqual = "less_than_or_equal"
	// AlertConditionEquals fires when value equals threshold.
	AlertConditionEquals = "equals"

	// AlertStatusFiring means the condition is currently true.
	AlertStatusFiring = "firing"
	// AlertStatusAcknowledged means a user has acknowledged the event.
	AlertStatusAcknowledged = "acknowledged"
	// AlertStatusResolved means the condition is no longer true.
	AlertStatusResolved = "resolved"
)

var (
	// ErrAlertRuleNotFound is returned when an alert rule does not exist.
	ErrAlertRuleNotFound = errors.New("alert rule not found")
	// ErrAlertEventNotFound is returned when an alert event does not exist.
	ErrAlertEventNotFound = errors.New("alert event not found")
)

// AlertRule stores a metric threshold rule.
type AlertRule struct {
	ID                     string
	Name                   string
	Description            string
	Metric                 string
	Condition              string
	Threshold              float64
	Window                 string
	Filters                ConfigMap
	Severity               string
	IsActive               bool
	Cooldown               string
	NotificationChannelIDs []string
	CreatedBy              string
	CreatedAt              time.Time
	UpdatedAt              time.Time
}

// AlertEvent stores one fired/resolved alert lifecycle.
type AlertEvent struct {
	ID             string
	AlertRuleID    string
	Status         string
	Value          float64
	StartedAt      time.Time
	ResolvedAt     *time.Time
	LastNotifiedAt *time.Time
	Details        ConfigMap
}

// AlertRuleRepository provides persistence operations for alert rules.
type AlertRuleRepository interface {
	Create(ctx context.Context, rule *AlertRule) error
	Update(ctx context.Context, rule *AlertRule) error
	Disable(ctx context.Context, id string) error
	GetByID(ctx context.Context, id string) (*AlertRule, error)
	List(ctx context.Context, limit, offset int) ([]AlertRule, int, error)
	ListActive(ctx context.Context) ([]AlertRule, error)
}

// AlertEventRepository provides persistence operations for alert events.
type AlertEventRepository interface {
	Create(ctx context.Context, event *AlertEvent) error
	Update(ctx context.Context, event *AlertEvent) error
	GetByID(ctx context.Context, id string) (*AlertEvent, error)
	List(ctx context.Context, limit, offset int) ([]AlertEvent, int, error)
	ActiveForRule(ctx context.Context, ruleID string) (*AlertEvent, error)
}
