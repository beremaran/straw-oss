# Management Backend Work Tracker

Source spec: `docs/management-backend-spec.md`

Status values: `not-started`, `in-progress`, `blocked`, `done`.

## Current Work

| Field | Value |
| --- | --- |
| Current task | none |
| Current owner | unassigned |
| Last updated | 2026-06-30 |
| Notes | MB-022 completed; management backend task queue is complete. |

## Phase 1: Identity And Audit Foundation

| Done | ID | Status | Task |
| --- | --- | --- | --- |
| [x] | [MB-001](tasks/MB-001-auth-rbac-compatibility.md) | done | Auth and RBAC compatibility foundation |
| [x] | [MB-002](tasks/MB-002-identity-schema-repositories.md) | done | Identity schema and repositories |
| [x] | [MB-003](tasks/MB-003-local-sessions-bootstrap.md) | done | Local login, refresh sessions, logout, bootstrap owner |
| [x] | [MB-004](tasks/MB-004-users-roles-idp-apis.md) | done | Users, roles, and identity-provider management APIs |
| [x] | [MB-005](tasks/MB-005-oidc-sso.md) | done | OIDC SSO login flow |
| [x] | [MB-006](tasks/MB-006-structured-audit-foundation.md) | done | Structured audit foundation and redaction |
| [x] | [MB-007](tasks/MB-007-audit-viewer-apis.md) | done | Audit viewer APIs and export |

## Phase 2: API Key And Fingerprint Lifecycle

| Done | ID | Status | Task |
| --- | --- | --- | --- |
| [x] | [MB-008](tasks/MB-008-api-key-token-history.md) | done | API key token history and auth lookup migration |
| [x] | [MB-009](tasks/MB-009-api-key-lifecycle-apis.md) | done | API key detail, update, rotate, reactivate, revoke APIs |
| [x] | [MB-010](tasks/MB-010-fingerprint-delete.md) | done | Fingerprint detail and protected delete |

## Phase 3: Endpoint Control Plane

| Done | ID | Status | Task |
| --- | --- | --- | --- |
| [x] | [MB-011](tasks/MB-011-endpoint-registry-persistence.md) | done | Endpoint registry persistence and command schema |
| [x] | [MB-012](tasks/MB-012-endpoint-management-apis.md) | done | Endpoint registry, desired-state, and command APIs |
| [x] | [MB-013](tasks/MB-013-endpoint-control-plane.md) | done | Broker endpoint control plane and worker command subscriber |
| [x] | [MB-014](tasks/MB-014-endpoint-logs.md) | done | Endpoint log ingestion, query, stream, and retention |

## Phase 4: Cost Multipliers And Billing Integration

| Done | ID | Status | Task |
| --- | --- | --- | --- |
| [x] | [MB-015](tasks/MB-015-cost-multiplier-management.md) | done | Cost multiplier repository and management APIs |
| [x] | [MB-016](tasks/MB-016-billing-multiplier-integration.md) | done | Billing multiplier integration and pricing metadata |

## Phase 5: Reports, Schedules, Alerts, And Notifications

| Done | ID | Status | Task |
| --- | --- | --- | --- |
| [x] | [MB-017](tasks/MB-017-saved-reports.md) | done | Saved reports, report runs, and artifact download |
| [x] | [MB-018](tasks/MB-018-report-scheduler.md) | done | Report scheduler and due-run worker |
| [x] | [MB-019](tasks/MB-019-notification-settings.md) | done | Notification channels and preferences |
| [x] | [MB-020](tasks/MB-020-alerting.md) | done | Alert rules, evaluator, events, and delivery |

## Cross-Cutting Closeout

| Done | ID | Status | Task |
| --- | --- | --- | --- |
| [x] | [MB-021](tasks/MB-021-openapi-docs-contracts.md) | done | OpenAPI, docs, and contract test sweep |
| [x] | [MB-022](tasks/MB-022-final-acceptance.md) | done | Final acceptance and compatibility pass |

## Blocked Items

None.

## Decisions

- Keep task tracking in Markdown so `rg`, reviews, and ordinary diffs are enough.
- Do not edit `mkdocs.yml` until these planning docs need to be published.
