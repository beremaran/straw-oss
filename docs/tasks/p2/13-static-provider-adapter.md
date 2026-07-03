# 13 - Static Provider Adapter

Status: not started

## Objective

Implement one static provider adapter only if the P2 Provider Adapter Baseline decision selects a real adapter.

## Required Planning Docs

- `docs/planning/05-component-boundaries.md`
- `docs/planning/16-egress-execution.md`
- `docs/planning/27-security-controls.md`

## Prerequisites

- Task 12 completed with a decision selecting a real static adapter.

## Out of Scope

- Do not implement this task if the baseline decision selected scaffolding only.
- Do not add marketplace, billing, or automatic account provisioning.
- Do not weaken destination policy enforcement.

## Expected Files

- Create: one static adapter implementation.
- Test: static adapter tests.

## Steps

- [ ] Read all required planning docs.
- [ ] Confirm task 12 selected a real static adapter.
- [ ] Implement operator-configured provider credentials and adapter startup.
- [ ] Enforce equivalent destination policy through provider-specific controls.
- [ ] Report constrained execution facts back to Control.
- [ ] Use the provider adapter protocol from task 12 for registration, heartbeat, assignment, stream, and errors.
- [ ] Add tests for registration, destination denial, provider failure mapping, credential redaction, and no marketplace
      behavior.
- [ ] Run focused static adapter tests.
- [ ] Run `make check`.
- [ ] Write a handoff note.

## Tests

- Focused static adapter tests.
- `make check`

## Acceptance Criteria

- Exactly one static adapter is implemented if the decision requires it.
- Destination policy enforcement is equivalent or the request is rejected.
- No marketplace or billing scope is added.

## Handoff Notes

- Document provider-specific constraints and redactions.

## Stop Conditions

- Stop if task 12 selected protocol scaffolding only.
- Stop if a deferral would have no owning task file.
