package handlers

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/beremaran/straw/internal/domain"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
)

type MockRuleVersionManager struct {
	mock.Mock
}

func (m *MockRuleVersionManager) IncrementRulesVersion(ctx context.Context) (int64, error) {
	args := m.Called(ctx)

	return args.Get(0).(int64), args.Error(1)
}

func TestRoutingRuleHandler_HandleListRoutingRules(t *testing.T) {
	req := httptest.NewRequestWithContext(context.Background(), http.MethodGet, "/management/rules", nil)
	rec := httptest.NewRecorder()

	mockRepo := new(MockRoutingRuleRepo)
	mockVerMgr := new(MockRuleVersionManager)
	handler := NewRoutingRuleHandler(mockRepo, mockVerMgr)

	rules := []domain.RoutingRule{
		{ID: "rule1", Name: "Rule 1"},
	}
	mockRepo.On("ListRules", mock.Anything, 20, 0).Return(rules, 1, nil).Once()

	handler.HandleListRoutingRules(rec, req)

	assert.Equal(t, http.StatusOK, rec.Code)
}

func TestRoutingRuleHandler_HandleGetRoutingRule(t *testing.T) {
	req := httptest.NewRequestWithContext(context.Background(), http.MethodGet, "/management/rules/rule1", nil)
	req.SetPathValue("id", "rule1")
	rec := httptest.NewRecorder()

	mockRepo := new(MockRoutingRuleRepo)
	mockVerMgr := new(MockRuleVersionManager)
	handler := NewRoutingRuleHandler(mockRepo, mockVerMgr)

	rule := &domain.RoutingRule{ID: "rule1", Name: "Rule 1"}
	mockRepo.On("GetRuleByID", mock.Anything, "rule1").Return(rule, nil).Once()

	handler.HandleGetRoutingRule(rec, req)

	assert.Equal(t, http.StatusOK, rec.Code)
}

func TestRoutingRuleHandler_HandleCreateRoutingRule(t *testing.T) {
	rule := domain.RoutingRule{Name: "New Rule", Priority: 10}
	body, err := json.Marshal(rule)
	assert.NoError(t, err)
	req := httptest.NewRequestWithContext(context.Background(), http.MethodPost, "/management/rules", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()

	mockRepo := new(MockRoutingRuleRepo)
	mockVerMgr := new(MockRuleVersionManager)
	handler := NewRoutingRuleHandler(mockRepo, mockVerMgr)

	mockRepo.On("CreateRule", mock.Anything, mock.MatchedBy(func(r *domain.RoutingRule) bool {
		return r.Name == "New Rule" && r.Priority == 10
	})).Return(nil).Once()

	mockVerMgr.On("IncrementRulesVersion", mock.Anything).Return(int64(2), nil).Once()

	handler.HandleCreateRoutingRule(rec, req)

	assert.Equal(t, http.StatusCreated, rec.Code)
	mockVerMgr.AssertExpectations(t)
}

func TestRoutingRuleHandler_HandleUpdateRoutingRule(t *testing.T) {
	rule := domain.RoutingRule{ID: "rule1", Name: "Updated Rule", Priority: 20}
	body, err := json.Marshal(rule)
	assert.NoError(t, err)
	req := httptest.NewRequestWithContext(context.Background(), http.MethodPut, "/management/rules/rule1", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req.SetPathValue("id", "rule1")
	rec := httptest.NewRecorder()

	mockRepo := new(MockRoutingRuleRepo)
	mockVerMgr := new(MockRuleVersionManager)
	handler := NewRoutingRuleHandler(mockRepo, mockVerMgr)

	mockRepo.On("UpdateRule", mock.Anything, mock.MatchedBy(func(r *domain.RoutingRule) bool {
		return r.ID == "rule1" && r.Name == "Updated Rule" && !r.UpdatedAt.IsZero()
	})).Return(nil).Once()

	mockVerMgr.On("IncrementRulesVersion", mock.Anything).Return(int64(3), nil).Once()

	handler.HandleUpdateRoutingRule(rec, req)

	assert.Equal(t, http.StatusOK, rec.Code)
	mockVerMgr.AssertExpectations(t)
}

func TestRoutingRuleHandler_HandleDeleteRoutingRule(t *testing.T) {
	req := httptest.NewRequestWithContext(context.Background(), http.MethodDelete, "/management/rules/rule1", nil)
	req.SetPathValue("id", "rule1")
	rec := httptest.NewRecorder()

	mockRepo := new(MockRoutingRuleRepo)
	mockVerMgr := new(MockRuleVersionManager)
	handler := NewRoutingRuleHandler(mockRepo, mockVerMgr)

	mockRepo.On("DeleteRule", mock.Anything, "rule1").Return(nil).Once()

	mockVerMgr.On("IncrementRulesVersion", mock.Anything).Return(int64(4), nil).Once()

	handler.HandleDeleteRoutingRule(rec, req)

	assert.Equal(t, http.StatusNoContent, rec.Code)
	mockVerMgr.AssertExpectations(t)
}
