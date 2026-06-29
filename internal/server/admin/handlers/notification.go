package handlers

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/google/uuid"

	"github.com/beremaran/straw/internal/domain"
	"github.com/beremaran/straw/internal/server/admin/middleware"
	"github.com/beremaran/straw/internal/server/dto"
	"github.com/beremaran/straw/internal/server/helper"
)

const notificationTimeout = 5 * time.Second

var (
	errNotificationNameRequired  = errors.New("name is required")
	errNotificationTypeInvalid   = errors.New("unsupported notification channel type")
	errNotificationUserRequired  = errors.New("notification preferences require a user actor")
	errNotificationDisabled      = errors.New("notification channel is disabled")
	errNotificationMissingSecret = errors.New("notification channel has no secret")
	errNotificationHTTPStatus    = errors.New("notification endpoint returned error")
	errNotificationEmailTo       = errors.New("email channel requires config.to")
)

// NotificationDeliverer sends test notifications.
type NotificationDeliverer interface {
	Deliver(ctx context.Context, channel *domain.NotificationChannel) error
}

// NotificationHandler manages notification channels and preferences.
type NotificationHandler struct {
	channelRepo domain.NotificationChannelRepository
	prefRepo    domain.NotificationPreferenceRepository
	auditRepo   domain.ManagementAuditRepository
	deliverer   NotificationDeliverer
}

// NewNotificationHandler creates a NotificationHandler.
func NewNotificationHandler(
	channelRepo domain.NotificationChannelRepository,
	prefRepo domain.NotificationPreferenceRepository,
	auditRepo domain.ManagementAuditRepository,
	deliverer NotificationDeliverer,
) *NotificationHandler {
	if deliverer == nil {
		deliverer = httpNotificationDeliverer{client: &http.Client{Timeout: notificationTimeout}}
	}

	return &NotificationHandler{
		channelRepo: channelRepo,
		prefRepo:    prefRepo,
		auditRepo:   auditRepo,
		deliverer:   deliverer,
	}
}

// HandleListChannels lists notification channels.
func (h *NotificationHandler) HandleListChannels(w http.ResponseWriter, r *http.Request) {
	page, limit := reportPageLimit(r)

	channels, total, err := h.channelRepo.List(r.Context(), limit, (page-1)*limit)
	if err != nil {
		helper.WriteError(w, http.StatusInternalServerError, "failed to list notification channels")

		return
	}

	helper.WriteJSON(w, http.StatusOK, dto.ListNotificationChannelsResponse{
		Data:  dto.FromNotificationChannels(channels),
		Total: total,
		Page:  page,
		Limit: limit,
	})
}

// HandleCreateChannel creates a notification channel.
func (h *NotificationHandler) HandleCreateChannel(w http.ResponseWriter, r *http.Request) {
	var req dto.CreateNotificationChannelRequest

	err := helper.ReadJSON(r, &req)
	if err != nil {
		helper.WriteError(w, http.StatusBadRequest, "invalid request body")

		return
	}

	channel := notificationChannelFromCreate(req, createdByFromContext(r.Context()))

	err = validateNotificationChannel(channel)
	if err != nil {
		helper.WriteError(w, http.StatusBadRequest, err.Error())

		return
	}

	err = h.channelRepo.Create(r.Context(), channel)
	if err != nil {
		helper.WriteError(w, http.StatusInternalServerError, "failed to create notification channel")

		return
	}

	resp := dto.FromNotificationChannel(channel)
	h.audit(r, domain.ActionCreate, channel.ID, nil, resp)

	helper.WriteJSON(w, http.StatusCreated, resp)
}

// HandleUpdateChannel updates a notification channel.
func (h *NotificationHandler) HandleUpdateChannel(w http.ResponseWriter, r *http.Request) {
	channel := h.loadChannel(w, r)
	if channel == nil {
		return
	}

	oldChannel := *channel

	var req dto.UpdateNotificationChannelRequest

	err := helper.ReadJSON(r, &req)
	if err != nil {
		helper.WriteError(w, http.StatusBadRequest, "invalid request body")

		return
	}

	applyNotificationChannelUpdate(channel, req)

	err = validateNotificationChannel(channel)
	if err != nil {
		helper.WriteError(w, http.StatusBadRequest, err.Error())

		return
	}

	channel.UpdatedAt = time.Now().UTC()

	err = h.channelRepo.Update(r.Context(), channel)
	if err != nil {
		helper.WriteError(w, http.StatusInternalServerError, "failed to update notification channel")

		return
	}

	resp := dto.FromNotificationChannel(channel)
	h.audit(r, domain.ActionUpdate, channel.ID, dto.FromNotificationChannel(&oldChannel), resp)

	helper.WriteJSON(w, http.StatusOK, resp)
}

// HandleDeleteChannel disables a notification channel.
func (h *NotificationHandler) HandleDeleteChannel(w http.ResponseWriter, r *http.Request) {
	channel := h.loadChannel(w, r)
	if channel == nil {
		return
	}

	err := h.channelRepo.Disable(r.Context(), channel.ID)
	if err != nil {
		helper.WriteError(w, http.StatusInternalServerError, "failed to delete notification channel")

		return
	}

	h.audit(r, domain.ActionDelete, channel.ID, dto.FromNotificationChannel(channel), nil)
	w.WriteHeader(http.StatusNoContent)
}

// HandleTestChannel sends a test notification through a channel.
func (h *NotificationHandler) HandleTestChannel(w http.ResponseWriter, r *http.Request) {
	channel := h.loadChannel(w, r)
	if channel == nil {
		return
	}

	err := h.deliverer.Deliver(r.Context(), channel)
	resp := dto.TestNotificationResponse{Attempted: true, Success: err == nil}

	if err != nil {
		resp.Error = err.Error()
	}

	helper.WriteJSON(w, http.StatusOK, resp)
}

// HandleGetPreferences returns current-user notification preferences.
func (h *NotificationHandler) HandleGetPreferences(w http.ResponseWriter, r *http.Request) {
	userID, err := notificationUserID(r)
	if err != nil {
		helper.WriteError(w, http.StatusBadRequest, err.Error())

		return
	}

	preferences, err := h.prefRepo.ListByUserID(r.Context(), userID)
	if err != nil {
		helper.WriteError(w, http.StatusInternalServerError, "failed to list notification preferences")

		return
	}

	helper.WriteJSON(w, http.StatusOK, dto.NotificationPreferencesResponse{Data: dto.FromNotificationPreferences(preferences)})
}

// HandleUpdatePreferences replaces current-user notification preferences.
func (h *NotificationHandler) HandleUpdatePreferences(w http.ResponseWriter, r *http.Request) {
	userID, err := notificationUserID(r)
	if err != nil {
		helper.WriteError(w, http.StatusBadRequest, err.Error())

		return
	}

	var req dto.UpdateNotificationPreferencesRequest

	err = helper.ReadJSON(r, &req)
	if err != nil {
		helper.WriteError(w, http.StatusBadRequest, "invalid request body")

		return
	}

	preferences := make([]domain.NotificationPreference, len(req.Preferences))
	for i, pref := range req.Preferences {
		preferences[i] = domain.NotificationPreference{
			ID:        uuid.New().String(),
			UserID:    userID,
			EventType: strings.TrimSpace(pref.EventType),
			ChannelID: strings.TrimSpace(pref.ChannelID),
			IsEnabled: pref.IsEnabled,
		}
	}

	err = h.prefRepo.ReplaceForUser(r.Context(), userID, preferences)
	if err != nil {
		helper.WriteError(w, http.StatusInternalServerError, "failed to update notification preferences")

		return
	}

	helper.WriteJSON(w, http.StatusOK, dto.NotificationPreferencesResponse{Data: dto.FromNotificationPreferences(preferences)})
}

func (h *NotificationHandler) loadChannel(w http.ResponseWriter, r *http.Request) *domain.NotificationChannel {
	channel, err := h.channelRepo.GetByID(r.Context(), r.PathValue("id"))
	if err != nil {
		helper.WriteError(w, http.StatusInternalServerError, "failed to get notification channel")

		return nil
	}

	if channel == nil {
		helper.WriteError(w, http.StatusNotFound, "notification channel not found")

		return nil
	}

	return channel
}

func (h *NotificationHandler) audit(r *http.Request, action, id string, oldValue, newValue any) {
	if h.auditRepo == nil {
		return
	}

	event := middleware.NewAuditEvent(r, action, "notification_channel", id, oldValue, newValue)
	_ = h.auditRepo.Create(r.Context(), event)
}

func notificationChannelFromCreate(req dto.CreateNotificationChannelRequest, createdBy string) *domain.NotificationChannel {
	now := time.Now().UTC()

	active := true
	if req.IsEnabled != nil {
		active = *req.IsEnabled
	}

	return &domain.NotificationChannel{
		ID:        uuid.New().String(),
		Name:      strings.TrimSpace(req.Name),
		Type:      strings.TrimSpace(req.Type),
		Config:    notificationConfig(req.Config),
		SecretRef: notificationSecret(req.SecretRef, req.Secret),
		IsEnabled: active,
		CreatedBy: createdBy,
		CreatedAt: now,
		UpdatedAt: now,
	}
}

func applyNotificationChannelUpdate(channel *domain.NotificationChannel, req dto.UpdateNotificationChannelRequest) {
	if req.Name != nil {
		channel.Name = strings.TrimSpace(*req.Name)
	}

	if req.Type != nil {
		channel.Type = strings.TrimSpace(*req.Type)
	}

	if req.Config != nil {
		channel.Config = notificationConfig(*req.Config)
	}

	if req.SecretRef != nil || req.Secret != nil {
		secretRef := ""
		secret := ""

		if req.SecretRef != nil {
			secretRef = *req.SecretRef
		}

		if req.Secret != nil {
			secret = *req.Secret
		}

		channel.SecretRef = notificationSecret(secretRef, secret)
	}

	if req.IsEnabled != nil {
		channel.IsEnabled = *req.IsEnabled
	}
}

func notificationConfig(config domain.ConfigMap) domain.ConfigMap {
	if config == nil {
		return domain.ConfigMap{}
	}

	delete(config, "secret")
	delete(config, "secret_ref")
	delete(config, "webhook_url")
	delete(config, "password")

	return config
}

func notificationSecret(secretRef string, secret string) string {
	if strings.TrimSpace(secretRef) != "" {
		return strings.TrimSpace(secretRef)
	}

	return strings.TrimSpace(secret)
}

func validateNotificationChannel(channel *domain.NotificationChannel) error {
	if channel.Name == "" {
		return errNotificationNameRequired
	}

	switch channel.Type {
	case domain.NotificationChannelWebhook, domain.NotificationChannelEmail, domain.NotificationChannelSlackWebhook:
		return nil
	default:
		return errNotificationTypeInvalid
	}
}

func notificationUserID(r *http.Request) (string, error) {
	actor, ok := middleware.ActorFromContext(r.Context())
	if !ok || actor.Type != middleware.ActorTypeUser {
		return "", errNotificationUserRequired
	}

	_, err := uuid.Parse(actor.ID)
	if err != nil {
		return "", errNotificationUserRequired
	}

	return actor.ID, nil
}

type httpNotificationDeliverer struct {
	client *http.Client
}

func (d httpNotificationDeliverer) Deliver(ctx context.Context, channel *domain.NotificationChannel) error {
	if !channel.IsEnabled {
		return errNotificationDisabled
	}

	switch channel.Type {
	case domain.NotificationChannelWebhook, domain.NotificationChannelSlackWebhook:
		return d.postWebhook(ctx, channel)
	case domain.NotificationChannelEmail:
		return deliverEmail(channel)
	default:
		return errNotificationTypeInvalid
	}
}

func (d httpNotificationDeliverer) postWebhook(ctx context.Context, channel *domain.NotificationChannel) error {
	if channel.SecretRef == "" {
		return errNotificationMissingSecret
	}

	body, err := json.Marshal(map[string]string{"text": "Straw test notification"})
	if err != nil {
		return fmt.Errorf("marshal test notification: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, channel.SecretRef, bytes.NewReader(body))
	if err != nil {
		return fmt.Errorf("create notification request: %w", err)
	}

	req.Header.Set("Content-Type", "application/json")

	resp, err := d.client.Do(req)
	if err != nil {
		return fmt.Errorf("send notification: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode >= http.StatusBadRequest {
		return fmt.Errorf("%w: %d", errNotificationHTTPStatus, resp.StatusCode)
	}

	return nil
}

func deliverEmail(channel *domain.NotificationChannel) error {
	if filterString(channel.Config, "to") == "" {
		return errNotificationEmailTo
	}

	return nil
}
