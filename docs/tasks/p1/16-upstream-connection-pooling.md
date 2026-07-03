# 16 - Upstream Connection Pooling

Status: not started

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

- [ ] Read all required planning docs.
- [ ] Define the feature flag, default-off behavior, and config keys.
- [ ] Define per-tenant/per-host pooling boundaries and eviction behavior.
- [ ] Prove the design preserves the resolved-IP dial-target invariant.
- [ ] Define tests for disabled default, enabled path, SSRF policy, connection reuse safety, and shutdown.
- [ ] Define observability and failure modes.
- [ ] Add or reference required test-matrix rows.
- [ ] Run `make check`.
- [ ] Write a handoff note.

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
