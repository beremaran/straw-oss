package handlers

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"

	"github.com/beremaran/straw/internal/domain"
	"github.com/beremaran/straw/internal/server/dto"
)

const (
	usageTestDate = "2023-01-01"
	usageTestURL  = "/management/billing/estimate?start=2023-01-01&end=2023-01-02"
)

type MockUsageRepo struct {
	mock.Mock
}

func (m *MockUsageRepo) GetDailySummaries(ctx context.Context, apiKeyID string, start, end time.Time) ([]domain.UsageSummary, error) {
	args := m.Called(ctx, apiKeyID, start, end)

	return args.Get(0).([]domain.UsageSummary), args.Error(1)
}

func TestUsageHandler_HandleGetUsageSummary(t *testing.T) {
	mockRepo := new(MockUsageRepo)
	h := NewUsageHandler(mockRepo)

	t.Run("success", func(t *testing.T) {
		req := httptest.NewRequestWithContext(context.Background(), http.MethodGet, "/management/usage/summary?start=2023-01-01&end=2023-01-02", nil)
		rec := httptest.NewRecorder()

		summaries := []domain.UsageSummary{
			{Date: usageTestDate, TotalRequests: 100},
		}

		mockRepo.On("GetDailySummaries", mock.Anything, "", mock.AnythingOfType("time.Time"), mock.AnythingOfType("time.Time")).Return(summaries, nil)

		h.HandleGetUsageSummary(rec, req)
		assert.Equal(t, http.StatusOK, rec.Code)

		var resp map[string]any
		_ = json.Unmarshal(rec.Body.Bytes(), &resp)
		assert.NotEmpty(t, resp["data"])
	})
}

func TestUsageHandler_HandleGetBillingEstimateUsesExistingCostUnitsWithoutMultipliers(t *testing.T) {
	mockRepo := new(MockUsageRepo)
	h := NewUsageHandler(mockRepo)

	req := httptest.NewRequestWithContext(context.Background(), http.MethodGet, usageTestURL, nil)
	rec := httptest.NewRecorder()

	summaries := []domain.UsageSummary{
		{Date: usageTestDate, CostUnits: 100, Breakdown: map[string]int64{testCostMultiplierTag: 10}},
	}

	mockRepo.On("GetDailySummaries", mock.Anything, "", mock.AnythingOfType("time.Time"), mock.AnythingOfType("time.Time")).Return(summaries, nil).Once()

	h.HandleGetBillingEstimate(rec, req)

	assert.Equal(t, http.StatusOK, rec.Code)

	var resp dtoBillingEstimate
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &resp))
	assert.InEpsilon(t, 100, resp.TotalCostUnits, 0.0001)
	assert.Equal(t, basePricingVersion, resp.PricingVersion)
	mockRepo.AssertExpectations(t)
}

func TestUsageHandler_HandleGetBillingEstimateAppliesActiveMultiplier(t *testing.T) {
	mockRepo := new(MockUsageRepo)
	costRepo := new(MockCostMultiplierRepo)
	h := NewUsageHandler(mockRepo, costRepo)

	req := httptest.NewRequestWithContext(context.Background(), http.MethodGet, usageTestURL, nil)
	rec := httptest.NewRecorder()

	summaries := []domain.UsageSummary{
		{Date: usageTestDate, CostUnits: 100, Breakdown: map[string]int64{testCostMultiplierTag: 10}},
	}
	multipliers := []domain.CostMultiplier{
		{EndpointTag: testCostMultiplierTag, Multiplier: 2, Version: 3, IsActive: true},
	}

	mockRepo.On("GetDailySummaries", mock.Anything, "", mock.AnythingOfType("time.Time"), mock.AnythingOfType("time.Time")).Return(summaries, nil).Once()
	costRepo.On("ListActive", mock.Anything).Return(multipliers, nil).Once()

	h.HandleGetBillingEstimate(rec, req)

	assert.Equal(t, http.StatusOK, rec.Code)

	var resp dtoBillingEstimate
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &resp))
	assert.InEpsilon(t, 200, resp.TotalCostUnits, 0.0001)
	assert.Equal(t, "cost-multipliers:type:residential@3", resp.PricingVersion)
	assert.Len(t, resp.Multipliers, 1)
	mockRepo.AssertExpectations(t)
	costRepo.AssertExpectations(t)
}

func TestUsageHandler_HandleGetBillingEstimateIgnoresInactiveMultipliers(t *testing.T) {
	mockRepo := new(MockUsageRepo)
	costRepo := new(MockCostMultiplierRepo)
	h := NewUsageHandler(mockRepo, costRepo)

	req := httptest.NewRequestWithContext(context.Background(), http.MethodGet, usageTestURL, nil)
	rec := httptest.NewRecorder()

	summaries := []domain.UsageSummary{
		{Date: usageTestDate, CostUnits: 100, Breakdown: map[string]int64{testCostMultiplierTag: 10}},
	}

	mockRepo.On("GetDailySummaries", mock.Anything, "", mock.AnythingOfType("time.Time"), mock.AnythingOfType("time.Time")).Return(summaries, nil).Once()
	costRepo.On("ListActive", mock.Anything).Return([]domain.CostMultiplier{}, nil).Once()

	h.HandleGetBillingEstimate(rec, req)

	assert.Equal(t, http.StatusOK, rec.Code)

	var resp dtoBillingEstimate
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &resp))
	assert.InEpsilon(t, 100, resp.TotalCostUnits, 0.0001)
	assert.Empty(t, resp.Multipliers)
	mockRepo.AssertExpectations(t)
	costRepo.AssertExpectations(t)
}

type dtoBillingEstimate struct {
	TotalCostUnits float64                    `json:"total_cost_units"`
	PricingVersion string                     `json:"pricing_version"`
	Multipliers    []dto.BillingMultiplierDTO `json:"multipliers"`
}
