# 05 - BodyRef Transport Selection

Status: not started

## Objective

Enable BodyRef transport selection after the response-body mode decision is resolved.

## Required Planning Docs

- `docs/planning/18-large-body-transport-p2.md`
- `docs/planning/13-protobuf-contract.md`
- `docs/planning/14-canonical-error-registry.md`
- `docs/planning/24-static-configuration.md`
- `docs/planning/32-open-decisions.md`

## Prerequisites

- Decision `P2 BodyRef Response-Body Mode` resolved.
- P0 task 24 completed.

## Out of Scope

- Do not implement object storage client internals (task 06).
- Do not ship both response-body modes.
- Do not enable BodyRef in P0.

## Expected Files

- Create or modify: body transport selection code/config.
- Test: transport selection tests.

## Steps

- [ ] Read all required planning docs.
- [ ] Add config for large-body threshold and enabled body transports.
- [ ] Select DataFrames, S3 BodyRef, DirectStreamRef, or `body_too_large` according to Section 18.
- [ ] Reject unsupported BodyRef variants when config disables them.
- [ ] Map unavailable body refs to `body_ref_unavailable`.
- [ ] Record and enforce the single chosen response-body mode from the open decision.
- [ ] Add tests for threshold edges, disabled transports, BodyRef variants, response mode selection, and errors.
- [ ] Run focused transport selection tests.
- [ ] Run `make check`.
- [ ] Write a handoff note.

## Tests

- Focused BodyRef selection tests.
- `make check`

## Acceptance Criteria

- Body transport selection matches the Section 18 table.
- Only the decided response-body mode is enabled.
- P0 DataFrame behavior remains unchanged for small bodies.

## Handoff Notes

- Link the resolved decision and list config keys.

## Stop Conditions

- Stop if `P2 BodyRef Response-Body Mode` is unresolved.
- Stop if a deferral would have no owning task file.
