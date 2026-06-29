package handlers

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/beremaran/straw/internal/domain"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
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
			{Date: "2023-01-01", TotalRequests: 100},
		}

		mockRepo.On("GetDailySummaries", mock.Anything, "", mock.AnythingOfType("time.Time"), mock.AnythingOfType("time.Time")).Return(summaries, nil)

		h.HandleGetUsageSummary(rec, req)
		assert.Equal(t, http.StatusOK, rec.Code)

		var resp map[string]interface{}
		_ = json.Unmarshal(rec.Body.Bytes(), &resp)
		assert.NotEmpty(t, resp["data"])
	})
}
