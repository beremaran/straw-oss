-- +goose Up
-- +goose StatementBegin
CREATE TABLE usage_daily_summary
(
    id             BIGSERIAL PRIMARY KEY,
    api_key_id     UUID           NOT NULL REFERENCES api_keys (id) ON DELETE CASCADE,
    date           DATE           NOT NULL,
    total_requests BIGINT         NOT NULL,
    total_bytes    BIGINT         NOT NULL,
    cost_units     DECIMAL(12, 4) NOT NULL, -- Weighted by multipliers
    breakdown      JSONB,                   -- {"residential": 1000, "datacenter": 5000}
    UNIQUE (api_key_id, date)
);

-- Index for date-range billing queries
CREATE INDEX idx_usage_daily_summary_date ON usage_daily_summary (date);

-- Index for API key monthly billing
CREATE INDEX idx_usage_daily_summary_api_key ON usage_daily_summary (api_key_id, date);

COMMENT
ON TABLE usage_daily_summary IS 'Pre-aggregated daily usage summaries for billing';
COMMENT
ON COLUMN usage_daily_summary.cost_units IS 'Total cost units weighted by endpoint type multipliers';
COMMENT
ON COLUMN usage_daily_summary.breakdown IS 'Request count breakdown by endpoint type';
-- +goose StatementEnd

-- +goose Down
-- +goose StatementBegin
DROP INDEX IF EXISTS idx_usage_daily_summary_api_key;
DROP INDEX IF EXISTS idx_usage_daily_summary_date;
DROP TABLE IF EXISTS usage_daily_summary;
-- +goose StatementEnd
