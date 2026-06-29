package handlers

import (
	"encoding/csv"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"strconv"
	"time"

	"github.com/beremaran/straw/internal/domain"
	"github.com/beremaran/straw/internal/server/admin/middleware"
	"github.com/beremaran/straw/internal/server/dto"
	"github.com/beremaran/straw/internal/server/helper"
)

type AuditHandler struct {
	repo         domain.ManagementAuditRepository
	identityRepo domain.IdentityRepository
}

func NewAuditHandler(repo domain.ManagementAuditRepository, identityRepo domain.IdentityRepository) *AuditHandler {
	return &AuditHandler{repo: repo, identityRepo: identityRepo}
}

func (h *AuditHandler) HandleListEvents(w http.ResponseWriter, r *http.Request) {
	filter := h.parseFilter(r)
	redactBody := h.shouldRedact(r)

	events, total, err := h.repo.ListEvents(r.Context(), filter)
	if err != nil {
		helper.WriteError(w, http.StatusInternalServerError, "failed to list audit events")

		return
	}

	page := 1
	if filter.Limit > 0 && filter.Offset >= 0 {
		page = (filter.Offset / filter.Limit) + 1
	}

	helper.WriteJSON(w, http.StatusOK, dto.ListAuditEventsResponse{
		Data:  dto.FromAuditEvents(events, redactBody),
		Total: total,
		Page:  page,
		Limit: filter.Limit,
	})
}

func (h *AuditHandler) HandleGetEvent(w http.ResponseWriter, r *http.Request) {
	idStr := r.PathValue("id")
	id, err := strconv.ParseInt(idStr, 10, 64)
	if err != nil {
		helper.WriteError(w, http.StatusBadRequest, "invalid event id")

		return
	}

	event, err := h.repo.GetEventByID(r.Context(), id)
	if err != nil {
		helper.WriteError(w, http.StatusInternalServerError, "failed to get audit event")

		return
	}

	if event == nil {
		helper.WriteError(w, http.StatusNotFound, "audit event not found")

		return
	}

	redactBody := h.shouldRedact(r)
	helper.WriteJSON(w, http.StatusOK, dto.FromAuditEvent(event, redactBody))
}

func (h *AuditHandler) HandleListRequests(w http.ResponseWriter, r *http.Request) {
	filter := h.parseFilter(r)

	includeBody := !h.shouldRedact(r)
	reqs, total, err := h.repo.ListRequests(r.Context(), filter, includeBody)
	if err != nil {
		helper.WriteError(w, http.StatusInternalServerError, "failed to list audit requests")

		return
	}

	page := 1
	if filter.Limit > 0 && filter.Offset >= 0 {
		page = (filter.Offset / filter.Limit) + 1
	}

	helper.WriteJSON(w, http.StatusOK, dto.ListAuditRequestsResponse{
		Data:  dto.FromAuditRequests(reqs),
		Total: total,
		Page:  page,
		Limit: filter.Limit,
	})
}

func (h *AuditHandler) HandleExport(w http.ResponseWriter, r *http.Request) {
	filter := h.parseFilter(r)
	if filter.StartDate == nil || filter.EndDate == nil {
		helper.WriteError(w, http.StatusBadRequest, "start_date and end_date are required for export")

		return
	}

	duration := filter.EndDate.Sub(*filter.StartDate)
	if duration > 31*24*time.Hour {
		helper.WriteError(w, http.StatusBadRequest, "export date range cannot exceed 31 days")

		return
	}

	// Temporarily override limit and offset for full export
	filter.Limit = 0
	filter.Offset = 0

	format := r.URL.Query().Get("format")
	if format != "csv" && format != "ndjson" {
		format = "csv" // Default
	}

	events, _, err := h.repo.ListEvents(r.Context(), filter)
	if err != nil {
		helper.WriteError(w, http.StatusInternalServerError, "failed to query export events")

		return
	}

	redactBody := h.shouldRedact(r)
	dtos := dto.FromAuditEvents(events, redactBody)

	switch format {
	case "csv":
		h.exportCSV(w, dtos)
	case "ndjson":
		h.exportNDJSON(w, dtos)
	}
}

func (h *AuditHandler) exportCSV(w http.ResponseWriter, dtos []*dto.AuditEvent) {
	w.Header().Set("Content-Type", "text/csv")
	w.Header().Set("Content-Disposition", "attachment; filename=\"audit_events_export.csv\"")

	cw := csv.NewWriter(w)
	_ = cw.Write([]string{"ID", "OccurredAt", "ActorType", "ActorID", "ActorDisplay", "Action", "EntityType", "EntityID", "IP", "UserAgent", "RequestID", "TraceID"})
	for _, e := range dtos {
		_ = cw.Write([]string{
			fmt.Sprintf("%d", e.ID),
			e.OccurredAt.Format(time.RFC3339),
			e.ActorType,
			e.ActorID,
			e.ActorDisplay,
			e.Action,
			e.EntityType,
			e.EntityID,
			e.IP,
			e.UserAgent,
			e.RequestID,
			e.TraceID,
		})
	}
	cw.Flush()
}

func (h *AuditHandler) exportNDJSON(w http.ResponseWriter, dtos []*dto.AuditEvent) {
	w.Header().Set("Content-Type", "application/x-ndjson")
	w.Header().Set("Content-Disposition", "attachment; filename=\"audit_events_export.ndjson\"")

	encoder := json.NewEncoder(w)
	for _, e := range dtos {
		err := encoder.Encode(e)
		if err != nil {
			// We can't really do much if encoding fails midway, but we should log it
			// For now, just break out of the loop
			break
		}
	}
}

func (h *AuditHandler) shouldRedact(r *http.Request) bool {
	actor, ok := middleware.ActorFromContext(r.Context())
	if !ok {
		return true
	}

	if actor.Type == middleware.ActorTypeLegacy {
		return false // Legacy is Owner
	}

	if actor.Type == middleware.ActorTypeUser && h.identityRepo != nil {
		roles, err := h.identityRepo.ListUserRoles(r.Context(), actor.ID)
		if err == nil {
			for _, role := range roles {
				if role.Name == domain.RoleOwner || role.Name == domain.RoleSecurityAuditor {
					return false
				}
			}
		}
	}

	return true
}

func (h *AuditHandler) parseFilter(r *http.Request) domain.AuditEventFilter {
	filter := domain.AuditEventFilter{}
	q := r.URL.Query()

	parsePagination(q, &filter)
	parseDates(q, &filter)

	if action := q.Get("action"); action != "" {
		filter.Action = &action
	}

	if actorID := q.Get("actor_id"); actorID != "" {
		filter.ActorID = &actorID
	}

	return filter
}

func parsePagination(q url.Values, filter *domain.AuditEventFilter) {
	if limitStr := q.Get("limit"); limitStr != "" {
		limit, _ := strconv.Atoi(limitStr)
		if limit > 500 {
			limit = 500
		}
		filter.Limit = limit
	} else {
		filter.Limit = 20
	}

	if pageStr := q.Get("page"); pageStr != "" {
		page, err := strconv.Atoi(pageStr)
		if err == nil && page > 0 {
			filter.Offset = (page - 1) * filter.Limit
		}
	}
}

func parseDates(q url.Values, filter *domain.AuditEventFilter) {
	if startStr := q.Get("start_date"); startStr != "" {
		t, err := time.Parse(time.RFC3339, startStr)
		if err == nil {
			filter.StartDate = &t
		}
	}

	if endStr := q.Get("end_date"); endStr != "" {
		t, err := time.Parse(time.RFC3339, endStr)
		if err == nil {
			filter.EndDate = &t
		}
	}
}

