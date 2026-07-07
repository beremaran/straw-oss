# 05 - BodyRef Transport Selection

Status: done

## Objective

Enable BodyRef transport selection using the response-body mode resolved on 2026-07-07.

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

- [x] Read all required planning docs.
- [x] Add config for large-body threshold and enabled body transports.
- [x] Select DataFrames, S3 BodyRef, DirectStreamRef, or `body_too_large` according to Section 18.
- [x] Reject unsupported BodyRef variants when config disables them.
- [x] Map unavailable body refs to `body_ref_unavailable`.
- [x] Enforce the resolved response-body mode from `docs/planning/32-open-decisions.md`: executor streams through
      Control while teeing to object storage. (Mode enforced at config/selection level; the streaming-tee runtime is
      owned by task 06/08, per Out of Scope.)
- [x] Add tests for threshold edges, disabled transports, BodyRef variants, response mode selection, and errors.
- [x] Run focused transport selection tests.
- [x] Run `make check`.
- [x] Write a handoff note.

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

- Stop if `P2 BodyRef Response-Body Mode` is removed or superseded.
- Stop if a deferral would have no owning task file.
