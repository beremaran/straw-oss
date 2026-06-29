package handlers

import (
	"errors"
	"fmt"
	"net/http"
	"strconv"
	"time"

	"github.com/google/uuid"

	"github.com/beremaran/straw/internal/domain"
	"github.com/beremaran/straw/internal/server/admin/middleware"
	"github.com/beremaran/straw/internal/server/dto"
	"github.com/beremaran/straw/internal/server/helper"
)

// CostMultiplierHandler manages cost multiplier configuration.
type CostMultiplierHandler struct {
	repo      domain.CostMultiplierRepository
	auditRepo domain.ManagementAuditRepository
}

// NewCostMultiplierHandler creates a new CostMultiplierHandler.
func NewCostMultiplierHandler(repo domain.CostMultiplierRepository, auditRepo domain.ManagementAuditRepository) *CostMultiplierHandler {
	return &CostMultiplierHandler{repo: repo, auditRepo: auditRepo}
}

// HandleListCostMultipliers lists cost multipliers.
func (h *CostMultiplierHandler) HandleListCostMultipliers(w http.ResponseWriter, r *http.Request) {
	page, _ := strconv.Atoi(r.URL.Query().Get("page"))
	if page < 1 {
		page = 1
	}

	limit, _ := strconv.Atoi(r.URL.Query().Get("limit"))
	if limit < 1 || limit > 100 {
		limit = 20
	}

	offset := (page - 1) * limit

	multipliers, total, err := h.repo.List(r.Context(), limit, offset)
	if err != nil {
		helper.WriteError(w, http.StatusInternalServerError, "failed to list cost multipliers")

		return
	}

	helper.WriteJSON(w, http.StatusOK, dto.ListCostMultipliersResponse{
		Data:  dto.FromCostMultipliers(multipliers),
		Total: total,
		Page:  page,
		Limit: limit,
	})
}

// HandleGetCostMultiplier returns a cost multiplier.
func (h *CostMultiplierHandler) HandleGetCostMultiplier(w http.ResponseWriter, r *http.Request) {
	multiplier := h.loadCostMultiplier(w, r)
	if multiplier == nil {
		return
	}

	helper.WriteJSON(w, http.StatusOK, dto.FromCostMultiplier(multiplier))
}

// HandleCreateCostMultiplier creates a cost multiplier.
func (h *CostMultiplierHandler) HandleCreateCostMultiplier(w http.ResponseWriter, r *http.Request) {
	var req dto.CreateCostMultiplierRequest

	err := helper.ReadJSON(r, &req)
	if err != nil {
		helper.WriteError(w, http.StatusBadRequest, "invalid request body")

		return
	}

	multiplier, err := costMultiplierFromCreate(req)
	if err != nil {
		helper.WriteError(w, http.StatusBadRequest, err.Error())

		return
	}

	err = h.repo.Create(r.Context(), multiplier)
	if err != nil {
		writeConflictOrServerError(w, err, "endpoint_tag already exists", "failed to create cost multiplier")

		return
	}

	resp := dto.FromCostMultiplier(multiplier)
	h.audit(r, domain.ActionCreate, multiplier.ID, nil, resp)

	helper.WriteJSON(w, http.StatusCreated, resp)
}

// HandleUpdateCostMultiplier updates a cost multiplier.
func (h *CostMultiplierHandler) HandleUpdateCostMultiplier(w http.ResponseWriter, r *http.Request) {
	oldMultiplier := h.loadCostMultiplier(w, r)
	if oldMultiplier == nil {
		return
	}

	var req dto.UpdateCostMultiplierRequest

	err := helper.ReadJSON(r, &req)
	if err != nil {
		helper.WriteError(w, http.StatusBadRequest, "invalid request body")

		return
	}

	if req.Version < 1 {
		helper.WriteError(w, http.StatusBadRequest, "version is required")

		return
	}

	multiplier := *oldMultiplier
	multiplier.EndpointTag = req.EndpointTag
	multiplier.Multiplier = req.Multiplier
	multiplier.Description = req.Description
	multiplier.Version = req.Version
	multiplier.UpdatedAt = time.Now().UTC()

	if req.IsActive != nil {
		multiplier.IsActive = *req.IsActive
	}

	err = domain.ValidateCostMultiplier(&multiplier)
	if err != nil {
		helper.WriteError(w, http.StatusBadRequest, err.Error())

		return
	}

	err = h.repo.Update(r.Context(), &multiplier)
	if err != nil {
		if errors.Is(err, domain.ErrCostMultiplierVersionConflict) {
			helper.WriteError(w, http.StatusConflict, "version conflict")

			return
		}

		writeConflictOrServerError(w, err, "endpoint_tag already exists", "failed to update cost multiplier")

		return
	}

	resp := dto.FromCostMultiplier(&multiplier)
	h.audit(r, domain.ActionUpdate, multiplier.ID, dto.FromCostMultiplier(oldMultiplier), resp)

	helper.WriteJSON(w, http.StatusOK, resp)
}

// HandleDeleteCostMultiplier soft deactivates a cost multiplier.
func (h *CostMultiplierHandler) HandleDeleteCostMultiplier(w http.ResponseWriter, r *http.Request) {
	oldMultiplier := h.loadCostMultiplier(w, r)
	if oldMultiplier == nil {
		return
	}

	multiplier, err := h.repo.Deactivate(r.Context(), oldMultiplier.ID)
	if err != nil {
		if errors.Is(err, domain.ErrCostMultiplierNotFound) {
			helper.WriteError(w, http.StatusNotFound, "cost multiplier not found")

			return
		}

		helper.WriteError(w, http.StatusInternalServerError, "failed to delete cost multiplier")

		return
	}

	h.audit(r, domain.ActionDelete, oldMultiplier.ID, dto.FromCostMultiplier(oldMultiplier), dto.FromCostMultiplier(multiplier))
	w.WriteHeader(http.StatusNoContent)
}

func (h *CostMultiplierHandler) loadCostMultiplier(w http.ResponseWriter, r *http.Request) *domain.CostMultiplier {
	id := r.PathValue("id")

	multiplier, err := h.repo.GetByID(r.Context(), id)
	if err != nil {
		helper.WriteError(w, http.StatusInternalServerError, "failed to get cost multiplier")

		return nil
	}

	if multiplier == nil {
		helper.WriteError(w, http.StatusNotFound, "cost multiplier not found")

		return nil
	}

	return multiplier
}

func costMultiplierFromCreate(req dto.CreateCostMultiplierRequest) (*domain.CostMultiplier, error) {
	active := true
	if req.IsActive != nil {
		active = *req.IsActive
	}

	now := time.Now().UTC()
	multiplier := &domain.CostMultiplier{
		ID:          uuid.New().String(),
		EndpointTag: req.EndpointTag,
		Multiplier:  req.Multiplier,
		Description: req.Description,
		IsActive:    active,
		Version:     1,
		CreatedAt:   now,
		UpdatedAt:   now,
	}

	err := domain.ValidateCostMultiplier(multiplier)
	if err != nil {
		return nil, fmt.Errorf("validate cost multiplier: %w", err)
	}

	return multiplier, nil
}

func (h *CostMultiplierHandler) audit(r *http.Request, action, id string, oldValue, newValue any) {
	if h.auditRepo == nil {
		return
	}

	event := middleware.NewAuditEvent(r, action, "cost_multiplier", id, oldValue, newValue)
	_ = h.auditRepo.Create(r.Context(), event)
}
