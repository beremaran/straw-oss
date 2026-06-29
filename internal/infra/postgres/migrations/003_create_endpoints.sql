-- +goose Up
-- +goose StatementBegin
CREATE TABLE endpoints
(
    id             TEXT PRIMARY KEY,            -- e.g., "endpoint-us-res-001"
    tags           JSONB NOT NULL DEFAULT '[]', -- capabilities: ["type:residential", "region:us"]
    last_heartbeat TIMESTAMPTZ,
    is_healthy     BOOLEAN        DEFAULT true,
    metadata       JSONB          DEFAULT '{}', -- version, IP, etc.
    created_at     TIMESTAMPTZ    DEFAULT now()
);

-- Index for efficient health-based queries
CREATE INDEX idx_endpoints_healthy ON endpoints (is_healthy) WHERE is_healthy;

-- Index for heartbeat-based health checks
CREATE INDEX idx_endpoints_heartbeat ON endpoints (last_heartbeat);

COMMENT
ON TABLE endpoints IS 'Registered endpoint workers';
COMMENT
ON COLUMN endpoints.id IS 'Unique endpoint identifier, e.g. "endpoint-us-res-001"';
COMMENT
ON COLUMN endpoints.tags IS 'Endpoint capabilities, e.g. ["type:residential", "region:us"]';
COMMENT
ON COLUMN endpoints.last_heartbeat IS 'Last heartbeat timestamp for health detection';
COMMENT
ON COLUMN endpoints.metadata IS 'Additional metadata like version, IP, etc.';
-- +goose StatementEnd

-- +goose Down
-- +goose StatementBegin
DROP INDEX IF EXISTS idx_endpoints_heartbeat;
DROP INDEX IF EXISTS idx_endpoints_healthy;
DROP TABLE IF EXISTS endpoints;
-- +goose StatementEnd
