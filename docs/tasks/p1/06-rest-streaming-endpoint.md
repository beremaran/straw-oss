# 06 - REST Streaming Endpoint

Status: done

## Objective

Implement `POST /api/v1/requests:stream` using the resolved P1 binary frame format.

## Required Planning Docs

- `docs/planning/07-public-api-surface.md`
- `docs/planning/15-http-semantics.md`
- `docs/planning/12-nats-protocol.md`
- `docs/planning/32-open-decisions.md`

## Prerequisites

- Decision `P1 REST Streaming Response Format` resolved on 2026-07-06: use binary framing.
- Task 03 completed.

## Out of Scope

- Do not change `POST /api/v1/requests`.
- Do not implement BodyRef.
- Do not implement proxy ingress.

## Expected Files

- Create or modify: REST streaming handler.
- Test: REST streaming endpoint tests.

## Steps

- [x] Read all required planning docs.
- [x] Implement the binary frame format from Section 7: 1 byte frame type, 4 byte big-endian payload length, then
      payload bytes.
- [x] Emit `Content-Type: application/vnd.straw.request-stream.v1+binary`.
- [x] Reuse the raw streaming response path where possible.
- [x] Emit frame type 1 metadata before any frame type 2 body bytes.
- [x] Emit frame type 3 trailers, frame type 4 end, and frame type 5 error according to Section 7.
- [x] Handle upstream errors after partial body and client cancellation.
- [x] Enforce request-body limit and trailer behavior from the decision; do not apply inline response-body buffering
      limits to streamed response bytes.
- [x] Add tests for metadata ordering, partial upstream error, cancellation, body limits, trailers, and auth/RBAC.
- [x] Run focused REST streaming tests.
- [x] Run `make check`.
- [x] Write a handoff note.

## Tests

- Focused REST streaming endpoint tests.
- `make check`

## Acceptance Criteria

- `/api/v1/requests:stream` returns the resolved binary content type and frame layout exactly.
- Existing non-streaming REST remains unchanged.
- Required decision acceptance tests are implemented.

## Handoff Notes

- Link the resolved decision and list the implemented frame types.

## Stop Conditions

- Stop if planning changes away from the resolved binary framing before implementation starts.
- Stop before adding BodyRef behavior.
- Stop if a deferral would have no owning task file.
