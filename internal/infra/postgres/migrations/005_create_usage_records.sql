-- +goose Up
-- +goose StatementBegin
CREATE TABLE usage_records
(
    id                BIGSERIAL PRIMARY KEY,
    api_key_id        UUID        NOT NULL REFERENCES api_keys (id) ON DELETE CASCADE,
    quota_key         TEXT        NOT NULL, -- e.g., "target:amazon"
    endpoint_tier     TEXT,                 -- "residential", "datacenter", "mobile"
    request_count     INT         NOT NULL DEFAULT 1,
    bytes_transferred BIGINT               DEFAULT 0,
    period_start      TIMESTAMPTZ NOT NULL, -- Hourly bucket start
    period_end        TIMESTAMPTZ NOT NULL,
    created_at        TIMESTAMPTZ          DEFAULT now()
);

-- Index for API key + time lookups (billing queries)
CREATE INDEX idx_usage_records_lookup ON usage_records (api_key_id, period_start);

-- Index for billing aggregation by period and quota key
CREATE INDEX idx_usage_records_billing ON usage_records (period_start, quota_key);

-- Index for endpoint tier analysis
CREATE INDEX idx_usage_records_tier ON usage_records (endpoint_tier, period_start);

COMMENT
ON TABLE usage_records IS 'Hourly usage records for billing and analytics';
COMMENT
ON COLUMN usage_records.quota_key IS 'Tag-derived quota key, e.g. "target:amazon"';
COMMENT
ON COLUMN usage_records.endpoint_tier IS 'Endpoint type: residential, datacenter, mobile';
COMMENT
ON COLUMN usage_records.period_start IS 'Start of the hourly bucket';
COMMENT
ON COLUMN usage_records.period_end IS 'End of the hourly bucket';
-- +goose StatementEnd

-- +goose Down
-- +goose StatementBegin
DROP INDEX IF EXISTS idx_usage_records_tier;
DROP INDEX IF EXISTS idx_usage_records_billing;
DROP INDEX IF EXISTS idx_usage_records_lookup;
DROP TABLE IF EXISTS usage_records;
-- +goose StatementEnd
