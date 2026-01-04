package handlers

import (
	"context"
	"net/http"
	"strconv"
	"time"

	"github.com/google/uuid"
	"github.com/kwilabs/straw-proxy-server/internal/domain"
	"github.com/kwilabs/straw-proxy-server/internal/server/dto"
	"github.com/labstack/echo/v4"
)

type RuleVersionManager interface {
	IncrementRulesVersion(ctx context.Context) (int64, error)
}

type RoutingRuleHandler struct {
	repo       domain.RoutingRuleRepository
	versionMgr RuleVersionManager
}

func NewRoutingRuleHandler(repo domain.RoutingRuleRepository, versionMgr RuleVersionManager) *RoutingRuleHandler {
	return &RoutingRuleHandler{
		repo:       repo,
		versionMgr: versionMgr,
	}
}

// HandleListRoutingRules lists all routing rules
//
//	@Summary		List Routing Rules
//	@Description	Returns paginated list of all routing rules
//	@Tags			routing-rules
//	@Accept			json
//	@Produce		json
//	@Param			page	query		int	false	"Page number (default: 1)"
//	@Param			limit	query		int	false	"Items per page (default: 20, max: 100)"
//	@Success		200		{object}	dto.ListRoutingRulesResponse	"Paginated list of routing rules"
//	@Failure		500		{object}	map[string]string		"Internal server error"
//	@Security		AdminKeyAuth
//	@Router			/rules [get]
func (h *RoutingRuleHandler) HandleListRoutingRules(c echo.Context) error {
	page, _ := strconv.Atoi(c.QueryParam("page"))
	if page < 1 {
		page = 1
	}
	limit, _ := strconv.Atoi(c.QueryParam("limit"))
	if limit < 1 || limit > 100 {
		limit = 20
	}
	offset := (page - 1) * limit

	rules, total, err := h.repo.ListRules(c.Request().Context(), limit, offset)
	if err != nil {
		return c.JSON(http.StatusInternalServerError, map[string]string{"error": "failed to list routing rules"})
	}

	return c.JSON(http.StatusOK, dto.ListRoutingRulesResponse{
		Data:  dto.FromRoutingRules(rules),
		Total: total,
		Page:  page,
		Limit: limit,
	})
}

// HandleGetRoutingRule gets a specific routing rule
//
//	@Summary		Get Routing Rule
//	@Description	Returns a specific routing rule by ID
//	@Tags			routing-rules
//	@Produce		json
//	@Param			id	path		string	true	"Routing Rule ID"
//	@Success		200	{object}	dto.RoutingRuleResponse	"Routing rule"
//	@Failure		400	{object}	map[string]string	"ID required"
//	@Failure		404	{object}	map[string]string	"Rule not found"
//	@Failure		500	{object}	map[string]string	"Internal server error"
//	@Security		AdminKeyAuth
//	@Router			/rules/{id} [get]
func (h *RoutingRuleHandler) HandleGetRoutingRule(c echo.Context) error {
	id := c.Param("id")
	if id == "" {
		return c.JSON(http.StatusBadRequest, map[string]string{"error": "id is required"})
	}

	rule, err := h.repo.GetRuleByID(c.Request().Context(), id)
	if err != nil {
		return c.JSON(http.StatusInternalServerError, map[string]string{"error": "failed to get routing rule"})
	}
	if rule == nil {
		return c.JSON(http.StatusNotFound, map[string]string{"error": "routing rule not found"})
	}

	return c.JSON(http.StatusOK, dto.FromRoutingRule(rule))
}

// HandleCreateRoutingRule creates a new routing rule
//
//	@Summary		Create Routing Rule
//	@Description	Creates a new routing rule for request matching
//	@Tags			routing-rules
//	@Accept			json
//	@Produce		json
//	@Param			rule	body		dto.CreateRoutingRuleRequest	true	"Routing rule to create"
//	@Success		201		{object}	dto.RoutingRuleResponse	"Created routing rule"
//	@Failure		400		{object}	map[string]string	"Invalid request"
//	@Failure		500		{object}	map[string]string	"Internal server error"
//	@Security		AdminKeyAuth
//	@Router			/rules [post]
func (h *RoutingRuleHandler) HandleCreateRoutingRule(c echo.Context) error {
	var req dto.CreateRoutingRuleRequest
	if err := c.Bind(&req); err != nil {
		return c.JSON(http.StatusBadRequest, map[string]string{"error": "invalid request body"})
	}

	if req.Name == "" {
		return c.JSON(http.StatusBadRequest, map[string]string{"error": "name is required"})
	}

	rule, err := req.ToDomain()
	if err != nil {
		return c.JSON(http.StatusBadRequest, map[string]string{"error": err.Error()})
	}

	rule.ID = uuid.New().String()
	rule.CreatedAt = time.Now()
	rule.UpdatedAt = time.Now()
	rule.Version = 1

	if err := h.repo.CreateRule(c.Request().Context(), rule); err != nil {
		return c.JSON(http.StatusInternalServerError, map[string]string{"error": "failed to create routing rule"})
	}

	// Invalidate Cache
	_, _ = h.versionMgr.IncrementRulesVersion(c.Request().Context())

	return c.JSON(http.StatusCreated, dto.FromRoutingRule(rule))
}

// HandleUpdateRoutingRule updates an existing routing rule
//
//	@Summary		Update Routing Rule
//	@Description	Updates an existing routing rule by ID
//	@Tags			routing-rules
//	@Accept			json
//	@Produce		json
//	@Param			id		path		string				true	"Routing Rule ID"
//	@Param			rule	body		dto.UpdateRoutingRuleRequest	true	"Updated routing rule"
//	@Success		200		{object}	dto.RoutingRuleResponse	"Updated routing rule"
//	@Failure		400		{object}	map[string]string	"Invalid request or ID mismatch"
//	@Failure		500		{object}	map[string]string	"Internal server error"
//	@Security		AdminKeyAuth
//	@Router			/rules/{id} [put]
func (h *RoutingRuleHandler) HandleUpdateRoutingRule(c echo.Context) error {
	id := c.Param("id")
	if id == "" {
		return c.JSON(http.StatusBadRequest, map[string]string{"error": "id is required"})
	}

	var req dto.UpdateRoutingRuleRequest
	if err := c.Bind(&req); err != nil {
		return c.JSON(http.StatusBadRequest, map[string]string{"error": "invalid request body"})
	}

	rule, err := req.ToDomain()
	if err != nil {
		return c.JSON(http.StatusBadRequest, map[string]string{"error": err.Error()})
	}

	rule.ID = id
	rule.UpdatedAt = time.Now()
	rule.Version = req.Version

	err = h.repo.UpdateRule(c.Request().Context(), rule)
	if err != nil {
		return c.JSON(http.StatusInternalServerError, map[string]string{"error": err.Error()})
	}

	// Invalidate Cache
	_, _ = h.versionMgr.IncrementRulesVersion(c.Request().Context())

	return c.JSON(http.StatusOK, dto.FromRoutingRule(rule))
}

// HandleDeleteRoutingRule deletes a routing rule
//
//	@Summary		Delete Routing Rule
//	@Description	Deletes a routing rule by ID
//	@Tags			routing-rules
//	@Param			id	path		string	true	"Routing Rule ID"
//	@Success		204	"Rule deleted successfully"
//	@Failure		400	{object}	map[string]string	"ID required"
//	@Failure		500	{object}	map[string]string	"Internal server error"
//	@Security		AdminKeyAuth
//	@Router			/rules/{id} [delete]
func (h *RoutingRuleHandler) HandleDeleteRoutingRule(c echo.Context) error {
	id := c.Param("id")
	if id == "" {
		return c.JSON(http.StatusBadRequest, map[string]string{"error": "id is required"})
	}

	if err := h.repo.DeleteRule(c.Request().Context(), id); err != nil {
		return c.JSON(http.StatusInternalServerError, map[string]string{"error": "failed to delete routing rule"})
	}

	// Invalidate Cache
	_, _ = h.versionMgr.IncrementRulesVersion(c.Request().Context())

	return c.NoContent(http.StatusNoContent)
}
