package domain

import (
	"context"
	"errors"
	"time"
)

const (
	// NotificationChannelWebhook posts generic JSON webhooks.
	NotificationChannelWebhook = "webhook"
	// NotificationChannelEmail represents email delivery.
	NotificationChannelEmail = "email"
	// NotificationChannelSlackWebhook posts Slack-compatible JSON webhooks.
	NotificationChannelSlackWebhook = "slack_webhook"
)

// ErrNotificationChannelNotFound is returned when a notification channel does not exist.
var ErrNotificationChannelNotFound = errors.New("notification channel not found")

// NotificationChannel stores a delivery channel.
type NotificationChannel struct {
	ID        string
	Name      string
	Type      string
	Config    ConfigMap
	SecretRef string
	IsEnabled bool
	CreatedBy string
	CreatedAt time.Time
	UpdatedAt time.Time
}

// NotificationPreference stores a user's preference for an event/channel pair.
type NotificationPreference struct {
	ID        string
	UserID    string
	EventType string
	ChannelID string
	IsEnabled bool
}

// NotificationChannelRepository provides persistence for notification channels.
type NotificationChannelRepository interface {
	Create(ctx context.Context, channel *NotificationChannel) error
	Update(ctx context.Context, channel *NotificationChannel) error
	Disable(ctx context.Context, id string) error
	GetByID(ctx context.Context, id string) (*NotificationChannel, error)
	List(ctx context.Context, limit, offset int) ([]NotificationChannel, int, error)
}

// NotificationPreferenceRepository provides persistence for notification preferences.
type NotificationPreferenceRepository interface {
	ListByUserID(ctx context.Context, userID string) ([]NotificationPreference, error)
	ReplaceForUser(ctx context.Context, userID string, preferences []NotificationPreference) error
}
