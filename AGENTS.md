# Agent Guide

This is the entrypoint for coding agents working on Straw.

## Project

Straw is a Go distributed HTTP/HTTPS proxy control plane and egress worker. P0 is a vertical slice: one Control service, one official Go Egress Worker, REST request transport, Core NATS assignment, Postgres config, Redis runtime state, ClickHouse metadata, and docker-compose for local development.

Start with:

1. `docs/planning/01-purpose.md`
2. `docs/planning/02-phase-boundaries.md`
3. `docs/planning/04-canonical-architecture.md`
4. `docs/planning/05-component-boundaries.md`
5. `docs/planning/31-implementation-order.md`
6. The task file assigned from `docs/tasks/p0.md`

## Workflow

- Pick exactly one task from `docs/tasks/p0.md`.
- Read only the planning docs named by that task before editing.
- Keep P0 work inside P0. Do not build P1/P2 features unless the task explicitly says so.
- Prefer existing package boundaries and the standard library before new code or dependencies.
- Write the smallest test that fails for real behavior, then the smallest implementation that passes it.
- Update the task file status only when the work is actually complete and verified.
- Leave a handoff note using `docs/agents/templates/handoff.md`.

## Repo Map

- `cmd/control`: Control service entrypoint.
- `cmd/egress`: Egress worker entrypoint.
- `internal/control`: Control implementation packages.
- `internal/egress`: Egress worker implementation packages.
- `internal/config`: Config loading and validation.
- `internal/natsx`: NATS connection and subject helpers.
- `internal/testutil`: Shared test helpers.
- `api/proto/straw/v1`: Protobuf contracts.
- `migrations/postgres`: Postgres migrations.
- `deploy/docker`: Docker-local deployment files.
- `docs/planning`: Product and architecture source of truth.
- `docs/tasks`: Agent task packs.

## Verification

Run before handoff:

```sh
make check
```

`make check` runs `gofmt` verification and `go test ./...`.

## Stop Conditions

Stop and ask before continuing if:

- the assigned task conflicts with `docs/planning`;
- a task requires a P1/P2 decision from `docs/planning/32-open-decisions.md`;
- tests fail for reasons unrelated to your change;
- you need a new dependency;
- the smallest safe fix would cross task boundaries.
