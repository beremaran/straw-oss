-- +goose Up
-- +goose StatementBegin
CREATE TABLE admin_users
(
    id             UUID PRIMARY KEY     DEFAULT gen_random_uuid(),
    email          TEXT        NOT NULL UNIQUE,
    display_name   TEXT        NOT NULL,
    password_hash  TEXT,
    is_active      BOOLEAN     NOT NULL DEFAULT true,
    is_super_admin BOOLEAN     NOT NULL DEFAULT false,
    last_login_at  TIMESTAMPTZ,
    created_at     TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at     TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE TABLE admin_roles
(
    id          UUID PRIMARY KEY     DEFAULT gen_random_uuid(),
    name        TEXT        NOT NULL UNIQUE,
    description TEXT,
    is_builtin  BOOLEAN     NOT NULL DEFAULT false,
    created_at  TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at  TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE TABLE admin_role_permissions
(
    role_id    UUID NOT NULL REFERENCES admin_roles (id) ON DELETE CASCADE,
    permission TEXT NOT NULL,
    PRIMARY KEY (role_id, permission)
);

CREATE TABLE admin_user_roles
(
    user_id UUID NOT NULL REFERENCES admin_users (id) ON DELETE CASCADE,
    role_id UUID NOT NULL REFERENCES admin_roles (id) ON DELETE CASCADE,
    PRIMARY KEY (user_id, role_id)
);

CREATE TABLE admin_sessions
(
    id                 UUID PRIMARY KEY     DEFAULT gen_random_uuid(),
    user_id            UUID        NOT NULL REFERENCES admin_users (id) ON DELETE CASCADE,
    refresh_token_hash TEXT        NOT NULL UNIQUE,
    user_agent         TEXT,
    ip                 TEXT,
    expires_at         TIMESTAMPTZ NOT NULL,
    revoked_at         TIMESTAMPTZ,
    created_at         TIMESTAMPTZ NOT NULL DEFAULT now(),
    last_used_at       TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE TABLE admin_identity_providers
(
    id                UUID PRIMARY KEY     DEFAULT gen_random_uuid(),
    name              TEXT        NOT NULL UNIQUE,
    type              TEXT        NOT NULL,
    issuer_url        TEXT,
    client_id         TEXT,
    client_secret_ref TEXT,
    jwks_url          TEXT,
    scopes            TEXT[] NOT NULL DEFAULT ARRAY['openid', 'email', 'profile'],
    role_claim        TEXT,
    default_role_id   UUID REFERENCES admin_roles (id),
    is_enabled        BOOLEAN     NOT NULL DEFAULT true,
    config            JSONB       NOT NULL DEFAULT '{}',
    created_at        TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at        TIMESTAMPTZ NOT NULL DEFAULT now(),
    CONSTRAINT admin_identity_providers_no_secret_config CHECK (
        NOT (config ? 'client_secret' OR config ? 'clientSecret' OR config ? 'secret')
        )
);

CREATE INDEX idx_admin_users_active ON admin_users (is_active) WHERE is_active;
CREATE INDEX idx_admin_role_permissions_permission ON admin_role_permissions (permission);
CREATE INDEX idx_admin_user_roles_role_id ON admin_user_roles (role_id);
CREATE INDEX idx_admin_sessions_user_id ON admin_sessions (user_id);
CREATE INDEX idx_admin_sessions_expires_at ON admin_sessions (expires_at);
CREATE INDEX idx_admin_identity_providers_enabled ON admin_identity_providers (is_enabled) WHERE is_enabled;

WITH built_in_roles(name, description) AS (VALUES ('Owner', 'All management permissions'),
                                                  ('Operator',
                                                   'Operational read/write except user management and cost multiplier write'),
                                                  ('Security auditor', 'Read-only management plus audit access'),
                                                  ('Finance', 'Usage, billing, and report execution'),
                                                  ('Read only', 'Read-only access to non-secret management resources'))
INSERT
INTO admin_roles (name, description, is_builtin)
SELECT name, description, true
FROM built_in_roles ON CONFLICT (name) DO
UPDATE
    SET description = EXCLUDED.description,
    is_builtin = true,
    updated_at = now();

WITH role_permissions(role_name, permission) AS (VALUES ('Owner', 'management:read'),
                                                        ('Owner', 'users:read'),
                                                        ('Owner', 'users:write'),
                                                        ('Owner', 'api_keys:read'),
                                                        ('Owner', 'api_keys:write'),
                                                        ('Owner', 'api_keys:rotate'),
                                                        ('Owner', 'api_keys:revoke'),
                                                        ('Owner', 'routing_rules:read'),
                                                        ('Owner', 'routing_rules:write'),
                                                        ('Owner', 'endpoints:read'),
                                                        ('Owner', 'endpoints:write'),
                                                        ('Owner', 'endpoints:control'),
                                                        ('Owner', 'endpoints:logs'),
                                                        ('Owner', 'fingerprints:read'),
                                                        ('Owner', 'fingerprints:write'),
                                                        ('Owner', 'fingerprints:delete'),
                                                        ('Owner', 'fingerprints:broadcast'),
                                                        ('Owner', 'usage:read'),
                                                        ('Owner', 'billing:read'),
                                                        ('Owner', 'cost_multipliers:read'),
                                                        ('Owner', 'cost_multipliers:write'),
                                                        ('Owner', 'audit:read'),
                                                        ('Owner', 'reports:read'),
                                                        ('Owner', 'reports:write'),
                                                        ('Owner', 'reports:run'),
                                                        ('Owner', 'alerts:read'),
                                                        ('Owner', 'alerts:write'),
                                                        ('Owner', 'notifications:write'),
                                                        ('Owner', 'cache:read'),
                                                        ('Owner', 'cache:write'),
                                                        ('Operator', 'management:read'),
                                                        ('Operator', 'api_keys:read'),
                                                        ('Operator', 'api_keys:write'),
                                                        ('Operator', 'api_keys:rotate'),
                                                        ('Operator', 'api_keys:revoke'),
                                                        ('Operator', 'routing_rules:read'),
                                                        ('Operator', 'routing_rules:write'),
                                                        ('Operator', 'endpoints:read'),
                                                        ('Operator', 'endpoints:write'),
                                                        ('Operator', 'endpoints:control'),
                                                        ('Operator', 'endpoints:logs'),
                                                        ('Operator', 'fingerprints:read'),
                                                        ('Operator', 'fingerprints:write'),
                                                        ('Operator', 'fingerprints:delete'),
                                                        ('Operator', 'fingerprints:broadcast'),
                                                        ('Operator', 'usage:read'),
                                                        ('Operator', 'billing:read'),
                                                        ('Operator', 'cost_multipliers:read'),
                                                        ('Operator', 'audit:read'),
                                                        ('Operator', 'reports:read'),
                                                        ('Operator', 'reports:write'),
                                                        ('Operator', 'reports:run'),
                                                        ('Operator', 'alerts:read'),
                                                        ('Operator', 'alerts:write'),
                                                        ('Operator', 'notifications:write'),
                                                        ('Operator', 'cache:read'),
                                                        ('Operator', 'cache:write'),
                                                        ('Security auditor', 'management:read'),
                                                        ('Security auditor', 'routing_rules:read'),
                                                        ('Security auditor', 'endpoints:read'),
                                                        ('Security auditor', 'endpoints:logs'),
                                                        ('Security auditor', 'fingerprints:read'),
                                                        ('Security auditor', 'usage:read'),
                                                        ('Security auditor', 'billing:read'),
                                                        ('Security auditor', 'cost_multipliers:read'),
                                                        ('Security auditor', 'audit:read'),
                                                        ('Security auditor', 'reports:read'),
                                                        ('Security auditor', 'alerts:read'),
                                                        ('Security auditor', 'cache:read'),
                                                        ('Finance', 'usage:read'),
                                                        ('Finance', 'billing:read'),
                                                        ('Finance', 'reports:read'),
                                                        ('Finance', 'reports:run'),
                                                        ('Read only', 'management:read'),
                                                        ('Read only', 'routing_rules:read'),
                                                        ('Read only', 'endpoints:read'),
                                                        ('Read only', 'endpoints:logs'),
                                                        ('Read only', 'fingerprints:read'),
                                                        ('Read only', 'usage:read'),
                                                        ('Read only', 'billing:read'),
                                                        ('Read only', 'cost_multipliers:read'),
                                                        ('Read only', 'reports:read'),
                                                        ('Read only', 'alerts:read'),
                                                        ('Read only', 'cache:read'))
INSERT
INTO admin_role_permissions (role_id, permission)
SELECT r.id, rp.permission
FROM role_permissions rp
         JOIN admin_roles r ON r.name = rp.role_name ON CONFLICT (role_id, permission) DO NOTHING;
-- +goose StatementEnd

-- +goose Down
-- +goose StatementBegin
DROP INDEX IF EXISTS idx_admin_identity_providers_enabled;
DROP INDEX IF EXISTS idx_admin_sessions_expires_at;
DROP INDEX IF EXISTS idx_admin_sessions_user_id;
DROP INDEX IF EXISTS idx_admin_user_roles_role_id;
DROP INDEX IF EXISTS idx_admin_role_permissions_permission;
DROP INDEX IF EXISTS idx_admin_users_active;
DROP TABLE IF EXISTS admin_identity_providers;
DROP TABLE IF EXISTS admin_sessions;
DROP TABLE IF EXISTS admin_user_roles;
DROP TABLE IF EXISTS admin_role_permissions;
DROP TABLE IF EXISTS admin_roles;
DROP TABLE IF EXISTS admin_users;
-- +goose StatementEnd
