# Management UI Work Tracker

Source spec: `docs/management-ui-spec.md`

Status values: `not-started`, `in-progress`, `blocked`, `done`.

## Current Work

| Field | Value |
| --- | --- |
| Current task | none |
| Current owner | unassigned |
| Last updated | 2026-06-29 |
| Notes | Backlog created from the Management UI Specification. |

## Phase 1: UI Foundation

| Done | ID | Status | Task |
| --- | --- | --- | --- |
| [x] | [MU-001](tasks/MU-001-frontend-workspace-build.md) | done | Frontend workspace and build integration |
| [x] | [MU-002](tasks/MU-002-api-client-connection.md) | done | Management API client and connection model |
| [x] | [MU-003](tasks/MU-003-sign-in-session.md) | done | Sign-in, token handling, and session lifecycle |
| [x] | [MU-004](tasks/MU-004-app-shell-routes-data.md) | done | App shell, routes, navigation, and global data states |
| [x] | [MU-005](tasks/MU-005-shared-controls-validation.md) | done | Shared controls, validation, and mutation workflows |

## Phase 2: Core Operations Surfaces

| Done | ID | Status | Task |
| --- | --- | --- | --- |
| [x] | [MU-006](tasks/MU-006-overview-dashboard.md) | done | Overview dashboard and operational attention panels |
| [x] | [MU-007](tasks/MU-007-api-key-management.md) | done | API key list, creation, raw-key capture, and revoke |
| [ ] | [MU-008](tasks/MU-008-routing-rule-list-detail.md) | not-started | Routing rule list, detail view, filters, and attention indicators |
| [ ] | [MU-009](tasks/MU-009-routing-rule-editor.md) | not-started | Routing rule create, edit, duplicate, and version-conflict handling |
| [ ] | [MU-010](tasks/MU-010-endpoint-management.md) | not-started | Endpoint list, stale-state handling, and drain workflow |
| [ ] | [MU-011](tasks/MU-011-fingerprint-presets.md) | not-started | Fingerprint preset list, JSON editor, upsert, duplicate, and broadcast |

## Phase 3: Reporting, Cache, And System

| Done | ID | Status | Task |
| --- | --- | --- | --- |
| [ ] | [MU-012](tasks/MU-012-usage-billing.md) | not-started | Usage summary, billing estimate, filters, charts, and CSV export |
| [ ] | [MU-013](tasks/MU-013-cache-controls.md) | not-started | Cache stats, Redis info viewer, and pattern clear workflow |
| [ ] | [MU-014](tasks/MU-014-system-diagnostics.md) | not-started | System diagnostics, capability detection, and backend gap display |

## Phase 4: Release Readiness

| Done | ID | Status | Task |
| --- | --- | --- | --- |
| [ ] | [MU-015](tasks/MU-015-accessibility-responsive-visual.md) | not-started | Accessibility, responsive behavior, and visual design pass |
| [ ] | [MU-016](tasks/MU-016-tests-mocked-api.md) | not-started | Tests, mocked Management API coverage, and route regression checks |
| [ ] | [MU-017](tasks/MU-017-docs-operator-handoff.md) | not-started | Operator documentation and implementation handoff |

## Cross-Cutting Closeout

| Done | ID | Status | Task |
| --- | --- | --- | --- |
| [ ] | [MU-018](tasks/MU-018-final-acceptance.md) | not-started | Final first-release acceptance pass |

## Blocked Items

None.

## Decisions

- Keep task tracking in Markdown so `rg`, reviews, and ordinary diffs are enough.
- Keep the UI backlog separate from `docs/management-backend-tasks`; unsupported backend actions stay documented gaps until APIs exist.
- Do not edit `mkdocs.yml` until these planning docs need to be published.

