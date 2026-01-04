package handlers

import (
	"net/http"

	"github.com/kwilabs/straw-proxy-server/internal/server/dto"
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
//
//	@Summary		List Endpoints
//	@Description	Returns all registered proxy endpoints with their health status
//	@Tags			endpoints
//	@Produce		json
//	@Success		200	{array}		dto.EndpointHealthResponse	"List of endpoints"
//	@Failure		500	{object}	map[string]string		"Internal server error"
//	@Security		AdminKeyAuth
//	@Router			/endpoints [get]
func (h *EndpointHandler) HandleListEndpoints(c echo.Context) error {
	endpoints, err := h.healthService.ListAllEndpoints(c.Request().Context())
	if err != nil {
		return c.JSON(http.StatusInternalServerError, map[string]string{"error": "failed to list endpoints"})
	}
	response := make([]dto.EndpointHealthResponse, len(endpoints))
	for i, e := range endpoints {
		response[i] = dto.EndpointHealthResponse{
			EndpointID:  e.EndpointID,
			State:       e.State,
			Tags:        e.Tags,
			Version:     e.Version,
			ActiveTasks: e.ActiveTasks,
			LastSeen:    e.LastSeen.Format("2006-01-02T15:04:05Z07:00"),
		}
	}

	return c.JSON(http.StatusOK, response)
}

// HandleDrainEndpoint marks an endpoint as draining
//
//	@Summary		Drain Endpoint
//	@Description	Marks an endpoint as draining, preventing new requests from being routed to it
//	@Tags			endpoints
//	@Param			id	path		string	true	"Endpoint ID"
//	@Success		200	"Endpoint marked for draining"
//	@Failure		400	{object}	map[string]string	"ID required"
//	@Failure		500	{object}	map[string]string	"Internal server error"
//	@Security		AdminKeyAuth
//	@Router			/endpoints/{id}/drain [post]
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
