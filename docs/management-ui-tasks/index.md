# Management UI Task Index

Status: planning backlog

Source spec: `docs/management-ui-spec.md`

Use this directory as the implementation queue for the Management UI Specification. Search by task ID, route, API endpoint, UI state, validation rule, or phase:

```sh
rg "MU-007|api-keys|raw_key|bulk revoke" docs/management-ui-tasks
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
| [MU-001](tasks/MU-001-frontend-workspace-build.md) | 1 | done | Frontend workspace and build integration | none | frontend workspace, dev server, build, test, `web/management` |
| [MU-002](tasks/MU-002-api-client-connection.md) | 1 | done | Management API client and connection model | MU-001 | api client, base URL, bearer token, error normalization, OpenAPI |
| [MU-003](tasks/MU-003-sign-in-session.md) | 1 | done | Sign-in, token handling, and session lifecycle | MU-002 | `/login`, auth, local storage, healthz, management token |
| [MU-004](tasks/MU-004-app-shell-routes-data.md) | 1 | done | App shell, routes, navigation, and global data states | MU-003 | app shell, routes, navigation, refresh, loading, errors |
| [MU-005](tasks/MU-005-shared-controls-validation.md) | 1 | done | Shared controls, validation, and mutation workflows | MU-004 | validation, tags, scopes, dates, JSON, confirmation dialogs |
| [MU-006](tasks/MU-006-overview-dashboard.md) | 2 | not-started | Overview dashboard and operational attention panels | MU-004, MU-005 | `/overview`, dashboard, endpoint pressure, routing attention, partial failure |
| [MU-007](tasks/MU-007-api-key-management.md) | 2 | not-started | API key list, creation, raw-key capture, and revoke | MU-005 | `/api-keys`, raw_key, revoke, scopes, bulk revoke |
| [MU-008](tasks/MU-008-routing-rule-list-detail.md) | 2 | not-started | Routing rule list, detail view, filters, and attention indicators | MU-005 | `/routing-rules`, priorities, fingerprints, endpoint pools, raw JSON |
| [MU-009](tasks/MU-009-routing-rule-editor.md) | 2 | not-started | Routing rule create, edit, duplicate, and version-conflict handling | MU-008 | routing rule form, `CreateRoutingRuleRequest`, `UpdateRoutingRuleRequest`, version |
| [MU-010](tasks/MU-010-endpoint-management.md) | 2 | not-started | Endpoint list, stale-state handling, and drain workflow | MU-005 | `/endpoints`, drain, health TTL, tags, active tasks |
| [MU-011](tasks/MU-011-fingerprint-presets.md) | 2 | not-started | Fingerprint preset list, JSON editor, upsert, duplicate, and broadcast | MU-005 | `/fingerprints`, JSON config, upsert, broadcast, NATS |
| [MU-012](tasks/MU-012-usage-billing.md) | 3 | not-started | Usage summary, billing estimate, filters, charts, and CSV export | MU-005, MU-007 | `/usage`, billing estimate, date range, CSV, charts |
| [MU-013](tasks/MU-013-cache-controls.md) | 3 | not-started | Cache stats, Redis info viewer, and pattern clear workflow | MU-005 | `/cache`, Redis, INFO, clear pattern, CLEAR ALL |
| [MU-014](tasks/MU-014-system-diagnostics.md) | 3 | not-started | System diagnostics, capability detection, and backend gap display | MU-004 | `/system`, healthz, capabilities, backend gaps, docs links |
| [MU-015](tasks/MU-015-accessibility-responsive-visual.md) | 4 | not-started | Accessibility, responsive behavior, and visual design pass | MU-006 through MU-014 | accessibility, responsive, mobile cards, keyboard, charts |
| [MU-016](tasks/MU-016-tests-mocked-api.md) | 4 | not-started | Tests, mocked Management API coverage, and route regression checks | MU-002 through MU-014 | tests, mocked API, e2e, route coverage, error states |
| [MU-017](tasks/MU-017-docs-operator-handoff.md) | 4 | not-started | Operator documentation and implementation handoff | MU-001 through MU-014 | docs, runbook, backend gaps, unsupported actions |
| [MU-018](tasks/MU-018-final-acceptance.md) | cross-cutting | not-started | Final first-release acceptance pass | MU-015, MU-016, MU-017 | acceptance checklist, route coverage, unsupported backend actions |

## Coverage Map

| Spec area | Task IDs |
| --- | --- |
| Frontend workspace, build, and repo integration | MU-001 |
| API client, auth headers, errors, and query behavior | MU-002, MU-004 |
| Sign-in, storage, token safety, sign-out | MU-003 |
| App shell, routes, global loading/error/empty/stale states | MU-004, MU-005 |
| Overview dashboard | MU-006 |
| API key workflows | MU-007 |
| Routing rule list, details, create/edit, duplicate, deactivate | MU-008, MU-009 |
| Endpoint monitoring and drain | MU-010 |
| Fingerprint preset upsert and broadcast | MU-011 |
| Usage, billing, charts, export | MU-012 |
| Cache stats and cache clear | MU-013 |
| System diagnostics and backend gaps | MU-014, MU-017 |
| Accessibility, responsive design, tests, final acceptance | MU-015, MU-016, MU-018 |

