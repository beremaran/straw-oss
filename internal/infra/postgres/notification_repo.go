package postgres

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/beremaran/straw/internal/domain"
)

// NotificationChannelRepository persists notification channels.
type NotificationChannelRepository struct {
	client *Client
}

// NewNotificationChannelRepository creates a NotificationChannelRepository.
func NewNotificationChannelRepository(client *Client) *NotificationChannelRepository {
	return &NotificationChannelRepository{client: client}
}

// Create inserts a notification channel.
func (r *NotificationChannelRepository) Create(ctx context.Context, channel *domain.NotificationChannel) error {
	configJSON, err := json.Marshal(channel.Config)
	if err != nil {
		return fmt.Errorf("marshal notification config: %w", err)
	}

	err = r.client.Execute(func() error {
		_, execErr := r.client.Pool.Exec(ctx, `
			INSERT INTO notification_channels (
				id, name, type, config, secret_ref, is_enabled, created_by, created_at, updated_at
			) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9)
		`,
			channel.ID,
			channel.Name,
			channel.Type,
			configJSON,
			nullableString(channel.SecretRef),
			channel.IsEnabled,
			nullableString(channel.CreatedBy),
			channel.CreatedAt,
			channel.UpdatedAt,
		)
		if execErr != nil {
			return fmt.Errorf("insert notification channel: %w", execErr)
		}

		return nil
	})
	if err != nil {
		return fmt.Errorf("create notification channel: %w", err)
	}

	return nil
}

// Update modifies a notification channel.
func (r *NotificationChannelRepository) Update(ctx context.Context, channel *domain.NotificationChannel) error {
	configJSON, err := json.Marshal(channel.Config)
	if err != nil {
		return fmt.Errorf("marshal notification config: %w", err)
	}

	var rows int64

	err = r.client.Execute(func() error {
		res, execErr := r.client.Pool.Exec(ctx, `
			UPDATE notification_channels
			SET name = $2,
				type = $3,
				config = $4,
				secret_ref = $5,
				is_enabled = $6,
				updated_at = $7
			WHERE id = $1
		`,
			channel.ID,
			channel.Name,
			channel.Type,
			configJSON,
			nullableString(channel.SecretRef),
			channel.IsEnabled,
			channel.UpdatedAt,
		)
		if execErr != nil {
			return fmt.Errorf("update notification channel: %w", execErr)
		}

		rows = res.RowsAffected()

		return nil
	})
	if err != nil {
		return fmt.Errorf("update notification channel: %w", err)
	}

	if rows == 0 {
		return domain.ErrNotificationChannelNotFound
	}

	return nil
}

// Disable marks a notification channel disabled.
func (r *NotificationChannelRepository) Disable(ctx context.Context, id string) error {
	var rows int64

	err := r.client.Execute(func() error {
		res, execErr := r.client.Pool.Exec(ctx, `
			UPDATE notification_channels
			SET is_enabled = false,
				updated_at = $2
			WHERE id = $1
		`, id, time.Now().UTC())
		if execErr != nil {
			return fmt.Errorf("disable notification channel: %w", execErr)
		}

		rows = res.RowsAffected()

		return nil
	})
	if err != nil {
		return fmt.Errorf("disable notification channel: %w", err)
	}

	if rows == 0 {
		return domain.ErrNotificationChannelNotFound
	}

	return nil
}

// GetByID returns a notification channel by ID.
func (r *NotificationChannelRepository) GetByID(ctx context.Context, id string) (*domain.NotificationChannel, error) {
	var channel domain.NotificationChannel

	err := r.client.Execute(func() error {
		return scanNotificationChannel(r.client.Pool.QueryRow(ctx, notificationChannelSelectSQL()+` WHERE id = $1`, id), &channel)
	})
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, nil
		}

		return nil, fmt.Errorf("get notification channel: %w", err)
	}

	return &channel, nil
}

// List returns notification channels.
func (r *NotificationChannelRepository) List(ctx context.Context, limit, offset int) ([]domain.NotificationChannel, int, error) {
	var total int

	err := r.client.Execute(func() error {
		return r.client.Pool.QueryRow(ctx, `SELECT COUNT(*) FROM notification_channels`).Scan(&total)
	})
	if err != nil {
		return nil, 0, fmt.Errorf("count notification channels: %w", err)
	}

	var rows pgx.Rows

	err = r.client.Execute(func() error {
		var queryErr error

		rows, queryErr = r.client.Pool.Query(ctx, notificationChannelSelectSQL()+` ORDER BY created_at DESC LIMIT $1 OFFSET $2`, limit, offset)
		if queryErr != nil {
			return fmt.Errorf("query notification channels: %w", queryErr)
		}

		return nil
	})
	if err != nil {
		return nil, 0, fmt.Errorf("list notification channels: %w", err)
	}
	defer rows.Close()

	var channels []domain.NotificationChannel

	for rows.Next() {
		var channel domain.NotificationChannel

		err = scanNotificationChannel(rows, &channel)
		if err != nil {
			return nil, 0, err
		}

		channels = append(channels, channel)
	}

	err = rows.Err()
	if err != nil {
		return nil, 0, fmt.Errorf("iterate notification channels: %w", err)
	}

	return channels, total, nil
}

// NotificationPreferenceRepository persists notification preferences.
type NotificationPreferenceRepository struct {
	client *Client
}

// NewNotificationPreferenceRepository creates a NotificationPreferenceRepository.
func NewNotificationPreferenceRepository(client *Client) *NotificationPreferenceRepository {
	return &NotificationPreferenceRepository{client: client}
}

// ListByUserID lists notification preferences for a user.
func (r *NotificationPreferenceRepository) ListByUserID(ctx context.Context, userID string) ([]domain.NotificationPreference, error) {
	rows, err := r.client.Pool.Query(ctx, `
		SELECT id, user_id, event_type, channel_id, is_enabled
		FROM notification_preferences
		WHERE user_id = $1
		ORDER BY event_type, channel_id
	`, userID)
	if err != nil {
		return nil, fmt.Errorf("list notification preferences: %w", err)
	}
	defer rows.Close()

	var prefs []domain.NotificationPreference

	for rows.Next() {
		var pref domain.NotificationPreference

		err = rows.Scan(&pref.ID, &pref.UserID, &pref.EventType, &pref.ChannelID, &pref.IsEnabled)
		if err != nil {
			return nil, fmt.Errorf("scan notification preference: %w", err)
		}

		prefs = append(prefs, pref)
	}

	err = rows.Err()
	if err != nil {
		return nil, fmt.Errorf("iterate notification preferences: %w", err)
	}

	return prefs, nil
}

// ReplaceForUser replaces all notification preferences for a user.
func (r *NotificationPreferenceRepository) ReplaceForUser(ctx context.Context, userID string, preferences []domain.NotificationPreference) error {
	tx, err := r.client.Pool.Begin(ctx)
	if err != nil {
		return fmt.Errorf("begin notification preference replace: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()

	_, err = tx.Exec(ctx, `DELETE FROM notification_preferences WHERE user_id = $1`, userID)
	if err != nil {
		return fmt.Errorf("delete notification preferences: %w", err)
	}

	for _, pref := range preferences {
		_, err = tx.Exec(ctx, `
			INSERT INTO notification_preferences (id, user_id, event_type, channel_id, is_enabled)
			VALUES ($1, $2, $3, $4, $5)
		`, pref.ID, userID, pref.EventType, pref.ChannelID, pref.IsEnabled)
		if err != nil {
			return fmt.Errorf("insert notification preference: %w", err)
		}
	}

	err = tx.Commit(ctx)
	if err != nil {
		return fmt.Errorf("commit notification preference replace: %w", err)
	}

	return nil
}

func notificationChannelSelectSQL() string {
	return `
		SELECT id, name, type, config, COALESCE(secret_ref, ''), is_enabled,
		       COALESCE(created_by::text, ''), created_at, updated_at
		FROM notification_channels
	`
}

func scanNotificationChannel(row scanner, channel *domain.NotificationChannel) error {
	var configJSON []byte

	err := row.Scan(
		&channel.ID,
		&channel.Name,
		&channel.Type,
		&configJSON,
		&channel.SecretRef,
		&channel.IsEnabled,
		&channel.CreatedBy,
		&channel.CreatedAt,
		&channel.UpdatedAt,
	)
	if err != nil {
		return fmt.Errorf("scan notification channel: %w", err)
	}

	if len(configJSON) > 0 {
		err = json.Unmarshal(configJSON, &channel.Config)
		if err != nil {
			return fmt.Errorf("unmarshal notification config: %w", err)
		}
	}

	if channel.Config == nil {
		channel.Config = domain.ConfigMap{}
	}

	return nil
}
