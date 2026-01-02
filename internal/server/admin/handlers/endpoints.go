package handlers

import (
	"net/http"

	"github.com/kwilabs/straw-proxy-server/internal/service/endpoint"
	"github.com/labstack/echo/v4"
)

type EndpointHandler struct {
	healthService *endpoint.HealthService
}

func NewEndpointHandler(healthService *endpoint.HealthService) *EndpointHandler {
	return &EndpointHandler{healthService: healthService}
}

// HandleListEndpoints lists all known endpoints
func (h *EndpointHandler) HandleListEndpoints(c echo.Context) error {
	endpoints, err := h.healthService.ListAllEndpoints(c.Request().Context())
	if err != nil {
		return c.JSON(http.StatusInternalServerError, map[string]string{"error": "failed to list endpoints"})
	}
	return c.JSON(http.StatusOK, endpoints)
}

// HandleDrainEndpoint marks an endpoint as draining
func (h *EndpointHandler) HandleDrainEndpoint(c echo.Context) error {
	id := c.Param("id")
	if id == "" {
		return c.JSON(http.StatusBadRequest, map[string]string{"error": "id is required"})
	}

	if err := h.healthService.DrainEndpoint(c.Request().Context(), id); err != nil {
		return c.JSON(http.StatusInternalServerError, map[string]string{"error": "failed to drain endpoint"})
	}

	return c.NoContent(http.StatusOK)
}
