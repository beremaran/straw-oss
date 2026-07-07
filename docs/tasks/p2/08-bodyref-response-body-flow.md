# 08 - BodyRef Response Body Flow

Status: done

## Objective

Implement the single response-body BodyRef mode resolved on 2026-07-07.

## Required Planning Docs

- `docs/planning/18-large-body-transport-p2.md`
- `docs/planning/29-operational-behavior.md`
- `docs/planning/32-open-decisions.md`

## Prerequisites

- Task 05 completed.
- Task 06 completed.
- Task 07 completed.

## Out of Scope

- Do not implement both response-body modes.
- Do not implement payload capture storage.
- Do not bypass response-size and outage tests.

## Expected Files

- Create or modify: Control/Egress response BodyRef flow.
- Test: response-body BodyRef tests.

## Steps

- [x] Read all required planning docs.
- [x] Implement exactly the resolved `P2 BodyRef Response-Body Mode`: executor streams through Control while teeing
      to object storage.
- [x] Enforce response size thresholds and `body_too_large` behavior.
- [x] Verify size/checksum and object retention.
- [x] Handle cancellation and cleanup.
- [x] Handle object storage outage according to Section 29 and the resolved decision.
- [x] Add tests for chosen mode, cancellation cleanup, checksum/size validation, object retention, outage, and
      response body too large.
- [x] Run focused response BodyRef tests.
- [x] Run `make check`.
- [x] Write a handoff note.

## Tests

- Focused response-body BodyRef tests.
- `make check`

## Acceptance Criteria

- Only one response BodyRef mode ships.
- Required open-decision acceptance tests pass.
- Object storage outage behavior is explicit.

## Handoff Notes

- Link the resolved decision and list any unsupported mode.

## Stop Conditions

- Stop if the response-body mode decision is removed or superseded.
- Stop if implementation starts to support both modes without explicit selection rules.
- Stop if a deferral would have no owning task file.
