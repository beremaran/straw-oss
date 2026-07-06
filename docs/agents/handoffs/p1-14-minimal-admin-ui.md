# Handoff

Task: `docs/tasks/p1/14-minimal-admin-ui.md`

## Changed

- `cmd/control/admin_ui.go`: embeds and serves the minimal admin UI at `/admin/`.
- `cmd/control/admin_ui/index.html`: adds API-key based local operator UI for Requests, Workers, Audit, Tenants, Routes, Deny rules, and Injection policies, using only existing APIs.
- `cmd/control/main.go`: wires the UI into the Control API mux.
- `cmd/control/admin_ui_test.go`: adds static and browser smoke coverage for navigation, auth failure, worker rendering, request detail lookup, config views, and UI redaction.
- `cmd/control/admin_routes_test.go`: shares the tenant route literal with the new UI tests.
- `docs/tasks/p1.md`, `docs/tasks/p1/14-minimal-admin-ui.md`: mark task complete after verification.

## Acceptance Criteria Verdicts

Filled from the independent verifier's report (straw-task-runner workflow step 12), not from the implementer's self-assessment.

| Criterion | Verdict | Implementation (file:line) | Proving test |
|-----------|---------|----------------------------|--------------|
| The first screen is a usable admin surface, not a landing page. | VERIFIED | `cmd/control/admin_ui/index.html:130`, `cmd/control/admin_ui/index.html:173`, `cmd/control/main.go:713` | `TestAdminUIServesUsableFirstScreen` |
| UI actions map only to existing APIs. | VERIFIED | `cmd/control/admin_ui/index.html:174`, `cmd/control/admin_ui/index.html:331`, `cmd/control/main.go:865`, `cmd/control/main.go:874` | `TestAdminUIServesUsableFirstScreen`; existing route registration coverage in `TestServeAdminRoutesCanonicalConfigPaths` |
| Tenant-facing redaction rules are preserved. | VERIFIED | `cmd/control/admin_ui/index.html:182`, `cmd/control/admin_ui/index.html:289`, `cmd/control/admin_ui/index.html:316` | `TestAdminUIRedirectAndRedactionGuardrails`; `TestAdminUIBrowserSmoke` |

## Planning-Doc Coverage

Every in-phase field/endpoint/behavior from the task's cited planning-doc sections, accounted for:

| Planning item | Status | Evidence / owning task |
|---------------|--------|------------------------|
| P1 includes minimal UI surfaces. | implemented | `cmd/control/admin_ui/index.html:130` |
| Public base path `/api/v1`. | implemented | `cmd/control/admin_ui/index.html:173` |
| Telemetry read APIs for requests/workers/audit are P1. | implemented | Requests use `/api/v1/telemetry/requests` at `cmd/control/admin_ui/index.html:174`; worker runtime view uses existing `/api/v1/admin/workers` at `cmd/control/admin_ui/index.html:175`; config audit uses durable `/api/v1/config/changes` at `cmd/control/admin_ui/index.html:176`. |
| Config endpoints under `/api/v1/config/*`. | implemented | Tenants, routing rules, deny rules, and injection policies at `cmd/control/admin_ui/index.html:176`-`cmd/control/admin_ui/index.html:180`. |
| Runtime admin endpoints under `/api/v1/admin/*`. | implemented | Request cancel at `cmd/control/admin_ui/index.html:174`; worker actions at `cmd/control/admin_ui/index.html:331`-`cmd/control/admin_ui/index.html:338`. |
| Tenant worker views must omit `session_id` and NATS subjects. | implemented | UI hidden-field guard at `cmd/control/admin_ui/index.html:182`, plus backend policy remains in `internal/control/worker_handlers.go:78`. |
| Config list pagination uses `limit`. | implemented | UI sends `limit` for list calls at `cmd/control/admin_ui/index.html:232`-`cmd/control/admin_ui/index.html:235`. |
| Create/update/delete config APIs and secret-returning key APIs. | out of scope | Task is read-mostly and says actions only where existing APIs already authorize them; no write UI added. |

## Verification

```sh
go test ./cmd/control -run 'TestAdminUI|TestServeAdminRoutesCanonicalConfigPaths' -count=1
make check
```

Result:

- Focused UI tests: passed.
- `make check`: passed (`go test ./...`; `golangci-lint run --max-issues-per-linter 0 --max-same-issues 0` with 0 issues).
- Postgres-backed tests: not exercised (diff does not touch `postgres_*` files or migrations).
- Live compose verification: skipped because this task adds an admin UI over existing APIs and does not change the runtime request path.
- Browser/visual verification: Playwright with local Chrome verified desktop 1440x900 and mobile 390x844 screenshots; no horizontal overflow; auth failure and Workers navigation rendered. Concept image inspected at `/Users/beremaran/.codex/generated_images/019f3691-0401-77b0-a779-fddd5984eb79/ig_0e5846e7bdcdf27d016a4b68f67e94819484dfc88e762bc5fb.png`.

## Reviewer Start Points

- `cmd/control/admin_ui/index.html`
- `cmd/control/admin_ui_test.go`
- `cmd/control/admin_ui.go`

## Remaining Work

- None.

## Blockers

- None.
