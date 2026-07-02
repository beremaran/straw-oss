# 01 - Repository Scaffold, Config Loader, Schema Validation, Generated Protobuf

Status: not started

## Objective

Create the first buildable Go scaffold for P0, including static config loading, config validation, and generated protobuf plumbing.

## Required Planning Docs

- `docs/planning/02-phase-boundaries.md`
- `docs/planning/04-canonical-architecture.md`
- `docs/planning/05-component-boundaries.md`
- `docs/planning/13-protobuf-contract.md`
- `docs/planning/24-static-configuration.md`
- `docs/planning/30-testing-matrix.md`
- `docs/planning/31-implementation-order.md`

## Prerequisites

- This is the first implementation task.
- `go test ./...` passes before starting.

## Out of Scope

- Do not implement Control REST transport.
- Do not implement NATS assignment.
- Do not implement Postgres, Redis, ClickHouse, or worker state.
- Do not add P1/P2 proxy, CONNECT, MITM, BodyRef, or payload capture behavior.

## Expected Files

- Create: `internal/config`
- Create: `api/proto/straw/v1`
- Create: `buf.yaml` and `buf.gen.yaml` only if protobuf generation is implemented in this task.
- Modify: `cmd/control/main.go`
- Modify: `cmd/egress/main.go`
- Test: `internal/config/*_test.go`

## Steps

- [ ] Read all required planning docs.
- [ ] Define the smallest static config shape needed to start Control and Egress locally.
- [ ] Add table-driven tests for valid config, missing required fields, invalid limits, and unknown fields.
- [ ] Implement the config loader with standard library parsing where possible; add no new dependency unless the file format requires it.
- [ ] Add schema validation in `internal/config`.
- [ ] Wire `cmd/control` and `cmd/egress` to load config and exit clearly on invalid config.
- [ ] Add protobuf generation config only if the contract files are introduced here.
- [ ] Run focused config tests.
- [ ] Run `make check`.
- [ ] Write a handoff note.

## Tests

- `go test ./internal/config`
- `go test ./cmd/control ./cmd/egress`
- `make check`

## Acceptance Criteria

- Control and Egress can load a local P0 static config.
- Invalid config fails before starting services.
- Config tests cover success and failure cases named above.
- Generated protobuf setup, if added, is reproducible from checked-in config.

## Handoff Notes

- State the config file format and why it was chosen.
- List any config keys intentionally deferred.

## Stop Conditions

- Stop if static config requirements contradict `docs/planning/24-static-configuration.md`.
- Stop before adding a new config dependency unless standard library parsing is insufficient.
