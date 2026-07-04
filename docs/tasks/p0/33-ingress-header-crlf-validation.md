# 33 - Ingress Header Value CR/LF Validation

Status: not started

## Objective

Reject client-supplied header values whose base64-decoded bytes contain CR/LF (or other control characters) at REST
ingress with `invalid_request`, instead of letting them pass validation and surface downstream as
`executor_internal_error`.

## Context (gap being closed)

The 2026-07-04 review found `validateHeaders` in `internal/control/request.go` checks the raw base64 string for CR/LF
(which base64 can never contain) and never scans the decoded bytes. A header whose decoded value contains CR/LF
therefore passes ingress and is only caught at the egress boundary (`safeOutboundHeader`), returning
`executor_internal_error` (HTTP 502) instead of a clean `invalid_request` (HTTP 400) at ingress. This is not an
injection hole — egress and Go's `net/http` both block the bytes before the wire — but the rejection is at the wrong
layer with a misleading code, and the existing test `TestValidateRequestCRInHeaderValue` documents that the decoded
value is not checked.

## Required Planning Docs

- `docs/planning/15-http-semantics.md` (header handling and rejection semantics)
- `docs/planning/27-security-controls.md` (header safety)
- `docs/planning/07-public-api-surface.md` (REST request schema)
- `docs/planning/30-testing-matrix.md` (HTTP validation row)

## Prerequisites

- Task 06 completed (REST request validation).

## Out of Scope

- Do not change injection-policy header validation (owned by tasks 20/22).
- Do not change the egress-side `safeOutboundHeader` defense (defense in depth stays).

## Expected Files

- Modify: `internal/control/request.go` (`validateHeaders` scans the base64-decoded value for CR/LF and rejects).
- Modify: `internal/control/handler_test.go` (replace the placeholder assertion with a real decoded-CR/LF rejection).
- Test: `internal/control/handler_test.go` / request validation tests.

## Steps

- [ ] Read the required planning docs.
- [ ] After base64-decoding each header value, reject with `invalid_request` if the decoded bytes contain CR or LF;
      keep the existing raw-string and HTTP-token checks.
- [ ] Update `TestValidateRequestCRInHeaderValue` to assert an actual rejection for a value whose decoded bytes
      contain CR/LF (the current test asserts no error and documents the gap).
- [ ] Update `docs/agents/testing-matrix-audit.md` so the "CR/LF header injection rejected" row maps to the ingress
      test rather than the egress fallback.
- [ ] Run the focused tests.
- [ ] Run `make check`.
- [ ] Write a handoff note.

## Tests

- `go test ./internal/control`
- `make check`

## Acceptance Criteria

- A header whose base64-decoded value contains CR or LF is rejected at REST ingress with HTTP 400 `invalid_request`.
- The egress-side check remains as defense in depth.
- The testing-matrix CR/LF row is backed by an ingress test.

## Handoff Notes

- Note the exact control characters rejected and where.

## Stop Conditions

- Stop if rejecting decoded control characters would contradict `docs/planning/15` header semantics.
