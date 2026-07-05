# 41 - Request Phase Timing Accuracy

Status: done

## Objective

Make the per-phase timings in `request_events` (`routing_ms`, `assignment_ms`, `egress_ms`, `total_ms`) reflect the
real elapsed phases of a successful dispatch, so ClickHouse telemetry is usable for the latency analysis
`docs/planning/23` expects.

## Context (gap being closed)

Observed on the live compose stack on 2026-07-05: two successful REST -> NATS -> egress -> `https://example.com`
round-trips recorded

| request | routing_ms | assignment_ms | egress_ms | total_ms |
|---------|-----------|---------------|-----------|----------|
| req_1783218379439088304 | 0 | 3 | 0 | 257 |
| req_1783218544603559033 | 0 | 1 | 0 | 179 |

`egress_ms = 0` on a ~200ms upstream fetch is wrong: nearly all of `total_ms` is the egress phase.
`routing_ms = 0` may be legitimately sub-millisecond — verify rather than assume.

Code pointers from the initial look (unverified beyond this): the egress worker does emit `OutboundStartFrame`
(`internal/egress/executor.go:742`), Control stamps `egressStarted` on that frame and computes
`result.egressMs` on the `End` frame (`internal/control/dispatcher.go`, `acceptResponseProgress` /
`acceptResponseTerminal`), and `egressMillis` returns 0 when the start is zero. Something in that chain loses the
start time or the subtraction on the live path — the in-process tests did not catch it, so the first step is a
failing test that reproduces the zero.

## Required Planning Docs

- `docs/planning/23-observability.md` (timing fields and their intent)
- `docs/planning/09-canonical-request-lifecycle.md` (phase boundaries)
- `docs/planning/22-canonical-clickhouse-schema.md` (`request_events` columns)

## Prerequisites

- Task 32 completed (request-outcome telemetry this task corrects).

## Out of Scope

- No new timing fields or schema changes.
- No latency dashboards or read APIs (P1 tasks 12/13).

## Expected Files

- Modify: `internal/control/dispatcher.go` (wherever the root cause lands).
- Test: `internal/control/dispatcher_test.go` — extend the round-trip test(s) to assert `egress_ms > 0` and
  phase sums are consistent with `total_ms` for a successful dispatch with a measurable upstream delay.

## Steps

- [x] Read the required planning docs.
- [x] Write the failing test first: a dispatcher round-trip against a test upstream with a deliberate delay
      (e.g. 50ms) must record `egress_ms` in that ballpark, not 0. Use the injected clock if the harness has one.
      (`TestDispatcherEgressPhaseTiming`, 100ms upstream delay; reproduced `EgressMs = 0` before the fix.)
- [x] Trace the live path to the root cause (start-time never set, overwritten, or dropped between frame handling
      and `applyRequestOutcome`) and fix it at the source — not by re-deriving `egress_ms` from `total_ms`.
      (Root cause was in egress, not Control: `Executor.Execute` batched OutboundStart with all other frames and
      the worker published them only after the upstream call finished.)
- [x] Check `routing_ms` while in there: confirm sub-ms is the true explanation or fix it under the same test.
      (Legitimate: `routing_ms` wraps only the in-memory `d.route` evaluation over a cached snapshot.)
- [x] Verify live: re-run the compose round-trip from `deploy/docker/README.md` and confirm the ClickHouse row
      carries non-zero, plausible `egress_ms`. (egress_ms=124/total_ms=140 and egress_ms=95/total_ms=107.)
- [x] Run `make check`.
- [x] Write a handoff note.

## Tests

- `go test ./internal/control/ -run Dispatcher`
- Live compose round-trip + ClickHouse row check (documented in the handoff).
- `make check`

## Acceptance Criteria

- A successful dispatch with a delayed upstream records `egress_ms` reflecting that delay (asserted by test).
- Live compose `request_events` rows show phase timings that plausibly sum toward `total_ms`.
- No schema or field changes.

## Handoff Notes

- Name the root cause and why the in-process tests missed it.

## Stop Conditions

- Stop if the fix requires protocol (protobuf frame) changes.
- Stop if a deferral would have no owning task file.
