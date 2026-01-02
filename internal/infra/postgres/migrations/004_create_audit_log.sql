-- +goose Up
-- +goose StatementBegin
CREATE TABLE audit_log (
    id BIGSERIAL PRIMARY KEY,
    entity_type TEXT NOT NULL,                  -- "routing_rule", "api_key", "endpoint"
    entity_id TEXT NOT NULL,
    action TEXT NOT NULL,                       -- "create", "update", "delete"
    actor TEXT,                                 -- who made the change
    old_value JSONB,
    new_value JSONB,
    created_at TIMESTAMPTZ DEFAULT now()
);

-- Index for efficient entity-based lookups
CREATE INDEX idx_audit_log_entity ON audit_log(entity_type, entity_id);

-- Index for time-based queries
CREATE INDEX idx_audit_log_created_at ON audit_log(created_at);

-- Index for actor-based queries
CREATE INDEX idx_audit_log_actor ON audit_log(actor) WHERE actor IS NOT NULL;

COMMENT ON TABLE audit_log IS 'Audit trail for all configuration changes';
COMMENT ON COLUMN audit_log.entity_type IS 'Type of entity: routing_rule, api_key, endpoint';
COMMENT ON COLUMN audit_log.entity_id IS 'ID of the modified entity';
COMMENT ON COLUMN audit_log.action IS 'Action performed: create, update, delete';
COMMENT ON COLUMN audit_log.actor IS 'User or system that made the change';
COMMENT ON COLUMN audit_log.old_value IS 'Previous value before change (null for create)';
COMMENT ON COLUMN audit_log.new_value IS 'New value after change (null for delete)';
-- +goose StatementEnd

-- +goose Down
-- +goose StatementBegin
DROP INDEX IF EXISTS idx_audit_log_actor;
DROP INDEX IF EXISTS idx_audit_log_created_at;
DROP INDEX IF EXISTS idx_audit_log_entity;
DROP TABLE IF EXISTS audit_log;
-- +goose StatementEnd
