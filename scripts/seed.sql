-- Seed data for development environment
-- This script should be run manually or as part of a development setup process

-- Default cost multipliers for endpoint types
INSERT INTO cost_multipliers (endpoint_tag, multiplier, description, is_active) VALUES
    ('type:datacenter', 1.00, 'Datacenter proxies - base cost', true),
    ('type:residential', 10.00, 'Residential proxies - 10x base cost', true),
    ('type:mobile', 15.00, 'Mobile proxies - 15x base cost', true)
ON CONFLICT (endpoint_tag) DO NOTHING;

-- Sample routing rules for testing
INSERT INTO routing_rules (id, name, priority, required_tags, excluded_tags, config, is_active) VALUES
    (
        'a1b2c3d4-e5f6-7890-abcd-ef1234567890',
        'Default Rule',
        0,
        '[]'::jsonb,
        '[]'::jsonb,
        '{
            "hard_timeout": 30000000000,
            "rate_limit_per_minute": 10000000,
            "rate_limit_per_second": 10000000,
            "allowed_endpoint_types": ["datacenter", "residential"],
            "fingerprint_preset": "chrome-130",
            "quota_key": "default"
        }'::jsonb,
        true
    ),
    (
        'b2c3d4e5-f6a7-8901-bcde-f12345678901',
        'Amazon Rule',
        100,
        '["target:amazon"]'::jsonb,
        '[]'::jsonb,
        '{
            "hard_timeout": 45000000000,
            "rate_limit_per_minute": 10000000,
            "rate_limit_per_second": 10000000,
            "allowed_endpoint_types": ["residential"],
            "required_endpoint_caps": ["capability:stealth"],
            "fingerprint_preset": "chrome-130",
            "quota_key": "target:amazon"
        }'::jsonb,
        true
    ),
    (
        'c3d4e5f6-a7b8-9012-cdef-123456789012',
        'Google Search Rule',
        100,
        '["target:google", "type:search"]'::jsonb,
        '[]'::jsonb,
        '{
            "hard_timeout": 20000000000,
            "rate_limit_per_minute": 10000000,
            "rate_limit_per_second": 10000000,
            "allowed_endpoint_types": ["residential", "mobile"],
            "fingerprint_preset": "chrome-130",
            "quota_key": "target:google"
        }'::jsonb,
        true
    )
ON CONFLICT (id) DO NOTHING;

-- Sample API key for development testing
-- Token: dev-test-token-12345
-- Use with: Authorization: Bearer dev-test-token-12345
INSERT INTO api_keys (id, name, token_hash, scopes, rate_limit_override, is_active) VALUES
    (
        '8a7b6c5d-4e3f-2a1b-0c9d-8e7f6a5b4c3d',
        'Development Test Key',
        -- SHA256('dev-test-token-12345')
        'c6f458907e1575186d7d48e3b5862be5db8429dda3b2792d5d26c1e4912f8162',
        '["*"]'::jsonb,
        10000000,
        true
    )
ON CONFLICT (id) DO NOTHING;

-- Load test API key for k6 load testing
-- Token: load-test-token-67890
-- Use with: Authorization: Bearer load-test-token-67890
INSERT INTO api_keys (id, name, token_hash, scopes, rate_limit_override, is_active) VALUES
    (
        '9d78136e-308b-49fd-967f-e62b9b91f1d8',
        'Load Test Key',
        -- SHA256('load-test-token-67890')
        '50063e8d7406d15bfc0b2718b6f2de06ecc93b7fb252c28ecda6524cdcb38182',
        '["*"]'::jsonb,
        10000000,
        true
    )
ON CONFLICT (id) DO NOTHING;

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
    )
ON CONFLICT (id) DO NOTHING;
