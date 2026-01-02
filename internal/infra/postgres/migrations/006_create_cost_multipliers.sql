-- +goose Up
-- +goose StatementBegin
CREATE TABLE cost_multipliers (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    endpoint_tag TEXT NOT NULL UNIQUE,          -- e.g., "type:residential"
    multiplier DECIMAL(5,2) NOT NULL,           -- e.g., 10.00 (10x base cost)
    description TEXT,
    is_active BOOLEAN DEFAULT true
);

-- Index for efficient active multiplier lookups
CREATE INDEX idx_cost_multipliers_active ON cost_multipliers(is_active) WHERE is_active;

COMMENT ON TABLE cost_multipliers IS 'Cost multipliers for different endpoint types';
COMMENT ON COLUMN cost_multipliers.endpoint_tag IS 'Endpoint tag this multiplier applies to, e.g. "type:residential"';
COMMENT ON COLUMN cost_multipliers.multiplier IS 'Cost multiplier, e.g. 10.00 means 10x base cost';
-- +goose StatementEnd

-- +goose Down
-- +goose StatementBegin
DROP INDEX IF EXISTS idx_cost_multipliers_active;
DROP TABLE IF EXISTS cost_multipliers;
-- +goose StatementEnd
