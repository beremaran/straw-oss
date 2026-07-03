# 08 - BodyRef Response Body Flow

Status: not started

## Objective

Implement the single response-body BodyRef mode chosen by the P2 open decision.

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

- [ ] Read all required planning docs.
- [ ] Implement exactly the chosen response-body mode from `P2 BodyRef Response-Body Mode`.
- [ ] Enforce response size thresholds and `body_too_large` behavior.
- [ ] Verify size/checksum and object retention.
- [ ] Handle cancellation and cleanup.
- [ ] Handle object storage outage according to Section 29 and the resolved decision.
- [ ] Add tests for chosen mode, cancellation cleanup, checksum/size validation, object retention, outage, and
      response body too large.
- [ ] Run focused response BodyRef tests.
- [ ] Run `make check`.
- [ ] Write a handoff note.

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

- Stop if the response-body mode decision is unresolved.
- Stop if implementation starts to support both modes without explicit selection rules.
- Stop if a deferral would have no owning task file.
