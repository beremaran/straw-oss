# 12 - Provider Adapter Protocol

Status: not started

## Objective

Define and implement the Provider Adapter protocol baseline chosen by the P2 open decision.

## Required Planning Docs

- `docs/planning/05-component-boundaries.md`
- `docs/planning/16-egress-execution.md`
- `docs/planning/12-nats-protocol.md`
- `docs/planning/13-protobuf-contract.md`
- `docs/planning/02-phase-boundaries.md`
- `docs/planning/32-open-decisions.md`

## Prerequisites

- Decision `P2 Provider Adapter Baseline` resolved.
- P0 task 23 completed.

## Out of Scope

- Do not implement marketplace discovery.
- Do not implement provider billing or account provisioning.
- Do not build a static adapter unless the decision selects one.

## Expected Files

- Create or modify: provider adapter protocol scaffolding.
- Test: provider adapter protocol tests.

## Steps

- [ ] Read all required planning docs.
- [ ] Record how the resolved decision handles the contradiction between P2 promising at least one static adapter and
      Section 32 allowing protocol scaffolding only.
- [ ] Reuse the same registration, heartbeat, assignment, stream, and error protocol as Egress workers.
- [ ] Add adapter executor type and capability handling.
- [ ] Define constrained destination-policy facts adapters must report back to Control.
- [ ] Enforce no marketplace, billing, or automatic provider-account behavior.
- [ ] Add tests for adapter registration, assignment, destination-policy enforcement, constrained error facts, and
      absence of marketplace/billing behavior.
- [ ] Run focused adapter protocol tests.
- [ ] Run `make check`.
- [ ] Write a handoff note.

## Tests

- Focused provider adapter protocol tests.
- `make check`

## Acceptance Criteria

- Provider adapters participate in the same wire protocol as Egress workers.
- The implementation matches the resolved baseline decision.
- Adapter errors are constrained and public-safe.

## Handoff Notes

- Link the resolved decision and state whether task 13 remains applicable.

## Stop Conditions

- Stop if `P2 Provider Adapter Baseline` is unresolved.
- Stop before adding marketplace or billing behavior.
- Stop if a deferral would have no owning task file.
