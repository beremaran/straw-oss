# 17 - Quota Reconciliation

Status: done

## Objective

Implement billing-grade quota reconciliation, as resolved by the P2 quota accuracy decision on 2026-07-07.

## Required Planning Docs

- `docs/planning/20-rate-limits-and-quotas.md`
- `docs/planning/22-canonical-clickhouse-schema.md`
- `docs/planning/32-open-decisions.md`
- `docs/planning/33-risks.md`

## Prerequisites

- Decision `P2 Quota Reconciliation Accuracy` resolved.
- P0 task 13 completed.
- P0 task 14 completed.

## Out of Scope

- Do not claim billing-grade accuracy without the required tests.
- Do not change P0 admission-control quota semantics without migration notes.

## Expected Files

- Create or modify: quota reconciliation job/store code.
- Test: quota reconciliation tests.

## Steps

- [x] Read all required planning docs.
- [x] Implement billing-grade reconciliation as chosen by `P2 Quota Reconciliation Accuracy`.
- [x] Define and use a durable usage-event source from ClickHouse.
- [x] Add idempotent aggregation keys.
- [x] Handle late events and correction policy for Redis hot counters.
- [x] Preserve request-count versus attempt-count semantics.
- [x] Define user-visible quota display semantics.
- [x] Add tests for idempotency, late events, correction, display semantics, Redis loss, and bandwidth/request accounting.
- [x] Run focused quota reconciliation tests.
- [x] Run `make check`.
- [x] Write a handoff note.

## Tests

- Focused quota reconciliation tests.
- `make check`

## Acceptance Criteria

- Reconciliation matches the resolved accuracy decision.
- Request-count quota still counts external requests, not fallback attempts.
- Late and duplicate events are handled deterministically.

## Handoff Notes

- Link the resolved decision and document accuracy limits.

## Stop Conditions

- Stop if `P2 Quota Reconciliation Accuracy` is removed or superseded.
- Stop if a deferral would have no owning task file.
