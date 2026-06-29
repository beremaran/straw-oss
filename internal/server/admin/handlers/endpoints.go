package handlers

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/google/uuid"

	"github.com/beremaran/straw/internal/domain"
	"github.com/beremaran/straw/internal/server/admin/middleware"
	"github.com/beremaran/straw/internal/server/dto"
	"github.com/beremaran/straw/internal/server/helper"
	"github.com/beremaran/straw/internal/service/endpoint"
	"github.com/beremaran/straw/pkg/broker"
	"github.com/beremaran/straw/pkg/protocol"
)

const (
	maxEndpointListLimit = 100
	maxLogQueryLimit     = 500
	recentCommandCount   = 5
)

// EndpointHandler manages endpoint CRUD and lifecycle operations.
type EndpointHandler struct {
	healthService *endpoint.HealthService
	endpointRepo  domain.EndpointRepository
	commandRepo   domain.EndpointCommandRepository
	broker        broker.MessageBroker
	auditRepo     domain.ManagementAuditRepository
	logRepo       domain.EndpointLogRepository
}

// NewEndpointHandler creates a new EndpointHandler.
func NewEndpointHandler(
	healthService *endpoint.HealthService,
	endpointRepo domain.EndpointRepository,
	commandRepo domain.EndpointCommandRepository,
	broker broker.MessageBroker,
	auditRepo domain.ManagementAuditRepository,
	logRepo domain.EndpointLogRepository,
) *EndpointHandler {
	return &EndpointHandler{
		healthService: healthService,
		endpointRepo:  endpointRepo,
		commandRepo:   commandRepo,
		broker:        broker,
		auditRepo:     auditRepo,
		logRepo:       logRepo,
	}
}

// HandleCreateEndpoint creates or reactivates an endpoint.
func (h *EndpointHandler) HandleCreateEndpoint(w http.ResponseWriter, r *http.Request) {
	var req dto.CreateEndpointRequest

	err := json.NewDecoder(r.Body).Decode(&req)
	if err != nil {
		helper.WriteError(w, http.StatusBadRequest, "invalid request body")

		return
	}

	if req.ID == "" {
		helper.WriteError(w, http.StatusBadRequest, "id is required")

		return
	}

	if h.endpointRepo == nil {
		helper.WriteError(w, http.StatusNotImplemented, "persistence is disabled")

		return
	}

	ep, isReactivation, ok := h.resolveCreateEndpoint(r.Context(), req, w)
	if !ok {
		return
	}

	err = h.saveNewOrReactivatedEndpoint(r.Context(), ep, isReactivation)
	if err != nil {
		helper.WriteError(w, http.StatusInternalServerError, "failed to save endpoint")

		return
	}

	if h.healthService != nil {
		_ = h.healthService.SetDeleted(r.Context(), ep.ID, false)
	}

	h.auditCreate(r, ep, isReactivation)

	resp := h.mapToResponse(r.Context(), ep)
	helper.WriteJSON(w, http.StatusCreated, resp)
}

// HandleGetEndpoint retrieves a single endpoint.
func (h *EndpointHandler) HandleGetEndpoint(w http.ResponseWriter, r *http.Request) {
	ep, ok := h.loadEndpoint(w, r)
	if !ok {
		return
	}

	resp := h.mapToResponse(r.Context(), ep)
	helper.WriteJSON(w, http.StatusOK, resp)
}

// HandlePatchEndpoint updates an existing endpoint.
func (h *EndpointHandler) HandlePatchEndpoint(w http.ResponseWriter, r *http.Request) {
	ep, ok := h.loadEndpoint(w, r)
	if !ok {
		return
	}

	var req dto.PatchEndpointRequest

	err := json.NewDecoder(r.Body).Decode(&req)
	if err != nil {
		helper.WriteError(w, http.StatusBadRequest, "invalid request body")

		return
	}

	oldEp := *ep
	h.applyPatchFields(ep, req)

	var (
		commandID string
		reqBy     *string
	)
	if actor, ok := middleware.ActorFromContext(r.Context()); ok {
		reqBy = &actor.ID
	}

	if req.DesiredState != nil {
		newState := domain.DesiredState(*req.DesiredState)
		if newState != oldEp.DesiredState {
			ep.DesiredState = newState
			ep.UpdatedAt = time.Now().UTC()
			commandID, _ = h.handleDesiredStateChange(r.Context(), ep, reqBy, newState)
		}
	}

	err = h.endpointRepo.Update(r.Context(), ep)
	if err != nil {
		helper.WriteError(w, http.StatusInternalServerError, "failed to update endpoint")

		return
	}

	if h.auditRepo != nil {
		event := middleware.NewAuditEvent(r, domain.ActionUpdate, "endpoint", ep.ID, oldEp, ep)
		_ = h.auditRepo.Create(r.Context(), event)
	}

	resp := h.mapToResponse(r.Context(), ep)
	h.reloadPatchResponseCommands(r.Context(), ep.ID, commandID, &resp)

	helper.WriteJSON(w, http.StatusOK, resp)
}

// HandleDeleteEndpoint deletes an endpoint.
func (h *EndpointHandler) HandleDeleteEndpoint(w http.ResponseWriter, r *http.Request) {
	ep, ok := h.loadEndpoint(w, r)
	if !ok {
		return
	}

	err := h.endpointRepo.Delete(r.Context(), ep.ID)
	if err != nil {
		helper.WriteError(w, http.StatusInternalServerError, "failed to delete endpoint")

		return
	}

	if h.healthService != nil {
		_ = h.healthService.DeleteHealth(r.Context(), ep.ID)
		_ = h.healthService.SetDraining(r.Context(), ep.ID, false)
		_ = h.healthService.SetDeleted(r.Context(), ep.ID, true)
	}

	if h.auditRepo != nil {
		event := middleware.NewAuditEvent(r, domain.ActionDelete, "endpoint", ep.ID, ep, nil)
		_ = h.auditRepo.Create(r.Context(), event)
	}

	w.WriteHeader(http.StatusNoContent)
}

// HandleListEndpoints lists all endpoints.
func (h *EndpointHandler) HandleListEndpoints(w http.ResponseWriter, r *http.Request) {
	page, limit, includeDeleted := h.parseListParams(r)

	if h.endpointRepo == nil {
		h.handleDynamicListFallback(w, r)

		return
	}

	endpoints, total, err := h.endpointRepo.List(r.Context(), limit, (page-1)*limit, includeDeleted)
	if err != nil {
		helper.WriteError(w, http.StatusInternalServerError, "failed to list endpoints")

		return
	}

	data := make([]dto.EndpointResponse, len(endpoints))
	for i, ep := range endpoints {
		data[i] = h.mapToResponse(r.Context(), &ep)
	}

	helper.WriteJSON(w, http.StatusOK, dto.EndpointListResponse{
		Data:  data,
		Total: total,
		Page:  page,
		Limit: limit,
	})
}

// HandleDrainEndpoint drains an endpoint.
func (h *EndpointHandler) HandleDrainEndpoint(w http.ResponseWriter, r *http.Request) {
	ep, ok := h.loadEndpoint(w, r)
	if !ok {
		return
	}

	ep.DesiredState = domain.DesiredStateDraining
	ep.UpdatedAt = time.Now().UTC()

	err := h.endpointRepo.Update(r.Context(), ep)
	if err != nil {
		helper.WriteError(w, http.StatusInternalServerError, "failed to update endpoint state")

		return
	}

	if h.healthService != nil {
		err = h.healthService.DrainEndpoint(r.Context(), ep.ID)
		if err != nil {
			helper.WriteError(w, http.StatusInternalServerError, "failed to drain endpoint in health store")

			return
		}
	}

	var reqBy *string
	if actor, ok := middleware.ActorFromContext(r.Context()); ok {
		reqBy = &actor.ID
	}

	cmdID, err := h.publishControlCommand(r.Context(), ep.ID, "drain", reqBy, nil)
	if err != nil {
		helper.WriteError(w, http.StatusInternalServerError, "failed to issue drain command")

		return
	}

	if h.auditRepo != nil {
		event := middleware.NewAuditEvent(r, domain.ActionDrain, "endpoint", ep.ID, ep, nil)
		_ = h.auditRepo.Create(r.Context(), event)
	}

	helper.WriteJSON(w, http.StatusOK, dto.EndpointDrainResponse{
		EndpointID:   ep.ID,
		DesiredState: "draining",
		CommandID:    cmdID,
	})
}

// HandleUndrainEndpoint undrains an endpoint.
func (h *EndpointHandler) HandleUndrainEndpoint(w http.ResponseWriter, r *http.Request) {
	ep, ok := h.loadEndpoint(w, r)
	if !ok {
		return
	}

	ep.DesiredState = domain.DesiredStateActive
	ep.UpdatedAt = time.Now().UTC()

	err := h.endpointRepo.Update(r.Context(), ep)
	if err != nil {
		helper.WriteError(w, http.StatusInternalServerError, "failed to update endpoint state")

		return
	}

	if h.healthService != nil {
		_ = h.healthService.SetDraining(r.Context(), ep.ID, false)
		_ = h.healthService.SetDeleted(r.Context(), ep.ID, false)
	}

	var reqBy *string
	if actor, ok := middleware.ActorFromContext(r.Context()); ok {
		reqBy = &actor.ID
	}

	cmdID, err := h.publishControlCommand(r.Context(), ep.ID, "undrain", reqBy, nil)
	if err != nil {
		helper.WriteError(w, http.StatusInternalServerError, "failed to issue undrain command")

		return
	}

	if h.auditRepo != nil {
		event := middleware.NewAuditEvent(r, "undrain", "endpoint", ep.ID, ep, nil)
		_ = h.auditRepo.Create(r.Context(), event)
	}

	helper.WriteJSON(w, http.StatusOK, dto.EndpointDrainResponse{
		EndpointID:   ep.ID,
		DesiredState: "active",
		CommandID:    cmdID,
	})
}

// HandleRestartEndpoint restarts an endpoint.
func (h *EndpointHandler) HandleRestartEndpoint(w http.ResponseWriter, r *http.Request) {
	ep, ok := h.loadEndpoint(w, r)
	if !ok {
		return
	}

	var reqBy *string
	if actor, ok := middleware.ActorFromContext(r.Context()); ok {
		reqBy = &actor.ID
	}

	cmdID, err := h.publishControlCommand(r.Context(), ep.ID, "restart", reqBy, nil)
	if err != nil {
		helper.WriteError(w, http.StatusInternalServerError, "failed to issue restart command")

		return
	}

	if h.auditRepo != nil {
		event := middleware.NewAuditEvent(r, "restart", "endpoint", ep.ID, ep, nil)
		_ = h.auditRepo.Create(r.Context(), event)
	}

	desiredStr := "active"
	if ep != nil {
		desiredStr = string(ep.DesiredState)
	}

	helper.WriteJSON(w, http.StatusOK, dto.EndpointDrainResponse{
		EndpointID:   ep.ID,
		DesiredState: desiredStr,
		CommandID:    cmdID,
	})
}

// HandleListEndpointCommands lists commands for an endpoint.
func (h *EndpointHandler) HandleListEndpointCommands(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	if id == "" {
		helper.WriteError(w, http.StatusBadRequest, "id is required")

		return
	}

	if h.commandRepo == nil {
		helper.WriteError(w, http.StatusNotImplemented, "persistence is disabled")

		return
	}

	page, limit := h.parseCommandParams(r)

	commands, total, err := h.commandRepo.ListByEndpointID(r.Context(), id, limit, (page-1)*limit)
	if err != nil {
		helper.WriteError(w, http.StatusInternalServerError, "failed to list commands")

		return
	}

	helper.WriteJSON(w, http.StatusOK, dto.EndpointCommandListResponse{
		Data:  h.mapCommands(commands),
		Total: total,
		Page:  page,
		Limit: limit,
	})
}

// HandleGetEndpointCommand retrieves a single endpoint command.
func (h *EndpointHandler) HandleGetEndpointCommand(w http.ResponseWriter, r *http.Request) {
	cmdID := r.PathValue("id")
	if cmdID == "" {
		helper.WriteError(w, http.StatusBadRequest, "id is required")

		return
	}

	if h.commandRepo == nil {
		helper.WriteError(w, http.StatusNotImplemented, "persistence is disabled")

		return
	}

	cmd, err := h.commandRepo.GetByID(r.Context(), cmdID)
	if err != nil {
		helper.WriteError(w, http.StatusInternalServerError, "failed to get command")

		return
	}

	if cmd == nil {
		helper.WriteError(w, http.StatusNotFound, "command not found")

		return
	}

	helper.WriteJSON(w, http.StatusOK, h.mapCommand(cmd))
}

// HandleGetEndpointLogs retrieves logs for an endpoint.
func (h *EndpointHandler) HandleGetEndpointLogs(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	if id == "" {
		helper.WriteError(w, http.StatusBadRequest, "id is required")

		return
	}

	if h.logRepo == nil {
		helper.WriteError(w, http.StatusNotImplemented, "persistence is disabled")

		return
	}

	filter, err := h.parseLogFilterParams(w, r)
	if err != nil {
		return
	}

	entries, err := h.logRepo.Query(r.Context(), id, filter)
	if err != nil {
		helper.WriteError(w, http.StatusInternalServerError, "failed to query logs")

		return
	}

	resp := h.buildLogResponse(entries, filter.Limit)

	helper.WriteJSON(w, http.StatusOK, resp)
}

// HandleStreamEndpointLogs streams endpoint logs via SSE.
func (h *EndpointHandler) HandleStreamEndpointLogs(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	if id == "" {
		helper.WriteError(w, http.StatusBadRequest, "id is required")

		return
	}

	if h.broker == nil {
		helper.WriteError(w, http.StatusNotImplemented, "message broker is disabled")

		return
	}

	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")
	w.Header().Set("X-Accel-Buffering", "no")

	flusher, ok := w.(http.Flusher)
	if !ok {
		helper.WriteError(w, http.StatusInternalServerError, "streaming not supported")

		return
	}

	flusher.Flush()

	ctx, cancel := context.WithCancel(r.Context())
	defer cancel()

	subject := "endpoint.logs." + id

	err := h.broker.Subscribe(ctx, subject, h.makeStreamHandler(w, flusher, cancel), broker.WithTransient())
	if err != nil {
		writeSSEEvent(w, "error", []byte(sanitizeSSE(err.Error())))

		flusher.Flush()

		return
	}

	<-ctx.Done()
}

// Helpers and unexported methods placed after exported ones.

func (h *EndpointHandler) loadEndpoint(w http.ResponseWriter, r *http.Request) (*domain.Endpoint, bool) {
	id := r.PathValue("id")
	if id == "" {
		helper.WriteError(w, http.StatusBadRequest, "id is required")

		return nil, false
	}

	if h.endpointRepo == nil {
		helper.WriteError(w, http.StatusNotImplemented, "persistence is disabled")

		return nil, false
	}

	ep, err := h.endpointRepo.GetByID(r.Context(), id)
	if err != nil {
		helper.WriteError(w, http.StatusInternalServerError, "failed to get endpoint")

		return nil, false
	}

	if ep == nil {
		helper.WriteError(w, http.StatusNotFound, "endpoint not found")

		return nil, false
	}

	return ep, true
}

func (h *EndpointHandler) resolveCreateEndpoint(ctx context.Context, req dto.CreateEndpointRequest, w http.ResponseWriter) (*domain.Endpoint, bool, bool) {
	existing, err := h.endpointRepo.GetByID(ctx, req.ID)
	if err != nil {
		helper.WriteError(w, http.StatusInternalServerError, "failed to check existing endpoint")

		return nil, false, false
	}

	if existing != nil {
		if existing.DeletedAt == nil {
			helper.WriteError(w, http.StatusConflict, "endpoint already exists")

			return nil, false, false
		}

		h.reactivateEndpoint(existing, req)

		return existing, true, true
	}

	ep := h.createNewEndpoint(req)

	return ep, false, true
}

func (h *EndpointHandler) saveNewOrReactivatedEndpoint(ctx context.Context, ep *domain.Endpoint, isReactivation bool) error {
	if isReactivation {
		err := h.endpointRepo.Update(ctx, ep)
		if err != nil {
			return fmt.Errorf("updating endpoint: %w", err)
		}

		return nil
	}

	err := h.endpointRepo.Create(ctx, ep)
	if err != nil {
		return fmt.Errorf("creating endpoint: %w", err)
	}

	return nil
}

func (h *EndpointHandler) reactivateEndpoint(ep *domain.Endpoint, req dto.CreateEndpointRequest) {
	ep.DeletedAt = nil
	ep.IsRegistered = true

	ep.DesiredState = domain.DesiredState(req.DesiredState)
	if ep.DesiredState == "" {
		ep.DesiredState = domain.DesiredStateActive
	}

	ep.Tags = req.Tags
	if req.Metadata != nil {
		ep.Metadata = domain.EndpointMetadata{
			Version:        req.Metadata.Version,
			IP:             req.Metadata.IP,
			ActiveTasks:    req.Metadata.ActiveTasks,
			MaxConcurrency: req.Metadata.MaxConcurrency,
			Provider:       req.Metadata.Provider,
		}
	}

	ep.UpdatedAt = time.Now().UTC()
}

func (h *EndpointHandler) createNewEndpoint(req dto.CreateEndpointRequest) *domain.Endpoint {
	ep := domain.NewEndpoint(req.ID, req.Tags)
	if req.DesiredState != "" {
		ep.DesiredState = domain.DesiredState(req.DesiredState)
	}

	if req.Metadata != nil {
		ep.Metadata = domain.EndpointMetadata{
			Version:        req.Metadata.Version,
			IP:             req.Metadata.IP,
			ActiveTasks:    req.Metadata.ActiveTasks,
			MaxConcurrency: req.Metadata.MaxConcurrency,
			Provider:       req.Metadata.Provider,
		}
	}

	return ep
}

func (h *EndpointHandler) applyPatchFields(ep *domain.Endpoint, req dto.PatchEndpointRequest) {
	if req.Tags != nil {
		ep.Tags = *req.Tags
	}

	if req.Metadata != nil {
		ep.Metadata = domain.EndpointMetadata{
			Version:        req.Metadata.Version,
			IP:             req.Metadata.IP,
			ActiveTasks:    req.Metadata.ActiveTasks,
			MaxConcurrency: req.Metadata.MaxConcurrency,
			Provider:       req.Metadata.Provider,
		}
	}

	if req.IsRegistered != nil {
		ep.IsRegistered = *req.IsRegistered
	}
}

func (h *EndpointHandler) handleDesiredStateChange(ctx context.Context, ep *domain.Endpoint, reqBy *string, newState domain.DesiredState) (string, error) {
	var (
		commandID string
		err       error
	)

	switch newState {
	case domain.DesiredStateDraining:
		if h.healthService != nil {
			_ = h.healthService.DrainEndpoint(ctx, ep.ID)
		}

		commandID, err = h.publishControlCommand(ctx, ep.ID, "drain", reqBy, nil)
	case domain.DesiredStateActive:
		if h.healthService != nil {
			_ = h.healthService.SetDeleted(ctx, ep.ID, false)
			_ = h.healthService.SetDraining(ctx, ep.ID, false)
		}

		commandID, err = h.publishControlCommand(ctx, ep.ID, "undrain", reqBy, nil)
	case domain.DesiredStateDisabled:
		if h.healthService != nil {
			_ = h.healthService.SetDraining(ctx, ep.ID, false)
		}

		commandID, err = h.publishControlCommand(ctx, ep.ID, "disable", reqBy, nil)
	case domain.DesiredStateDeleted:
		// Handled via separate DELETE API path
	default:
		// Catch-all
	}

	return commandID, err
}

func (h *EndpointHandler) publishControlCommand(ctx context.Context, endpointID, commandName string, reqBy *string, payload map[string]any) (string, error) {
	cmdID := uuid.New().String()
	now := time.Now().UTC()

	if h.commandRepo != nil {
		cmd := &domain.EndpointCommand{
			ID:          cmdID,
			EndpointID:  endpointID,
			Command:     commandName,
			Status:      domain.CommandStatusAccepted,
			Payload:     payload,
			RequestedBy: reqBy,
			RequestedAt: now,
		}

		err := h.commandRepo.Create(ctx, cmd)
		if err != nil {
			return "", fmt.Errorf("creating command: %w", err)
		}
	}

	if h.broker != nil {
		cmdPayload := protocol.ControlCommand{
			CommandID:  cmdID,
			EndpointID: endpointID,
			Command:    commandName,
			IssuedAt:   now,
			Payload:    payload,
		}

		body, err := json.Marshal(cmdPayload)
		if err != nil {
			return "", fmt.Errorf("marshaling command payload: %w", err)
		}

		subject := "endpoint.control." + endpointID

		err = h.broker.Publish(ctx, subject, body)
		if err != nil {
			return "", fmt.Errorf("publishing control command: %w", err)
		}
	}

	return cmdID, nil
}

func (h *EndpointHandler) auditCreate(r *http.Request, ep *domain.Endpoint, isReactivation bool) {
	if h.auditRepo != nil {
		action := domain.ActionCreate
		if isReactivation {
			action = "reactivate"
		}

		event := middleware.NewAuditEvent(r, action, "endpoint", ep.ID, nil, ep)
		_ = h.auditRepo.Create(r.Context(), event)
	}
}

func (h *EndpointHandler) parseListParams(r *http.Request) (int, int, bool) {
	pageStr := r.URL.Query().Get("page")
	limitStr := r.URL.Query().Get("limit")
	includeDeletedStr := r.URL.Query().Get("include_deleted")

	page := 1
	limit := 20
	includeDeleted := false

	if pageStr != "" {
		p, err := strconv.Atoi(pageStr)
		if err == nil && p > 0 {
			page = p
		}
	}

	if limitStr != "" {
		l, err := strconv.Atoi(limitStr)
		if err == nil && l > 0 {
			limit = min(l, maxEndpointListLimit)
		}
	}

	if includeDeletedStr == "true" {
		includeDeleted = true
	}

	return page, limit, includeDeleted
}

func (h *EndpointHandler) handleDynamicListFallback(w http.ResponseWriter, r *http.Request) {
	if h.healthService == nil {
		helper.WriteError(w, http.StatusInternalServerError, "health service and repository not available")

		return
	}

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
			LastSeen:    e.LastSeen.Format(time.RFC3339),
		}
	}

	helper.WriteJSON(w, http.StatusOK, response)
}

func (h *EndpointHandler) parseCommandParams(r *http.Request) (int, int) {
	pageStr := r.URL.Query().Get("page")
	limitStr := r.URL.Query().Get("limit")

	page := 1
	limit := 20

	if pageStr != "" {
		p, err := strconv.Atoi(pageStr)
		if err == nil && p > 0 {
			page = p
		}
	}

	if limitStr != "" {
		l, err := strconv.Atoi(limitStr)
		if err == nil && l > 0 {
			limit = min(l, maxEndpointListLimit)
		}
	}

	return page, limit
}

func (h *EndpointHandler) reloadPatchResponseCommands(ctx context.Context, epID string, commandID string, resp *dto.EndpointResponse) {
	if commandID != "" && len(resp.RecentCommands) > 0 {
		if h.commandRepo != nil {
			cmds, _, _ := h.commandRepo.ListByEndpointID(ctx, epID, recentCommandCount, 0)
			resp.RecentCommands = h.mapCommands(cmds)
		}
	}
}

func (h *EndpointHandler) mapToResponse(ctx context.Context, ep *domain.Endpoint) dto.EndpointResponse {
	resp := dto.EndpointResponse{
		ID:            ep.ID,
		Tags:          ep.Tags,
		LastHeartbeat: ep.LastHeartbeat.Format(time.RFC3339),
		IsHealthy:     ep.IsHealthy,
		Metadata: dto.EndpointMetadataDTO{
			Version:        ep.Metadata.Version,
			IP:             ep.Metadata.IP,
			ActiveTasks:    ep.Metadata.ActiveTasks,
			MaxConcurrency: ep.Metadata.MaxConcurrency,
			Provider:       ep.Metadata.Provider,
		},
		DesiredState: string(ep.DesiredState),
		IsRegistered: ep.IsRegistered,
		CreatedAt:    ep.CreatedAt.Format(time.RFC3339),
		UpdatedAt:    ep.UpdatedAt.Format(time.RFC3339),
	}

	if ep.DeletedAt != nil {
		deletedStr := ep.DeletedAt.Format(time.RFC3339)
		resp.DeletedAt = &deletedStr
	}

	if h.healthService != nil {
		live, err := h.healthService.GetEndpointHealth(ctx, ep.ID)
		if err == nil && live != nil {
			resp.Health = &dto.EndpointHealthDTO{
				EndpointID:  live.EndpointID,
				State:       live.State,
				Tags:        live.Tags,
				Version:     live.Version,
				ActiveTasks: live.ActiveTasks,
				LastSeen:    live.LastSeen.Format(time.RFC3339),
			}

			if ep.DeletedAt != nil && time.Since(live.LastSeen) < 30*time.Second {
				resp.Health.State = "deleted-heartbeating"
				resp.IsHealthy = false
			} else if ep.DeletedAt != nil {
				resp.IsHealthy = false
			}
		}
	}

	if h.commandRepo != nil {
		cmds, _, err := h.commandRepo.ListByEndpointID(ctx, ep.ID, recentCommandCount, 0)
		if err == nil {
			resp.RecentCommands = h.mapCommands(cmds)
		}
	}

	return resp
}

func (h *EndpointHandler) mapCommands(cmds []domain.EndpointCommand) []dto.EndpointCommandDTO {
	res := make([]dto.EndpointCommandDTO, len(cmds))
	for i, c := range cmds {
		res[i] = h.mapCommand(&c)
	}

	return res
}

func (h *EndpointHandler) mapCommand(c *domain.EndpointCommand) dto.EndpointCommandDTO {
	dtoCmd := dto.EndpointCommandDTO{
		ID:          c.ID,
		EndpointID:  c.EndpointID,
		Command:     c.Command,
		Status:      string(c.Status),
		Payload:     c.Payload,
		RequestedBy: c.RequestedBy,
		RequestedAt: c.RequestedAt.Format(time.RFC3339),
	}

	if c.AcceptedAt != nil {
		accStr := c.AcceptedAt.Format(time.RFC3339)
		dtoCmd.AcceptedAt = &accStr
	}

	if c.CompletedAt != nil {
		compStr := c.CompletedAt.Format(time.RFC3339)
		dtoCmd.CompletedAt = &compStr
	}

	if c.Error != nil {
		dtoCmd.Error = c.Error
	}

	return dtoCmd
}

func (h *EndpointHandler) parseLogFilterParams(w http.ResponseWriter, r *http.Request) (domain.LogFilter, error) {
	q := r.URL.Query()
	filter := domain.LogFilter{}

	t, err := parseLogTimeParam(q.Get("start"))
	if err != nil {
		helper.WriteError(w, http.StatusBadRequest, "invalid start timestamp format, must be RFC3339")

		return filter, fmt.Errorf("parsing start timestamp: %w", err)
	}

	filter.Start = t

	t, err = parseLogTimeParam(q.Get("end"))
	if err != nil {
		helper.WriteError(w, http.StatusBadRequest, "invalid end timestamp format, must be RFC3339")

		return filter, fmt.Errorf("parsing end timestamp: %w", err)
	}

	filter.End = t

	filter.Level = q.Get("level")
	filter.Q = q.Get("q")
	filter.TraceID = q.Get("trace_id")
	filter.RequestID = q.Get("request_id")

	if cursorStr := q.Get("cursor"); cursorStr != "" {
		c, err := strconv.ParseInt(cursorStr, 10, 64)
		if err != nil {
			helper.WriteError(w, http.StatusBadRequest, "invalid cursor, must be an integer")

			return filter, fmt.Errorf("parsing cursor: %w", err)
		}

		filter.Cursor = c
	}

	limit := 50

	if limitStr := q.Get("limit"); limitStr != "" {
		l, err := strconv.Atoi(limitStr)
		if err == nil && l > 0 {
			limit = l
		}
	}

	if limit > maxLogQueryLimit {
		limit = maxLogQueryLimit
	}

	filter.Limit = limit + 1

	return filter, nil
}

func (h *EndpointHandler) makeStreamHandler(w http.ResponseWriter, flusher http.Flusher, cancel context.CancelFunc) broker.Handler {
	return func(_ context.Context, body []byte) error {
		var entry protocol.LogEntry

		err := json.Unmarshal(body, &entry)
		if err != nil {
			// Skip invalid log entries silently
			return skipErr()
		}

		dtoEntry := dto.EndpointLogDTO{
			EndpointID: entry.EndpointID,
			ObservedAt: entry.ObservedAt.Format(time.RFC3339),
			Level:      entry.Level,
			Message:    entry.Message,
			Attrs:      entry.Attrs,
		}
		if entry.TraceID != "" {
			dtoEntry.TraceID = &entry.TraceID
		}

		if entry.RequestID != "" {
			dtoEntry.RequestID = &entry.RequestID
		}

		respBytes, err := json.Marshal(dtoEntry)
		if err != nil {
			// Skip entries that fail to marshal
			return skipErr()
		}

		_, err = fmt.Fprintf(w, "data: %s\n\n", string(respBytes))
		if err != nil {
			cancel()

			return fmt.Errorf("writing log entry: %w", err)
		}

		flusher.Flush()

		return nil
	}
}

func (h *EndpointHandler) buildLogResponse(entries []domain.EndpointLogEntry, limit int) dto.EndpointLogListResponse {
	limit--
	hasMore := len(entries) > limit

	var responseEntries []domain.EndpointLogEntry
	if hasMore {
		responseEntries = entries[:limit]
	} else {
		responseEntries = entries
	}

	data := make([]dto.EndpointLogDTO, len(responseEntries))
	for i, entry := range responseEntries {
		data[i] = dto.EndpointLogDTO{
			ID:         entry.ID,
			EndpointID: entry.EndpointID,
			ObservedAt: entry.ObservedAt.Format(time.RFC3339),
			Level:      entry.Level,
			Message:    entry.Message,
			Attrs:      entry.Attrs,
			TraceID:    entry.TraceID,
			RequestID:  entry.RequestID,
		}
	}

	var nextCursor string
	if hasMore && len(responseEntries) > 0 {
		nextCursor = strconv.FormatInt(responseEntries[len(responseEntries)-1].ID, 10)
	}

	return dto.EndpointLogListResponse{
		Data:       data,
		NextCursor: nextCursor,
		HasMore:    hasMore,
	}
}

func parseLogTimeParam(val string) (*time.Time, error) {
	if val == "" {
		return nil, nil
	}

	t, err := time.Parse(time.RFC3339, val)
	if err != nil {
		return nil, fmt.Errorf("parsing timestamp: %w", err)
	}

	return &t, nil
}

func writeSSEEvent(w http.ResponseWriter, event string, data []byte) {
	var buf strings.Builder
	buf.WriteString("event: ")
	buf.WriteString(event)
	buf.WriteString("\ndata: ")
	buf.Write(data)
	buf.WriteString("\n\n")
	_, _ = w.Write([]byte(buf.String()))
}

func sanitizeSSE(s string) string {
	return strings.NewReplacer("\n", "\\n", "\r", "\\r").Replace(s)
}

func skipErr() error {
	return nil
}
