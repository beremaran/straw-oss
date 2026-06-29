# MB-015: Cost Multiplier Management

Status: done
Phase: 4
Depends on: MB-001, MB-006
Search tags: cost_multipliers, version, optimistic locking, `cost_multipliers:write`

## Objective

Expose CRUD-style management for cost multipliers with optimistic versioning and audit events.

## Scope

- Add timestamps and `version` to `cost_multipliers`.
- Add repository, domain model, DTOs, handlers, and tests.
- Add list, create, detail, update, and soft deactivate endpoints.
- Validate endpoint tags and multiplier values.
- Emit structured audit events on mutation.

## Repo Touchpoints

- `internal/infra/postgres/migrations/006_create_cost_multipliers.sql`
- `internal/infra/postgres/migrations/*.sql`
- `internal/domain/*cost*.go`
- `internal/infra/postgres/*cost*_repo.go`
- `internal/server/admin/handlers/*cost*.go`
- `internal/server/admin/server.go`
- `api/openapi.yaml`

## Implementation Tasks

- [x] Add migration for timestamps and version.
- [x] Add domain validation using existing tag parsing.
- [x] Add repository with duplicate-tag and version-conflict handling.
- [x] Add handlers for `GET`, `POST`, `GET {id}`, `PUT {id}`, and `DELETE {id}`.
- [x] Add handler and repository tests.

## Done Criteria

- [x] Duplicate `endpoint_tag` returns `409`.
- [x] Update requires matching `version`.
- [x] Delete soft deactivates the multiplier.
- [x] Mutations write structured audit events.
