# Management Backend Task Index

Status: planning backlog

Source spec: `docs/management-backend-spec.md`

Use this directory as the implementation queue for the Management Backend Specification. Search by task ID, endpoint, permission, domain term, or phase:

```sh
rg "MB-009|api-keys|api_keys:rotate" docs/management-backend-tasks
```

Update `tracker.md` when work starts, lands, or is blocked. Keep task files focused on scope and acceptance criteria; implementation notes belong in the task while active.

## Status Values

- `not-started`
- `in-progress`
- `blocked`
- `done`

## Searchable Task Index

| ID | Phase | Status | Task | Depends on | Search tags |
| --- | --- | --- | --- | --- | --- |
| [MB-001](tasks/MB-001-auth-rbac-compatibility.md) | 1 | done | Auth and RBAC compatibility foundation | none | auth, rbac, legacy token, permissions, actor context |
| [MB-002](tasks/MB-002-identity-schema-repositories.md) | 1 | done | Identity schema and repositories | MB-001 | admin_users, roles, sessions, identity providers |
| [MB-003](tasks/MB-003-local-sessions-bootstrap.md) | 1 | done | Local login, refresh sessions, logout, bootstrap owner | MB-001, MB-002 | login, refresh token, session rotation, bootstrap |
| [MB-004](tasks/MB-004-users-roles-idp-apis.md) | 1 | not-started | Users, roles, and identity-provider management APIs | MB-001, MB-002 | users, roles, identity-providers, permissions |
| [MB-005](tasks/MB-005-oidc-sso.md) | 1 | done | OIDC SSO login flow | MB-002, MB-003, MB-004 | sso, oidc, callback, jwks, provider |
| [MB-006](tasks/MB-006-structured-audit-foundation.md) | 1 | not-started | Structured audit foundation and redaction | MB-001 | audit events, actor, redaction, request log |
| [MB-007](tasks/MB-007-audit-viewer-apis.md) | 1 | not-started | Audit viewer APIs and export | MB-006 | audit:read, filters, csv, ndjson, export |
| [MB-008](tasks/MB-008-api-key-token-history.md) | 2 | not-started | API key token history and auth lookup migration | MB-001 | api_key_tokens, token hash, grace, revoke, auth lookup |
| [MB-009](tasks/MB-009-api-key-lifecycle-apis.md) | 2 | not-started | API key detail, update, rotate, reactivate, revoke APIs | MB-008 | api_keys:rotate, expires_at, raw_key, scopes |
| [MB-010](tasks/MB-010-fingerprint-delete.md) | 2 | not-started | Fingerprint detail and protected delete | MB-001, MB-006 | fingerprints, delete, routing rule dependency, broadcast |
| [MB-011](tasks/MB-011-endpoint-registry-persistence.md) | 3 | not-started | Endpoint registry persistence and command schema | MB-001 | endpoints, desired_state, endpoint_commands, registry |
| [MB-012](tasks/MB-012-endpoint-management-apis.md) | 3 | not-started | Endpoint registry, desired-state, and command APIs | MB-011 | endpoint detail, drain, undrain, restart, commands |
| [MB-013](tasks/MB-013-endpoint-control-plane.md) | 3 | not-started | Broker endpoint control plane and worker command subscriber | MB-011, MB-012 | endpoint_control, ack, restart, worker subscriber |
| [MB-014](tasks/MB-014-endpoint-logs.md) | 3 | not-started | Endpoint log ingestion, query, stream, and retention | MB-011, MB-013 | endpoint logs, sse, retention, cursor |
| [MB-015](tasks/MB-015-cost-multiplier-management.md) | 4 | not-started | Cost multiplier repository and management APIs | MB-001, MB-006 | cost_multipliers, version, optimistic locking |
| [MB-016](tasks/MB-016-billing-multiplier-integration.md) | 4 | not-started | Billing multiplier integration and pricing metadata | MB-015 | billing estimate, pricing_version, usage cost |
| [MB-017](tasks/MB-017-saved-reports.md) | 5 | not-started | Saved reports, report runs, and artifact download | MB-001, MB-006 | reports, report_runs, download, artifacts |
| [MB-018](tasks/MB-018-report-scheduler.md) | 5 | not-started | Report scheduler and due-run worker | MB-017 | schedules, cron, row locking, report worker |
| [MB-019](tasks/MB-019-notification-settings.md) | 5 | not-started | Notification channels and preferences | MB-001, MB-006 | notification channels, preferences, webhook, email, slack |
| [MB-020](tasks/MB-020-alerting.md) | 5 | not-started | Alert rules, evaluator, events, and delivery | MB-019 | alerts, evaluator, ack, resolved, delivery |
| [MB-021](tasks/MB-021-openapi-docs-contracts.md) | cross-cutting | not-started | OpenAPI, docs, and contract test sweep | MB-003 through MB-020 | openapi, management-api, contract tests, compatibility |
| [MB-022](tasks/MB-022-final-acceptance.md) | cross-cutting | not-started | Final acceptance and compatibility pass | MB-021 | acceptance checklist, legacy routes, regression |

## Coverage Map

| Spec area | Task IDs |
| --- | --- |
| Cross-cutting auth, permissions, actor context | MB-001, MB-003, MB-004 |
| Identity, SSO, roles, sessions | MB-002, MB-003, MB-004, MB-005 |
| Audit viewer and structured audit | MB-006, MB-007 |
| API key lifecycle | MB-008, MB-009 |
| Endpoint registry, control, logs | MB-011, MB-012, MB-013, MB-014 |
| Fingerprint deletion | MB-010 |
| Cost multipliers and billing | MB-015, MB-016 |
| Saved reports, schedules, alerts, notifications | MB-017, MB-018, MB-019, MB-020 |
| OpenAPI, docs, compatibility, acceptance | MB-021, MB-022 |
