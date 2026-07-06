# 15 - Outbound HTTP/2

Status: done

## Objective

Implement outbound HTTP/2 behind an explicit tested feature flag after task 14 defines the semantics.

## Required Planning Docs

- `docs/planning/15-http-semantics.md`
- `docs/planning/16-egress-execution.md`
- `docs/planning/02-phase-boundaries.md`

## Prerequisites

- Task 14 completed.

## Out of Scope

- Do not enable HTTP/2 by default.
- Do not implement ingress HTTP/2 or MITM ALPN.
- Do not change HTTP/1.1 defaults.

## Expected Files

- Modify: Egress outbound transport.
- Test: outbound HTTP/2 tests.

## Steps

- [x] Read all required planning docs.
- [x] Add explicit default-off feature flag/config for outbound HTTP/2.
- [x] Implement outbound HTTP/2 according to task 14.
- [x] Preserve downgrade behavior from the task 14 spec.
- [x] Preserve destination policy and dial-target invariants.
- [x] Keep HTTP/1.1 behavior unchanged when disabled.
- [x] Add tests for disabled default, enabled path, downgrade, cancellation, trailers, and policy enforcement.
- [x] Run focused outbound HTTP/2 tests.
- [x] Run `make check`.
- [x] Write a handoff note.

## Tests

- Focused outbound HTTP/2 tests.
- `make check`

## Acceptance Criteria

- Outbound HTTP/2 is feature-flagged and disabled by default.
- Implementation follows task 14 exactly.
- HTTP/1.1 remains the P0/P1 default.

## Handoff Notes

- Document flag/config and downgrade behavior.

## Stop Conditions

- Stop if task 14 does not define required semantics.
- Stop if a deferral would have no owning task file.
