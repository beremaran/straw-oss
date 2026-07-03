# 11 - Telemetry Schema and Query Limits Spec

Status: not started

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

- [ ] Read all required planning docs.
- [ ] Specify request metadata list/detail schemas.
- [ ] Specify worker and audit telemetry schemas.
- [ ] Define filters, pagination, sorting, and maximum query windows.
- [ ] Define ClickHouse query limits and timeout behavior.
- [ ] Define tenant-facing omissions or stable aliases for internal topology fields.
- [ ] Add P1 telemetry test rows to the testing matrix or to the spec.
- [ ] Run `make check`.
- [ ] Write a handoff note.

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
