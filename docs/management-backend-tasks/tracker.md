# Management Backend Work Tracker

Source spec: `docs/management-backend-spec.md`

Status values: `not-started`, `in-progress`, `blocked`, `done`.

## Current Work

| Field | Value |
| --- | --- |
| Current task | none |
| Current owner | unassigned |
| Last updated | 2026-06-29 |
| Notes | Backlog created from the Management Backend Specification. |

## Phase 1: Identity And Audit Foundation

| Done | ID | Status | Task |
| --- | --- | --- | --- |
| [x] | [MB-001](tasks/MB-001-auth-rbac-compatibility.md) | done | Auth and RBAC compatibility foundation |
| [x] | [MB-002](tasks/MB-002-identity-schema-repositories.md) | done | Identity schema and repositories |
| [x] | [MB-003](tasks/MB-003-local-sessions-bootstrap.md) | done | Local login, refresh sessions, logout, bootstrap owner |
| [ ] | [MB-004](tasks/MB-004-users-roles-idp-apis.md) | not-started | Users, roles, and identity-provider management APIs |
| [ ] | [MB-005](tasks/MB-005-oidc-sso.md) | not-started | OIDC SSO login flow |
| [ ] | [MB-006](tasks/MB-006-structured-audit-foundation.md) | not-started | Structured audit foundation and redaction |
| [ ] | [MB-007](tasks/MB-007-audit-viewer-apis.md) | not-started | Audit viewer APIs and export |

## Phase 2: API Key And Fingerprint Lifecycle

| Done | ID | Status | Task |
| --- | --- | --- | --- |
| [ ] | [MB-008](tasks/MB-008-api-key-token-history.md) | not-started | API key token history and auth lookup migration |
| [ ] | [MB-009](tasks/MB-009-api-key-lifecycle-apis.md) | not-started | API key detail, update, rotate, reactivate, revoke APIs |
| [ ] | [MB-010](tasks/MB-010-fingerprint-delete.md) | not-started | Fingerprint detail and protected delete |

## Phase 3: Endpoint Control Plane

| Done | ID | Status | Task |
| --- | --- | --- | --- |
| [ ] | [MB-011](tasks/MB-011-endpoint-registry-persistence.md) | not-started | Endpoint registry persistence and command schema |
| [ ] | [MB-012](tasks/MB-012-endpoint-management-apis.md) | not-started | Endpoint registry, desired-state, and command APIs |
| [ ] | [MB-013](tasks/MB-013-endpoint-control-plane.md) | not-started | Broker endpoint control plane and worker command subscriber |
| [ ] | [MB-014](tasks/MB-014-endpoint-logs.md) | not-started | Endpoint log ingestion, query, stream, and retention |

## Phase 4: Cost Multipliers And Billing Integration

| Done | ID | Status | Task |
| --- | --- | --- | --- |
| [ ] | [MB-015](tasks/MB-015-cost-multiplier-management.md) | not-started | Cost multiplier repository and management APIs |
| [ ] | [MB-016](tasks/MB-016-billing-multiplier-integration.md) | not-started | Billing multiplier integration and pricing metadata |

## Phase 5: Reports, Schedules, Alerts, And Notifications

| Done | ID | Status | Task |
| --- | --- | --- | --- |
| [ ] | [MB-017](tasks/MB-017-saved-reports.md) | not-started | Saved reports, report runs, and artifact download |
| [ ] | [MB-018](tasks/MB-018-report-scheduler.md) | not-started | Report scheduler and due-run worker |
| [ ] | [MB-019](tasks/MB-019-notification-settings.md) | not-started | Notification channels and preferences |
| [ ] | [MB-020](tasks/MB-020-alerting.md) | not-started | Alert rules, evaluator, events, and delivery |

## Cross-Cutting Closeout

| Done | ID | Status | Task |
| --- | --- | --- | --- |
| [ ] | [MB-021](tasks/MB-021-openapi-docs-contracts.md) | not-started | OpenAPI, docs, and contract test sweep |
| [ ] | [MB-022](tasks/MB-022-final-acceptance.md) | not-started | Final acceptance and compatibility pass |

## Blocked Items

None.

## Decisions

- Keep task tracking in Markdown so `rg`, reviews, and ordinary diffs are enough.
- Do not edit `mkdocs.yml` until these planning docs need to be published.
