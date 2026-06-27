package handlers

import (
	"net/http"
	"time"

	"github.com/beremaran/straw/internal/domain"
	"github.com/beremaran/straw/internal/server/dto"
	"github.com/beremaran/straw/internal/server/helper"
)

type UsageHandler struct {
	repo domain.UsageRepository
}

func NewUsageHandler(repo domain.UsageRepository) *UsageHandler {
	return &UsageHandler{repo: repo}
}

func (h *UsageHandler) HandleGetUsageSummary(w http.ResponseWriter, r *http.Request) {
	var start, end time.Time
	var err error

	layout := "2006-01-02"
	now := time.Now()

	if startStr := r.URL.Query().Get("start"); startStr != "" {
		start, err = time.Parse(layout, startStr)
		if err != nil {
			helper.WriteError(w, http.StatusBadRequest, "invalid start date format (YYYY-MM-DD)")
			return
		}
	} else {

		start = now.AddDate(0, 0, -30)
	}

	if endStr := r.URL.Query().Get("end"); endStr != "" {
		end, err = time.Parse(layout, endStr)
		if err != nil {
			helper.WriteError(w, http.StatusBadRequest, "invalid end date format (YYYY-MM-DD)")
			return
		}
	} else {
		end = now
	}

	apiKeyID := r.URL.Query().Get("api_key_id")

	summaries, err := h.repo.GetDailySummaries(r.Context(), apiKeyID, start, end)
	if err != nil {
		helper.WriteError(w, http.StatusInternalServerError, "failed to fetch usage summaries")
		return
	}

	helper.WriteJSON(w, http.StatusOK, dto.UsageSummaryResponse{
		Data:  dto.FromUsageSummaries(summaries),
		Start: start.Format(layout),
		End:   end.Format(layout),
	})
}

func (h *UsageHandler) HandleGetBillingEstimate(w http.ResponseWriter, r *http.Request) {
	var start, end time.Time
	var err error

	layout := "2006-01-02"
	now := time.Now()

	if startStr := r.URL.Query().Get("start"); startStr != "" {
		start, err = time.Parse(layout, startStr)
		if err != nil {
			helper.WriteError(w, http.StatusBadRequest, "invalid start date format (YYYY-MM-DD)")
			return
		}
	} else {

		y, m, _ := now.Date()
		start = time.Date(y, m, 1, 0, 0, 0, 0, time.UTC)
	}

	if endStr := r.URL.Query().Get("end"); endStr != "" {
		end, err = time.Parse(layout, endStr)
		if err != nil {
			helper.WriteError(w, http.StatusBadRequest, "invalid end date format (YYYY-MM-DD)")
			return
		}
	} else {
		end = now
	}

	apiKeyID := r.URL.Query().Get("api_key_id")

	summaries, err := h.repo.GetDailySummaries(r.Context(), apiKeyID, start, end)
	if err != nil {
		helper.WriteError(w, http.StatusInternalServerError, "failed to fetch usage for billing")
		return
	}

	totalCost := 0.0
	for _, s := range summaries {
		totalCost += s.CostUnits
	}

	estimatedUSD := totalCost * 0.0001

	helper.WriteJSON(w, http.StatusOK, dto.BillingEstimateResponse{
		TotalCostUnits: totalCost,
		EstimatedUSD:   estimatedUSD,
		Currency:       "USD",
		Start:          start.Format(layout),
		End:            end.Format(layout),
	})
}
