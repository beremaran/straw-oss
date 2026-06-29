-- +goose Up
-- +goose StatementBegin
ALTER TABLE admin_audit_log
    ADD COLUMN actor_type TEXT,
    ADD COLUMN actor_id TEXT,
    ADD COLUMN actor_display_name TEXT,
    ADD COLUMN session_id TEXT,
    ADD COLUMN request_id TEXT,
    ADD COLUMN trace_id TEXT;

CREATE TABLE management_audit_events (
    id BIGSERIAL PRIMARY KEY,
    occurred_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    actor_type TEXT NOT NULL,
    actor_id TEXT,
    actor_display TEXT,
    action TEXT NOT NULL,
    entity_type TEXT NOT NULL,
    entity_id TEXT NOT NULL,
    old_value JSONB,
    new_value JSONB,
    request_id TEXT,
    trace_id TEXT,
    ip TEXT,
    user_agent TEXT
);

CREATE INDEX idx_management_audit_events_time ON management_audit_events(occurred_at DESC);
CREATE INDEX idx_management_audit_events_entity ON management_audit_events(entity_type, entity_id);
CREATE INDEX idx_management_audit_events_actor ON management_audit_events(actor_type, actor_id);
CREATE INDEX idx_management_audit_events_action ON management_audit_events(action);
-- +goose StatementEnd

-- +goose Down
-- +goose StatementBegin
DROP TABLE IF EXISTS management_audit_events;

ALTER TABLE admin_audit_log
    DROP COLUMN actor_type,
    DROP COLUMN actor_id,
    DROP COLUMN actor_display_name,
    DROP COLUMN session_id,
    DROP COLUMN request_id,
    DROP COLUMN trace_id;
-- +goose StatementEnd
