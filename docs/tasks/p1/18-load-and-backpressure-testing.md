# 18 - Load and Backpressure Testing

Status: not started

## Objective

Add load and backpressure tests that validate routing SLOs, active request limits, worker capacity behavior, and NATS
credit flow under pressure.

## Required Planning Docs

- `docs/planning/23-observability.md`
- `docs/planning/29-operational-behavior.md`
- `docs/planning/30-testing-matrix.md`
- `docs/planning/20-rate-limits-and-quotas.md`
- `docs/planning/12-nats-protocol.md`

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
- Results do not overclaim production capacity.

## Handoff Notes

- Record hardware/runtime assumptions for any local load numbers.

## Stop Conditions

- Stop if test resource needs exceed local/CI limits without an opt-in mode.
- Stop if a deferral would have no owning task file.
