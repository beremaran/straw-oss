package handlers

import (
	"bytes"
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"

	"github.com/beremaran/straw/internal/domain"
)

const (
	testAlertRuleID    = "rule-1"
	testAlertEventID   = "event-1"
	testAlertChannelID = "83fce6a8-0942-4b34-8bb5-bde8cc3531fb"
)

type MockAlertRuleRepo struct {
	mock.Mock
}

func (m *MockAlertRuleRepo) Create(ctx context.Context, rule *domain.AlertRule) error {
	args := m.Called(ctx, rule)

	return args.Error(0)
}

func (m *MockAlertRuleRepo) Update(ctx context.Context, rule *domain.AlertRule) error {
	args := m.Called(ctx, rule)

	return args.Error(0)
}

func (m *MockAlertRuleRepo) Disable(ctx context.Context, id string) error {
	args := m.Called(ctx, id)

	return args.Error(0)
}

func (m *MockAlertRuleRepo) GetByID(ctx context.Context, id string) (*domain.AlertRule, error) {
	args := m.Called(ctx, id)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}

	return args.Get(0).(*domain.AlertRule), args.Error(1)
}

func (m *MockAlertRuleRepo) List(ctx context.Context, limit, offset int) ([]domain.AlertRule, int, error) {
	args := m.Called(ctx, limit, offset)
	if args.Get(0) == nil {
		return nil, 0, args.Error(2)
	}

	return args.Get(0).([]domain.AlertRule), args.Int(1), args.Error(2)
}

func (m *MockAlertRuleRepo) ListActive(ctx context.Context) ([]domain.AlertRule, error) {
	args := m.Called(ctx)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}

	return args.Get(0).([]domain.AlertRule), args.Error(1)
}

type MockAlertEventRepo struct {
	mock.Mock
}

func (m *MockAlertEventRepo) Create(ctx context.Context, event *domain.AlertEvent) error {
	args := m.Called(ctx, event)

	return args.Error(0)
}

func (m *MockAlertEventRepo) Update(ctx context.Context, event *domain.AlertEvent) error {
	args := m.Called(ctx, event)

	return args.Error(0)
}

func (m *MockAlertEventRepo) GetByID(ctx context.Context, id string) (*domain.AlertEvent, error) {
	args := m.Called(ctx, id)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}

	return args.Get(0).(*domain.AlertEvent), args.Error(1)
}

func (m *MockAlertEventRepo) List(ctx context.Context, limit, offset int) ([]domain.AlertEvent, int, error) {
	args := m.Called(ctx, limit, offset)
	if args.Get(0) == nil {
		return nil, 0, args.Error(2)
	}

	return args.Get(0).([]domain.AlertEvent), args.Int(1), args.Error(2)
}

func (m *MockAlertEventRepo) ActiveForRule(ctx context.Context, ruleID string) (*domain.AlertEvent, error) {
	args := m.Called(ctx, ruleID)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}

	return args.Get(0).(*domain.AlertEvent), args.Error(1)
}

func TestAlertHandler_HandleCreateRuleRejectsInvalidRule(t *testing.T) {
	tests := []struct {
		name string
		body string
	}{
		{
			name: "unsupported metric",
			body: `{"name":"Bad","metric":"missing","condition":"greater_than","threshold":1}`,
		},
		{
			name: "unsupported condition",
			body: `{"name":"Bad","metric":"usage_requests","condition":"near","threshold":1}`,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ruleRepo := new(MockAlertRuleRepo)
			handler := NewAlertHandler(ruleRepo, new(MockAlertEventRepo), nil)
			req := httptest.NewRequestWithContext(context.Background(), http.MethodPost, "/management/alerts/rules", bytes.NewBufferString(tt.body))
			rec := httptest.NewRecorder()

			handler.HandleCreateRule(rec, req)

			require.Equal(t, http.StatusBadRequest, rec.Code)
			ruleRepo.AssertNotCalled(t, "Create")
		})
	}
}

func TestAlertHandler_HandleRuleCRUDAndAcknowledge(t *testing.T) {
	ruleRepo := new(MockAlertRuleRepo)
	eventRepo := new(MockAlertEventRepo)
	handler := NewAlertHandler(ruleRepo, eventRepo, nil)
	rule := testAlertRule()
	event := testAlertEvent()

	ruleRepo.On("List", mock.Anything, defaultReportLimit, 0).Return([]domain.AlertRule{*rule}, 1, nil).Once()
	handler.HandleListRules(httptest.NewRecorder(), httptest.NewRequestWithContext(context.Background(), http.MethodGet, "/management/alerts/rules", nil))

	createReq := httptest.NewRequestWithContext(context.Background(), http.MethodPost, "/management/alerts/rules", bytes.NewBufferString(`{
		"name": "High requests",
		"metric": "usage_requests",
		"condition": "greater_than",
		"threshold": 10,
		"notification_channel_ids": ["83fce6a8-0942-4b34-8bb5-bde8cc3531fb"]
	}`))
	createRec := httptest.NewRecorder()
	ruleRepo.On("Create", mock.Anything, mock.MatchedBy(func(rule *domain.AlertRule) bool {
		return rule.Name == "High requests" && rule.Window == defaultAlertWindow && len(rule.NotificationChannelIDs) == 1
	})).Return(nil).Once()
	handler.HandleCreateRule(createRec, createReq)
	require.Equal(t, http.StatusCreated, createRec.Code)

	getReq := httptest.NewRequestWithContext(context.Background(), http.MethodGet, "/management/alerts/rules/rule-1", nil)
	getReq.SetPathValue("id", testAlertRuleID)
	ruleRepo.On("GetByID", mock.Anything, testAlertRuleID).Return(rule, nil).Once()
	handler.HandleGetRule(httptest.NewRecorder(), getReq)

	updateReq := httptest.NewRequestWithContext(context.Background(), http.MethodPatch, "/management/alerts/rules/rule-1", bytes.NewBufferString(`{"description":"Updated"}`))
	updateReq.SetPathValue("id", testAlertRuleID)
	ruleRepo.On("GetByID", mock.Anything, testAlertRuleID).Return(rule, nil).Once()
	ruleRepo.On("Update", mock.Anything, mock.MatchedBy(func(rule *domain.AlertRule) bool {
		return rule.Description == "Updated"
	})).Return(nil).Once()
	handler.HandleUpdateRule(httptest.NewRecorder(), updateReq)

	deleteReq := httptest.NewRequestWithContext(context.Background(), http.MethodDelete, "/management/alerts/rules/rule-1", nil)
	deleteReq.SetPathValue("id", testAlertRuleID)
	ruleRepo.On("GetByID", mock.Anything, testAlertRuleID).Return(rule, nil).Once()
	ruleRepo.On("Disable", mock.Anything, testAlertRuleID).Return(nil).Once()
	deleteRec := httptest.NewRecorder()
	handler.HandleDeleteRule(deleteRec, deleteReq)
	require.Equal(t, http.StatusNoContent, deleteRec.Code)

	eventRepo.On("List", mock.Anything, defaultReportLimit, 0).Return([]domain.AlertEvent{*event}, 1, nil).Once()
	handler.HandleListEvents(httptest.NewRecorder(), httptest.NewRequestWithContext(context.Background(), http.MethodGet, "/management/alerts/events", nil))

	ackReq := httptest.NewRequestWithContext(context.Background(), http.MethodPost, "/management/alerts/events/event-1/ack", nil)
	ackReq.SetPathValue("id", testAlertEventID)
	eventRepo.On("GetByID", mock.Anything, testAlertEventID).Return(event, nil).Once()
	eventRepo.On("Update", mock.Anything, mock.MatchedBy(func(event *domain.AlertEvent) bool {
		return event.Status == domain.AlertStatusAcknowledged
	})).Return(nil).Once()
	ackRec := httptest.NewRecorder()
	handler.HandleAcknowledgeEvent(ackRec, ackReq)
	require.Equal(t, http.StatusOK, ackRec.Code)

	ruleRepo.AssertExpectations(t)
	eventRepo.AssertExpectations(t)
}

func testAlertRule() *domain.AlertRule {
	now := time.Now().UTC()

	return &domain.AlertRule{
		ID:                     testAlertRuleID,
		Name:                   "High requests",
		Metric:                 domain.AlertMetricUsageRequests,
		Condition:              domain.AlertConditionGreaterThan,
		Threshold:              10,
		Window:                 defaultAlertWindow,
		Filters:                domain.ConfigMap{},
		Severity:               defaultAlertSeverity,
		IsActive:               true,
		Cooldown:               defaultAlertCooldown,
		NotificationChannelIDs: []string{testAlertChannelID},
		CreatedAt:              now,
		UpdatedAt:              now,
	}
}

func testAlertEvent() *domain.AlertEvent {
	return &domain.AlertEvent{
		ID:          testAlertEventID,
		AlertRuleID: testAlertRuleID,
		Status:      domain.AlertStatusFiring,
		Value:       12,
		StartedAt:   time.Now().UTC(),
		Details:     domain.ConfigMap{},
	}
}
