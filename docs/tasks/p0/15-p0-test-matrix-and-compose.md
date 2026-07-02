# 15 - P0 Test Matrix and Compose

Status: not started

## Objective

Close the P0 test matrix and provide a local docker-compose environment for the full vertical slice.

## Required Planning Docs

- `docs/planning/02-phase-boundaries.md`
- `docs/planning/28-deployment.md`
- `docs/planning/29-operational-behavior.md`
- `docs/planning/30-testing-matrix.md`
- `docs/planning/31-implementation-order.md`

## Prerequisites

- Tasks 01 through 14 completed.

## Out of Scope

- Do not add Kubernetes, Swarm, or production deployment templates.
- Do not add P1/P2 proxy, CONNECT, MITM, BodyRef, provider adapter, telemetry UI, or payload capture tests.

## Expected Files

- Modify: `docker-compose.yml`
- Create or modify: `deploy/docker`
- Create or modify: end-to-end tests under the repo's chosen test layout.
- Modify: docs only to record local run commands.

## Steps

- [ ] Read all required planning docs.
- [ ] Audit `docs/planning/30-testing-matrix.md` row by row against implemented tests.
- [ ] Add missing P0 tests with the smallest useful scope.
- [ ] Configure docker-compose for local Control, Egress, NATS, Postgres, Redis, and ClickHouse. Preserve the NATS `max_payload` config (`deploy/docker/nats-server.conf`, mounted by compose) — the stock NATS 1 MiB default fails Control startup validation against the default frame size.
- [ ] Wire Control `/healthz` and `/readyz` on the metrics port (planning Section 7) if not already implemented, and use them for compose healthchecks; `/readyz` must go non-2xx when shutdown drain begins.
- [ ] Add a full P0 E2E test for REST request through Control to Egress and back.
- [ ] Add outage tests for NATS, Redis, and ClickHouse behaviors required by P0.
- [ ] Document local compose startup and teardown commands.
- [ ] Run the full test command chosen for local E2E.
- [ ] Run `make check`.
- [ ] Write a handoff note.

## Tests

- Full local E2E command chosen by the implementation.
- `go test ./...`
- `make check`

## Acceptance Criteria

- Every P0 row in `docs/planning/30-testing-matrix.md` has a test or an explicit documented reason it is not applicable.
- docker-compose starts the P0 local stack.
- Full vertical slice test passes locally.
- No P1/P2 test rows are claimed as shipped.

## Handoff Notes

- Include compose commands and expected ports.
- Include any test rows still blocked and why.

## Stop Conditions

- Stop if a missing test requires an undecided P1/P2 policy.
- Stop before adding production deployment manifests.
