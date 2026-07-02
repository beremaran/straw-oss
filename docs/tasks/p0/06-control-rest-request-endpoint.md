# 06 - Control REST Request Endpoint

Status: not started

## Objective

Implement Control's minimal synchronous REST `/api/v1/requests` endpoint for P0.

## Required Planning Docs

- `docs/planning/07-public-api-surface.md`
- `docs/planning/09-canonical-request-lifecycle.md`
- `docs/planning/14-canonical-error-registry.md`
- `docs/planning/15-http-semantics.md`
- `docs/planning/30-testing-matrix.md`

## Prerequisites

- Task 05 completed.
- Config snapshots can be loaded.

## Out of Scope

- Do not implement REST response streaming.
- Do not implement HTTP forward proxy, CONNECT, MITM, redirects, or HTTP/2.
- Do not implement SDK, CLI, or UI.

## Expected Files

- Create or modify: `internal/control`
- Modify: `cmd/control/main.go`
- Test: Control REST handler tests.

## Steps

- [ ] Read all required planning docs.
- [ ] Define request and response envelope types for `/api/v1/requests`.
- [ ] Reject invalid fields, URL fragments, Host header override, unsupported CONNECT, redirect flags, and capture hints other than `none`.
- [ ] Preserve duplicate headers according to the REST schema plan.
- [ ] Enforce inline body limits.
- [ ] Return upstream status inside a successful API envelope and Straw transport errors as ErrorResponse.
- [ ] Add handler tests for valid request, invalid fields, duplicate headers, body limits, CONNECT rejection, upstream 404/500 envelope, and Straw errors.
- [ ] Run focused Control handler tests.
- [ ] Run `make check`.
- [ ] Write a handoff note.

## Tests

- Focused Control REST tests.
- `go test ./...`
- `make check`

## Acceptance Criteria

- `/api/v1/requests` exists and validates P0 request shape.
- Unsupported P1/P2 features are rejected.
- Tests cover the REST schema and REST outcome rows in `docs/planning/30-testing-matrix.md`.

## Handoff Notes

- Include sample valid and invalid request payloads.
- Note any execution path still stubbed by later tasks.

## Stop Conditions

- Stop before adding streaming or proxy behavior.
- Stop if response semantics conflict with `docs/planning/15-http-semantics.md`.
