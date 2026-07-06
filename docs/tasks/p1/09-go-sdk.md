# 09 - Go SDK

Status: done

## Objective

Create the minimal Go SDK for Straw request transport and public error handling.

## Required Planning Docs

- `docs/planning/07-public-api-surface.md`
- `docs/planning/14-canonical-error-registry.md`
- `docs/planning/15-http-semantics.md`

## Prerequisites

- P0 task 24 completed.

## Out of Scope

- Do not add non-Go SDKs.
- Do not add retry orchestration beyond documented replayable defaults.
- Do not wrap P2 features.

## Expected Files

- Create: Go SDK package under repo-approved boundary.
- Test: SDK client tests.

## Steps

- [x] Read all required planning docs.
- [x] Implement typed client calls for `POST /api/v1/requests`.
- [x] Add typed `ErrorResponse` parsing using lower-snake-case categories and codes.
- [x] Default `replayable=true` for GET, HEAD, and OPTIONS only before submission.
- [x] Document that clients inspect the JSON envelope `status`, not the outer HTTP status, for upstream status.
- [x] Add support for `/api/v1/requests:stream` only if task 06 is complete. Task 06 is not complete, so no stream API was added.
- [x] Add tests for request encoding, error parsing, replayable defaults, upstream status handling, and cancellation.
- [x] Run focused SDK tests.
- [x] Run `make check`.
- [x] Write a handoff note.

## Tests

- Focused SDK tests.
- `make check`

## Acceptance Criteria

- The SDK can submit P0 REST requests and parse canonical errors.
- Replayable defaults match Section 7 and Section 10.
- No P2-only API surface is exposed.

## Handoff Notes

- Document package path, public types, and supported endpoints.

## Stop Conditions

- Stop before adding retry workflows or non-Go SDKs.
- Stop if a deferral would have no owning task file.
