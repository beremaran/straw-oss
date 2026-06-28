package handlers

import (
	"net/http"

	"github.com/beremaran/straw/internal/server/dto"
	"github.com/beremaran/straw/internal/server/helper"
	"github.com/beremaran/straw/internal/service/endpoint"
)

type EndpointHandler struct {
	healthService *endpoint.HealthService
}

func NewEndpointHandler(healthService *endpoint.HealthService) *EndpointHandler {
	return &EndpointHandler{healthService: healthService}
}

func (h *EndpointHandler) HandleListEndpoints(w http.ResponseWriter, r *http.Request) {
	endpoints, err := h.healthService.ListAllEndpoints(r.Context())
	if err != nil {
		helper.WriteError(w, http.StatusInternalServerError, "failed to list endpoints")
		return
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

	helper.WriteJSON(w, http.StatusOK, response)
}

func (h *EndpointHandler) HandleDrainEndpoint(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	if id == "" {
		helper.WriteError(w, http.StatusBadRequest, "id is required")
		return
	}

	if err := h.healthService.DrainEndpoint(r.Context(), id); err != nil {
		helper.WriteError(w, http.StatusInternalServerError, "failed to drain endpoint")
		return
	}

	w.WriteHeader(http.StatusOK)
}
