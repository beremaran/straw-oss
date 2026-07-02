# 11 - Egress Outbound Execution

Status: done

## Objective

Implement the official Go Egress outbound HTTP execution path with P0 transport defaults, deadlines, and resolved-IP DestinationPolicy enforcement.

## Required Planning Docs

- `docs/planning/05-component-boundaries.md`
- `docs/planning/13-protobuf-contract.md` (executor-emittable ErrorCode set and ErrorFrame rules)
- `docs/planning/15-http-semantics.md`
- `docs/planning/16-egress-execution.md`
- `docs/planning/27-security-controls.md`
- `docs/planning/30-testing-matrix.md`

## Prerequisites

- Task 10 completed.

## Out of Scope

- Do not let Egress query Postgres, Redis, or ClickHouse.
- Do not enable outbound HTTP/2 or upstream keep-alives except behind an explicit tested P0 feature flag.
- Do not implement redirects, CONNECT, MITM, payload capture, or provider adapters.

## Expected Files

- Create or modify: `internal/egress`
- Modify: `cmd/egress/main.go`
- Test: Egress execution and destination policy tests.

## Steps

- [x] Read all required planning docs.
- [x] Implement outbound HTTP/HTTPS execution from Control-resolved instructions.
- [x] Apply Control-resolved header and cookie injection operations.
- [x] Enforce total request deadline.
- [x] Enforce DestinationPolicy against resolved IPs without querying Control databases.
- [x] Disable redirects, CONNECT, outbound HTTP/2, and upstream keep-alives for P0.
- [x] Report failures per the Section 16 boundary: map the low-level fact to a canonical executor-emittable `ErrorCode`, emit it in `ErrorFrame` with the fact string in `details["fact"]`.
- [x] Add tests for deadline enforcement, resolved-IP deny, DNS rebinding guard, header validation, private IP denial, metadata IP denial, HTTP behavior defaults, and redaction boundaries.
- [x] Run focused Egress tests.
- [x] Run `make check`.
- [x] Write a handoff note.

## Tests

- Focused Egress execution tests.
- `go test ./...`
- `make check`

## Acceptance Criteria

- Egress performs outbound requests only from assigned instructions.
- DestinationPolicy is enforced locally on resolved IPs.
- P0 transport defaults are disabled as planned.
- Tests cover Egress policy, HTTP behavior, SSRF, timeout, and validation rows in `docs/planning/30-testing-matrix.md`.

## Handoff Notes

- Document transport defaults.
- State exactly which error facts and canonical codes Egress emits (must stay within the Section 13 executor-emittable set).

## Stop Conditions

- Stop before adding P1/P2 transport modes.
- Stop if a requirement would make Egress stateful over Control databases.
