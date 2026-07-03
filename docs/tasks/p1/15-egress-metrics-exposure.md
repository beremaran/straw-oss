# 15 - Egress Metrics Exposure

Status: not started

## Objective

Implement the egress metrics exposure mode chosen by the P1 open decision.

## Required Planning Docs

- `docs/planning/23-observability.md`
- `docs/planning/32-open-decisions.md`
- `docs/planning/28-deployment.md`

## Prerequisites

- Decision `P1 Egress Metrics Exposure` resolved.
- P0 task 25 completed.

## Out of Scope

- Do not expose worker-local metrics without endpoint access control.
- Do not add unrelated dashboard work.

## Expected Files

- Create or modify: egress metrics exposure or Control aggregation code.
- Modify: deployment templates if the decision exposes a worker-local port.
- Test: metrics exposure tests.

## Steps

- [ ] Read all required planning docs.
- [ ] Implement only the exposure mode chosen by `P1 Egress Metrics Exposure`.
- [ ] Enforce metric cardinality bounds.
- [ ] Enforce access control for any worker-local endpoint.
- [ ] Define outage behavior when Control cannot aggregate worker metrics.
- [ ] Update deployment port mapping only for shipped endpoints.
- [ ] Add tests for cardinality, endpoint access control, outage behavior, and disabled modes.
- [ ] Run focused metrics tests.
- [ ] Run `make check`.
- [ ] Write a handoff note.

## Tests

- Focused metrics exposure tests.
- `make check`

## Acceptance Criteria

- The implementation matches the resolved decision exactly.
- Metrics cannot expose unbounded tenant/request labels.
- Deployment docs expose only enabled ports.

## Handoff Notes

- Link the resolved decision and list metrics added.

## Stop Conditions

- Stop if `P1 Egress Metrics Exposure` is unresolved.
- Stop if a deferral would have no owning task file.
