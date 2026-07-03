# 13 - Observability Dashboards

Status: not started

## Objective

Add operational dashboards for the documented metrics, health, outage, and SLO signals.

## Required Planning Docs

- `docs/planning/23-observability.md`
- `docs/planning/29-operational-behavior.md`

## Prerequisites

- P0 task 25 completed.
- Task 12 completed if dashboards use telemetry APIs.

## Out of Scope

- Do not implement telemetry API schemas.
- Do not expose tenant-private data in shared dashboards.

## Expected Files

- Create or modify: dashboard assets under deployment/observability paths.
- Test: dashboard lint/render checks where supported.

## Steps

- [ ] Read all required planning docs.
- [ ] Inventory the metrics, health checks, and SLOs dashboards must show.
- [ ] Add Control service, Egress worker, request lifecycle, NATS, Redis, Postgres, and ClickHouse dashboard views.
- [ ] Add outage panels matching Section 29 failure modes.
- [ ] Add alerts or documented thresholds for the Section 23 SLOs.
- [ ] Ensure dashboards do not expose secrets or tenant-private topology.
- [ ] Add render/lint checks or snapshot tests for dashboard assets.
- [ ] Run focused dashboard checks.
- [ ] Run `make check`.
- [ ] Write a handoff note.

## Tests

- Dashboard lint/render checks.
- `make check`

## Acceptance Criteria

- Dashboards cover documented P0/P1 operational signals.
- SLO panels and outage panels are present.
- Dashboard assets are deployable with the local/prod observability stack.

## Handoff Notes

- List dashboard files and required data sources.

## Stop Conditions

- Stop before adding tenant data not authorized by telemetry rules.
- Stop if a deferral would have no owning task file.
