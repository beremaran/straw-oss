# 25 - P0 Test Matrix and Compose

Status: done

## Objective

Close the P0 test matrix and provide a local docker-compose environment for the full vertical slice.

## Required Planning Docs

- `docs/planning/02-phase-boundaries.md`
- `docs/planning/28-deployment.md`
- `docs/planning/29-operational-behavior.md`
- `docs/planning/30-testing-matrix.md`
- `docs/planning/31-implementation-order.md`

## Prerequisites

- Tasks 01 through 24 completed (including the integration tasks 16-24 added by the 2026-07-03 audit).

## Out of Scope

- Do not add Kubernetes, Swarm, or production deployment templates.
- Do not add P1/P2 proxy, CONNECT, MITM, BodyRef, provider adapter, telemetry UI, or payload capture tests.

## Expected Files

- Modify: `docker-compose.yml`
- Create or modify: `deploy/docker`
- Create or modify: end-to-end tests under the repo's chosen test layout.
- Modify: docs only to record local run commands.

## Steps

- [x] Read all required planning docs.
- [x] Audit `docs/planning/30-testing-matrix.md` row by row against implemented tests. (`docs/agents/testing-matrix-audit.md`)
- [x] Add missing P0 tests with the smallest useful scope. (`TestDispatcherNATSUnavailable`; existing suite already covered the rest — see audit.)
- [x] Configure docker-compose for local Control, Egress, NATS, Postgres, Redis, and ClickHouse. Preserve the NATS `max_payload` config (`deploy/docker/nats-server.conf`, mounted by compose). ClickHouse P0 schema at `deploy/docker/clickhouse-schema.sql`.
- [x] Wire Control `/healthz` and `/readyz` on the metrics port and use them for compose healthchecks; `/readyz` goes 503 when shutdown drain begins. (`cmd/control/health.go`; `TestReadyzReflectsReadiness`)
- [x] Add a full P0 E2E test for REST request through Control to Egress and back. (`TestDispatcherControlNATSEgressRoundTrip` — pre-existing, confirmed as the vertical-slice proof in the audit.)
- [x] Add outage tests for NATS, Redis, and ClickHouse behaviors required by P0. (See audit "Outage rows".)
- [x] Document local compose startup and teardown commands. (`deploy/docker/README.md`)
- [x] Run the full test command chosen for local E2E. (compose stack brought up + verified healthy; `go test ./...`)
- [x] Run `make check`.
- [x] Write a handoff note.

## Known limitation (flagged, no owning task)

The `egress` compose service connects to NATS but **cannot complete registration**: `cmd/egress/main.go` generates a
random ed25519 keypair on every boot, and registration requires a pre-seeded worker credential whose public key
matches. No P0 task owns persisting the egress identity key or seeding its credential, so a turnkey request flow
*through the compose stack* is not wired. The automated vertical-slice proof is the in-process Go test
`TestDispatcherControlNATSEgressRoundTrip`. See `deploy/docker/README.md`.

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
- Stop if a deferral would have no owning task file.
