# 02 - Canonical Protobuf and Buf

Status: not started

## Objective

Define the canonical `straw.v1` protobuf contract and Buf checks for P0 transport.

## Required Planning Docs

- `docs/planning/12-nats-protocol.md`
- `docs/planning/13-protobuf-contract.md`
- `docs/planning/14-canonical-error-registry.md`
- `docs/planning/16-egress-execution.md`
- `docs/planning/30-testing-matrix.md`

## Prerequisites

- Task 01 completed.
- Protobuf generation path exists and compiles.

## Out of Scope

- Do not implement request execution.
- Do not add P2 BodyRef behavior beyond contract definitions required by the plan.
- Do not add provider adapter-specific messages.

## Expected Files

- Create or modify: `api/proto/straw/v1/*.proto`
- Modify: `buf.yaml`
- Modify: `buf.gen.yaml`
- Create or modify: generated Go protobuf files under the chosen generated-code path.
- Test: protobuf or Go contract tests for enum rejection and required fields.

## Steps

- [ ] Read all required planning docs.
- [ ] Define Envelope, StreamFrame, AssignRequest, DestinationPolicy, and ErrorResponse messages for P0.
- [ ] Include credit fields and sequence/offset fields required by the NATS protocol.
- [ ] Define enums with unknown-value rejection behavior covered by tests.
- [ ] Configure `buf lint` and `buf breaking` for the repository.
- [ ] Generate Go code.
- [ ] Add contract tests proving BodyRefFrame compiles, AssignRequest credit fields exist, and unknown enums are rejected at validation boundaries.
- [ ] Run `buf lint`.
- [ ] Run `buf breaking` against the configured baseline if available.
- [ ] Run `make check`.
- [ ] Write a handoff note.

## Tests

- `buf lint`
- `buf breaking`
- `go test ./...`
- `make check`

## Acceptance Criteria

- `straw.v1` protobuf files are checked in.
- Generated Go code compiles.
- Buf commands are documented and runnable.
- Tests cover the protobuf rows required by `docs/planning/30-testing-matrix.md`.

## Handoff Notes

- State where generated Go files live.
- State how to regenerate protobuf files.

## Stop Conditions

- Stop if Buf is unavailable and no repo-approved install path exists.
- Stop if the protobuf contract conflicts with `docs/planning/13-protobuf-contract.md`.
