# Management Backend Specification

Status: ready for implementation planning

Last verified against the repository on 2026-06-29.

This document specifies the backend work required to support the Management UI capabilities that are listed as "Out Of Scope Until Backend Support Exists" in `docs/management-ui-spec.md`.

## Source Coverage

The spec was researched and cross-checked against:

- `docs/management-ui-spec.md`, especially the out-of-scope backend dependency list.
- `internal/server/admin/server.go` for the currently registered Management API routes.
- `internal/server/admin/middleware/auth.go` and `internal/server/admin/middleware/audit.go` for current management authentication and audit write behavior.
- `internal/server/admin/handlers/*.go` for current CRUD and operation semantics.
- `internal/domain/*.go` and `internal/server/dto/*.go` for current data contracts.
- `internal/infra/postgres/migrations/*.sql` for existing persisted tables.
- `internal/infra/postgres/*_repo.go` for current repository coverage.
- `internal/infra/redis/endpoint_health.go` and `internal/service/endpoint/health.go` for live endpoint state, drain state, and heartbeat behavior.
- `pkg/endpoint/worker.go`, `pkg/endpoint/heartbeat.go`, and `internal/endpoint/update/*` for worker lifecycle, heartbeat, and restart/update primitives.

## Objective

Add backend capabilities for:

- User accounts, SSO, per-role permissions, and session refresh.
- Audit-log viewer read APIs.
- API key update, rotation, reactivation, expiration editing, and detail APIs.
- Endpoint creation, deletion, undrain, restart, and live log viewing.
- Fingerprint deletion.
- Cost multiplier management.
- Saved reports, scheduled exports, alerts, and notification preferences.

The current Management API must continue to work during migration. Existing clients using `MANAGEMENT_API_KEY` should have a compatibility path until the new identity system is enabled and operators have created at least one administrator.

## Requirement Coverage Matrix

| UI spec missing backend item | Backend coverage in this spec |
| --- | --- |
| User accounts, SSO, per-role permissions, and session refresh | Identity, SSO, Roles, And Sessions; Cross-Cutting API Conventions |
| Audit-log viewer | Audit Viewer And Structured Audit |
| API key update, rotation, reactivation, expiration editing, and scoped key detail | API Key Lifecycle |
| Endpoint creation, deletion, undrain, restart, and live log viewing | Endpoint Registry, Control, And Logs |
| Fingerprint deletion | Fingerprint Deletion |
| Cost multiplier management | Cost Multiplier Management |
| Saved reports, scheduled exports, alerts, and notification preferences | Saved Reports, Scheduled Exports, Alerts, And Notifications |

## Current Backend Baseline

| Area | Current support | Gap |
| --- | --- | --- |
| Management auth | Static `MANAGEMENT_API_KEY` Bearer token checked on every `/management/*` request | No users, SSO, roles, refresh sessions, or actor identity |
| Admin audit | Non-GET requests are written to `admin_audit_log` | No read API, no actor ID, no structured entity fields, request body may contain sensitive data |
| Entity audit | `audit_log` table exists | Current handlers do not consistently write structured entity audit events |
| API keys | List, create, and revoke by setting `is_active=false` | No detail route, update, rotate, reactivate, expiration edit, or multiple token secrets per key |
| Endpoints | Live health list and drain flag in Redis | No persistent registry management, undrain, restart command, delete, detail route, logs, or command acknowledgements |
| Fingerprints | List and upsert by `POST /management/fingerprints`; repository has `DeletePreset` | No HTTP delete route or rule dependency protection |
| Cost multipliers | `cost_multipliers` table exists | No repository, handlers, OpenAPI routes, audit events, or billing integration controls |
| Reports and alerts | Usage and billing read endpoints | No saved report definitions, scheduled exports, alert rules, notification channels, or preferences |

## Design Principles

- Preserve compatibility with existing Management API routes unless replacing them is explicitly called out.
- Keep logical entities stable. For example, rotating an API key changes token secrets under the same API key ID.
- Treat mutating management operations as audited domain events with actor, entity type, entity ID, old value, and new value.
- Prefer explicit command records for asynchronous endpoint operations. A successful HTTP response means "command accepted" unless the endpoint acknowledgement proves completion.
- Store secrets only as hashes after the one-time creation or rotation response.
- Keep query APIs paginated, filterable, and export-friendly from the start.
- Avoid UI-only state for backend facts such as roles, schedules, alert delivery attempts, or endpoint command status.

## Cross-Cutting API Conventions

### Authentication Modes

Support two modes during rollout:

| Mode | Use | Behavior |
| --- | --- | --- |
| Legacy token | Existing deployments and bootstrap | `Authorization: Bearer <MANAGEMENT_API_KEY>` maps to a synthetic `system:legacy-admin` actor with full permissions |
| User session | New Management UI | Access token plus refresh token issued by the backend; actor is the authenticated admin user |

The legacy mode can be disabled later with `MANAGEMENT_LEGACY_TOKEN_ENABLED=false`, but it should default to enabled until migration documentation is complete.

### Response Shapes

Use the existing error shape:

```json
{
  "error": "human readable message",
  "code": "stable_error_code",
  "details": {}
}
```

List responses should keep the existing paginated shape:

```json
{
  "data": [],
  "total": 0,
  "page": 1,
  "limit": 20
}
```

For cursor-friendly high-volume logs, use:

```json
{
  "data": [],
  "next_cursor": "opaque",
  "has_more": false
}
```

### Permissions

Permission names should be explicit strings:

| Permission | Scope |
| --- | --- |
| `management:read` | Overview and non-sensitive read access |
| `users:read` / `users:write` | Admin user and role management |
| `api_keys:read` / `api_keys:write` / `api_keys:rotate` / `api_keys:revoke` | API key lifecycle |
| `routing_rules:read` / `routing_rules:write` | Existing route management |
| `endpoints:read` / `endpoints:write` / `endpoints:control` / `endpoints:logs` | Endpoint registry, command, and logs |
| `fingerprints:read` / `fingerprints:write` / `fingerprints:delete` / `fingerprints:broadcast` | Fingerprint lifecycle |
| `usage:read` / `billing:read` | Usage and billing |
| `cost_multipliers:read` / `cost_multipliers:write` | Cost configuration |
| `audit:read` | Audit viewer |
| `reports:read` / `reports:write` / `reports:run` | Saved and scheduled reporting |
| `alerts:read` / `alerts:write` / `notifications:write` | Alerts and delivery settings |
| `cache:read` / `cache:write` | Cache stats and clear |

Built-in roles:

| Role | Permissions |
| --- | --- |
| Owner | All permissions |
| Operator | Operational read/write except user management and cost multiplier write |
| Security auditor | Read-only management plus `audit:read` |
| Finance | `usage:read`, `billing:read`, `reports:read`, `reports:run` |
| Read only | `management:read` and read permissions for non-secret resources |

## Identity, SSO, Roles, And Sessions

### Data Model

Add migrations for:

```sql
CREATE TABLE admin_users (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    email TEXT NOT NULL UNIQUE,
    display_name TEXT NOT NULL,
    password_hash TEXT,
    is_active BOOLEAN NOT NULL DEFAULT true,
    is_super_admin BOOLEAN NOT NULL DEFAULT false,
    last_login_at TIMESTAMPTZ,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE TABLE admin_roles (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    name TEXT NOT NULL UNIQUE,
    description TEXT,
    is_builtin BOOLEAN NOT NULL DEFAULT false,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE TABLE admin_role_permissions (
    role_id UUID NOT NULL REFERENCES admin_roles(id) ON DELETE CASCADE,
    permission TEXT NOT NULL,
    PRIMARY KEY (role_id, permission)
);

CREATE TABLE admin_user_roles (
    user_id UUID NOT NULL REFERENCES admin_users(id) ON DELETE CASCADE,
    role_id UUID NOT NULL REFERENCES admin_roles(id) ON DELETE CASCADE,
    PRIMARY KEY (user_id, role_id)
);

CREATE TABLE admin_sessions (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    user_id UUID NOT NULL REFERENCES admin_users(id) ON DELETE CASCADE,
    refresh_token_hash TEXT NOT NULL UNIQUE,
    user_agent TEXT,
    ip TEXT,
    expires_at TIMESTAMPTZ NOT NULL,
    revoked_at TIMESTAMPTZ,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    last_used_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE TABLE admin_identity_providers (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    name TEXT NOT NULL UNIQUE,
    type TEXT NOT NULL,
    issuer_url TEXT,
    client_id TEXT,
    client_secret_ref TEXT,
    jwks_url TEXT,
    scopes TEXT[] NOT NULL DEFAULT ARRAY['openid','email','profile'],
    role_claim TEXT,
    default_role_id UUID REFERENCES admin_roles(id),
    is_enabled BOOLEAN NOT NULL DEFAULT true,
    config JSONB NOT NULL DEFAULT '{}',
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now()
);
```

`client_secret_ref` should point to Vault or an encrypted secret store when available. Do not store plaintext provider secrets in JSON config.

### Auth Endpoints

| Method | Path | Permission | Purpose |
| --- | --- | --- | --- |
| `POST` | `/management/auth/login` | Public | Local email/password login |
| `POST` | `/management/auth/refresh` | Refresh token | Rotate refresh token and issue a new access token |
| `POST` | `/management/auth/logout` | Authenticated | Revoke current session |
| `GET` | `/management/auth/me` | Authenticated | Current user, roles, permissions, and session metadata |
| `GET` | `/management/auth/sso/{provider}/start` | Public | Start OIDC authorization-code flow |
| `GET` | `/management/auth/sso/{provider}/callback` | Public | Complete SSO flow and issue session |
| `GET` | `/management/users` | `users:read` | List admin users |
| `POST` | `/management/users` | `users:write` | Create admin user |
| `GET` | `/management/users/{id}` | `users:read` | User details |
| `PATCH` | `/management/users/{id}` | `users:write` | Update display name, active state, roles |
| `DELETE` | `/management/users/{id}` | `users:write` | Deactivate user |
| `GET` | `/management/roles` | `users:read` | List roles and permissions |
| `POST` | `/management/roles` | `users:write` | Create role |
| `PATCH` | `/management/roles/{id}` | `users:write` | Update role |
| `DELETE` | `/management/roles/{id}` | `users:write` | Delete non-built-in role |
| `GET` | `/management/identity-providers` | `users:read` | List SSO providers |
| `POST` | `/management/identity-providers` | `users:write` | Create SSO provider |
| `PATCH` | `/management/identity-providers/{id}` | `users:write` | Update SSO provider |
| `DELETE` | `/management/identity-providers/{id}` | `users:write` | Disable/delete SSO provider |

### Token Requirements

- Access tokens should be short-lived, default 15 minutes.
- Refresh tokens should be opaque random values, stored as SHA256 hashes.
- Refresh token rotation is required. Using an old refresh token after rotation revokes the session family.
- Local passwords should be hashed with Argon2id or bcrypt. Prefer Argon2id if adding a new dependency is acceptable.
- Access tokens must include actor ID, session ID, and permissions or a permissions version.
- Middleware must set actor context for handlers and audit logging.

### Bootstrap

- If no `admin_users` rows exist, legacy token users with full permissions may create the first owner through `POST /management/users/bootstrap`.
- Bootstrap route requires the legacy management token and is disabled once an active owner exists.
- Startup logs should warn when no owner exists.

## Audit Viewer And Structured Audit

### Data Model

Keep `admin_audit_log` for raw HTTP request audit, but add actor fields and structured event support:

```sql
ALTER TABLE admin_audit_log
    ADD COLUMN actor_type TEXT,
    ADD COLUMN actor_id TEXT,
    ADD COLUMN request_id TEXT,
    ADD COLUMN trace_id TEXT;

CREATE TABLE management_audit_events (
    id BIGSERIAL PRIMARY KEY,
    occurred_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    actor_type TEXT NOT NULL,
    actor_id TEXT,
    actor_display TEXT,
    action TEXT NOT NULL,
    entity_type TEXT NOT NULL,
    entity_id TEXT NOT NULL,
    old_value JSONB,
    new_value JSONB,
    request_id TEXT,
    trace_id TEXT,
    ip TEXT,
    user_agent TEXT
);

CREATE INDEX idx_management_audit_events_time ON management_audit_events(occurred_at DESC);
CREATE INDEX idx_management_audit_events_entity ON management_audit_events(entity_type, entity_id);
CREATE INDEX idx_management_audit_events_actor ON management_audit_events(actor_type, actor_id);
CREATE INDEX idx_management_audit_events_action ON management_audit_events(action);
```

The existing `audit_log` table can be migrated into `management_audit_events` or kept as a compatibility table. The preferred target is one structured audit event table.

### Endpoints

| Method | Path | Permission | Purpose |
| --- | --- | --- | --- |
| `GET` | `/management/audit/events` | `audit:read` | Structured domain events with filters |
| `GET` | `/management/audit/events/{id}` | `audit:read` | Event detail including diffs |
| `GET` | `/management/audit/requests` | `audit:read` | Raw management HTTP request log |
| `GET` | `/management/audit/export` | `audit:read` | CSV or NDJSON export for a bounded date range |

Filters:

- `start`, `end`.
- `actor_id`.
- `actor_type`.
- `action`.
- `entity_type`.
- `entity_id`.
- `status_min`, `status_max` for request audit.
- `q` for path, actor display, or entity ID contains.
- `page`, `limit`, with maximum 500 for audit logs.

Security:

- Redact `raw_key`, `token`, `password`, `client_secret`, `Authorization`, and cookie-like fields before writing audit rows.
- Never return full request bodies by default. `include_body=true` should require Owner or Security auditor and still return redacted bodies.

## API Key Lifecycle

### Data Model

Current `api_keys` mixes logical key metadata with one active token hash. Add token-secret history:

```sql
CREATE TABLE api_key_tokens (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    api_key_id UUID NOT NULL REFERENCES api_keys(id) ON DELETE CASCADE,
    token_hash TEXT NOT NULL UNIQUE,
    status TEXT NOT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    expires_at TIMESTAMPTZ,
    revoked_at TIMESTAMPTZ,
    last_used_at TIMESTAMPTZ
);

CREATE INDEX idx_api_key_tokens_lookup ON api_key_tokens(token_hash) WHERE status IN ('active', 'grace');
CREATE INDEX idx_api_key_tokens_key ON api_key_tokens(api_key_id, created_at DESC);
```

Migration:

- Copy existing `api_keys.token_hash` values into `api_key_tokens` with `status='active'`.
- Keep `api_keys.token_hash` temporarily for rollback, but update auth lookup to read `api_key_tokens`.
- Later remove or ignore `api_keys.token_hash` after one release cycle.

Token statuses:

- `active`: accepted.
- `grace`: accepted until `expires_at` during rotation grace period.
- `revoked`: never accepted.
- `expired`: not accepted.

### Endpoints

| Method | Path | Permission | Purpose |
| --- | --- | --- | --- |
| `GET` | `/management/api-keys/{id}` | `api_keys:read` | Key detail with token history metadata, no token hashes |
| `PATCH` | `/management/api-keys/{id}` | `api_keys:write` | Update name, scopes, rate limit override, expires_at, active state |
| `POST` | `/management/api-keys/{id}/rotate` | `api_keys:rotate` | Generate a new token under the same key ID |
| `POST` | `/management/api-keys/{id}/reactivate` | `api_keys:write` | Set key active if it is not expired |
| `POST` | `/management/api-keys/{id}/revoke` | `api_keys:revoke` | Revoke key and all token secrets |
| `DELETE` | `/management/api-keys/{id}` | `api_keys:revoke` | Compatibility alias for revoke |

### Update Request

```json
{
  "name": "Production crawler",
  "scopes": ["target:*", "type:residential"],
  "rate_limit_override": 100,
  "expires_at": "2026-12-31T23:59:59Z",
  "is_active": true
}
```

All fields are optional. Empty `expires_at` clears expiration. Invalid scope syntax returns `400`.

### Rotation Request And Response

Request:

```json
{
  "grace_period": "24h",
  "revoke_existing": false
}
```

Response:

```json
{
  "api_key_id": "uuid",
  "raw_key": "one-time-secret",
  "token_id": "uuid",
  "previous_tokens_grace_until": "2026-06-30T12:00:00Z"
}
```

Rules:

- `raw_key` is returned once.
- If `revoke_existing=true`, existing active tokens become revoked immediately.
- If `grace_period` is set, existing active tokens become grace tokens until that time.
- Rotation must write a structured audit event without storing `raw_key`.

## Endpoint Registry, Control, And Logs

### Data Model

Enhance endpoint persistence and command tracking:

```sql
ALTER TABLE endpoints
    ADD COLUMN desired_state TEXT NOT NULL DEFAULT 'active',
    ADD COLUMN is_registered BOOLEAN NOT NULL DEFAULT true,
    ADD COLUMN deleted_at TIMESTAMPTZ,
    ADD COLUMN updated_at TIMESTAMPTZ NOT NULL DEFAULT now();

CREATE TABLE endpoint_commands (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    endpoint_id TEXT NOT NULL,
    command TEXT NOT NULL,
    status TEXT NOT NULL,
    payload JSONB NOT NULL DEFAULT '{}',
    requested_by TEXT,
    requested_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    accepted_at TIMESTAMPTZ,
    completed_at TIMESTAMPTZ,
    error TEXT
);

CREATE INDEX idx_endpoint_commands_endpoint ON endpoint_commands(endpoint_id, requested_at DESC);
CREATE INDEX idx_endpoint_commands_status ON endpoint_commands(status, requested_at DESC);

CREATE TABLE endpoint_log_entries (
    id BIGSERIAL PRIMARY KEY,
    endpoint_id TEXT NOT NULL,
    observed_at TIMESTAMPTZ NOT NULL,
    level TEXT NOT NULL,
    message TEXT NOT NULL,
    attrs JSONB NOT NULL DEFAULT '{}',
    trace_id TEXT,
    request_id TEXT
);

CREATE INDEX idx_endpoint_logs_endpoint_time ON endpoint_log_entries(endpoint_id, observed_at DESC);
CREATE INDEX idx_endpoint_logs_level_time ON endpoint_log_entries(level, observed_at DESC);
```

Endpoint desired states:

- `active`: route traffic normally.
- `draining`: complete active work and reject new work.
- `disabled`: do not route traffic even if heartbeats arrive.
- `deleted`: hidden by default and not routeable.

Command statuses:

- `accepted`: API accepted command and published or queued it.
- `acknowledged`: worker received command.
- `running`: worker is executing command.
- `succeeded`: worker completed command.
- `failed`: worker reported failure.
- `timed_out`: no acknowledgement or completion by timeout.

### Broker Control Plane

Add a control stream:

- Stream: `endpoint_control`.
- Subjects:
  - `endpoint.control.<endpoint_id>` for commands to one endpoint.
  - `endpoint.control.broadcast` for future fleet operations.
  - `endpoint.control.ack.<command_id>` for acknowledgements and status updates.
  - `endpoint.logs.<endpoint_id>` for worker log forwarding.

Command payload:

```json
{
  "command_id": "uuid",
  "endpoint_id": "worker-us-1",
  "command": "restart",
  "issued_at": "2026-06-29T12:00:00Z",
  "payload": {}
}
```

Worker acknowledgement payload:

```json
{
  "command_id": "uuid",
  "endpoint_id": "worker-us-1",
  "status": "acknowledged",
  "message": "restart accepted",
  "ts": "2026-06-29T12:00:01Z"
}
```

Worker support required:

- Subscribe to `endpoint.control.<ENDPOINT_ID>`.
- Handle `drain`, `undrain`, `restart`, `disable`, and `enable`.
- Publish acknowledgements before long-running operations.
- Forward structured logs to `endpoint.logs.<ENDPOINT_ID>` when `ENDPOINT_LOG_STREAM_ENABLED=true`.

### Endpoints API

| Method | Path | Permission | Purpose |
| --- | --- | --- | --- |
| `POST` | `/management/endpoints` | `endpoints:write` | Create/pre-register an endpoint record with tags and metadata |
| `GET` | `/management/endpoints/{id}` | `endpoints:read` | Endpoint detail with registry, health, desired state, and recent commands |
| `PATCH` | `/management/endpoints/{id}` | `endpoints:write` | Update tags, metadata, registration state, desired state |
| `DELETE` | `/management/endpoints/{id}` | `endpoints:write` | Mark endpoint deleted and remove health/drain state |
| `POST` | `/management/endpoints/{id}/drain` | `endpoints:control` | Existing action, now records command and desired state |
| `POST` | `/management/endpoints/{id}/undrain` | `endpoints:control` | Clear drain state and publish control command |
| `POST` | `/management/endpoints/{id}/restart` | `endpoints:control` | Publish restart command and return command ID |
| `GET` | `/management/endpoints/{id}/commands` | `endpoints:read` | Command history |
| `GET` | `/management/endpoints/commands/{command_id}` | `endpoints:read` | Command status |
| `GET` | `/management/endpoints/{id}/logs` | `endpoints:logs` | Paginated log entries |
| `GET` | `/management/endpoints/{id}/logs/stream` | `endpoints:logs` | Server-sent events for live log tail |

Drain compatibility:

- The existing drain route should keep returning `200` for compatibility.
- New response body may include command ID:

```json
{
  "endpoint_id": "worker-us-1",
  "desired_state": "draining",
  "command_id": "uuid"
}
```

Deletion rules:

- Deleting a live endpoint sets desired state to `deleted`, removes Redis health and draining flags, and writes an audit event.
- If a deleted endpoint continues heartbeating, backend should keep it non-routeable and surface it as "deleted but still heartbeating" in detail.

Logs:

- Default retention: 7 days or 5 GB, whichever comes first.
- Query filters: `start`, `end`, `level`, `q`, `trace_id`, `request_id`, `cursor`, `limit`.
- Maximum `limit`: 500.
- Live streaming should be SSE over HTTP. WebSocket is unnecessary for first release.

## Fingerprint Deletion

### Endpoint

| Method | Path | Permission | Purpose |
| --- | --- | --- | --- |
| `GET` | `/management/fingerprints/{id}` | `fingerprints:read` | Preset detail |
| `DELETE` | `/management/fingerprints/{id}` | `fingerprints:delete` | Delete preset |

### Delete Behavior

- Check routing rules for references in `fingerprint_preset` and `fingerprint_ab_test.variants`.
- If any active rule references the preset, return `409 conflict` with referencing rule IDs and names.
- Support `?force=true` only for Owner role. Force delete must leave affected rules invalid but inactive or require `deactivate_referencing_rules=true`.
- After delete, publish fingerprint broadcast unless `broadcast=false` is supplied.
- Write an audit event containing the deleted preset metadata and redacted config if necessary.

Response:

```json
{
  "id": "chrome-133",
  "deleted": true,
  "broadcast_requested": true
}
```

## Cost Multiplier Management

### Data Model

The existing table is a good base. Add timestamps and optimistic versioning:

```sql
ALTER TABLE cost_multipliers
    ADD COLUMN created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    ADD COLUMN updated_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    ADD COLUMN version INT NOT NULL DEFAULT 1;
```

### Endpoints

| Method | Path | Permission | Purpose |
| --- | --- | --- | --- |
| `GET` | `/management/cost-multipliers` | `cost_multipliers:read` | List cost multipliers |
| `POST` | `/management/cost-multipliers` | `cost_multipliers:write` | Create multiplier |
| `GET` | `/management/cost-multipliers/{id}` | `cost_multipliers:read` | Detail |
| `PUT` | `/management/cost-multipliers/{id}` | `cost_multipliers:write` | Update with version |
| `DELETE` | `/management/cost-multipliers/{id}` | `cost_multipliers:write` | Soft deactivate |

Create/update request:

```json
{
  "endpoint_tag": "type:residential",
  "multiplier": 10.0,
  "description": "Residential egress",
  "is_active": true,
  "version": 1
}
```

Validation:

- `endpoint_tag` must parse as a tag.
- `multiplier` must be greater than or equal to 0.
- Duplicate `endpoint_tag` returns `409`.
- Update requires matching `version`.

Billing integration:

- Usage cost-unit computation should use active multipliers by endpoint tag.
- `GET /management/billing/estimate` should include the multiplier version or `pricing_version` used for the response.
- Changing a multiplier does not rewrite historical usage summaries by default.
- Add an explicit recomputation job later if historical repricing is required.

## Saved Reports, Scheduled Exports, Alerts, And Notifications

### Data Model

```sql
CREATE TABLE saved_reports (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    name TEXT NOT NULL,
    description TEXT,
    type TEXT NOT NULL,
    filters JSONB NOT NULL DEFAULT '{}',
    format TEXT NOT NULL DEFAULT 'csv',
    created_by UUID REFERENCES admin_users(id),
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE TABLE report_schedules (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    report_id UUID NOT NULL REFERENCES saved_reports(id) ON DELETE CASCADE,
    cron TEXT NOT NULL,
    timezone TEXT NOT NULL DEFAULT 'UTC',
    destination_channel_id UUID,
    is_active BOOLEAN NOT NULL DEFAULT true,
    next_run_at TIMESTAMPTZ,
    last_run_at TIMESTAMPTZ,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE TABLE report_runs (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    report_id UUID NOT NULL REFERENCES saved_reports(id) ON DELETE CASCADE,
    schedule_id UUID REFERENCES report_schedules(id) ON DELETE SET NULL,
    status TEXT NOT NULL,
    started_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    completed_at TIMESTAMPTZ,
    artifact_url TEXT,
    error TEXT
);

CREATE TABLE notification_channels (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    name TEXT NOT NULL,
    type TEXT NOT NULL,
    config JSONB NOT NULL DEFAULT '{}',
    secret_ref TEXT,
    is_enabled BOOLEAN NOT NULL DEFAULT true,
    created_by UUID REFERENCES admin_users(id),
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE TABLE notification_preferences (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    user_id UUID REFERENCES admin_users(id) ON DELETE CASCADE,
    event_type TEXT NOT NULL,
    channel_id UUID REFERENCES notification_channels(id) ON DELETE CASCADE,
    is_enabled BOOLEAN NOT NULL DEFAULT true,
    UNIQUE(user_id, event_type, channel_id)
);

CREATE TABLE alert_rules (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    name TEXT NOT NULL,
    description TEXT,
    metric TEXT NOT NULL,
    condition TEXT NOT NULL,
    threshold DOUBLE PRECISION NOT NULL,
    window TEXT NOT NULL,
    filters JSONB NOT NULL DEFAULT '{}',
    severity TEXT NOT NULL,
    is_active BOOLEAN NOT NULL DEFAULT true,
    cooldown TEXT NOT NULL DEFAULT '15m',
    notification_channel_ids UUID[] NOT NULL DEFAULT '{}',
    created_by UUID REFERENCES admin_users(id),
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE TABLE alert_events (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    alert_rule_id UUID NOT NULL REFERENCES alert_rules(id) ON DELETE CASCADE,
    status TEXT NOT NULL,
    value DOUBLE PRECISION,
    started_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    resolved_at TIMESTAMPTZ,
    last_notified_at TIMESTAMPTZ,
    details JSONB NOT NULL DEFAULT '{}'
);
```

### Report Endpoints

| Method | Path | Permission | Purpose |
| --- | --- | --- | --- |
| `GET` | `/management/reports` | `reports:read` | List saved reports |
| `POST` | `/management/reports` | `reports:write` | Create saved report |
| `GET` | `/management/reports/{id}` | `reports:read` | Detail |
| `PATCH` | `/management/reports/{id}` | `reports:write` | Update saved report |
| `DELETE` | `/management/reports/{id}` | `reports:write` | Delete saved report and schedules |
| `POST` | `/management/reports/{id}/run` | `reports:run` | Run now |
| `GET` | `/management/reports/{id}/runs` | `reports:read` | Run history |
| `GET` | `/management/report-runs/{run_id}` | `reports:read` | Run detail |
| `GET` | `/management/report-runs/{run_id}/download` | `reports:read` | Download artifact |
| `GET` | `/management/report-schedules` | `reports:read` | List schedules |
| `POST` | `/management/report-schedules` | `reports:write` | Create schedule |
| `PATCH` | `/management/report-schedules/{id}` | `reports:write` | Update schedule |
| `DELETE` | `/management/report-schedules/{id}` | `reports:write` | Disable/delete schedule |

Supported first-release report types:

- `usage_summary`.
- `billing_estimate`.
- `api_key_inventory`.
- `endpoint_health`.
- `audit_events`.

Report worker:

- Runs in the relay process or a separate worker process with the same Postgres connection.
- Uses row locking to claim due schedules.
- Stores artifacts on local disk by default, with an interface for S3-compatible storage later.
- Enforces maximum date range and row count per report type.

### Alert Endpoints

| Method | Path | Permission | Purpose |
| --- | --- | --- | --- |
| `GET` | `/management/alerts/rules` | `alerts:read` | List alert rules |
| `POST` | `/management/alerts/rules` | `alerts:write` | Create alert rule |
| `GET` | `/management/alerts/rules/{id}` | `alerts:read` | Alert rule detail |
| `PATCH` | `/management/alerts/rules/{id}` | `alerts:write` | Update alert rule |
| `DELETE` | `/management/alerts/rules/{id}` | `alerts:write` | Disable/delete alert rule |
| `GET` | `/management/alerts/events` | `alerts:read` | List fired/resolved events |
| `POST` | `/management/alerts/events/{id}/ack` | `alerts:write` | Acknowledge event |

Supported first-release metrics:

- `endpoint_unhealthy_count`.
- `endpoint_draining_count`.
- `endpoint_active_tasks`.
- `usage_requests`.
- `usage_bytes`.
- `billing_estimated_usd`.
- `cache_error_rate` if Redis stats are available.

Conditions:

- `greater_than`.
- `greater_than_or_equal`.
- `less_than`.
- `less_than_or_equal`.
- `equals`.

### Notification Endpoints

| Method | Path | Permission | Purpose |
| --- | --- | --- | --- |
| `GET` | `/management/notification-channels` | `alerts:read` | List delivery channels |
| `POST` | `/management/notification-channels` | `notifications:write` | Create channel |
| `PATCH` | `/management/notification-channels/{id}` | `notifications:write` | Update channel |
| `DELETE` | `/management/notification-channels/{id}` | `notifications:write` | Disable/delete channel |
| `POST` | `/management/notification-channels/{id}/test` | `notifications:write` | Send test notification |
| `GET` | `/management/notification-preferences` | Authenticated | Current user's preferences |
| `PATCH` | `/management/notification-preferences` | Authenticated | Update current user's preferences |

Channel types:

- `webhook` for generic HTTP POST.
- `email` if SMTP configuration is provided.
- `slack_webhook` if operators provide a webhook URL secret.

Secrets:

- Webhook URLs and SMTP passwords must be stored as secret references or encrypted values, never returned by read APIs.
- Read responses include `has_secret=true` instead of the secret value.

## OpenAPI, Docs, And Compatibility

Every new route must be added to:

- `api/openapi.yaml`.
- `docs/management-api.md`.
- Handler tests under `internal/server/admin/handlers`.
- Contract tests if response schemas are added.

Backward-compatible route behavior:

- Existing `POST /management/api-keys`, `GET /management/api-keys`, and `DELETE /management/api-keys/{id}` continue to work.
- Existing `POST /management/endpoints/{id}/drain` continues to work.
- Existing `POST /management/fingerprints` keeps upsert behavior.
- Existing billing estimate fields remain, with optional new pricing metadata.

New route registration should keep all Management API routes under `/management/*` and pass through the new auth/RBAC middleware.

## Implementation Phases

### Phase 1: Identity And Audit Foundation

- Add admin users, roles, permissions, sessions, identity providers.
- Add auth endpoints and RBAC middleware.
- Preserve legacy token compatibility.
- Add actor context.
- Add structured audit events and audit read endpoints.
- Update existing handlers to emit structured audit events.

### Phase 2: API Key And Fingerprint Lifecycle

- Add `api_key_tokens` table and migrate existing token hashes.
- Update client API auth lookup to read token secret history.
- Add API-key detail, update, rotate, reactivate, and revoke routes.
- Add fingerprint detail and delete routes with routing-rule dependency checks.

### Phase 3: Endpoint Control Plane

- Enhance endpoint registry persistence.
- Add endpoint detail, create, patch, delete, undrain, restart, commands, and logs routes.
- Add broker control stream and worker command subscriber.
- Add log forwarding and retention.
- Preserve existing drain behavior.

### Phase 4: Cost Multipliers And Billing Integration

- Add cost multiplier repository, DTOs, handlers, tests, and OpenAPI schemas.
- Add versioning and audit events.
- Integrate active multipliers into billing calculations and expose pricing metadata.

### Phase 5: Reports, Schedules, Alerts, And Notifications

- Add saved reports, schedules, report runs, alert rules, alert events, notification channels, and preferences.
- Add background scheduler and report runner.
- Add alert evaluator and delivery worker.
- Add report artifact storage and retention policy.

## Acceptance Checklist

- User accounts can be created, deactivated, assigned roles, and authenticated without the legacy management token.
- SSO providers can be configured, enabled, disabled, and used for OIDC login.
- Access tokens expire and refresh tokens rotate.
- RBAC middleware denies actions without the required permission and allows legacy token full-admin access while compatibility is enabled.
- Audit viewer APIs can query structured domain events and raw management request logs with filters.
- Existing mutating handlers write structured audit events with actor information.
- API keys have detail, update, rotate, reactivate, expiration editing, and revoke behavior under a stable logical key ID.
- API-key rotation returns the raw secret once and supports immediate revoke or grace-period behavior for old tokens.
- Endpoint records can be created, updated, deleted, drained, undrained, restarted, inspected, and queried for command history.
- Worker command acknowledgement updates endpoint command status.
- Endpoint logs can be queried and streamed with retention limits.
- Fingerprints can be fetched by ID and deleted with routing-rule dependency protection.
- Cost multipliers can be listed, created, updated with optimistic versioning, and deactivated.
- Billing estimates expose the pricing metadata used by the estimate.
- Saved reports can be created, run immediately, scheduled, downloaded, and deleted.
- Alerts can be created, evaluated, fired, acknowledged, resolved, and delivered through notification channels.
- Notification preferences can be read and updated per user.
- All new routes are represented in OpenAPI and Management API docs.
- Existing Management API routes continue to pass their current tests.
