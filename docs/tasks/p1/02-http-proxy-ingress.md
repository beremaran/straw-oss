# 02 - HTTP Proxy Ingress

Status: done

## Objective

Implement the HTTP forward proxy listener on port 8081 and translate accepted proxy requests into the same internal
request pipeline used by REST.

## Required Planning Docs

- `docs/planning/07-public-api-surface.md`
- `docs/planning/15-http-semantics.md`
- `docs/planning/10-routing-model.md`
- `docs/planning/28-deployment.md`
- `docs/planning/27-security-controls.md`

## Prerequisites

- Task 01 completed.
- P0 task 24 completed.

## Out of Scope

- Do not implement CONNECT.
- Do not implement REST streaming.
- Do not implement MITM.

## Expected Files

- Create or modify: proxy ingress package under existing Control boundaries.
- Modify: `cmd/control/main.go`
- Test: HTTP proxy ingress tests.

## Steps

- [x] Read all required planning docs.
- [x] Add a listener on port 8081 behind static config.
- [x] Authenticate proxy requests according to the task 01 spec.
- [x] Parse HTTP proxy requests into the internal decoded request model.
- [x] Strip `Proxy-Authorization`, `X-Straw-*`, hop-by-hop headers, and invalid internal routing headers.
- [x] Reject CONNECT on this listener unless task 05 owns the request.
- [x] Send proxy responses without the REST JSON envelope. Full unbuffered raw streaming/backpressure was completed by
      `docs/tasks/p1/03-raw-streaming-response-path.md`.
- [x] Add tests for auth, header stripping, routing input, malformed requests, and raw upstream response passthrough.
- [x] Run focused proxy ingress tests.
- [x] Run `make check`.
- [x] Write a handoff note.

## Tests

- Focused proxy ingress tests.
- `make check`

## Acceptance Criteria

- HTTP forward proxy requests can use the P0 dispatch pipeline without the REST JSON envelope.
- `Proxy-Authorization` and internal routing headers are never forwarded outbound.
- Port 8081 is only exposed when the listener is enabled.

## Handoff Notes

- Document listener config and supported request forms.

## Stop Conditions

- Stop before accepting CONNECT.
- Stop before adding MITM behavior.
- Stop if a deferral would have no owning task file.
