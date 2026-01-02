-- +goose Up
-- +goose StatementBegin
-- Update rate limits for load testing
-- This migration increases the rate limits on the Default Rule to allow proper load testing

UPDATE routing_rules
SET config = jsonb_set(
    jsonb_set(
        config,
        '{rate_limit_per_second}',
        '2000'::jsonb
    ),
    '{rate_limit_per_minute}',
    '120000'::jsonb
)
WHERE name = 'Default Rule';
-- +goose StatementEnd

-- +goose Down
-- +goose StatementBegin
-- Revert rate limits to original values

UPDATE routing_rules
SET config = jsonb_set(
    jsonb_set(
        config,
        '{rate_limit_per_second}',
        '5'::jsonb
    ),
    '{rate_limit_per_minute}',
        '60'::jsonb
)
WHERE name = 'Default Rule';
-- +goose StatementEnd
