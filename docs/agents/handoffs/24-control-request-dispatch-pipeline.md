# Handoff

Task: `docs/tasks/p0/24-control-request-dispatch-pipeline.md`

## Changed

- `internal/control/dispatcher.go`:
  - `sendRequestStart` now returns `(uint64, error)` — the next c2e sequence number after all
    request frames have been published.
  - `readResponse` now accepts `c2eSubject`, `in`, and `cancelSeq` and sends a sequenced
    `CancelFrame` to the egress worker on client disconnect (`ctx.Done`), total-deadline expiry,
    and frame-idle timeout. Cancellation is best-effort per docs/planning/09.
  - New `sendCancel` helper publishes the cancel frame and silently drops errors.
- `internal/control/dispatcher_test.go`:
  - `TestDispatcherAssignmentTimeout` — NATS connected, no worker listening; verifies
    `assignment_timeout` after `AssignmentAckTimeout` elapses.
  - `TestDispatcherStreamProtocolError` — stub worker replies ACCEPTED then sends a frame with
    `stream_seq=99` (sequence gap); verifies `protocol_error`.
  - `TestDispatcherCancellation` — slow upstream (blocks 10 s), context cancelled after 150 ms;
    verifies `cancelled` and that `CancelFrame` is sent to the egress worker.
  - Added `time`, `nats.go`, and `strawpb` imports required by new tests.

## Verification

```sh
go test ./internal/control ./internal/egress ./cmd/...
make check
```

Result: all tests pass, 0 linter issues.

## Reviewer Start Points

- `internal/control/dispatcher.go` — `sendRequestStart`, `readResponse`, `sendCancel`
- `internal/control/dispatcher_test.go` — three new tests at the bottom

## Remaining Work

- `docs/tasks/p0/24-control-request-dispatch-pipeline.md`: fallback/replay before the
  `RequestStart` boundary is not implemented. Current dispatcher surfaces the pre-start assignment
  failure directly. P0 conservative replay boundary (docs/planning/09 §5) allows retry on another
  eligible session; implementing multi-attempt dispatch would require significant rework.
  **[Update 2026-07-05: owned by `docs/tasks/p1/17-worker-loss-and-nats-outage-hardening.md`,
  which carries the pre-connect fallback decision and the replay-boundary acceptance criteria.]**
- `docs/tasks/p0/24-control-request-dispatch-pipeline.md`: `CreditFrame` replenishment on the
  c2e channel was not sent in the original P0 slice, and the egress executor originally published
  the full response in one shot.
  **[Update 2026-07-06: closed by `docs/tasks/p1/24-streaming-credit-replenishment.md`; Egress now gates response
  `DataFrame`s on download credit and Control replenishes c2e `CreditFrame`s as decoded/raw response bytes are
  consumed.]**

## Blockers

- None.
