-- +goose Up
-- +goose StatementBegin
CREATE TABLE notification_channels
(
    id          UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    name        TEXT        NOT NULL,
    type        TEXT        NOT NULL,
    config      JSONB       NOT NULL DEFAULT '{}',
    secret_ref  TEXT,
    is_enabled  BOOLEAN     NOT NULL DEFAULT true,
    created_by  UUID REFERENCES admin_users (id),
    created_at  TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at  TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE TABLE notification_preferences
(
    id          UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    user_id     UUID REFERENCES admin_users (id) ON DELETE CASCADE,
    event_type  TEXT    NOT NULL,
    channel_id  UUID REFERENCES notification_channels (id) ON DELETE CASCADE,
    is_enabled  BOOLEAN NOT NULL DEFAULT true,
    UNIQUE (user_id, event_type, channel_id)
);

CREATE INDEX idx_notification_preferences_user_id ON notification_preferences (user_id);
-- +goose StatementEnd

-- +goose Down
-- +goose StatementBegin
DROP INDEX IF EXISTS idx_notification_preferences_user_id;
DROP TABLE IF EXISTS notification_preferences;
DROP TABLE IF EXISTS notification_channels;
-- +goose StatementEnd
