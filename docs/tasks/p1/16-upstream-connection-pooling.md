# 16 - Upstream Connection Pooling

Status: done

## Objective

Specify optional upstream connection pooling and the feature flag/testing bar required before any implementation.

## Required Planning Docs

- `docs/planning/16-egress-execution.md`
- `docs/planning/02-phase-boundaries.md`
- `docs/planning/27-security-controls.md`
- `docs/planning/30-testing-matrix.md`
- `docs/planning/24-static-configuration.md`

## Prerequisites

- P0 task 11 completed.

## Out of Scope

- Do not implement pooling in this spec task.
- Do not enable outbound HTTP/2.
- Do not weaken the resolver/validator/dialer invariant.

## Expected Files

- Create: `docs/planning/b-upstream-connection-pooling.md`
- Modify: `docs/planning/32-open-decisions.md` only if the spec records a decision proposal.

## Steps

- [x] Read all required planning docs.
- [x] Define the feature flag, default-off behavior, and config keys.
- [x] Define per-tenant/per-host pooling boundaries and eviction behavior.
- [x] Prove the design preserves the resolved-IP dial-target invariant.
- [x] Define tests for disabled default, enabled path, SSRF policy, connection reuse safety, and shutdown.
- [x] Define observability and failure modes.
- [x] Add or reference required test-matrix rows.
- [x] Run `make check`.
- [x] Write a handoff note.

## Tests

- Documentation/spec review.
- `make check`

## Acceptance Criteria

- Pooling remains default-off and explicitly tested before code work starts.
- The design does not rely on cross-request reuse for correctness.
- The SSRF invariant is preserved.

## Handoff Notes

- Link the spec and list implementation tasks it enables.

## Stop Conditions

- Stop before changing Egress transport code.
- Stop if pooling would cause a second DNS resolution after policy validation.
- Stop if a deferral would have no owning task file.
