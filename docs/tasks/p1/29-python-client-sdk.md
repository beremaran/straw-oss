# 29 - Python Client SDK

Status: not started

## Objective

Add a minimal Python client SDK for Straw's public request transport so Python callers can submit blocking JSON
requests, consume REST streaming frames incrementally, parse canonical public errors, and use the same replayable
defaults as the Go SDK without depending on Straw internals.

## Context (gap being closed)

This task was requested directly on 2026-07-08 to add a Python client SDK. Current code proves only the Go client SDK
exists:

- `sdk/doc.go:1-12` documents `sdk` as Straw's minimal Go SDK and lists only Go public types.
- `sdk/client.go:1-2` declares the existing SDK package as a Go client for Straw's public API.
- `sdk/stream.go:14-17` defines the Go client's `/api/v1/requests:stream` path and binary content type.
- `sdk/types.go:5-139` defines the Go request, response, error, stream-frame, and replayable-default types.
- `docs/tasks/p1/09-go-sdk.md:21` explicitly put non-Go SDKs out of scope for the original SDK task.
- `find . -maxdepth 3 \( -name 'pyproject.toml' -o -name 'setup.py' -o -name '*.py' \)` currently finds no Python
  client package; only `deploy/docker/kms-mock.py` exists.

## Required Planning Docs

- `docs/planning/02-phase-boundaries.md` (P1 SDK surfaces, lines ~59-73).
- `docs/planning/07-public-api-surface.md` (REST request schema, success/error behavior, and P1 REST Streaming Variant,
  lines ~18-182).
- `docs/planning/14-canonical-error-registry.md` (public ErrorResponse fields and canonical codes).
- `docs/planning/15-http-semantics.md` (client-visible HTTP semantics, trailers, and cancellation behavior).
- `docs/planning/31-implementation-order.md` (P1 SDK/CLI minimal surfaces, lines ~26-35).

## Prerequisites

- P1 task 09 completed (baseline Go SDK defines the client API behavior to mirror).
- P1 task 28 completed (REST streaming client behavior is implemented and tested in the Go SDK).

## Out of Scope

- Do not add retry orchestration beyond documented `replayable` defaults.
- Do not add Egress SDK, custom worker, BodyRef, MITM, payload-capture, or telemetry API clients.
- Do not add a CLI surface for Python.
- Do not publish to PyPI or add release automation.

## Expected Files

- Add: `python/straw/__init__.py` and `python/straw/client.py` for the public Python client package.
- Add: `python/tests/test_client.py` for blocking request, error, replayable default, and stream frame tests.
- Add: `python/README.md` or update `docs/public/sdk.md` with Python SDK usage if the repo's public docs should list
  both SDKs.
- Add or modify: minimal Python packaging metadata (`pyproject.toml` or `python/pyproject.toml`) only if the executor can
  do it without a new runtime dependency.

## Steps

- [ ] Read all required planning docs.
- [ ] Mirror the existing Go SDK's request/response/error data shape in Python using stdlib types or small dataclasses.
- [ ] Implement a blocking request method that posts JSON to `/api/v1/requests`, sets bearer auth when provided, parses
      successful response envelopes, parses non-200 public `ErrorResponse` bodies, and documents that upstream status is
      inside the response envelope.
- [ ] Implement a streaming request method that posts JSON to `/api/v1/requests:stream`, requests
      `application/vnd.straw.request-stream.v1+binary`, reads the 1-byte type plus 4-byte big-endian length frame format
      incrementally, and yields metadata, body, trailers, end, and error frames without buffering the whole response body.
- [ ] Preserve `replayable=true` defaults for `GET`, `HEAD`, and `OPTIONS` before submission.
- [ ] Add tests for JSON request encoding, bearer auth, canonical error parsing, replayable defaults, upstream status
      handling, stream frame parsing, malformed/truncated frames, and no full-response buffering before body chunks are
      yielded.
- [ ] Add minimal Python usage docs only for the implemented endpoints.
- [ ] Run focused Python tests, focused Go SDK tests if shared docs/contracts changed, then `make check`.
- [ ] Write a handoff note.

## Tests

- `python3 -m unittest discover python/tests`
- `go test ./sdk`
- `make check`

## Acceptance Criteria

- `python/straw/client.py` can submit `POST /api/v1/requests`, proven by a Python test that asserts the outgoing JSON
  envelope, `Authorization: Bearer ...`, and successful response parsing.
- Python canonical error handling parses non-200 public ErrorResponse bodies into a typed exception or equivalent public
  error object, proven by a Python test asserting `category`, `code`, `retryable`, `request_id`, and HTTP status.
- Python `GET`, `HEAD`, and `OPTIONS` requests default `replayable=true` before submission, while other methods do not,
  proven by Python tests.
- Python streaming support parses every documented frame type from `/api/v1/requests:stream` and yields body bytes before
  the terminal frame without buffering the complete stream, proven by Python tests with multiple body frames.
- `rg -n "github.com/beremaran/straw/v2/internal|\\.\\./internal|internal/" python` returns no matches; the Python SDK
  only talks to the public HTTP API.
- `rg -n "Python|python" docs/tasks/p1/09-go-sdk.md docs/tasks/p1/28-sdk-cli-rest-streaming-client.md` still shows those
  tasks did not claim Python SDK ownership; this task is the owner on the P1 board.

## Handoff Notes

- Record the Python package path, public API shape, error representation, streaming iterator behavior, packaging choice,
  and exact Python test command used.

## Stop Conditions

- Stop if implementing the Python client requires changing Control's public request or stream contract.
- Stop if a new Python runtime dependency is required for the smallest usable SDK.
- Stop if packaging location conflicts with the repo's module layout and no existing pattern resolves it.
- Stop if a deferral would have no owning task file.
