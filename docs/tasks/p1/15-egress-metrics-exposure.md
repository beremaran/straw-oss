# 15 - Egress Metrics Exposure

Status: done

## Objective

Implement Control-aggregated Egress metrics behind an explicit enablement flag.

## Required Planning Docs

- `docs/planning/23-observability.md`
- `docs/planning/32-open-decisions.md`
- `docs/planning/28-deployment.md`

## Prerequisites

- Decision `P1 Egress Metrics Exposure` resolved on 2026-07-06: Control-aggregated metrics only, behind an explicit
  enablement flag.
- P0 task 25 completed.

## Out of Scope

- Do not expose a worker-local `/metrics` endpoint or map a worker metrics port.
- Do not add unrelated dashboard work.

## Expected Files

- Create or modify: Egress telemetry reporting, Control aggregation, and the explicit enablement flag.
- Modify: deployment templates only to document the flag or Control-side exposure. Do not add worker metrics ports.
- Test: metrics exposure tests.

## Steps

- [x] Read all required planning docs.
- [x] Implement only Control-aggregated Egress metrics over the existing service boundary.
- [x] Gate the feature behind an explicit enablement flag.
- [x] Enforce metric cardinality bounds.
- [x] Verify no worker-local metrics endpoint or worker metrics port is exposed.
- [x] Define outage behavior when Control cannot aggregate worker metrics.
- [x] Update deployment port mapping only for shipped endpoints.
- [x] Add tests for cardinality, flag disabled/enabled behavior, no worker-local endpoint/port exposure, and outage
      behavior.
- [x] Run focused metrics tests.
- [x] Run `make check`.
- [x] Write a handoff note.

## Tests

- Focused metrics exposure tests.
- `make check`

## Acceptance Criteria

- The implementation matches the resolved decision exactly.
- Metrics cannot expose unbounded tenant/request labels.
- Deployment docs expose only Control-side metrics surfaces and the explicit flag; no worker metrics port is shipped.

## Handoff Notes

- Link the resolved decision and list metrics added.

## Stop Conditions

- Stop if implementation would require direct worker Prometheus scraping.
- Stop if a deferral would have no owning task file.
