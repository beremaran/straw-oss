-- +goose Up
-- +goose StatementBegin
ALTER TABLE endpoints
    ADD COLUMN desired_state TEXT NOT NULL DEFAULT 'active',
    ADD COLUMN is_registered BOOLEAN NOT NULL DEFAULT true,
    ADD COLUMN deleted_at TIMESTAMPTZ,
    ADD COLUMN updated_at TIMESTAMPTZ NOT NULL DEFAULT now();

CREATE TABLE endpoint_commands
(
    id           UUID PRIMARY KEY     DEFAULT gen_random_uuid(),
    endpoint_id  TEXT        NOT NULL,
    command      TEXT        NOT NULL,
    status       TEXT        NOT NULL,
    payload      JSONB       NOT NULL DEFAULT '{}',
    requested_by TEXT,
    requested_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    accepted_at  TIMESTAMPTZ,
    completed_at TIMESTAMPTZ,
    error        TEXT
);

CREATE INDEX idx_endpoint_commands_endpoint ON endpoint_commands (endpoint_id, requested_at DESC);
CREATE INDEX idx_endpoint_commands_status ON endpoint_commands (status, requested_at DESC);

CREATE TABLE endpoint_log_entries
(
    id          BIGSERIAL PRIMARY KEY,
    endpoint_id TEXT        NOT NULL,
    observed_at TIMESTAMPTZ NOT NULL,
    level       TEXT        NOT NULL,
    message     TEXT        NOT NULL,
    attrs       JSONB       NOT NULL DEFAULT '{}',
    trace_id    TEXT,
    request_id  TEXT
);

CREATE INDEX idx_endpoint_logs_endpoint_time ON endpoint_log_entries (endpoint_id, observed_at DESC);
CREATE INDEX idx_endpoint_logs_level_time ON endpoint_log_entries (level, observed_at DESC);
-- +goose StatementEnd

-- +goose Down
-- +goose StatementBegin
DROP INDEX IF EXISTS idx_endpoint_logs_level_time;
DROP INDEX IF EXISTS idx_endpoint_logs_endpoint_time;
DROP TABLE IF EXISTS endpoint_log_entries;

DROP INDEX IF EXISTS idx_endpoint_commands_status;
DROP INDEX IF EXISTS idx_endpoint_commands_endpoint;
DROP TABLE IF EXISTS endpoint_commands;

ALTER TABLE endpoints
DROP
COLUMN IF EXISTS desired_state,
    DROP
COLUMN IF EXISTS is_registered,
    DROP
COLUMN IF EXISTS deleted_at,
    DROP
COLUMN IF EXISTS updated_at;
-- +goose StatementEnd
