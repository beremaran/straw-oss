package alert

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/beremaran/straw/internal/domain"
)

const (
	testRuleID            = "rule-1"
	testEventID           = "event-1"
	testChannelID         = "channel-1"
	testDisabledChannelID = "disabled"
)

func TestEvaluator_FiresAndNotifies(t *testing.T) {
	repo := &memoryEventRepo{}
	notified := 0
	evaluator := NewEvaluator(repo, fixedMetric(2), func(_ context.Context, _ domain.AlertRule, _ *domain.AlertEvent) error {
		notified++

		return nil
	})
	now := time.Date(2026, time.June, 30, 12, 0, 0, 0, time.UTC)
	evaluator.now = func() time.Time { return now }

	event, err := evaluator.EvaluateRule(context.Background(), alertRule())

	require.NoError(t, err)
	require.NotNil(t, event)
	assert.Equal(t, domain.AlertStatusFiring, event.Status)
	assert.InEpsilon(t, 2.0, event.Value, 0.001)
	assert.Equal(t, 1, notified)
	require.NotNil(t, event.LastNotifiedAt)
	assert.Equal(t, now, *event.LastNotifiedAt)
}

func TestEvaluator_CooldownSuppressesDelivery(t *testing.T) {
	now := time.Date(2026, time.June, 30, 12, 0, 0, 0, time.UTC)
	lastNotified := now.Add(-time.Minute)
	repo := &memoryEventRepo{active: &domain.AlertEvent{
		ID:             testEventID,
		AlertRuleID:    testRuleID,
		Status:         domain.AlertStatusFiring,
		Value:          2,
		StartedAt:      now.Add(-time.Hour),
		LastNotifiedAt: &lastNotified,
	}}
	notified := 0
	evaluator := NewEvaluator(repo, fixedMetric(3), func(_ context.Context, _ domain.AlertRule, _ *domain.AlertEvent) error {
		notified++

		return nil
	})
	evaluator.now = func() time.Time { return now }

	event, err := evaluator.EvaluateRule(context.Background(), alertRule())

	require.NoError(t, err)
	require.NotNil(t, event)
	assert.Equal(t, 0, notified)
	assert.InEpsilon(t, 3.0, event.Value, 0.001)
	assert.Equal(t, lastNotified, *event.LastNotifiedAt)
}

func TestEvaluator_ResolvesActiveEvent(t *testing.T) {
	now := time.Date(2026, time.June, 30, 12, 0, 0, 0, time.UTC)
	repo := &memoryEventRepo{active: &domain.AlertEvent{
		ID:          testEventID,
		AlertRuleID: testRuleID,
		Status:      domain.AlertStatusFiring,
		Value:       2,
		StartedAt:   now.Add(-time.Hour),
	}}
	evaluator := NewEvaluator(repo, fixedMetric(0), nil)
	evaluator.now = func() time.Time { return now }

	event, err := evaluator.EvaluateRule(context.Background(), alertRule())

	require.NoError(t, err)
	require.NotNil(t, event)
	assert.Equal(t, domain.AlertStatusResolved, event.Status)
	require.NotNil(t, event.ResolvedAt)
	assert.Equal(t, now, *event.ResolvedAt)
}

func TestEvaluator_RejectsUnsupportedRuleParts(t *testing.T) {
	repo := &memoryEventRepo{}
	evaluator := NewEvaluator(repo, fixedMetric(0), nil)
	rule := alertRule()
	rule.Metric = "missing"

	_, err := evaluator.EvaluateRule(context.Background(), rule)
	require.ErrorIs(t, err, ErrUnsupportedMetric)

	rule = alertRule()
	rule.Condition = "near"

	_, err = evaluator.EvaluateRule(context.Background(), rule)
	require.ErrorIs(t, err, ErrUnsupportedCondition)
}

func TestChannelNotifier_UsesConfiguredChannels(t *testing.T) {
	channelRepo := &memoryChannelRepo{
		channels: map[string]*domain.NotificationChannel{
			testChannelID: {
				ID:        testChannelID,
				IsEnabled: true,
			},
			testDisabledChannelID: {
				ID:        testDisabledChannelID,
				IsEnabled: false,
			},
		},
	}
	delivered := 0
	notifier := NewChannelNotifier(channelRepo, func(_ context.Context, channel *domain.NotificationChannel) error {
		if channel.ID == testChannelID {
			delivered++
		}

		return nil
	})
	rule := alertRule()
	rule.NotificationChannelIDs = []string{testChannelID, testDisabledChannelID}

	err := notifier.Notify(context.Background(), rule, &domain.AlertEvent{ID: testEventID})

	require.NoError(t, err)
	assert.Equal(t, 1, delivered)
}

func fixedMetric(value float64) MetricProvider {
	return func(context.Context, domain.AlertRule) (float64, error) {
		return value, nil
	}
}

func alertRule() domain.AlertRule {
	return domain.AlertRule{
		ID:                     testRuleID,
		Metric:                 domain.AlertMetricUsageRequests,
		Condition:              domain.AlertConditionGreaterThan,
		Threshold:              1,
		Cooldown:               "15m",
		NotificationChannelIDs: []string{testChannelID},
	}
}

type memoryEventRepo struct {
	active *domain.AlertEvent
}

func (r *memoryEventRepo) Create(_ context.Context, event *domain.AlertEvent) error {
	r.active = event

	return nil
}

func (r *memoryEventRepo) Update(_ context.Context, event *domain.AlertEvent) error {
	r.active = event

	return nil
}

func (r *memoryEventRepo) GetByID(_ context.Context, id string) (*domain.AlertEvent, error) {
	if r.active != nil && r.active.ID == id {
		return r.active, nil
	}

	return nil, nil
}

func (r *memoryEventRepo) List(_ context.Context, _ int, _ int) ([]domain.AlertEvent, int, error) {
	if r.active == nil {
		return nil, 0, nil
	}

	return []domain.AlertEvent{*r.active}, 1, nil
}

func (r *memoryEventRepo) ActiveForRule(_ context.Context, ruleID string) (*domain.AlertEvent, error) {
	if r.active != nil && r.active.AlertRuleID == ruleID && r.active.Status != domain.AlertStatusResolved {
		return r.active, nil
	}

	return nil, nil
}

type memoryChannelRepo struct {
	channels map[string]*domain.NotificationChannel
}

func (r *memoryChannelRepo) Create(context.Context, *domain.NotificationChannel) error {
	return errors.New("not implemented")
}

func (r *memoryChannelRepo) Update(context.Context, *domain.NotificationChannel) error {
	return errors.New("not implemented")
}

func (r *memoryChannelRepo) Disable(context.Context, string) error {
	return errors.New("not implemented")
}

func (r *memoryChannelRepo) GetByID(_ context.Context, id string) (*domain.NotificationChannel, error) {
	return r.channels[id], nil
}

func (r *memoryChannelRepo) List(context.Context, int, int) ([]domain.NotificationChannel, int, error) {
	return nil, 0, errors.New("not implemented")
}
