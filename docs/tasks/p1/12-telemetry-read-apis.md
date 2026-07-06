# 12 - Telemetry Read APIs

Status: done

## Objective

Implement tenant-safe telemetry read APIs over ClickHouse metadata.

## Required Planning Docs

- `docs/planning/07-public-api-surface.md`
- `docs/planning/21-state-and-storage.md`
- `docs/planning/22-canonical-clickhouse-schema.md`
- `docs/planning/06-identity-roles-and-tenant-isolation.md`

## Prerequisites

- Task 11 completed.
- P0 task 14 completed.

## Out of Scope

- Do not add payload capture reads.
- Do not expose internal worker/session topology to tenant-scoped keys.
- Do not implement dashboards.

## Expected Files

- Create or modify: telemetry API handlers/stores.
- Test: telemetry API tests.

## Steps

- [x] Read all required planning docs.
- [x] Implement `GET /api/v1/telemetry/requests`.
- [x] Implement `GET /api/v1/telemetry/requests/{request_id}`.
- [x] Implement `GET /api/v1/telemetry/workers`.
- [x] Implement `GET /api/v1/telemetry/audit`.
- [x] Enforce tenant scoping, RBAC, query limits, pagination, and public-safe field omission/aliasing.
- [x] Add tests for tenant isolation, query bounds, role access, missing records, and topology redaction.
- [x] Run focused telemetry API tests.
- [x] Run `make check`.
- [x] Write a handoff note.

## Tests

- Focused telemetry API tests.
- `make check`

## Acceptance Criteria

- Telemetry APIs follow the task 11 spec.
- Tenant-facing responses do not expose worker IDs, session IDs, or selected executor unless aliased by the spec.
- Query limits protect ClickHouse.

## Handoff Notes

- Document supported filters and limits.

## Stop Conditions

- Stop if task 11 does not define the response schema.
- Stop if a deferral would have no owning task file.
