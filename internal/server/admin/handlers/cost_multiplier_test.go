package handlers

import (
	"bytes"
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"

	"github.com/beremaran/straw/internal/domain"
)

const (
	testCostMultiplierID  = "multiplier-1"
	testCostMultiplierTag = "type:residential"
)

type MockCostMultiplierRepo struct {
	mock.Mock
}

func (m *MockCostMultiplierRepo) List(ctx context.Context, limit, offset int) ([]domain.CostMultiplier, int, error) {
	args := m.Called(ctx, limit, offset)
	if args.Get(0) == nil {
		return nil, 0, args.Error(2)
	}

	return args.Get(0).([]domain.CostMultiplier), args.Int(1), args.Error(2)
}

func (m *MockCostMultiplierRepo) ListActive(ctx context.Context) ([]domain.CostMultiplier, error) {
	args := m.Called(ctx)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}

	return args.Get(0).([]domain.CostMultiplier), args.Error(1)
}

func (m *MockCostMultiplierRepo) GetByID(ctx context.Context, id string) (*domain.CostMultiplier, error) {
	args := m.Called(ctx, id)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}

	return args.Get(0).(*domain.CostMultiplier), args.Error(1)
}

func (m *MockCostMultiplierRepo) Create(ctx context.Context, multiplier *domain.CostMultiplier) error {
	args := m.Called(ctx, multiplier)

	return args.Error(0)
}

func (m *MockCostMultiplierRepo) Update(ctx context.Context, multiplier *domain.CostMultiplier) error {
	args := m.Called(ctx, multiplier)

	return args.Error(0)
}

func (m *MockCostMultiplierRepo) Deactivate(ctx context.Context, id string) (*domain.CostMultiplier, error) {
	args := m.Called(ctx, id)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}

	return args.Get(0).(*domain.CostMultiplier), args.Error(1)
}

func TestCostMultiplierHandler_HandleCreateCostMultiplier(t *testing.T) {
	repo := new(MockCostMultiplierRepo)
	auditRepo := new(MockManagementAuditRepo)
	handler := NewCostMultiplierHandler(repo, auditRepo)

	req := httptest.NewRequestWithContext(context.Background(), http.MethodPost, "/management/cost-multipliers", bytes.NewBufferString(`{
		"endpoint_tag": "type=residential",
		"multiplier": 10,
		"description": "Residential"
	}`))
	rec := httptest.NewRecorder()

	repo.On("Create", mock.Anything, mock.MatchedBy(func(multiplier *domain.CostMultiplier) bool {
		return multiplier.EndpointTag == testCostMultiplierTag && multiplier.Multiplier == 10 && multiplier.IsActive && multiplier.Version == 1
	})).Return(nil).Once()
	auditRepo.On("Create", mock.Anything, mock.MatchedBy(func(event *domain.ManagementAuditEvent) bool {
		return event.Action == domain.ActionCreate && event.EntityType == "cost_multiplier"
	})).Return(nil).Once()

	handler.HandleCreateCostMultiplier(rec, req)

	assert.Equal(t, http.StatusCreated, rec.Code)
	repo.AssertExpectations(t)
	auditRepo.AssertExpectations(t)
}

func TestCostMultiplierHandler_HandleCreateCostMultiplierRejectsDuplicateTag(t *testing.T) {
	repo := new(MockCostMultiplierRepo)
	handler := NewCostMultiplierHandler(repo, nil)

	req := httptest.NewRequestWithContext(context.Background(), http.MethodPost, "/management/cost-multipliers", bytes.NewBufferString(`{
		"endpoint_tag": "type:residential",
		"multiplier": 10
	}`))
	rec := httptest.NewRecorder()

	repo.On("Create", mock.Anything, mock.Anything).Return(errors.New("duplicate key value violates unique constraint")).Once()

	handler.HandleCreateCostMultiplier(rec, req)

	assert.Equal(t, http.StatusConflict, rec.Code)
	repo.AssertExpectations(t)
}

func TestCostMultiplierHandler_HandleUpdateCostMultiplierRequiresCurrentVersion(t *testing.T) {
	repo := new(MockCostMultiplierRepo)
	handler := NewCostMultiplierHandler(repo, nil)

	now := time.Now()
	existing := &domain.CostMultiplier{
		ID:          testCostMultiplierID,
		EndpointTag: testCostMultiplierTag,
		Multiplier:  10,
		IsActive:    true,
		Version:     2,
		CreatedAt:   now,
		UpdatedAt:   now,
	}

	req := httptest.NewRequestWithContext(context.Background(), http.MethodPut, "/management/cost-multipliers/multiplier-1", bytes.NewBufferString(`{
		"endpoint_tag": "type:residential",
		"multiplier": 11,
		"version": 1
	}`))
	req.SetPathValue("id", testCostMultiplierID)
	rec := httptest.NewRecorder()

	repo.On("GetByID", mock.Anything, testCostMultiplierID).Return(existing, nil).Once()
	repo.On("Update", mock.Anything, mock.MatchedBy(func(multiplier *domain.CostMultiplier) bool {
		return multiplier.Version == 1 && multiplier.Multiplier == 11
	})).Return(domain.ErrCostMultiplierVersionConflict).Once()

	handler.HandleUpdateCostMultiplier(rec, req)

	assert.Equal(t, http.StatusConflict, rec.Code)
	repo.AssertExpectations(t)
}

func TestCostMultiplierHandler_HandleDeleteCostMultiplierSoftDeactivatesAndAudits(t *testing.T) {
	repo := new(MockCostMultiplierRepo)
	auditRepo := new(MockManagementAuditRepo)
	handler := NewCostMultiplierHandler(repo, auditRepo)

	now := time.Now()
	oldMultiplier := &domain.CostMultiplier{
		ID:          testCostMultiplierID,
		EndpointTag: testCostMultiplierTag,
		Multiplier:  10,
		IsActive:    true,
		Version:     1,
		CreatedAt:   now,
		UpdatedAt:   now,
	}
	deactivated := *oldMultiplier
	deactivated.IsActive = false
	deactivated.Version = 2

	req := httptest.NewRequestWithContext(context.Background(), http.MethodDelete, "/management/cost-multipliers/multiplier-1", nil)
	req.SetPathValue("id", testCostMultiplierID)
	rec := httptest.NewRecorder()

	repo.On("GetByID", mock.Anything, testCostMultiplierID).Return(oldMultiplier, nil).Once()
	repo.On("Deactivate", mock.Anything, testCostMultiplierID).Return(&deactivated, nil).Once()
	auditRepo.On("Create", mock.Anything, mock.MatchedBy(func(event *domain.ManagementAuditEvent) bool {
		return event.Action == domain.ActionDelete && event.EntityID == testCostMultiplierID
	})).Return(nil).Once()

	handler.HandleDeleteCostMultiplier(rec, req)

	require.Equal(t, http.StatusNoContent, rec.Code)
	repo.AssertExpectations(t)
	auditRepo.AssertExpectations(t)
}
