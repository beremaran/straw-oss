CREATE TABLE IF NOT EXISTS payload_capture_policies (
    tenant_id uuid PRIMARY KEY REFERENCES tenants(id),
    enabled boolean NOT NULL DEFAULT false,
    allowed_decisions_jsonb jsonb NOT NULL DEFAULT '["none"]'::jsonb,
    created_at timestamptz NOT NULL DEFAULT now(),
    updated_at timestamptz NOT NULL DEFAULT now(),
    config_version bigint NOT NULL DEFAULT 0,
    CHECK (jsonb_typeof(allowed_decisions_jsonb) = 'array')
);
