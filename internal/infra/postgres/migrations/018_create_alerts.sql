-- +goose Up
-- +goose StatementBegin
CREATE TABLE alert_rules
(
    id                       UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    name                     TEXT             NOT NULL,
    description              TEXT,
    metric                   TEXT             NOT NULL,
    condition                TEXT             NOT NULL,
    threshold                DOUBLE PRECISION NOT NULL,
    evaluation_window        TEXT             NOT NULL,
    filters                  JSONB            NOT NULL DEFAULT '{}',
    severity                 TEXT             NOT NULL,
    is_active                BOOLEAN          NOT NULL DEFAULT true,
    cooldown                 TEXT             NOT NULL DEFAULT '15m',
    notification_channel_ids UUID[]           NOT NULL DEFAULT '{}',
    created_by               UUID REFERENCES admin_users (id),
    created_at               TIMESTAMPTZ      NOT NULL DEFAULT now(),
    updated_at               TIMESTAMPTZ      NOT NULL DEFAULT now()
);

CREATE TABLE alert_events
(
    id               UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    alert_rule_id    UUID        NOT NULL REFERENCES alert_rules (id) ON DELETE CASCADE,
    status           TEXT        NOT NULL,
    value            DOUBLE PRECISION,
    started_at       TIMESTAMPTZ NOT NULL DEFAULT now(),
    resolved_at      TIMESTAMPTZ,
    last_notified_at TIMESTAMPTZ,
    details          JSONB       NOT NULL DEFAULT '{}'
);

CREATE INDEX idx_alert_events_rule_status ON alert_events (alert_rule_id, status);
-- +goose StatementEnd

-- +goose Down
-- +goose StatementBegin
DROP INDEX IF EXISTS idx_alert_events_rule_status;
DROP TABLE IF EXISTS alert_events;
DROP TABLE IF EXISTS alert_rules;
-- +goose StatementEnd
