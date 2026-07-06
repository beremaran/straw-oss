# 11 - Telemetry Schema and Query Limits Spec

Status: done

## Objective

Specify tenant-facing telemetry schemas, filters, pagination, and ClickHouse query limits before implementing telemetry
read APIs.

## Required Planning Docs

- `docs/planning/07-public-api-surface.md`
- `docs/planning/22-canonical-clickhouse-schema.md`
- `docs/planning/21-state-and-storage.md`
- `docs/planning/23-observability.md`

## Prerequisites

- P0 task 14 completed.

## Out of Scope

- Do not implement telemetry endpoints.
- Do not expose worker IDs, session IDs, or selected executor values to tenant-facing APIs.

## Expected Files

- Create: `docs/planning/b-telemetry-read-api.md`

## Steps

- [x] Read all required planning docs.
- [x] Specify request metadata list/detail schemas.
- [x] Specify worker and audit telemetry schemas.
- [x] Define filters, pagination, sorting, and maximum query windows.
- [x] Define ClickHouse query limits and timeout behavior.
- [x] Define tenant-facing omissions or stable aliases for internal topology fields.
- [x] Add P1 telemetry test rows to the testing matrix or to the spec.
- [x] Run `make check`.
- [x] Write a handoff note.

## Tests

- Documentation/spec review.
- `make check`

## Acceptance Criteria

- Telemetry read schemas are implementable without exposing internal topology.
- Query limits are explicit before endpoint code starts.
- No production code is changed.

## Handoff Notes

- Link the spec and note any unresolved query tradeoffs.

## Stop Conditions

- Stop before implementing telemetry handlers.
- Stop if tenant-facing fields would leak worker/session topology.
- Stop if a deferral would have no owning task file.
