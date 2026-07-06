# 17 - Worker Loss and NATS Outage Hardening

Status: done

## Objective

Harden worker-loss and NATS-outage behavior beyond the P0 baseline, including the explicit decision on pre-connect
fallback after `RequestStart`.

## Required Planning Docs

- `docs/planning/29-operational-behavior.md`
- `docs/planning/09-canonical-request-lifecycle.md`
- `docs/planning/12-nats-protocol.md`
- `docs/planning/11-worker-discovery-and-health.md`

## Prerequisites

- P0 task 24 completed.

## Out of Scope

- Do not add automatic client retry workflows.
- Do not change replay rules without tests.
- Do not add persistent request queues.

## Expected Files

- Create or modify: hardening tests and minimal runtime fixes.
- Test: outage and worker-loss tests.

## Steps

- [x] Read all required planning docs.
- [x] Audit current worker-loss and NATS-outage behavior against Section 29.
- [x] Decide whether to include pre-connect fallback after `RequestStart` using `OutboundStartFrame`; document the
      choice before implementation.
- [x] Add worker-loss tests before `RequestStart`, after `RequestStart`, and after partial response.
- [x] Add NATS outage tests for assignment, stream, and in-flight loss behavior.
- [x] Tighten cooldown and synthesized terminal outcome behavior where tests reveal gaps.
- [x] Add metrics/logging assertions for outage paths where available.
- [x] Run focused outage hardening tests.
- [x] Run `make check`.
- [x] Write a handoff note.

## Tests

- Focused worker-loss and NATS-outage tests.
- `make check`

## Acceptance Criteria

- Worker-loss and NATS-outage behavior matches Section 29 under test.
- Replay/fallback boundaries are explicit and tested.
- No persistent queue behavior is added.

## Handoff Notes

- Document the pre-connect fallback decision.

## Stop Conditions

- Stop before adding persistent queues or automatic retry workflows.
- Stop if a deferral would have no owning task file.
