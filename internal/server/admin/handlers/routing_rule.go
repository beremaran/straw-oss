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
