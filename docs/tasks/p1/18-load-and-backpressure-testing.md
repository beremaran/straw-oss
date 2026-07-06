# 18 - Load and Backpressure Testing

Status: not started

## Objective

Add load and backpressure tests that validate routing SLOs, active request limits, worker capacity behavior, and NATS
credit flow under pressure, including asserting that request-metadata rows actually land in a live ClickHouse table
under load (the flag `docs/agents/handoffs/25-p0-test-matrix-and-compose.md` left unowned — this task is the owner).

## Required Planning Docs

- `docs/planning/23-observability.md`
- `docs/planning/29-operational-behavior.md`
- `docs/planning/30-testing-matrix.md`
- `docs/planning/20-rate-limits-and-quotas.md`
- `docs/planning/12-nats-protocol.md`
- `docs/planning/22-canonical-clickhouse-schema.md` (request_metadata table asserted under load)

## Prerequisites

- Task 17 completed.
- P0 task 25 completed.

## Out of Scope

- Do not claim production capacity benchmarks from laptop-only runs.
- Do not add billing-grade quota reconciliation.

## Expected Files

- Create or modify: load test harness and docs.
- Test: load/backpressure test targets.

## Steps

- [ ] Read all required planning docs.
- [ ] Define the local load harness inputs, limits, and reproducible run command.
- [ ] Validate Control routing/coordination p50 and p99 SLOs excluding outbound latency.
- [ ] Validate active request limits and worker capacity behavior.
- [ ] Validate upload/download credit backpressure and memory guardrails.
- [ ] Validate Redis failure policies for rate limits and quotas under load.
- [ ] Assert request-metadata rows land in a live ClickHouse table during a load run (row count matches completed
      requests within the async-writer flush window), in the opt-in local mode.
- [ ] Add CI-safe smoke mode and optional heavier local mode.
- [ ] Run focused load smoke tests.
- [ ] Run `make check`.
- [ ] Write a handoff note.

## Tests

- Load/backpressure smoke tests.
- `make check`

## Acceptance Criteria

- A reproducible load harness exists with CI-safe and local modes.
- SLOs and memory/backpressure guardrails are checked.
- The local mode proves rows land in live ClickHouse under load, closing the handoff-25 flag (whose note names this
  task).
- Results do not overclaim production capacity.

## Handoff Notes

- Record hardware/runtime assumptions for any local load numbers.

## Stop Conditions

- Stop if test resource needs exceed local/CI limits without an opt-in mode.
- Stop if a deferral would have no owning task file.
