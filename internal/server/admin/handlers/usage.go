package handlers

import (
	"net/http"
	"time"

	"github.com/kwilabs/straw-proxy-server/internal/domain"
	"github.com/labstack/echo/v4"
)

type UsageHandler struct {
	repo domain.UsageRepository
}

func NewUsageHandler(repo domain.UsageRepository) *UsageHandler {
	return &UsageHandler{repo: repo}
}

// HandleGetUsageSummary returns usage summaries for the given time range.
// Query Params:
// - start: YYYY-MM-DD
// - end: YYYY-MM-DD
// - api_key_id: Optional string
func (h *UsageHandler) HandleGetUsageSummary(c echo.Context) error {
	var start, end time.Time
	var err error

	layout := "2006-01-02"
	now := time.Now()

	if startStr := c.QueryParam("start"); startStr != "" {
		start, err = time.Parse(layout, startStr)
		if err != nil {
			return c.JSON(http.StatusBadRequest, map[string]string{"error": "invalid start date format (YYYY-MM-DD)"})
		}
	} else {
		// Default to last 30 days
		start = now.AddDate(0, 0, -30)
	}

	if endStr := c.QueryParam("end"); endStr != "" {
		end, err = time.Parse(layout, endStr)
		if err != nil {
			return c.JSON(http.StatusBadRequest, map[string]string{"error": "invalid end date format (YYYY-MM-DD)"})
		}
	} else {
		end = now
	}

	apiKeyID := c.QueryParam("api_key_id")

	summaries, err := h.repo.GetDailySummaries(c.Request().Context(), apiKeyID, start, end)
	if err != nil {
		return c.JSON(http.StatusInternalServerError, map[string]string{"error": "failed to fetch usage summaries"})
	}

	return c.JSON(http.StatusOK, map[string]interface{}{
		"data":  summaries,
		"start": start.Format(layout),
		"end":   end.Format(layout),
	})
}

// HandleGetBillingEstimate returns a simple cost estimate.
// This is a placeholder logic for now.
func (h *UsageHandler) HandleGetBillingEstimate(c echo.Context) error {
	// Re-use fetching logic
	// In a real system, we might have a dedicated billing service.
	// Here we just sum up the CostUnits from the usage summary.

	var start, end time.Time
	var err error

	layout := "2006-01-02"
	now := time.Now()

	if startStr := c.QueryParam("start"); startStr != "" {
		start, err = time.Parse(layout, startStr)
		if err != nil {
			return c.JSON(http.StatusBadRequest, map[string]string{"error": "invalid start date format (YYYY-MM-DD)"})
		}
	} else {
		// Default currently (start of month)
		y, m, _ := now.Date()
		start = time.Date(y, m, 1, 0, 0, 0, 0, time.UTC)
	}

	if endStr := c.QueryParam("end"); endStr != "" {
		end, err = time.Parse(layout, endStr)
		if err != nil {
			return c.JSON(http.StatusBadRequest, map[string]string{"error": "invalid end date format (YYYY-MM-DD)"})
		}
	} else {
		end = now
	}

	apiKeyID := c.QueryParam("api_key_id")

	summaries, err := h.repo.GetDailySummaries(c.Request().Context(), apiKeyID, start, end)
	if err != nil {
		return c.JSON(http.StatusInternalServerError, map[string]string{"error": "failed to fetch usage for billing"})
	}

	totalCost := 0.0
	for _, s := range summaries {
		totalCost += s.CostUnits
	}

	// Assuming 1 Cost Unit = $0.0001 (example rate)
	// This should be configurable.
	estimatedUSD := totalCost * 0.0001

	return c.JSON(http.StatusOK, map[string]interface{}{
		"total_cost_units": totalCost,
		"estimated_usd":    estimatedUSD,
		"currency":         "USD",
		"start":            start.Format(layout),
		"end":              end.Format(layout),
	})
}
