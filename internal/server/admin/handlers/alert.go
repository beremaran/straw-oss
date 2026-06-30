package handlers

import (
	"errors"
	"net/http"
	"strings"
	"time"

	"github.com/google/uuid"

	"github.com/beremaran/straw/internal/domain"
	"github.com/beremaran/straw/internal/server/admin/middleware"
	"github.com/beremaran/straw/internal/server/dto"
	"github.com/beremaran/straw/internal/server/helper"
	alertservice "github.com/beremaran/straw/internal/service/alert"
)

const (
	defaultAlertWindow   = "5m"
	defaultAlertCooldown = "15m"
	defaultAlertSeverity = "warning"
	alertRuleEntity      = "alert_rule"
	alertEventEntity     = "alert_event"
)

var (
	errAlertNameRequired     = errors.New("name is required")
	errAlertWindowInvalid    = errors.New("window must be a positive duration")
	errAlertCooldownInvalid  = errors.New("cooldown must be a positive duration")
	errAlertChannelIDInvalid = errors.New("notification_channel_ids must contain uuids")
	errAlertEventResolved    = errors.New("resolved alert events cannot be acknowledged")
)

// AlertHandler manages alert rules and events.
type AlertHandler struct {
	ruleRepo  domain.AlertRuleRepository
	eventRepo domain.AlertEventRepository
	auditRepo domain.ManagementAuditRepository
}

// NewAlertHandler creates an AlertHandler.
func NewAlertHandler(
	ruleRepo domain.AlertRuleRepository,
	eventRepo domain.AlertEventRepository,
	auditRepo domain.ManagementAuditRepository,
) *AlertHandler {
	return &AlertHandler{ruleRepo: ruleRepo, eventRepo: eventRepo, auditRepo: auditRepo}
}

// HandleListRules lists alert rules.
func (h *AlertHandler) HandleListRules(w http.ResponseWriter, r *http.Request) {
	page, limit := reportPageLimit(r)

	rules, total, err := h.ruleRepo.List(r.Context(), limit, (page-1)*limit)
	if err != nil {
		helper.WriteError(w, http.StatusInternalServerError, "failed to list alert rules")

		return
	}

	helper.WriteJSON(w, http.StatusOK, dto.ListAlertRulesResponse{
		Data:  dto.FromAlertRules(rules),
		Total: total,
		Page:  page,
		Limit: limit,
	})
}

// HandleCreateRule creates an alert rule.
func (h *AlertHandler) HandleCreateRule(w http.ResponseWriter, r *http.Request) {
	var req dto.CreateAlertRuleRequest

	err := helper.ReadJSON(r, &req)
	if err != nil {
		helper.WriteError(w, http.StatusBadRequest, "invalid request body")

		return
	}

	rule := alertRuleFromCreate(req, createdByFromContext(r.Context()))

	err = validateAlertRule(rule)
	if err != nil {
		helper.WriteError(w, http.StatusBadRequest, err.Error())

		return
	}

	err = h.ruleRepo.Create(r.Context(), rule)
	if err != nil {
		helper.WriteError(w, http.StatusInternalServerError, "failed to create alert rule")

		return
	}

	resp := dto.FromAlertRule(rule)
	h.audit(r, domain.ActionCreate, alertRuleEntity, rule.ID, nil, resp)

	helper.WriteJSON(w, http.StatusCreated, resp)
}

// HandleGetRule returns an alert rule.
func (h *AlertHandler) HandleGetRule(w http.ResponseWriter, r *http.Request) {
	rule := h.loadRule(w, r)
	if rule == nil {
		return
	}

	helper.WriteJSON(w, http.StatusOK, dto.FromAlertRule(rule))
}

// HandleUpdateRule updates an alert rule.
func (h *AlertHandler) HandleUpdateRule(w http.ResponseWriter, r *http.Request) {
	rule := h.loadRule(w, r)
	if rule == nil {
		return
	}

	oldRule := *rule

	var req dto.UpdateAlertRuleRequest

	err := helper.ReadJSON(r, &req)
	if err != nil {
		helper.WriteError(w, http.StatusBadRequest, "invalid request body")

		return
	}

	applyAlertRuleUpdate(rule, req)

	err = validateAlertRule(rule)
	if err != nil {
		helper.WriteError(w, http.StatusBadRequest, err.Error())

		return
	}

	rule.UpdatedAt = time.Now().UTC()

	err = h.ruleRepo.Update(r.Context(), rule)
	if err != nil {
		helper.WriteError(w, http.StatusInternalServerError, "failed to update alert rule")

		return
	}

	resp := dto.FromAlertRule(rule)
	h.audit(r, domain.ActionUpdate, alertRuleEntity, rule.ID, dto.FromAlertRule(&oldRule), resp)

	helper.WriteJSON(w, http.StatusOK, resp)
}

// HandleDeleteRule disables an alert rule.
func (h *AlertHandler) HandleDeleteRule(w http.ResponseWriter, r *http.Request) {
	rule := h.loadRule(w, r)
	if rule == nil {
		return
	}

	err := h.ruleRepo.Disable(r.Context(), rule.ID)
	if err != nil {
		helper.WriteError(w, http.StatusInternalServerError, "failed to delete alert rule")

		return
	}

	h.audit(r, domain.ActionDelete, alertRuleEntity, rule.ID, dto.FromAlertRule(rule), nil)
	w.WriteHeader(http.StatusNoContent)
}

// HandleListEvents lists alert events.
func (h *AlertHandler) HandleListEvents(w http.ResponseWriter, r *http.Request) {
	page, limit := reportPageLimit(r)

	events, total, err := h.eventRepo.List(r.Context(), limit, (page-1)*limit)
	if err != nil {
		helper.WriteError(w, http.StatusInternalServerError, "failed to list alert events")

		return
	}

	helper.WriteJSON(w, http.StatusOK, dto.ListAlertEventsResponse{
		Data:  dto.FromAlertEvents(events),
		Total: total,
		Page:  page,
		Limit: limit,
	})
}

// HandleAcknowledgeEvent acknowledges an alert event.
func (h *AlertHandler) HandleAcknowledgeEvent(w http.ResponseWriter, r *http.Request) {
	event := h.loadEvent(w, r)
	if event == nil {
		return
	}

	if event.Status == domain.AlertStatusResolved {
		helper.WriteError(w, http.StatusBadRequest, errAlertEventResolved.Error())

		return
	}

	oldEvent := *event
	event.Status = domain.AlertStatusAcknowledged

	err := h.eventRepo.Update(r.Context(), event)
	if err != nil {
		helper.WriteError(w, http.StatusInternalServerError, "failed to acknowledge alert event")

		return
	}

	resp := dto.FromAlertEvent(event)
	h.audit(r, domain.ActionUpdate, alertEventEntity, event.ID, dto.FromAlertEvent(&oldEvent), resp)

	helper.WriteJSON(w, http.StatusOK, resp)
}

func (h *AlertHandler) loadRule(w http.ResponseWriter, r *http.Request) *domain.AlertRule {
	rule, err := h.ruleRepo.GetByID(r.Context(), r.PathValue("id"))
	if err != nil {
		helper.WriteError(w, http.StatusInternalServerError, "failed to get alert rule")

		return nil
	}

	if rule == nil {
		helper.WriteError(w, http.StatusNotFound, "alert rule not found")

		return nil
	}

	return rule
}

func (h *AlertHandler) loadEvent(w http.ResponseWriter, r *http.Request) *domain.AlertEvent {
	event, err := h.eventRepo.GetByID(r.Context(), r.PathValue("id"))
	if err != nil {
		helper.WriteError(w, http.StatusInternalServerError, "failed to get alert event")

		return nil
	}

	if event == nil {
		helper.WriteError(w, http.StatusNotFound, "alert event not found")

		return nil
	}

	return event
}

func (h *AlertHandler) audit(r *http.Request, action string, entityType string, id string, oldValue any, newValue any) {
	if h.auditRepo == nil {
		return
	}

	event := middleware.NewAuditEvent(r, action, entityType, id, oldValue, newValue)
	_ = h.auditRepo.Create(r.Context(), event)
}

func alertRuleFromCreate(req dto.CreateAlertRuleRequest, createdBy string) *domain.AlertRule {
	now := time.Now().UTC()

	active := true
	if req.IsActive != nil {
		active = *req.IsActive
	}

	return &domain.AlertRule{
		ID:                     uuid.New().String(),
		Name:                   strings.TrimSpace(req.Name),
		Description:            strings.TrimSpace(req.Description),
		Metric:                 strings.TrimSpace(req.Metric),
		Condition:              strings.TrimSpace(req.Condition),
		Threshold:              req.Threshold,
		Window:                 defaultString(req.Window, defaultAlertWindow),
		Filters:                normalizeFilters(req.Filters),
		Severity:               defaultString(req.Severity, defaultAlertSeverity),
		IsActive:               active,
		Cooldown:               defaultString(req.Cooldown, defaultAlertCooldown),
		NotificationChannelIDs: trimStrings(req.NotificationChannelIDs),
		CreatedBy:              createdBy,
		CreatedAt:              now,
		UpdatedAt:              now,
	}
}

func applyAlertRuleUpdate(rule *domain.AlertRule, req dto.UpdateAlertRuleRequest) {
	applyAlertRuleTextUpdate(rule, req)
	applyAlertRuleMetricUpdate(rule, req)
	applyAlertRuleDeliveryUpdate(rule, req)
}

func applyAlertRuleTextUpdate(rule *domain.AlertRule, req dto.UpdateAlertRuleRequest) {
	if req.Name != nil {
		rule.Name = strings.TrimSpace(*req.Name)
	}

	if req.Description != nil {
		rule.Description = strings.TrimSpace(*req.Description)
	}
}

func applyAlertRuleMetricUpdate(rule *domain.AlertRule, req dto.UpdateAlertRuleRequest) {
	if req.Metric != nil {
		rule.Metric = strings.TrimSpace(*req.Metric)
	}

	if req.Condition != nil {
		rule.Condition = strings.TrimSpace(*req.Condition)
	}

	if req.Threshold != nil {
		rule.Threshold = *req.Threshold
	}

	if req.Window != nil {
		rule.Window = defaultString(*req.Window, defaultAlertWindow)
	}

	if req.Filters != nil {
		rule.Filters = normalizeFilters(*req.Filters)
	}
}

func applyAlertRuleDeliveryUpdate(rule *domain.AlertRule, req dto.UpdateAlertRuleRequest) {
	if req.Severity != nil {
		rule.Severity = defaultString(*req.Severity, defaultAlertSeverity)
	}

	if req.IsActive != nil {
		rule.IsActive = *req.IsActive
	}

	if req.Cooldown != nil {
		rule.Cooldown = defaultString(*req.Cooldown, defaultAlertCooldown)
	}

	if req.NotificationChannelIDs != nil {
		rule.NotificationChannelIDs = trimStrings(*req.NotificationChannelIDs)
	}
}

func validateAlertRule(rule *domain.AlertRule) error {
	if rule.Name == "" {
		return errAlertNameRequired
	}

	if !alertservice.IsMetricSupported(rule.Metric) {
		return alertservice.ErrUnsupportedMetric
	}

	if !alertservice.IsConditionSupported(rule.Condition) {
		return alertservice.ErrUnsupportedCondition
	}

	if !positiveDuration(rule.Window) {
		return errAlertWindowInvalid
	}

	if !positiveDuration(rule.Cooldown) {
		return errAlertCooldownInvalid
	}

	for _, channelID := range rule.NotificationChannelIDs {
		_, err := uuid.Parse(channelID)
		if err != nil {
			return errAlertChannelIDInvalid
		}
	}

	return nil
}

func positiveDuration(value string) bool {
	duration, err := time.ParseDuration(strings.TrimSpace(value))
	if err != nil {
		return false
	}

	return duration > 0
}

func defaultString(value string, fallback string) string {
	value = strings.TrimSpace(value)
	if value == "" {
		return fallback
	}

	return value
}

func trimStrings(values []string) []string {
	result := make([]string, len(values))
	for i, value := range values {
		result[i] = strings.TrimSpace(value)
	}

	return result
}
