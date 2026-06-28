-- Seed file for Straw Proxy

-- 1. Seed API Keys
-- Default bearer token is: default-token-123456
-- SHA256 token_hash: 92b1ea1b79f2321646e1393bdf05537374e4207257ee222c711be53250589e7a
INSERT INTO api_keys (id, token_hash, name, scopes, rate_limit_override, is_active, created_at, expires_at)
VALUES (
    'a0eebc99-9c0b-4ef8-bb6d-6bb9bd380a11',
    '92b1ea1b79f2321646e1393bdf05537374e4207257ee222c711be53250589e7a',
    'Default Administrator Key',
    '["target:*", "type:*", "region:*"]'::jsonb,
    100,
    true,
    now(),
    now() + interval '1 year'
)
ON CONFLICT (token_hash) DO NOTHING;

-- 2. Seed Routing Rules
INSERT INTO routing_rules (id, name, priority, required_tags, excluded_tags, config, quota_key, is_active, created_at, updated_at, version)
VALUES (
    'b0eebc99-9c0b-4ef8-bb6d-6bb9bd380a12',
    'Default Routing Rule (Residential US)',
    100,
    '["type:residential", "region:us"]'::jsonb,
    '[]'::jsonb,
    '{"mode":"tag_match","retry_limit":3}'::jsonb,
    'residential-us-quota',
    true,
    now(),
    now(),
    1
)
ON CONFLICT (id) DO NOTHING;

-- 3. Seed Cost Multipliers
INSERT INTO cost_multipliers (id, endpoint_tag, multiplier, description, is_active)
VALUES 
    ('c0eebc99-9c0b-4ef8-bb6d-6bb9bd380a13', 'type:residential', 10.00, '10x cost for residential proxy nodes', true),
    ('c0eebc99-9c0b-4ef8-bb6d-6bb9bd380a14', 'type:datacenter', 1.00, 'Base cost for datacenter nodes', true)
ON CONFLICT (endpoint_tag) DO NOTHING;

-- 4. Seed Fingerprint Presets
INSERT INTO fingerprint_presets (id, name, config, created_at, updated_at)
VALUES (
    'chrome-130',
    'Chrome 130 Preset',
    '{"user_agent": "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/130.0.0.0 Safari/537.36", "ja3": "771,4865-4866-4867-49195-49199-49196-49200-52393-52392-49171-49172,0-23-65281-10-11-35-16-5-13-18-51-45-43-21,29-23-24,0", "h2_settings": {"settings": {"1": 65536, "3": 1000, "4": 6291456, "6": 262144}, "connection_flow": 15663105}}'::jsonb,
    now(),
    now()
)
ON CONFLICT (id) DO NOTHING;
