package handlers

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/kwilabs/straw-proxy-server/internal/domain"
	"github.com/labstack/echo/v4"
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
	e := echo.New()
	req := httptest.NewRequest(http.MethodGet, "/admin/rules", nil)
	rec := httptest.NewRecorder()
	c := e.NewContext(req, rec)

	mockRepo := new(MockRoutingRuleRepo)
	mockVerMgr := new(MockRuleVersionManager)
	handler := NewRoutingRuleHandler(mockRepo, mockVerMgr)

	rules := []domain.RoutingRule{
		{ID: "rule1", Name: "Rule 1"},
	}
	mockRepo.On("ListRules", mock.Anything, 20, 0).Return(rules, 1, nil).Once()

	err := handler.HandleListRoutingRules(c)

	assert.NoError(t, err)
	assert.Equal(t, http.StatusOK, rec.Code)
}

func TestRoutingRuleHandler_HandleGetRoutingRule(t *testing.T) {
	e := echo.New()
	req := httptest.NewRequest(http.MethodGet, "/admin/rules/rule1", nil)
	rec := httptest.NewRecorder()
	c := e.NewContext(req, rec)
	c.SetParamNames("id")
	c.SetParamValues("rule1")

	mockRepo := new(MockRoutingRuleRepo)
	mockVerMgr := new(MockRuleVersionManager)
	handler := NewRoutingRuleHandler(mockRepo, mockVerMgr)

	rule := &domain.RoutingRule{ID: "rule1", Name: "Rule 1"}
	mockRepo.On("GetRuleByID", mock.Anything, "rule1").Return(rule, nil).Once()

	err := handler.HandleGetRoutingRule(c)

	assert.NoError(t, err)
	assert.Equal(t, http.StatusOK, rec.Code)
}

func TestRoutingRuleHandler_HandleCreateRoutingRule(t *testing.T) {
	e := echo.New()
	rule := domain.RoutingRule{Name: "New Rule", Priority: 10}
	body, _ := json.Marshal(rule)
	req := httptest.NewRequest(http.MethodPost, "/admin/rules", bytes.NewReader(body))
	req.Header.Set(echo.HeaderContentType, echo.MIMEApplicationJSON)
	rec := httptest.NewRecorder()
	c := e.NewContext(req, rec)

	mockRepo := new(MockRoutingRuleRepo)
	mockVerMgr := new(MockRuleVersionManager)
	handler := NewRoutingRuleHandler(mockRepo, mockVerMgr)

	mockRepo.On("CreateRule", mock.Anything, mock.MatchedBy(func(r *domain.RoutingRule) bool {
		return r.Name == "New Rule" && r.Priority == 10
	})).Return(nil).Once()

	mockVerMgr.On("IncrementRulesVersion", mock.Anything).Return(int64(2), nil).Once()

	err := handler.HandleCreateRoutingRule(c)

	assert.NoError(t, err)
	assert.Equal(t, http.StatusCreated, rec.Code)
	mockVerMgr.AssertExpectations(t)
}

func TestRoutingRuleHandler_HandleUpdateRoutingRule(t *testing.T) {
	e := echo.New()
	rule := domain.RoutingRule{ID: "rule1", Name: "Updated Rule", Priority: 20}
	body, _ := json.Marshal(rule)
	req := httptest.NewRequest(http.MethodPut, "/admin/rules/rule1", bytes.NewReader(body))
	req.Header.Set(echo.HeaderContentType, echo.MIMEApplicationJSON)
	rec := httptest.NewRecorder()
	c := e.NewContext(req, rec)
	c.SetParamNames("id")
	c.SetParamValues("rule1")

	mockRepo := new(MockRoutingRuleRepo)
	mockVerMgr := new(MockRuleVersionManager)
	handler := NewRoutingRuleHandler(mockRepo, mockVerMgr)

	mockRepo.On("UpdateRule", mock.Anything, mock.MatchedBy(func(r *domain.RoutingRule) bool {
		return r.ID == "rule1" && r.Name == "Updated Rule" && !r.UpdatedAt.IsZero()
	})).Return(nil).Once()

	mockVerMgr.On("IncrementRulesVersion", mock.Anything).Return(int64(3), nil).Once()

	err := handler.HandleUpdateRoutingRule(c)

	assert.NoError(t, err)
	assert.Equal(t, http.StatusOK, rec.Code)
	mockVerMgr.AssertExpectations(t)
}

func TestRoutingRuleHandler_HandleDeleteRoutingRule(t *testing.T) {
	e := echo.New()
	req := httptest.NewRequest(http.MethodDelete, "/admin/rules/rule1", nil)
	rec := httptest.NewRecorder()
	c := e.NewContext(req, rec)
	c.SetParamNames("id")
	c.SetParamValues("rule1")

	mockRepo := new(MockRoutingRuleRepo)
	mockVerMgr := new(MockRuleVersionManager)
	handler := NewRoutingRuleHandler(mockRepo, mockVerMgr)

	mockRepo.On("DeleteRule", mock.Anything, "rule1").Return(nil).Once()

	mockVerMgr.On("IncrementRulesVersion", mock.Anything).Return(int64(4), nil).Once()

	err := handler.HandleDeleteRoutingRule(c)

	assert.NoError(t, err)
	assert.Equal(t, http.StatusNoContent, rec.Code)
	mockVerMgr.AssertExpectations(t)
}
