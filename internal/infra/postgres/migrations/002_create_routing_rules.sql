-- +goose Up
-- +goose StatementBegin
CREATE TABLE routing_rules
(
    id            UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    name          TEXT  NOT NULL,
    priority      INT   NOT NULL   DEFAULT 0,
    required_tags JSONB NOT NULL   DEFAULT '[]',
    excluded_tags JSONB            DEFAULT '[]',
    config        JSONB NOT NULL,            -- RoutingRule fields as JSON
    quota_key     TEXT,                      -- Rate limiting quota key
    is_active     BOOLEAN          DEFAULT true,
    created_at    TIMESTAMPTZ      DEFAULT now(),
    updated_at    TIMESTAMPTZ      DEFAULT now(),
    version       INT              DEFAULT 1 -- for optimistic locking
);

-- Index for efficient rule lookup by priority
CREATE INDEX idx_routing_rules_priority ON routing_rules (priority DESC) WHERE is_active;

-- Index for efficient lookup by active status
CREATE INDEX idx_routing_rules_active ON routing_rules (is_active) WHERE is_active;

COMMENT
ON TABLE routing_rules IS 'Tag-based routing rules for request matching';
COMMENT
ON COLUMN routing_rules.priority IS 'Higher priority rules are evaluated first';
COMMENT
ON COLUMN routing_rules.required_tags IS 'Request must have ALL these tags to match';
COMMENT
ON COLUMN routing_rules.excluded_tags IS 'Request must have NONE of these tags to match';
COMMENT
ON COLUMN routing_rules.config IS 'Full routing rule configuration as JSON';
COMMENT
ON COLUMN routing_rules.version IS 'Optimistic locking version for concurrent updates';
-- +goose StatementEnd

-- +goose Down
-- +goose StatementBegin
DROP INDEX IF EXISTS idx_routing_rules_active;
DROP INDEX IF EXISTS idx_routing_rules_priority;
DROP TABLE IF EXISTS routing_rules;
-- +goose StatementEnd
