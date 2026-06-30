// Package alert evaluates alert rules and records alert events.
package alert

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"

	"github.com/beremaran/straw/internal/domain"
)

const (
	defaultRuleWindow   = 5 * time.Minute
	defaultCooldown     = 15 * time.Minute
	metricEndpointLimit = 10000
	costPerUnitUSD      = 0.0001
)

var (
	// ErrUnsupportedMetric is returned when a rule references an unknown metric.
	ErrUnsupportedMetric = errors.New("unsupported alert metric")
	// ErrUnsupportedCondition is returned when a rule references an unknown condition.
	ErrUnsupportedCondition = errors.New("unsupported alert condition")
	// ErrMetricSourceUnavailable is returned when a metric cannot be collected.
	ErrMetricSourceUnavailable = errors.New("alert metric source unavailable")
	// ErrEventRepositoryUnavailable is returned when events cannot be persisted.
	ErrEventRepositoryUnavailable = errors.New("alert event repository unavailable")
	// ErrMetricProviderUnavailable is returned when the evaluator has no metric provider.
	ErrMetricProviderUnavailable = errors.New("alert metric provider unavailable")
)

// MetricProvider returns the current value for a rule metric.
type MetricProvider func(ctx context.Context, rule domain.AlertRule) (float64, error)

// Notifier delivers an alert event notification.
type Notifier func(ctx context.Context, rule domain.AlertRule, event *domain.AlertEvent) error

// CacheMetricProvider returns the current cache error rate.
type CacheMetricProvider func(ctx context.Context, rule domain.AlertRule) (float64, error)

// Evaluator evaluates alert rules and updates alert events.
type Evaluator struct {
	eventRepo domain.AlertEventRepository
	metric    MetricProvider
	notify    Notifier
	now       func() time.Time
}

// NewEvaluator creates an alert evaluator.
func NewEvaluator(eventRepo domain.AlertEventRepository, metric MetricProvider, notify Notifier) *Evaluator {
	return &Evaluator{
		eventRepo: eventRepo,
		metric:    metric,
		notify:    notify,
		now:       func() time.Time { return time.Now().UTC() },
	}
}

// EvaluateRule evaluates one alert rule and returns the changed event, if any.
func (e *Evaluator) EvaluateRule(ctx context.Context, rule domain.AlertRule) (*domain.AlertEvent, error) {
	err := e.validate(rule)
	if err != nil {
		return nil, err
	}

	value, err := e.metric(ctx, rule)
	if err != nil {
		return nil, fmt.Errorf("get alert metric: %w", err)
	}

	active, err := e.eventRepo.ActiveForRule(ctx, rule.ID)
	if err != nil {
		return nil, fmt.Errorf("get active alert event: %w", err)
	}

	if !ConditionMatches(rule.Condition, value, rule.Threshold) {
		event, err := e.resolveIfActive(ctx, active, value)
		if err != nil {
			return nil, err
		}

		return event, nil
	}

	event, err := e.fire(ctx, rule, active, value)
	if err != nil {
		return nil, err
	}

	return event, nil
}

// RepositoryMetricProvider reads alert metrics from existing repositories.
type RepositoryMetricProvider struct {
	EndpointRepo       domain.EndpointRepository
	UsageRepo          domain.UsageRepository
	CostMultiplierRepo domain.CostMultiplierRepository
	CacheErrorRate     CacheMetricProvider
	Now                func() time.Time
}

// Value returns a repository-backed metric value.
func (p RepositoryMetricProvider) Value(ctx context.Context, rule domain.AlertRule) (float64, error) {
	switch rule.Metric {
	case domain.AlertMetricEndpointUnhealthyCount,
		domain.AlertMetricEndpointDrainingCount,
		domain.AlertMetricEndpointActiveTasks:
		value, err := p.endpointMetric(ctx, rule.Metric)
		if err != nil {
			return 0, err
		}

		return value, nil
	case domain.AlertMetricUsageRequests,
		domain.AlertMetricUsageBytes,
		domain.AlertMetricBillingEstimatedUSD:
		value, err := p.usageMetric(ctx, rule)
		if err != nil {
			return 0, err
		}

		return value, nil
	case domain.AlertMetricCacheErrorRate:
		if p.CacheErrorRate == nil {
			return 0, ErrMetricSourceUnavailable
		}

		value, err := p.CacheErrorRate(ctx, rule)
		if err != nil {
			return 0, fmt.Errorf("get cache error rate: %w", err)
		}

		return value, nil
	default:
		return 0, ErrUnsupportedMetric
	}
}

// ChannelDeliverer sends a notification through one channel.
type ChannelDeliverer func(ctx context.Context, channel *domain.NotificationChannel) error

// ChannelNotifier delivers alert notifications through configured channels.
type ChannelNotifier struct {
	channelRepo domain.NotificationChannelRepository
	deliver     ChannelDeliverer
}

// NewChannelNotifier creates a notifier backed by notification channels.
func NewChannelNotifier(channelRepo domain.NotificationChannelRepository, deliver ChannelDeliverer) *ChannelNotifier {
	return &ChannelNotifier{channelRepo: channelRepo, deliver: deliver}
}

// Notify sends an alert event through the rule's notification channels.
func (n *ChannelNotifier) Notify(ctx context.Context, rule domain.AlertRule, _ *domain.AlertEvent) error {
	if n == nil || n.channelRepo == nil || n.deliver == nil || len(rule.NotificationChannelIDs) == 0 {
		return nil
	}

	var errs []error

	for _, channelID := range rule.NotificationChannelIDs {
		channel, err := n.channelRepo.GetByID(ctx, channelID)
		if err != nil {
			errs = append(errs, fmt.Errorf("get notification channel %s: %w", channelID, err))

			continue
		}

		if channel == nil || !channel.IsEnabled {
			continue
		}

		err = n.deliver(ctx, channel)
		if err != nil {
			errs = append(errs, fmt.Errorf("deliver notification channel %s: %w", channelID, err))
		}
	}

	return errors.Join(errs...)
}

// IsMetricSupported reports whether a metric can be evaluated.
func IsMetricSupported(metric string) bool {
	switch metric {
	case domain.AlertMetricEndpointUnhealthyCount,
		domain.AlertMetricEndpointDrainingCount,
		domain.AlertMetricEndpointActiveTasks,
		domain.AlertMetricUsageRequests,
		domain.AlertMetricUsageBytes,
		domain.AlertMetricBillingEstimatedUSD,
		domain.AlertMetricCacheErrorRate:
		return true
	default:
		return false
	}
}

// IsConditionSupported reports whether a condition can be evaluated.
func IsConditionSupported(condition string) bool {
	switch condition {
	case domain.AlertConditionGreaterThan,
		domain.AlertConditionGreaterThanOrEqual,
		domain.AlertConditionLessThan,
		domain.AlertConditionLessThanOrEqual,
		domain.AlertConditionEquals:
		return true
	default:
		return false
	}
}

// ConditionMatches compares a metric value against a rule threshold.
func ConditionMatches(condition string, value float64, threshold float64) bool {
	switch condition {
	case domain.AlertConditionGreaterThan:
		return value > threshold
	case domain.AlertConditionGreaterThanOrEqual:
		return value >= threshold
	case domain.AlertConditionLessThan:
		return value < threshold
	case domain.AlertConditionLessThanOrEqual:
		return value <= threshold
	case domain.AlertConditionEquals:
		return value == threshold
	default:
		return false
	}
}

// RuleWindow parses a rule window or returns the default alert window.
func RuleWindow(rule domain.AlertRule) (time.Duration, error) {
	window := strings.TrimSpace(rule.Window)
	if window == "" {
		return defaultRuleWindow, nil
	}

	duration, err := time.ParseDuration(window)
	if err != nil {
		return 0, fmt.Errorf("parse alert window: %w", err)
	}

	return duration, nil
}

func (e *Evaluator) fire(ctx context.Context, rule domain.AlertRule, active *domain.AlertEvent, value float64) (*domain.AlertEvent, error) {
	now := e.now()

	event := active
	if event == nil {
		event = &domain.AlertEvent{
			ID:          uuid.New().String(),
			AlertRuleID: rule.ID,
			Status:      domain.AlertStatusFiring,
			StartedAt:   now,
		}
	}

	event.Value = value
	event.Details = alertDetails(rule, value)

	err := e.saveFiringEvent(ctx, event, active == nil)
	if err != nil {
		return nil, err
	}

	notify, err := shouldNotify(rule, event, now)
	if err != nil {
		return nil, err
	}

	if !notify || e.notify == nil {
		return event, nil
	}

	err = e.notify(ctx, rule, event)
	if err != nil {
		return nil, fmt.Errorf("notify alert event: %w", err)
	}

	event.LastNotifiedAt = &now

	err = e.eventRepo.Update(ctx, event)
	if err != nil {
		return nil, fmt.Errorf("update alert notification timestamp: %w", err)
	}

	return event, nil
}

func (e *Evaluator) validate(rule domain.AlertRule) error {
	if e.eventRepo == nil {
		return ErrEventRepositoryUnavailable
	}

	if e.metric == nil {
		return ErrMetricProviderUnavailable
	}

	if !IsMetricSupported(rule.Metric) {
		return ErrUnsupportedMetric
	}

	if !IsConditionSupported(rule.Condition) {
		return ErrUnsupportedCondition
	}

	return nil
}

func (e *Evaluator) resolveIfActive(ctx context.Context, active *domain.AlertEvent, value float64) (*domain.AlertEvent, error) {
	if active == nil {
		return nil, nil
	}

	event, err := e.resolve(ctx, active, value)
	if err != nil {
		return nil, err
	}

	return event, nil
}

func (e *Evaluator) resolve(ctx context.Context, event *domain.AlertEvent, value float64) (*domain.AlertEvent, error) {
	now := e.now()
	event.Status = domain.AlertStatusResolved
	event.ResolvedAt = &now
	event.Value = value

	err := e.eventRepo.Update(ctx, event)
	if err != nil {
		return nil, fmt.Errorf("resolve alert event: %w", err)
	}

	return event, nil
}

func (e *Evaluator) saveFiringEvent(ctx context.Context, event *domain.AlertEvent, create bool) error {
	if create {
		err := e.eventRepo.Create(ctx, event)
		if err != nil {
			return fmt.Errorf("create alert event: %w", err)
		}

		return nil
	}

	err := e.eventRepo.Update(ctx, event)
	if err != nil {
		return fmt.Errorf("update alert event: %w", err)
	}

	return nil
}

func (p RepositoryMetricProvider) endpointMetric(ctx context.Context, metric string) (float64, error) {
	if p.EndpointRepo == nil {
		return 0, ErrMetricSourceUnavailable
	}

	endpoints, _, err := p.EndpointRepo.List(ctx, metricEndpointLimit, 0, false)
	if err != nil {
		return 0, fmt.Errorf("list endpoints for alert metric: %w", err)
	}

	switch metric {
	case domain.AlertMetricEndpointUnhealthyCount:
		return endpointUnhealthyCount(endpoints), nil
	case domain.AlertMetricEndpointDrainingCount:
		return endpointDrainingCount(endpoints), nil
	case domain.AlertMetricEndpointActiveTasks:
		return endpointActiveTasks(endpoints), nil
	default:
		return 0, ErrUnsupportedMetric
	}
}

func (p RepositoryMetricProvider) usageMetric(ctx context.Context, rule domain.AlertRule) (float64, error) {
	if p.UsageRepo == nil {
		return 0, ErrMetricSourceUnavailable
	}

	window, err := RuleWindow(rule)
	if err != nil {
		return 0, err
	}

	end := p.now()
	start := end.Add(-window)
	apiKeyID := mapString(rule.Filters, "api_key_id")

	summaries, err := p.UsageRepo.GetDailySummaries(ctx, apiKeyID, start, end)
	if err != nil {
		return 0, fmt.Errorf("get usage summaries for alert metric: %w", err)
	}

	switch rule.Metric {
	case domain.AlertMetricUsageRequests:
		return usageRequests(summaries), nil
	case domain.AlertMetricUsageBytes:
		return usageBytes(summaries), nil
	case domain.AlertMetricBillingEstimatedUSD:
		estimated, err := p.billingEstimatedUSD(ctx, summaries)
		if err != nil {
			return 0, err
		}

		return estimated, nil
	default:
		return 0, ErrUnsupportedMetric
	}
}

func (p RepositoryMetricProvider) billingEstimatedUSD(ctx context.Context, summaries []domain.UsageSummary) (float64, error) {
	var multipliers []domain.CostMultiplier

	if p.CostMultiplierRepo != nil {
		var err error

		multipliers, err = p.CostMultiplierRepo.ListActive(ctx)
		if err != nil {
			return 0, fmt.Errorf("list active cost multipliers for alert metric: %w", err)
		}
	}

	return billingCostUnits(summaries, multipliers) * costPerUnitUSD, nil
}

func (p RepositoryMetricProvider) now() time.Time {
	if p.Now != nil {
		return p.Now()
	}

	return time.Now().UTC()
}

func shouldNotify(rule domain.AlertRule, event *domain.AlertEvent, now time.Time) (bool, error) {
	if len(rule.NotificationChannelIDs) == 0 {
		return false, nil
	}

	if event.LastNotifiedAt == nil {
		return true, nil
	}

	cooldown, err := ruleCooldown(rule)
	if err != nil {
		return false, err
	}

	return !event.LastNotifiedAt.Add(cooldown).After(now), nil
}

func ruleCooldown(rule domain.AlertRule) (time.Duration, error) {
	cooldown := strings.TrimSpace(rule.Cooldown)
	if cooldown == "" {
		return defaultCooldown, nil
	}

	duration, err := time.ParseDuration(cooldown)
	if err != nil {
		return 0, fmt.Errorf("parse alert cooldown: %w", err)
	}

	return duration, nil
}

func alertDetails(rule domain.AlertRule, value float64) domain.ConfigMap {
	return domain.ConfigMap{
		"metric":    rule.Metric,
		"condition": rule.Condition,
		"threshold": rule.Threshold,
		"value":     value,
	}
}

func endpointUnhealthyCount(endpoints []domain.Endpoint) float64 {
	total := 0.0

	for _, endpoint := range endpoints {
		if !endpoint.IsHealthy {
			total++
		}
	}

	return total
}

func endpointDrainingCount(endpoints []domain.Endpoint) float64 {
	total := 0.0

	for _, endpoint := range endpoints {
		if endpoint.DesiredState == domain.DesiredStateDraining {
			total++
		}
	}

	return total
}

func endpointActiveTasks(endpoints []domain.Endpoint) float64 {
	total := 0.0
	for _, endpoint := range endpoints {
		total += float64(endpoint.Metadata.ActiveTasks)
	}

	return total
}

func usageRequests(summaries []domain.UsageSummary) float64 {
	total := int64(0)
	for _, summary := range summaries {
		total += summary.TotalRequests
	}

	return float64(total)
}

func usageBytes(summaries []domain.UsageSummary) float64 {
	total := int64(0)
	for _, summary := range summaries {
		total += summary.TotalBytes
	}

	return float64(total)
}

func billingCostUnits(summaries []domain.UsageSummary, multipliers []domain.CostMultiplier) float64 {
	total := 0.0
	for _, summary := range summaries {
		total += summaryCostUnits(summary, multipliers)
	}

	return total
}

func summaryCostUnits(summary domain.UsageSummary, multipliers []domain.CostMultiplier) float64 {
	if len(multipliers) == 0 || len(summary.Breakdown) == 0 {
		return summary.CostUnits
	}

	totalWeight := int64(0)

	for _, count := range summary.Breakdown {
		if count > 0 {
			totalWeight += count
		}
	}

	if totalWeight == 0 {
		return summary.CostUnits
	}

	adjusted := 0.0
	matched := false

	for key, count := range summary.Breakdown {
		if count <= 0 {
			continue
		}

		units := summary.CostUnits * (float64(count) / float64(totalWeight))
		multiplier, ok := matchMultiplier(key, multipliers)

		if ok {
			units *= multiplier.Multiplier
			matched = true
		}

		adjusted += units
	}

	if !matched {
		return summary.CostUnits
	}

	return adjusted
}

func matchMultiplier(key string, multipliers []domain.CostMultiplier) (domain.CostMultiplier, bool) {
	for _, multiplier := range multipliers {
		if multiplier.EndpointTag == key {
			return multiplier, true
		}
	}

	for _, multiplier := range multipliers {
		_, value, ok := strings.Cut(multiplier.EndpointTag, ":")
		if ok && value == key {
			return multiplier, true
		}
	}

	return domain.CostMultiplier{}, false
}

func mapString(values domain.ConfigMap, key string) string {
	value, ok := values[key]
	if !ok || value == nil {
		return ""
	}

	switch v := value.(type) {
	case string:
		return strings.TrimSpace(v)
	default:
		return strings.TrimSpace(fmt.Sprint(v))
	}
}
