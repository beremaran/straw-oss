package handlers

import (
	"context"
	"net/http"
	"strconv"
	"time"

	"github.com/google/uuid"
	"github.com/kwilabs/straw-proxy-server/internal/domain"
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
//	@Success		200		{object}	map[string]interface{}	"Paginated list of routing rules"
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

	return c.JSON(http.StatusOK, map[string]interface{}{
		"data":  rules,
		"total": total,
		"page":  page,
		"limit": limit,
	})
}

// HandleGetRoutingRule gets a specific routing rule
//
//	@Summary		Get Routing Rule
//	@Description	Returns a specific routing rule by ID
//	@Tags			routing-rules
//	@Produce		json
//	@Param			id	path		string	true	"Routing Rule ID"
//	@Success		200	{object}	domain.RoutingRule	"Routing rule"
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

	return c.JSON(http.StatusOK, rule)
}

// HandleCreateRoutingRule creates a new routing rule
//
//	@Summary		Create Routing Rule
//	@Description	Creates a new routing rule for request matching
//	@Tags			routing-rules
//	@Accept			json
//	@Produce		json
//	@Param			rule	body		domain.RoutingRule	true	"Routing rule to create"
//	@Success		201		{object}	domain.RoutingRule	"Created routing rule"
//	@Failure		400		{object}	map[string]string	"Invalid request"
//	@Failure		500		{object}	map[string]string	"Internal server error"
//	@Security		AdminKeyAuth
//	@Router			/rules [post]
func (h *RoutingRuleHandler) HandleCreateRoutingRule(c echo.Context) error {
	var rule domain.RoutingRule
	if err := c.Bind(&rule); err != nil {
		return c.JSON(http.StatusBadRequest, map[string]string{"error": "invalid request body"})
	}

	if rule.Name == "" {
		return c.JSON(http.StatusBadRequest, map[string]string{"error": "name is required"})
	}

	rule.ID = uuid.New().String()
	rule.CreatedAt = time.Now()
	rule.UpdatedAt = time.Now()
	rule.Version = 1

	if err := h.repo.CreateRule(c.Request().Context(), &rule); err != nil {
		return c.JSON(http.StatusInternalServerError, map[string]string{"error": "failed to create routing rule"})
	}

	// Invalidate Cache
	_, _ = h.versionMgr.IncrementRulesVersion(c.Request().Context())

	return c.JSON(http.StatusCreated, rule)
}

// HandleUpdateRoutingRule updates an existing routing rule
//
//	@Summary		Update Routing Rule
//	@Description	Updates an existing routing rule by ID
//	@Tags			routing-rules
//	@Accept			json
//	@Produce		json
//	@Param			id		path		string				true	"Routing Rule ID"
//	@Param			rule	body		domain.RoutingRule	true	"Updated routing rule"
//	@Success		200		{object}	domain.RoutingRule	"Updated routing rule"
//	@Failure		400		{object}	map[string]string	"Invalid request or ID mismatch"
//	@Failure		500		{object}	map[string]string	"Internal server error"
//	@Security		AdminKeyAuth
//	@Router			/rules/{id} [put]
func (h *RoutingRuleHandler) HandleUpdateRoutingRule(c echo.Context) error {
	id := c.Param("id")
	if id == "" {
		return c.JSON(http.StatusBadRequest, map[string]string{"error": "id is required"})
	}

	// We need the existing rule to check version if enforcing optimistic locking strictly,
	// but the repo UpdateRule handles version checking.
	// However, we need to bind the incoming data.

	// Better pattern: fetch existing, apply updates, save.
	// Or trust the client to send the full object including version.
	// Let's assume the client sends the full object or we merge it.
	// For simplicity in this iteration, we expect the client to send the updated state.
	// But we must ensure ID matches.

	var rule domain.RoutingRule
	if err := c.Bind(&rule); err != nil {
		return c.JSON(http.StatusBadRequest, map[string]string{"error": "invalid request body"})
	}

	if rule.ID != "" && rule.ID != id {
		return c.JSON(http.StatusBadRequest, map[string]string{"error": "id mismatch"})
	}
	rule.ID = id
	rule.UpdatedAt = time.Now()

	err := h.repo.UpdateRule(c.Request().Context(), &rule)
	if err != nil {
		return c.JSON(http.StatusInternalServerError, map[string]string{"error": err.Error()})
	}

	// Invalidate Cache
	_, _ = h.versionMgr.IncrementRulesVersion(c.Request().Context())

	return c.JSON(http.StatusOK, rule)
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
