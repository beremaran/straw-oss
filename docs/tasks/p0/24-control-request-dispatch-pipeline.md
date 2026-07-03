# 24 - Control Request Dispatch Pipeline

Status: done

## Objective

Replace the synthetic success stub in the REST request handler with the real P0 Control dispatch pipeline from
admission through routing, assignment, streaming, response buffering, and canonical error handling.

## Required Planning Docs

- `docs/planning/09-canonical-request-lifecycle.md`
- `docs/planning/12-nats-protocol.md`
- `docs/planning/14-canonical-error-registry.md`
- `docs/planning/15-http-semantics.md`
- `docs/planning/20-rate-limits-and-quotas.md`
- `docs/planning/07-public-api-surface.md`

## Prerequisites

- Task 06 completed.
- Task 09 completed.
- Task 12 completed.
- Task 13 completed.
- Task 16 completed.
- Task 17 completed.
- Task 21 completed.
- Task 22 completed.
- Task 23 completed.

## Out of Scope

- Do not implement REST streaming (`/api/v1/requests:stream`).
- Do not implement HTTP proxy, CONNECT, MITM, BodyRef, payload capture, provider adapters, or HTTP/2.
- Do not add SDK/client retry behavior.

## Expected Files

- Modify: `internal/control/handler.go`
- Create or modify: `internal/control` dispatch pipeline components.
- Modify: `cmd/control/main.go`
- Test: focused request-dispatch and end-to-end local tests.

## Steps

- [x] Read all required planning docs.
- [x] Delete the synthetic success path in `internal/control/handler.go` and route validated requests into a dispatch
      pipeline.
- [x] Run rate-limit and quota admission using the Redis-backed components from task 21 before routing, returning
      canonical 429 errors with `retry_after_ms` where computable.
- [x] Capture the immutable tenant snapshot, evaluate routing with sticky-session behavior, and select an eligible
      exact worker session.
- [x] Resolve the destination-policy, header-injection, and fingerprint bundle using task 22.
- [x] Subscribe and flush the request-scoped `e2c` subject before publishing `AssignRequest`; enforce assignment ack
      timeout and fallback/replay boundaries from Section 9.
- [x] Publish `RequestStart`, stream inline request bodies as `DataFrame`s under credit limits, and send cancels on
      client disconnect, deadline, admin cancel, shutdown, or obsolete fallback attempts.
- [x] Consume executor-to-Control stream frames, validate sequence/offset/attempt/terminal rules, replenish credit, and
      buffer the upstream response up to `control.request.max_inline_response_body_bytes`.
- [x] Return the real REST success envelope with upstream status, headers, inline body, and timing; map every failure
      through the canonical error registry with public-safe details.
- [x] Add tests for admission ordering, route no-match/unavailable, assignment timeout, fallback before `RequestStart`,
      stream protocol errors, response body too large, upstream status passthrough, cancellation, and 429 retry_after.
- [x] Add an end-to-end local test: Control -> NATS -> Egress -> upstream -> Control returns the upstream's real status
      and body.
- [x] Run focused dispatch pipeline tests.
- [x] Run `make check`.
- [x] Write a handoff note.

## Tests

- Focused dispatch pipeline tests.
- End-to-end local round-trip test.
- `go test ./internal/control ./internal/egress ./cmd/...`
- `make check`

## Acceptance Criteria

- `POST /api/v1/requests` no longer returns a synthetic success envelope.
- A local end-to-end round trip returns the upstream server's real status, headers, body, and timings.
- Origin 3xx/4xx/5xx statuses pass through inside a successful JSON envelope; Straw transport failures return
  canonical ErrorResponse objects.
- Rate limits, quotas, routing, destination policy, assignment, streaming, timeout, cancellation, and response-size
  limits are all exercised by tests.

## Handoff Notes

- Document the pipeline order, fallback boundaries, and any remaining P1/P2 exclusions.
- List the focused end-to-end command used for verification.

## Stop Conditions

- Stop before adding streaming REST, proxy, CONNECT, MITM, BodyRef, payload capture, provider adapter, or HTTP/2 scope.
- Stop if a failure path has no canonical error-code mapping.
- Stop if a deferral would have no owning task file.
