# 12 - Error Registry and Mapping

Status: not started

## Objective

Implement the canonical ErrorResponse registry and HTTP/retry/category mapping.

## Required Planning Docs

- `docs/planning/14-canonical-error-registry.md`
- `docs/planning/15-http-semantics.md`
- `docs/planning/30-testing-matrix.md`

## Prerequisites

- Task 02 completed.
- Task 06 completed.
- Task 10 completed.
- Task 11 completed.

## Out of Scope

- Do not change upstream 4xx/5xx passthrough envelope semantics.
- Do not add undocumented error codes.
- Do not expose secret values in error details.

## Expected Files

- Create or modify: `internal/control`
- Create or modify: shared error mapping package if already established.
- Test: error registry and HTTP mapping tests.

## Steps

- [ ] Read all required planning docs.
- [ ] Define every canonical P0 ErrorCode in one registry.
- [ ] Map each ErrorCode to HTTP status, retryability, category, and public-safe detail behavior.
- [ ] Wire Control REST errors through the registry.
- [ ] Preserve upstream origin 4xx/5xx as successful API envelopes with upstream status.
- [ ] Add tests that every ErrorCode maps exactly once and origin errors are not ErrorResponse.
- [ ] Run focused error mapping tests.
- [ ] Run `make check`.
- [ ] Write a handoff note.

## Tests

- Focused error registry tests.
- `go test ./...`
- `make check`

## Acceptance Criteria

- Every P0 ErrorCode has a mapping.
- ErrorResponse output is public-safe.
- Origin 4xx/5xx passthrough is not treated as Straw transport failure.
- Tests cover the error mapping rows in `docs/planning/30-testing-matrix.md`.

## Handoff Notes

- List any error code names added or renamed.
- Note where new code should add future errors.

## Stop Conditions

- Stop before inventing new error categories.
- Stop if planning docs do not define the error code needed.
