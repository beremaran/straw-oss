# 03 - Raw Streaming Response Path

Status: done

## Objective

Add the Control-to-client raw response streaming path needed by proxy modes and shared by the future REST streaming
endpoint.

## Required Planning Docs

- `docs/planning/07-public-api-surface.md`
- `docs/planning/12-nats-protocol.md`
- `docs/planning/15-http-semantics.md`

## Prerequisites

- Task 01 completed.
- Task 02 completed.

## Out of Scope

- Do not implement `/api/v1/requests:stream`.
- Do not implement CONNECT tunneling.
- Do not change the P0 REST JSON envelope.

## Expected Files

- Create or modify: Control response streaming components.
- Test: raw response streaming tests.

## Steps

- [x] Read all required planning docs.
- [x] Implement a raw response writer that maps upstream status, headers, body frames, and trailers to a client socket.
- [x] Preserve upstream 3xx/4xx/5xx statuses as normal upstream responses.
- [x] Apply NATS credit/backpressure so Control does not buffer unbounded response bodies.
- [x] Handle upstream errors after partial response according to the task 01 spec.
- [x] Handle client cancellation and propagate cancel frames to the request pipeline.
- [x] Add tests for status passthrough, large streaming bodies, client cancellation, upstream error after partial body,
      and trailer handling.
- [x] Run focused raw streaming tests.
- [x] Run `make check`.
- [x] Write a handoff note.

## Tests

- Focused raw streaming response tests.
- `make check`

## Acceptance Criteria

- Proxy modes can stream raw upstream responses without JSON envelopes.
- Backpressure prevents unbounded buffering.
- Cancellation reaches the running request.

## Handoff Notes

- Document how trailers and post-header errors are represented.

## Stop Conditions

- Stop before changing P0 REST response behavior.
- Stop if an ingress-specific trailer rule is undefined.
- Stop if a deferral would have no owning task file.
