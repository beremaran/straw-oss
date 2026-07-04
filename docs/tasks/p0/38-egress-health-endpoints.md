# 38 - Egress Worker Health Endpoints

Status: done

## Objective

Expose local `/healthz` and `/readyz` on the egress worker per `docs/planning/23` ("P0 should prefer direct local
`/healthz` and `/readyz`" for egress), with readiness reflecting registration success and draining state.

## Context (gap being closed)

The 2026-07-04 review follow-up found the egress binary has no HTTP listener at all, so the `docs/planning/23`
P0-preferred worker health surface does not exist and the compose stack cannot healthcheck the worker container.
Control already has the pattern in `cmd/control/health.go`. Direct worker Prometheus `/metrics` scraping stays P1
(`docs/tasks/p1/15-egress-metrics-exposure.md`) and is not part of this task.

## Required Planning Docs

- `docs/planning/23-observability.md` (egress health endpoints)
- `docs/planning/29-operational-behavior.md` (worker draining semantics)
- `docs/planning/28-deployment.md` (port mapping rules; do not map unused ports)

## Prerequisites

- Task 17 completed (registration/heartbeat run loop the readiness signal hooks into).
- Note: in docker compose, `/readyz` only turns green once live registration succeeds, which requires task 35; the
  endpoint itself is fully testable without it.

## Out of Scope

- Do not expose `/metrics` on the worker (P1 task 15).
- Do not add remote health reporting beyond the existing heartbeat.

## Expected Files

- Modify: `internal/config/config.go` (egress `health_port` field with validation).
- Create: `cmd/egress/health.go` (mux with `/healthz` and `/readyz`, mirroring `cmd/control/health.go`).
- Modify: `cmd/egress/main.go` (serve the health mux; feed the ready signal).
- Modify: `internal/egress/runtime.go` (expose a ready/draining signal from the run loop — ready after successful
  registration, not ready while draining or after shutdown begins).
- Modify: `deploy/docker/egress.json` and the compose file (health port + container healthcheck).
- Test: `cmd/egress` health test mirroring `cmd/control/health_test.go`.

## Steps

- [x] Read all required planning docs.
- [x] Add the egress `health_port` config field with the same validation shape as Control's ports.
- [x] Serve `/healthz` (200 while the process runs) and `/readyz` (200 only after successful registration and while
      not draining) on the health port.
- [x] Wire the readiness transitions to the run loop: false at start, true on `RegisterAck` OK, false when draining
      begins.
- [x] Add the compose healthcheck and port wiring.
- [x] Add tests for the readiness transitions and both endpoints.
- [x] Run the focused tests.
- [x] Run `make check`.
- [x] Write a handoff note.

## Tests

- `go test ./cmd/... ./internal/egress`
- `make check`

## Acceptance Criteria

- `/healthz` returns 200 while the worker process is alive.
- `/readyz` returns 200 only after successful registration and returns non-200 while draining or before registration.
- The compose worker container has a healthcheck against the new port.

## Handoff Notes

- Document the readiness state transitions and the chosen port.

## Stop Conditions

- Stop before adding worker `/metrics` or new remote reporting.
- Stop if a deferral would have no owning task file.
