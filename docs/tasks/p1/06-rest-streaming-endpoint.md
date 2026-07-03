# 06 - REST Streaming Endpoint

Status: not started

## Objective

Implement `POST /api/v1/requests:stream` after the P1 streaming response format decision is resolved.

## Required Planning Docs

- `docs/planning/07-public-api-surface.md`
- `docs/planning/15-http-semantics.md`
- `docs/planning/12-nats-protocol.md`
- `docs/planning/32-open-decisions.md`

## Prerequisites

- Decision `P1 REST Streaming Response Format` resolved.
- Task 03 completed.

## Out of Scope

- Do not change `POST /api/v1/requests`.
- Do not implement BodyRef.
- Do not implement proxy ingress.

## Expected Files

- Create or modify: REST streaming handler.
- Test: REST streaming endpoint tests.

## Steps

- [ ] Read all required planning docs.
- [ ] Implement the exact response framing chosen by `P1 REST Streaming Response Format`.
- [ ] Reuse the raw streaming response path where possible.
- [ ] Emit metadata before body bytes according to the chosen format.
- [ ] Handle upstream errors after partial body and client cancellation.
- [ ] Enforce body limit and trailer behavior from the decision.
- [ ] Add tests for metadata ordering, partial upstream error, cancellation, body limits, trailers, and auth/RBAC.
- [ ] Run focused REST streaming tests.
- [ ] Run `make check`.
- [ ] Write a handoff note.

## Tests

- Focused REST streaming endpoint tests.
- `make check`

## Acceptance Criteria

- `/api/v1/requests:stream` follows the resolved streaming format exactly.
- Existing non-streaming REST remains unchanged.
- Required decision acceptance tests are implemented.

## Handoff Notes

- Link the resolved decision and list the chosen framing.

## Stop Conditions

- Stop if `P1 REST Streaming Response Format` is unresolved.
- Stop before adding BodyRef behavior.
- Stop if a deferral would have no owning task file.
