package handlers

import (
	"context"
	"encoding/json"
	"net/http"
	"strconv"
	"time"

	"github.com/beremaran/straw/internal/domain"
	"github.com/beremaran/straw/internal/server/admin/middleware"
	"github.com/beremaran/straw/internal/server/dto"
	"github.com/beremaran/straw/internal/server/helper"
	"github.com/beremaran/straw/internal/service/endpoint"
	"github.com/beremaran/straw/pkg/broker"
	"github.com/google/uuid"
)

type EndpointHandler struct {
	healthService *endpoint.HealthService
	endpointRepo  domain.EndpointRepository
	commandRepo   domain.EndpointCommandRepository
	broker        broker.MessageBroker
	auditRepo     domain.ManagementAuditRepository
}

func NewEndpointHandler(
	healthService *endpoint.HealthService,
	endpointRepo domain.EndpointRepository,
	commandRepo domain.EndpointCommandRepository,
	broker broker.MessageBroker,
	auditRepo domain.ManagementAuditRepository,
) *EndpointHandler {
	return &EndpointHandler{
		healthService: healthService,
		endpointRepo:  endpointRepo,
		commandRepo:   commandRepo,
		broker:        broker,
		auditRepo:     auditRepo,
	}
}

type ControlCommandPayload struct {
	CommandID  string         `json:"command_id"`
	EndpointID string         `json:"endpoint_id"`
	Command    string         `json:"command"`
	IssuedAt   string         `json:"issued_at"`
	Payload    map[string]any `json:"payload"`
}

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

func (h *EndpointHandler) HandleGetEndpoint(w http.ResponseWriter, r *http.Request) {
	ep, ok := h.loadEndpoint(w, r)
	if !ok {
		return
	}

	resp := h.mapToResponse(r.Context(), ep)
	helper.WriteJSON(w, http.StatusOK, resp)
}

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

	var commandID string
	var reqBy *string
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

func (h *EndpointHandler) HandleGetEndpointCommand(w http.ResponseWriter, r *http.Request) {
	cmdID := r.PathValue("command_id")
	if cmdID == "" {
		helper.WriteError(w, http.StatusBadRequest, "command_id is required")

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

func (h *EndpointHandler) HandleCommandDispatch(w http.ResponseWriter, r *http.Request) {
	seg3 := r.PathValue("segment3")
	seg4 := r.PathValue("segment4")

	if seg3 == "commands" {
		r.SetPathValue("command_id", seg4)
		h.HandleGetEndpointCommand(w, r)

		return
	}

	if seg4 == "commands" {
		r.SetPathValue("id", seg3)
		h.HandleListEndpointCommands(w, r)

		return
	}

	helper.WriteError(w, http.StatusNotFound, "path not found")
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
		return h.endpointRepo.Update(ctx, ep)
	}

	return h.endpointRepo.Create(ctx, ep)
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
	var commandID string
	var err error

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
			return "", err
		}
	}

	if h.broker != nil {
		cmdPayload := ControlCommandPayload{
			CommandID:  cmdID,
			EndpointID: endpointID,
			Command:    commandName,
			IssuedAt:   now.Format(time.RFC3339),
			Payload:    payload,
		}
		body, err := json.Marshal(cmdPayload)
		if err != nil {
			return "", err
		}
		subject := "endpoint.control." + endpointID
		err = h.broker.Publish(ctx, subject, body)
		if err != nil {
			return "", err
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
			limit = l
			if limit > 100 {
				limit = 100
			}
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
			limit = l
			if limit > 100 {
				limit = 100
			}
		}
	}

	return page, limit
}

func (h *EndpointHandler) reloadPatchResponseCommands(ctx context.Context, epID string, commandID string, resp *dto.EndpointResponse) {
	if commandID != "" && len(resp.RecentCommands) > 0 {
		if h.commandRepo != nil {
			cmds, _, _ := h.commandRepo.ListByEndpointID(ctx, epID, 5, 0)
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
		cmds, _, err := h.commandRepo.ListByEndpointID(ctx, ep.ID, 5, 0)
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
