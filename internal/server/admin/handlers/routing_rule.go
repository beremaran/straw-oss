package handlers

import (
	"context"
	"net/http"
	"strconv"
	"time"

	"github.com/google/uuid"

	"github.com/beremaran/straw/internal/domain"
	"github.com/beremaran/straw/internal/server/admin/middleware"
	"github.com/beremaran/straw/internal/server/dto"
	"github.com/beremaran/straw/internal/server/helper"
)

// RuleVersionManager handles routing rule version management.
type RuleVersionManager interface {
	IncrementRulesVersion(ctx context.Context) (int64, error)
}

// RoutingRuleHandler manages routing rule operations.
type RoutingRuleHandler struct {
	repo       domain.RoutingRuleRepository
	versionMgr RuleVersionManager
	auditRepo  domain.ManagementAuditRepository
}

// NewRoutingRuleHandler creates a new RoutingRuleHandler.
func NewRoutingRuleHandler(repo domain.RoutingRuleRepository, versionMgr RuleVersionManager, auditRepo domain.ManagementAuditRepository) *RoutingRuleHandler {
	return &RoutingRuleHandler{
		repo:       repo,
		versionMgr: versionMgr,
		auditRepo:  auditRepo,
	}
}

// HandleListRoutingRules lists all routing rules.
func (h *RoutingRuleHandler) HandleListRoutingRules(w http.ResponseWriter, r *http.Request) {
	page, _ := strconv.Atoi(r.URL.Query().Get("page"))
	if page < 1 {
		page = 1
	}

	limit, _ := strconv.Atoi(r.URL.Query().Get("limit"))
	if limit < 1 || limit > 100 {
		limit = 20
	}

	offset := (page - 1) * limit

	rules, total, err := h.repo.ListRules(r.Context(), limit, offset)
	if err != nil {
		helper.WriteError(w, http.StatusInternalServerError, "failed to list routing rules")

		return
	}

	helper.WriteJSON(w, http.StatusOK, dto.ListRoutingRulesResponse{
		Data:  dto.FromRoutingRules(rules),
		Total: total,
		Page:  page,
		Limit: limit,
	})
}

// HandleGetRoutingRule retrieves a single routing rule.
func (h *RoutingRuleHandler) HandleGetRoutingRule(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	if id == "" {
		helper.WriteError(w, http.StatusBadRequest, "id is required")

		return
	}

	rule, err := h.repo.GetRuleByID(r.Context(), id)
	if err != nil {
		helper.WriteError(w, http.StatusInternalServerError, "failed to get routing rule")

		return
	}

	if rule == nil {
		helper.WriteError(w, http.StatusNotFound, "routing rule not found")

		return
	}

	helper.WriteJSON(w, http.StatusOK, dto.FromRoutingRule(rule))
}

// HandleCreateRoutingRule creates a new routing rule.
func (h *RoutingRuleHandler) HandleCreateRoutingRule(w http.ResponseWriter, r *http.Request) {
	var req dto.CreateRoutingRuleRequest

	err := helper.ReadJSON(r, &req)
	if err != nil {
		helper.WriteError(w, http.StatusBadRequest, "invalid request body")

		return
	}

	if req.Name == "" {
		helper.WriteError(w, http.StatusBadRequest, "name is required")

		return
	}

	rule, err := req.ToDomain()
	if err != nil {
		helper.WriteError(w, http.StatusBadRequest, err.Error())

		return
	}

	rule.ID = uuid.New().String()
	rule.CreatedAt = time.Now()
	rule.UpdatedAt = time.Now()
	rule.Version = 1

	err = h.repo.CreateRule(r.Context(), rule)
	if err != nil {
		helper.WriteError(w, http.StatusInternalServerError, "failed to create routing rule")

		return
	}

	_, _ = h.versionMgr.IncrementRulesVersion(r.Context())

	ruleResp := dto.FromRoutingRule(rule)
	if h.auditRepo != nil {
		event := middleware.NewAuditEvent(r, domain.ActionCreate, "routing_rule", rule.ID, nil, ruleResp)
		_ = h.auditRepo.Create(r.Context(), event)
	}

	helper.WriteJSON(w, http.StatusCreated, ruleResp)
}

// HandleUpdateRoutingRule updates a routing rule.
func (h *RoutingRuleHandler) HandleUpdateRoutingRule(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	if id == "" {
		helper.WriteError(w, http.StatusBadRequest, "id is required")

		return
	}

	var req dto.UpdateRoutingRuleRequest

	err := helper.ReadJSON(r, &req)
	if err != nil {
		helper.WriteError(w, http.StatusBadRequest, "invalid request body")

		return
	}

	rule, err := req.ToDomain()
	if err != nil {
		helper.WriteError(w, http.StatusBadRequest, err.Error())

		return
	}

	rule.ID = id
	rule.UpdatedAt = time.Now()
	rule.Version = req.Version

	oldRule, _ := h.repo.GetRuleByID(r.Context(), id)

	err = h.repo.UpdateRule(r.Context(), rule)
	if err != nil {
		helper.WriteError(w, http.StatusInternalServerError, err.Error())

		return
	}

	_, _ = h.versionMgr.IncrementRulesVersion(r.Context())

	ruleResp := dto.FromRoutingRule(rule)

	if h.auditRepo != nil {
		var oldVal any
		if oldRule != nil {
			oldVal = dto.FromRoutingRule(oldRule)
		}

		event := middleware.NewAuditEvent(r, domain.ActionUpdate, "routing_rule", rule.ID, oldVal, ruleResp)
		_ = h.auditRepo.Create(r.Context(), event)
	}

	helper.WriteJSON(w, http.StatusOK, ruleResp)
}

// HandleDeleteRoutingRule deletes a routing rule.
func (h *RoutingRuleHandler) HandleDeleteRoutingRule(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	if id == "" {
		helper.WriteError(w, http.StatusBadRequest, "id is required")

		return
	}

	oldRule, _ := h.repo.GetRuleByID(r.Context(), id)

	err := h.repo.DeleteRule(r.Context(), id)
	if err != nil {
		helper.WriteError(w, http.StatusInternalServerError, "failed to delete routing rule")

		return
	}

	_, _ = h.versionMgr.IncrementRulesVersion(r.Context())

	if h.auditRepo != nil {
		var oldVal any
		if oldRule != nil {
			oldVal = dto.FromRoutingRule(oldRule)
		}

		event := middleware.NewAuditEvent(r, domain.ActionDelete, "routing_rule", id, oldVal, nil)
		_ = h.auditRepo.Create(r.Context(), event)
	}

	w.WriteHeader(http.StatusNoContent)
}
