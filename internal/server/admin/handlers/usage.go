package handlers

import (
	"fmt"
	"net/http"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/beremaran/straw/internal/domain"
	"github.com/beremaran/straw/internal/server/dto"
	"github.com/beremaran/straw/internal/server/helper"
)

const (
	costPerUnitUSD     = 0.0001
	basePricingVersion = "base"
)

// UsageHandler manages usage and billing operations.
type UsageHandler struct {
	repo               domain.UsageRepository
	costMultiplierRepo domain.CostMultiplierRepository
}

// NewUsageHandler creates a new UsageHandler.
func NewUsageHandler(repo domain.UsageRepository, costMultiplierRepo ...domain.CostMultiplierRepository) *UsageHandler {
	handler := &UsageHandler{repo: repo}
	if len(costMultiplierRepo) > 0 {
		handler.costMultiplierRepo = costMultiplierRepo[0]
	}

	return handler
}

// HandleGetUsageSummary returns usage summary data.
func (h *UsageHandler) HandleGetUsageSummary(w http.ResponseWriter, r *http.Request) {
	var (
		start, end time.Time
		err        error
	)

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

// HandleGetBillingEstimate returns a billing cost estimate.
func (h *UsageHandler) HandleGetBillingEstimate(w http.ResponseWriter, r *http.Request) {
	var (
		start, end time.Time
		err        error
	)

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

	multipliers, err := h.activeMultipliers(r)
	if err != nil {
		helper.WriteError(w, http.StatusInternalServerError, "failed to fetch cost multipliers")

		return
	}

	totalCost := billingCostUnits(summaries, multipliers)
	estimatedUSD := totalCost * costPerUnitUSD

	helper.WriteJSON(w, http.StatusOK, dto.BillingEstimateResponse{
		TotalCostUnits: totalCost,
		EstimatedUSD:   estimatedUSD,
		Currency:       "USD",
		Start:          start.Format(layout),
		End:            end.Format(layout),
		PricingVersion: pricingVersion(multipliers),
		Multipliers:    dto.FromBillingMultipliers(multipliers),
	})
}

func (h *UsageHandler) activeMultipliers(r *http.Request) ([]domain.CostMultiplier, error) {
	if h.costMultiplierRepo == nil {
		return nil, nil
	}

	multipliers, err := h.costMultiplierRepo.ListActive(r.Context())
	if err != nil {
		return nil, fmt.Errorf("list active cost multipliers: %w", err)
	}

	sort.Slice(multipliers, func(i, j int) bool {
		return multipliers[i].EndpointTag < multipliers[j].EndpointTag
	})

	return multipliers, nil
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
		if multiplier, ok := matchMultiplier(key, multipliers); ok {
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

func pricingVersion(multipliers []domain.CostMultiplier) string {
	if len(multipliers) == 0 {
		return basePricingVersion
	}

	parts := make([]string, len(multipliers))
	for i, multiplier := range multipliers {
		parts[i] = multiplier.EndpointTag + "@" + strconv.Itoa(multiplier.Version)
	}

	return "cost-multipliers:" + strings.Join(parts, ",")
}
