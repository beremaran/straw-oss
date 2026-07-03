# 04 - Routing Ingress Type and Worker Capability

Status: not started

## Objective

Thread `ingress_type` through routing and worker capability checks so REST, HTTP proxy, CONNECT, and MITM requests can
be routed only to compatible workers.

## Required Planning Docs

- `docs/planning/10-routing-model.md`
- `docs/planning/26-config-management-api-surface.md`
- `docs/planning/11-worker-discovery-and-health.md`

## Prerequisites

- P0 task 17 completed.
- P0 task 19 completed.

## Out of Scope

- Do not implement proxy, CONNECT, or MITM listeners.
- Do not add new ingress modes beyond the documented enum.

## Expected Files

- Modify: routing model and worker credential/capability code.
- Test: routing and worker capability tests.

## Steps

- [ ] Read all required planning docs.
- [ ] Add `ingress_type` to route match evaluation for `rest`, `http_proxy`, `connect`, and `mitm`.
- [ ] Load worker credential `supported_ingress_modes` from durable config.
- [ ] Include advertised worker ingress modes in registration/heartbeat-derived capability state.
- [ ] Reject routes to workers that do not support the request ingress mode.
- [ ] Preserve REST behavior as the default P0 mode.
- [ ] Add tests for each ingress mode, unsupported workers, missing capability defaults, and tenant isolation.
- [ ] Run focused routing/capability tests.
- [ ] Run `make check`.
- [ ] Write a handoff note.

## Tests

- Focused routing and capability tests.
- `make check`

## Acceptance Criteria

- `ingress_type` participates in route matching.
- Worker credential capability gates prevent incompatible ingress routing.
- Existing REST routing stays compatible.

## Handoff Notes

- Document default ingress-mode behavior for old workers/config.

## Stop Conditions

- Stop before adding undocumented ingress values.
- Stop if a deferral would have no owning task file.
