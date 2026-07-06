# Handoff

Task: `docs/tasks/p1/24-streaming-credit-replenishment.md`

## Changed

- `internal/egress/loop.go` splits decoded response `DataFrame`s by available download credit before publishing to e2c, preserving sequence/offset order and waiting for c2e `CreditFrame`s at zero credit.
- `internal/control/dispatcher.go` caps decoded/raw response validators by `max_inflight_download_bytes` and sends c2e `CreditFrame`s as decoded response bytes are consumed.
- `internal/egress/loop_test.go` and `internal/control/dispatcher_test.go` cover worker-side credit blocking/resume, Control replenishment, and max-in-flight capping.
- `docs/agents/handoffs/24-control-request-dispatch-pipeline.md` now records the old credit gap as closed by this task.

## Acceptance Criteria Verdicts

Filled from the independent verifier's report plus the follow-up fixes for its blocking findings.

| Criterion | Verdict | Implementation (file:line) | Proving test |
|-----------|---------|----------------------------|--------------|
| Response body larger than initial download credit completes; Egress sends multiple `DataFrame`s, blocks at zero credit, resumes after Control credit. | VERIFIED | `internal/egress/loop.go:296` splits data by `takeAvailable`; `internal/egress/loop.go:705` waits for credit; `internal/control/dispatcher.go:993` replenishes consumed bytes. | `TestWorkerDownloadCreditGatesResponseData`; `TestDispatcherReplenishesDownloadCreditForDecodedResponse` |
| Control never lets un-replenished buffered bytes exceed `max_inflight_download_bytes`. | VERIFIED | `internal/control/dispatcher.go:927` caps initial download credit to the max in-flight limit; `internal/control/dispatcher.go:996` grants back only consumed bytes. | `TestDispatcherReplenishesDownloadCreditForDecodedResponse` |
| Stale single-response-frame implementation comment is gone and handoff 24 names this task as owner/closure. | VERIFIED | `internal/egress/loop.go` has no stale single-frame comment; `docs/agents/handoffs/24-control-request-dispatch-pipeline.md:45` records closure. | `rg "one pass" internal/egress/loop.go` returned no matches. |
| `make check` passes. | VERIFIED | Full repository check completed. | `make check` |

## Planning-Doc Coverage

| Planning item | Status | Evidence / owning task |
|---------------|--------|------------------------|
| Initial upload/download credit and max in-flight limits are carried in `AssignRequest`. | already existed | `internal/control/dispatcher.go:1064` |
| `CreditFrame` grants additional byte credit for `DataFrame` payload bytes only. | implemented | `internal/control/dispatcher.go:913`; `internal/egress/loop.go:624` |
| Control/terminal frames do not consume byte credit. | implemented | `internal/egress/loop.go:298` applies credit only to data frames; `internal/natsx/stream.go:173` consumes only `DataFrame` bytes. |
| Receiver replenishes credit after it has processed/released buffered bytes. | implemented | `internal/control/dispatcher.go:993`; `internal/control/dispatcher.go:891` |
| When credit reaches zero, senders stop reading/sending where possible. | implemented | `internal/egress/loop.go:705` waits for grants before publishing more response data. |
| Response lifecycle sends `ResponseStart`, response `DataFrame`s, optional trailers, then terminal `EndFrame`/`ErrorFrame`. | already existed | `internal/egress/executor.go:217`; `internal/egress/loop.go:321` preserves sequencing. |

## Verification

```sh
go test ./internal/control ./internal/egress ./internal/natsx
make check
```

Result: passed.

- Postgres-backed tests: not exercised (diff does not touch Postgres surfaces).
- Live compose verification: skipped because this task is covered by focused real-NATS worker/control tests and does not change docker-compose wiring.

## Reviewer Start Points

- `internal/egress/loop.go`
- `internal/control/dispatcher.go`
- `internal/egress/loop_test.go`
- `internal/control/dispatcher_test.go`

## Remaining Work

- None.

## Blockers

- None.
