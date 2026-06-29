# MB-017: Saved Reports And Report Runs

Status: done
Phase: 5
Depends on: MB-001, MB-006
Search tags: saved_reports, report_runs, artifacts, download, `reports:run`

## Objective

Persist saved report definitions and allow users to run and download reports.

## Scope

- Add `saved_reports`, `report_schedules`, and `report_runs` migrations needed for report definitions and runs.
- Add saved report CRUD endpoints.
- Add run-now endpoint, run history, run detail, and artifact download.
- Support first-release report types from the spec.
- Store artifacts on local disk by default.

## Repo Touchpoints

- `internal/infra/postgres/migrations/*.sql`
- `internal/domain/*report*.go`
- `internal/infra/postgres/*report*_repo.go`
- `internal/server/admin/handlers/*report*.go`
- `internal/server/admin/server.go`
- `internal/config/config.go`

## Implementation Tasks

- [x] Add migrations and report domain models.
- [x] Add saved report and run repositories.
- [x] Add report generation for usage summary, billing estimate, API key inventory, endpoint health, and audit events.
- [x] Add local artifact storage with max range and row-count checks.
- [x] Add handlers and tests for CRUD, run, history, detail, and download.

## Done Criteria

- [x] Saved reports can be created, updated, listed, fetched, and deleted.
- [x] Reports can run immediately and produce downloadable artifacts.
- [x] Report generation enforces maximum date ranges and row counts.
- [x] Mutations and report runs are audited.
