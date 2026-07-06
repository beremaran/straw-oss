# 15 - Egress Metrics Exposure

Status: not started

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

- [ ] Read all required planning docs.
- [ ] Implement only Control-aggregated Egress metrics over the existing service boundary.
- [ ] Gate the feature behind an explicit enablement flag.
- [ ] Enforce metric cardinality bounds.
- [ ] Verify no worker-local metrics endpoint or worker metrics port is exposed.
- [ ] Define outage behavior when Control cannot aggregate worker metrics.
- [ ] Update deployment port mapping only for shipped endpoints.
- [ ] Add tests for cardinality, flag disabled/enabled behavior, no worker-local endpoint/port exposure, and outage
      behavior.
- [ ] Run focused metrics tests.
- [ ] Run `make check`.
- [ ] Write a handoff note.

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
