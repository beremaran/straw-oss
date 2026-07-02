-- straw P0 initial Postgres schema.

CREATE TABLE IF NOT EXISTS tenants (
    id uuid PRIMARY KEY,
    status text NOT NULL DEFAULT 'active' CHECK (status IN ('active', 'disabled', 'deleted')),
    soft_deleted_at timestamptz,
    created_at timestamptz NOT NULL DEFAULT now(),
    updated_at timestamptz NOT NULL DEFAULT now()
);

CREATE TABLE IF NOT EXISTS tenant_config_versions (
    tenant_id uuid PRIMARY KEY REFERENCES tenants(id),
    config_version bigint NOT NULL DEFAULT 0,
    created_at timestamptz NOT NULL DEFAULT now(),
    updated_at timestamptz NOT NULL DEFAULT now()
);

CREATE TABLE IF NOT EXISTS api_keys (
    id uuid PRIMARY KEY,
    scope_type text NOT NULL CHECK (scope_type IN ('platform', 'tenant')),
    tenant_id uuid REFERENCES tenants(id),
    role text NOT NULL CHECK (role IN ('system_admin', 'requester', 'viewer', 'operator', 'tenant_admin')),
    prefix text NOT NULL,
    secret_hash text NOT NULL,
    status text NOT NULL DEFAULT 'active' CHECK (status IN ('active', 'revoked')),
    created_at timestamptz NOT NULL DEFAULT now(),
    revoked_at timestamptz,
    config_version bigint NOT NULL DEFAULT 0,
    CHECK (
        (scope_type = 'platform' AND tenant_id IS NULL)
        OR (scope_type = 'tenant' AND tenant_id IS NOT NULL)
    )
);

CREATE INDEX IF NOT EXISTS api_keys_prefix_idx ON api_keys (prefix);
CREATE INDEX IF NOT EXISTS api_keys_tenant_prefix_idx ON api_keys (tenant_id, prefix) WHERE tenant_id IS NOT NULL;

CREATE TABLE IF NOT EXISTS worker_credentials (
    credential_id uuid PRIMARY KEY,
    status text NOT NULL DEFAULT 'active' CHECK (status IN ('active', 'revoked')),
    executor_type text NOT NULL DEFAULT 'egress' CHECK (executor_type IN ('egress', 'provider_adapter')),
    public_key_ed25519_base64 text NOT NULL,
    tenant_scope_jsonb jsonb NOT NULL DEFAULT '{}'::jsonb,
    allowed_pools_jsonb jsonb NOT NULL DEFAULT '[]'::jsonb,
    created_at timestamptz NOT NULL DEFAULT now(),
    updated_at timestamptz NOT NULL DEFAULT now(),
    config_version bigint NOT NULL DEFAULT 0
);

CREATE TABLE IF NOT EXISTS executor_pools (
    tenant_id uuid NOT NULL REFERENCES tenants(id),
    id text NOT NULL,
    executor_type text NOT NULL DEFAULT 'egress' CHECK (executor_type IN ('egress', 'provider_adapter')),
    tags_jsonb jsonb NOT NULL DEFAULT '[]'::jsonb,
    metadata_jsonb jsonb NOT NULL DEFAULT '{}'::jsonb,
    enabled boolean NOT NULL DEFAULT true,
    created_at timestamptz NOT NULL DEFAULT now(),
    updated_at timestamptz NOT NULL DEFAULT now(),
    config_version bigint NOT NULL DEFAULT 0,
    PRIMARY KEY (tenant_id, id)
);

CREATE TABLE IF NOT EXISTS worker_admin_state (
    worker_id text PRIMARY KEY,
    disabled boolean NOT NULL DEFAULT false,
    disabled_reason text,
    created_at timestamptz NOT NULL DEFAULT now(),
    updated_at timestamptz NOT NULL DEFAULT now(),
    config_version bigint NOT NULL DEFAULT 0
);

CREATE TABLE IF NOT EXISTS tenant_worker_admin_state (
    tenant_id uuid NOT NULL REFERENCES tenants(id),
    worker_id text NOT NULL,
    disabled boolean NOT NULL DEFAULT false,
    disabled_reason text,
    created_at timestamptz NOT NULL DEFAULT now(),
    updated_at timestamptz NOT NULL DEFAULT now(),
    config_version bigint NOT NULL DEFAULT 0,
    PRIMARY KEY (tenant_id, worker_id)
);

CREATE TABLE IF NOT EXISTS routing_rules (
    tenant_id uuid NOT NULL REFERENCES tenants(id),
    id text NOT NULL,
    priority integer NOT NULL,
    enabled boolean NOT NULL DEFAULT true,
    match_conditions_jsonb jsonb NOT NULL DEFAULT '{}'::jsonb,
    target_pool_id text NOT NULL,
    sticky_session_ttl_seconds integer,
    allow_sticky_fallback boolean NOT NULL DEFAULT false,
    created_at timestamptz NOT NULL DEFAULT now(),
    updated_at timestamptz NOT NULL DEFAULT now(),
    config_version bigint NOT NULL DEFAULT 0,
    PRIMARY KEY (tenant_id, id)
);

CREATE INDEX IF NOT EXISTS routing_rules_tenant_priority_idx ON routing_rules (tenant_id, priority);
CREATE INDEX IF NOT EXISTS routing_rules_tenant_target_pool_idx ON routing_rules (tenant_id, target_pool_id);

CREATE TABLE IF NOT EXISTS deny_rules (
    tenant_id uuid NOT NULL REFERENCES tenants(id),
    id text NOT NULL,
    rule_type text NOT NULL CHECK (rule_type IN ('host', 'cidr', 'cname', 'ip')),
    action text NOT NULL CHECK (action IN ('deny', 'allow')),
    enabled boolean NOT NULL DEFAULT true,
    raw_pattern text NOT NULL,
    normalized_host text,
    normalized_cidr cidr,
    normalized_ip inet,
    normalized_cname text,
    created_at timestamptz NOT NULL DEFAULT now(),
    updated_at timestamptz NOT NULL DEFAULT now(),
    config_version bigint NOT NULL DEFAULT 0,
    PRIMARY KEY (tenant_id, id),
    CHECK (
        normalized_host IS NOT NULL
        OR normalized_cidr IS NOT NULL
        OR normalized_ip IS NOT NULL
        OR normalized_cname IS NOT NULL
    )
);

CREATE INDEX IF NOT EXISTS deny_rules_tenant_rule_type_idx ON deny_rules (tenant_id, rule_type);
CREATE INDEX IF NOT EXISTS deny_rules_tenant_normalized_host_idx ON deny_rules (tenant_id, normalized_host) WHERE normalized_host IS NOT NULL;
CREATE INDEX IF NOT EXISTS deny_rules_tenant_normalized_cidr_idx ON deny_rules (tenant_id, normalized_cidr) WHERE normalized_cidr IS NOT NULL;

CREATE TABLE IF NOT EXISTS fingerprint_profiles (
    tenant_id uuid REFERENCES tenants(id),
    name text NOT NULL,
    scope_type text NOT NULL CHECK (scope_type IN ('global', 'tenant')),
    supported_by_worker boolean NOT NULL DEFAULT true,
    enabled boolean NOT NULL DEFAULT true,
    profile_jsonb jsonb NOT NULL DEFAULT '{}'::jsonb,
    created_at timestamptz NOT NULL DEFAULT now(),
    updated_at timestamptz NOT NULL DEFAULT now(),
    config_version bigint NOT NULL DEFAULT 0,
    CHECK (
        (scope_type = 'global' AND tenant_id IS NULL)
        OR (scope_type = 'tenant' AND tenant_id IS NOT NULL)
    )
);

CREATE UNIQUE INDEX IF NOT EXISTS fingerprint_profiles_global_name_uidx
    ON fingerprint_profiles (name)
    WHERE scope_type = 'global';

CREATE UNIQUE INDEX IF NOT EXISTS fingerprint_profiles_tenant_name_uidx
    ON fingerprint_profiles (tenant_id, name)
    WHERE scope_type = 'tenant';

CREATE TABLE IF NOT EXISTS injection_policies (
    tenant_id uuid NOT NULL REFERENCES tenants(id),
    id text NOT NULL,
    enabled boolean NOT NULL DEFAULT true,
    operations jsonb NOT NULL DEFAULT '[]'::jsonb,
    audit_redacted boolean NOT NULL DEFAULT true,
    created_at timestamptz NOT NULL DEFAULT now(),
    updated_at timestamptz NOT NULL DEFAULT now(),
    config_version bigint NOT NULL DEFAULT 0,
    PRIMARY KEY (tenant_id, id),
    CHECK (jsonb_typeof(operations) = 'array'),
    CHECK (jsonb_array_length(operations) <= 32)
);

CREATE TABLE IF NOT EXISTS rate_limit_configs (
    tenant_id uuid NOT NULL REFERENCES tenants(id),
    dimension text NOT NULL,
    key text NOT NULL,
    enabled boolean NOT NULL DEFAULT true,
    window_ms integer NOT NULL,
    max_entries integer NOT NULL DEFAULT 10000,
    max_keys_per_tenant integer NOT NULL DEFAULT 1000,
    memory_guardrail_bytes bigint,
    fail_policy text NOT NULL DEFAULT 'fail_open' CHECK (fail_policy IN ('fail_open', 'fail_closed')),
    limit_count bigint NOT NULL,
    created_at timestamptz NOT NULL DEFAULT now(),
    updated_at timestamptz NOT NULL DEFAULT now(),
    config_version bigint NOT NULL DEFAULT 0,
    PRIMARY KEY (tenant_id, dimension, key)
);

CREATE TABLE IF NOT EXISTS quota_configs (
    tenant_id uuid NOT NULL REFERENCES tenants(id),
    quota_period text NOT NULL CHECK (quota_period IN ('monthly')),
    enabled boolean NOT NULL DEFAULT true,
    request_count_limit bigint,
    bandwidth_bytes_limit bigint,
    count_on_admission boolean NOT NULL DEFAULT true,
    fail_policy text NOT NULL DEFAULT 'fail_closed' CHECK (fail_policy IN ('fail_open', 'fail_closed')),
    created_at timestamptz NOT NULL DEFAULT now(),
    updated_at timestamptz NOT NULL DEFAULT now(),
    config_version bigint NOT NULL DEFAULT 0,
    PRIMARY KEY (tenant_id, quota_period)
);

CREATE TABLE IF NOT EXISTS config_audit_source (
    id bigint GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    tenant_id uuid REFERENCES tenants(id),
    actor_type text NOT NULL,
    actor_id text NOT NULL,
    resource_type text NOT NULL,
    resource_id text NOT NULL,
    action text NOT NULL,
    request_id text,
    old_value_json jsonb,
    new_value_json jsonb,
    created_at timestamptz NOT NULL DEFAULT now(),
    CHECK (old_value_json IS NULL OR jsonb_typeof(old_value_json) IN ('object', 'array')),
    CHECK (new_value_json IS NULL OR jsonb_typeof(new_value_json) IN ('object', 'array'))
);

CREATE INDEX IF NOT EXISTS config_audit_source_tenant_created_at_idx ON config_audit_source (tenant_id, created_at DESC);
CREATE INDEX IF NOT EXISTS config_audit_source_actor_idx ON config_audit_source (actor_type, actor_id, created_at DESC);
