-- +goose Up
-- +goose StatementBegin
-- Seed data for development environment
-- This migration is optional and should only run in dev/test environments

-- Default cost multipliers for endpoint types
INSERT INTO cost_multipliers (endpoint_tag, multiplier, description, is_active) VALUES
    ('type:datacenter', 1.00, 'Datacenter proxies - base cost', true),
    ('type:residential', 10.00, 'Residential proxies - 10x base cost', true),
    ('type:mobile', 15.00, 'Mobile proxies - 15x base cost', true);

-- Sample routing rules for testing
INSERT INTO routing_rules (name, priority, required_tags, excluded_tags, config, is_active) VALUES
    (
        'Default Rule',
        0,
        '[]'::jsonb,
        '[]'::jsonb,
        '{
            "hard_timeout": 30000000000,
            "rate_limit_per_minute": 60,
            "rate_limit_per_second": 5,
            "allowed_endpoint_types": ["datacenter", "residential"],
            "fingerprint_preset": "chrome-130",
            "quota_key": "default"
        }'::jsonb,
        true
    ),
    (
        'Amazon Rule',
        100,
        '["target:amazon"]'::jsonb,
        '[]'::jsonb,
        '{
            "hard_timeout": 45000000000,
            "rate_limit_per_minute": 30,
            "rate_limit_per_second": 2,
            "allowed_endpoint_types": ["residential"],
            "required_endpoint_caps": ["capability:stealth"],
            "fingerprint_preset": "chrome-130",
            "quota_key": "target:amazon"
        }'::jsonb,
        true
    ),
    (
        'Google Search Rule',
        100,
        '["target:google", "type:search"]'::jsonb,
        '[]'::jsonb,
        '{
            "hard_timeout": 20000000000,
            "rate_limit_per_minute": 20,
            "rate_limit_per_second": 1,
            "allowed_endpoint_types": ["residential", "mobile"],
            "fingerprint_preset": "chrome-130",
            "quota_key": "target:google"
        }'::jsonb,
        true
    );

-- Sample API key for testing (hash of 'test-api-key-dev-only')
-- WARNING: Only use this in development! Generate real keys for production.
INSERT INTO api_keys (name, key_hash, scopes, rate_limit_override, is_active) VALUES
    (
        'Development Test Key',
        -- This is bcrypt hash of 'test-api-key-dev-only'
        '$2a$10$N9qo8uLOickgx2ZMRZoMyeIjZAgcfl7p92ldGxad68LJZdL17lhWy',
        '["*"]'::jsonb,
        1000,
        true
    );

-- Load test API key for k6 load testing
-- ID: 9d78136e-308b-49fd-967f-e62b9b91f1d8
-- Secret: load-test-secret
-- Format for use: 9d78136e-308b-49fd-967f-e62b9b91f1d8:load-test-secret
INSERT INTO api_keys (id, name, key_hash, scopes, rate_limit_override, is_active) VALUES
    (
        '9d78136e-308b-49fd-967f-e62b9b91f1d8',
        'Load Test Key',
        -- This is bcrypt hash of 'load-test-secret'
        '$2a$10$u3phQLkQ6/WX6N2L6Ba8Ie8OWtexbw54NzVJJQ8gYGYs5gZjB.QVi',
        '["*"]'::jsonb,
        10000,
        true
    );

-- Sample endpoints for testing
INSERT INTO endpoints (id, tags, last_heartbeat, is_healthy, metadata) VALUES
    (
        'endpoint-load-test-001',
        '["type:datacenter", "region:us"]'::jsonb,
        now(),
        true,
        '{"version": "1.0.0", "ip": "127.0.0.1"}'::jsonb
    ),
    (
        'endpoint-load-test-002',
        '["type:datacenter", "region:us"]'::jsonb,
        now(),
        true,
        '{"version": "1.0.0", "ip": "127.0.0.1"}'::jsonb
    ),
    (
        'endpoint-load-test-003',
        '["type:datacenter", "region:us"]'::jsonb,
        now(),
        true,
        '{"version": "1.0.0", "ip": "127.0.0.1"}'::jsonb
    );
-- +goose StatementEnd

-- +goose Down
-- +goose StatementBegin
-- Clean up seed data (in reverse order due to foreign keys)
DELETE FROM endpoints WHERE id LIKE 'endpoint-load-test-%';
DELETE FROM api_keys WHERE name IN ('Development Test Key', 'Load Test Key');
DELETE FROM routing_rules WHERE name IN ('Default Rule', 'Amazon Rule', 'Google Search Rule');
DELETE FROM cost_multipliers WHERE endpoint_tag IN ('type:datacenter', 'type:residential', 'type:mobile');
-- +goose StatementEnd
