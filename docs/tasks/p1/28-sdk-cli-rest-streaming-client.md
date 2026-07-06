# 28 - SDK and CLI REST Streaming Client Surface

Status: not started

## Objective

Expose the P1 `/api/v1/requests:stream` binary framing endpoint through the Go SDK and the CLI so clients can submit a
request once, receive upstream metadata before body bytes, consume body chunks without buffering the full response, and
observe trailers, terminal timing, and post-metadata error frames.

## Context (gap being closed)

P1 task 09 deliberately skipped SDK stream support while task 06 was incomplete. Task 06 now owns the server endpoint,
but the current SDK and CLI still expose only the non-streaming JSON request path:

- `sdk/client.go:14` defines only `requestsPath = "/api/v1/requests"`.
- `sdk/client.go:52` exposes only `Client.Do`, which reads the full JSON response body before returning.
- `sdk/doc.go:3-4` lists only `POST /api/v1/requests` as a supported SDK endpoint.
- `internal/cli/cli.go:117-150` implements the `request` command through `sdk.Client.Do`, so the CLI also buffers the
  non-streaming JSON envelope and has no stream mode.
- `docs/agents/handoffs/p1-09-go-sdk.md:42` recorded `/api/v1/requests:stream` as out of scope because task 06 was not
  complete; this task is the owner after task 06.

## Required Planning Docs

- `docs/planning/07-public-api-surface.md` (REST request schema and P1 REST Streaming Variant binary frame contract).
- `docs/planning/15-http-semantics.md` (origin status passthrough, trailer behavior, cancellation semantics).
- `docs/planning/31-implementation-order.md` (P1 SDK/CLI minimal surfaces).

## Prerequisites

- P1 task 06 completed (server-side `/api/v1/requests:stream` binary endpoint exists).
- P1 task 09 completed (baseline Go SDK exists).
- P1 task 10 completed (baseline CLI exists).

## Out of Scope

- Do not change Control's stream endpoint framing.
- Do not add BodyRef, MITM, proxy-ingress, retry orchestration, or non-Go SDKs.
- Do not buffer the entire streamed response in the SDK or CLI before exposing body bytes.

## Expected Files

- Modify: `sdk/client.go`, `sdk/types.go`, and `sdk/doc.go`.
- Modify: `sdk/client_test.go`.
- Modify: `internal/cli/cli.go`.
- Modify: `internal/cli/cli_test.go`.
- Modify: `docs/agents/handoffs/p1-09-go-sdk.md` and `docs/agents/handoffs/p1-10-cli.md` if their task-06 ownership
  notes would otherwise stay stale.

## Steps

- [ ] Read all required planning docs.
- [ ] Add SDK stream frame constants/types matching `docs/planning/07-public-api-surface.md`.
- [ ] Add an SDK method that posts to `/api/v1/requests:stream`, parses the 1-byte type plus 4-byte big-endian length
      framing, exposes metadata before body chunks, surfaces trailers/end timing, and returns public `ErrorResponse`
      data for pre-metadata HTTP errors and post-metadata error frames.
- [ ] Preserve auth, replayable defaults, and context cancellation behavior from `Client.Do`.
- [ ] Add a CLI stream mode for the existing `request` command using the same request-building flags, writing body
      bytes to stdout as they arrive and metadata/trailers/end/error information to stderr without leaking secrets.
- [ ] Add SDK tests for metadata-before-body ordering, partial-body error frames, trailers, terminal timing, HTTP error
      parsing before metadata, malformed frame length, and context cancellation.
- [ ] Add CLI tests proving stream mode uses the SDK stream endpoint, writes body bytes incrementally to stdout, and
      keeps metadata/error output separate from the body stream.
- [ ] Run focused tests, then `make check`.
- [ ] Write a handoff note.

## Tests

- `go test ./sdk ./internal/cli ./cmd/straw`
- `make check`

## Acceptance Criteria

- The SDK can call `POST /api/v1/requests:stream` and parse every documented frame type: metadata, body, trailers, end,
  and error, proven by SDK tests.
- The SDK does not buffer the full streamed body before exposing body chunks, proven by a test using multiple body
  frames.
- Pre-metadata HTTP ErrorResponse and post-metadata error frames are both surfaced with canonical error fields.
- The CLI exposes stream mode through the existing `request` command, sends the same request envelope shape as the
  non-streaming command, writes upstream body bytes to stdout, and sends metadata/trailers/end/error information to
  stderr, proven by CLI tests.
- `docs/agents/handoffs/p1-09-go-sdk.md` and `docs/agents/handoffs/p1-10-cli.md` no longer point stream client support
  only at the now-completed server task 06; they name this task where relevant.

## Handoff Notes

- Record the SDK API shape, the CLI invocation, stream frame parsing behavior, and how post-metadata errors are
  represented to callers.

## Stop Conditions

- Stop if the server framing in `docs/planning/07-public-api-surface.md` changes before implementation.
- Stop if streaming client support would require a retry workflow or BodyRef behavior.
- Stop if a deferral would have no owning task file.
