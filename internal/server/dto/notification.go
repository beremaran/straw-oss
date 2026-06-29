package dto

import (
	"time"

	"github.com/beremaran/straw/internal/domain"
)

// CreateNotificationChannelRequest creates a notification channel.
type CreateNotificationChannelRequest struct {
	Name      string           `json:"name"`
	Type      string           `json:"type"`
	Config    domain.ConfigMap `json:"config,omitempty"`
	Secret    string           `json:"secret,omitempty"`
	SecretRef string           `json:"secret_ref,omitempty"`
	IsEnabled *bool            `json:"is_enabled,omitempty"`
}

// UpdateNotificationChannelRequest updates a notification channel.
type UpdateNotificationChannelRequest struct {
	Name      *string           `json:"name,omitempty"`
	Type      *string           `json:"type,omitempty"`
	Config    *domain.ConfigMap `json:"config,omitempty"`
	Secret    *string           `json:"secret,omitempty"`
	SecretRef *string           `json:"secret_ref,omitempty"`
	IsEnabled *bool             `json:"is_enabled,omitempty"`
}

// NotificationChannelResponse represents a redacted notification channel.
type NotificationChannelResponse struct {
	ID        string           `json:"id"`
	Name      string           `json:"name"`
	Type      string           `json:"type"`
	Config    domain.ConfigMap `json:"config"`
	HasSecret bool             `json:"has_secret"`
	IsEnabled bool             `json:"is_enabled"`
	CreatedBy string           `json:"created_by,omitempty"`
	CreatedAt time.Time        `json:"created_at"`
	UpdatedAt time.Time        `json:"updated_at"`
}

// TestNotificationResponse reports a test delivery attempt.
type TestNotificationResponse struct {
	Attempted bool   `json:"attempted"`
	Success   bool   `json:"success"`
	Error     string `json:"error,omitempty"`
}

// NotificationPreferenceDTO represents a notification preference.
type NotificationPreferenceDTO struct {
	EventType string `json:"event_type"`
	ChannelID string `json:"channel_id"`
	IsEnabled bool   `json:"is_enabled"`
}

// UpdateNotificationPreferencesRequest replaces current-user preferences.
type UpdateNotificationPreferencesRequest struct {
	Preferences []NotificationPreferenceDTO `json:"preferences"`
}

// NotificationPreferencesResponse contains current-user preferences.
type NotificationPreferencesResponse struct {
	Data []NotificationPreferenceDTO `json:"data"`
}

// ListNotificationChannelsResponse is a paginated list of notification channels.
type ListNotificationChannelsResponse = PaginatedResponse[NotificationChannelResponse]

// FromNotificationChannel converts a notification channel to a redacted response.
func FromNotificationChannel(channel *domain.NotificationChannel) *NotificationChannelResponse {
	if channel == nil {
		return nil
	}

	return &NotificationChannelResponse{
		ID:        channel.ID,
		Name:      channel.Name,
		Type:      channel.Type,
		Config:    redactNotificationConfig(channel.Config),
		HasSecret: channel.SecretRef != "",
		IsEnabled: channel.IsEnabled,
		CreatedBy: channel.CreatedBy,
		CreatedAt: channel.CreatedAt,
		UpdatedAt: channel.UpdatedAt,
	}
}

// FromNotificationChannels converts notification channels to redacted responses.
func FromNotificationChannels(channels []domain.NotificationChannel) []NotificationChannelResponse {
	result := make([]NotificationChannelResponse, len(channels))
	for i, channel := range channels {
		resp := FromNotificationChannel(&channel)
		if resp != nil {
			result[i] = *resp
		}
	}

	return result
}

// FromNotificationPreferences converts preferences to DTOs.
func FromNotificationPreferences(preferences []domain.NotificationPreference) []NotificationPreferenceDTO {
	result := make([]NotificationPreferenceDTO, len(preferences))
	for i, pref := range preferences {
		result[i] = NotificationPreferenceDTO{
			EventType: pref.EventType,
			ChannelID: pref.ChannelID,
			IsEnabled: pref.IsEnabled,
		}
	}

	return result
}

func redactNotificationConfig(config domain.ConfigMap) domain.ConfigMap {
	redacted := domain.ConfigMap{}

	for key, value := range config {
		switch key {
		case "secret", "secret_ref", "webhook_url", "password":
			continue
		default:
			redacted[key] = value
		}
	}

	return redacted
}
