# MB-007: Audit Viewer APIs And Export

Status: done
Phase: 1
Depends on: MB-006
Search tags: audit viewer, `audit:read`, filters, csv, ndjson, export

## Objective

Expose structured audit events and raw management request audit rows to the Management UI.

## Scope

- Add `GET /management/audit/events`.
- Add `GET /management/audit/events/{id}`.
- Add `GET /management/audit/requests`.
- Add `GET /management/audit/export`.
- Support filters and pagination from the spec.
- Require `audit:read`; require Owner or Security auditor for redacted body inclusion.

## Repo Touchpoints

- `internal/server/admin/server.go`
- `internal/server/admin/handlers/*audit*.go`
- `internal/server/dto/*audit*.go`
- `internal/infra/postgres/*audit*_repo.go`
- `api/openapi.yaml`
- `docs/management-api.md`

## Implementation Tasks

- [x] Add query DTOs and validation for dates, status range, page, and limit.
- [x] Add repository queries with bounded limits.
- [x] Add CSV and NDJSON export for bounded date ranges.
- [x] Keep request bodies excluded by default.
- [x] Add handler tests for filtering, permissions, and export bounds.

## Done Criteria

- [x] Audit events can be listed, filtered, and fetched by ID.
- [x] Request audit can be listed without leaking unredacted bodies.
- [x] Export rejects unbounded or excessive ranges.
- [x] Pagination limit is capped at 500.
