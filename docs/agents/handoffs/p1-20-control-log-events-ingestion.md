# Handoff

Task: `docs/tasks/p1/20-log-events-ingestion.md`

## Changed

- Split task 20 to Control-only `log_events` ingestion and created
  `docs/tasks/p1/27-egress-log-events-nats-transport.md` to own Egress log transport over NATS to Control-backed
  ClickHouse.
- Added `logging.LogEvent` plus a `slog.Handler` tee that maps Control log records into the canonical `log_events`
  row shape and redacts sensitive-key attributes before enqueue.
- Added `LogEventWriter` on the shared async ClickHouse queue, `HTTPClickHouseSink.WriteLogEvents`, and Control wiring
  that installs the tee only when ClickHouse is configured.
- Updated ClickHouse metric help text from request-metadata-only to generic telemetry because queue depth and write
  errors now include log events too.

## Acceptance Criteria Verdicts

Filled from the independent verifier's report (straw-task-runner workflow step 12), not from the implementer's
self-assessment.

| Criterion | Verdict | Implementation (file:line) | Proving test |
|-----------|---------|----------------------------|--------------|
| Control structured logs appear as `log_events` rows with the `docs/planning/22` shape when ClickHouse is configured. | VERIFIED | `internal/logging/logging.go:20`, `internal/logging/logging.go:98`, `cmd/control/main.go:527`, `cmd/control/main.go:536`, `internal/control/request_metadata.go:403` | `TestTeeHandlerMapsAndRedactsLogEvents`, `TestHTTPClickHouseSinkLogEventsRequestFormat`, `go test ./cmd/control -run Test -count=1` |
| No Egress binary path receives ClickHouse config or writes directly to ClickHouse; Egress transport is owned by task 27. | VERIFIED | `docs/planning/04-canonical-architecture.md:19`, `internal/config/config.go:142`, `cmd/egress/main.go:69`, `docs/tasks/p1/27-egress-log-events-nats-transport.md:7` | `rg -n "ClickHouse\|clickhouse" cmd/egress internal/egress -S` returned no matches; no Egress/proto/NATS files changed |
| A ClickHouse outage never blocks or fails the logging caller; overflow drops are bounded and observable. | VERIFIED | `internal/logging/logging.go:111`, `internal/control/telemetry_events.go:177`, `internal/control/telemetry_events.go:200`, `cmd/control/main.go:488` | `TestLogEventWriterOutageKeepsQueuedEvents`, `TestLogEventWriterDropsOldestWhenFull`, `TestRequestMetadataWriterRecordsClickHouseMetrics` |
| Redaction invariants hold for logged values. | VERIFIED | `internal/logging/logging.go:136`, `internal/logging/logging.go:168`, `docs/planning/21-state-and-storage.md:91`, `docs/planning/27-security-controls.md:121` | `TestTeeHandlerMapsAndRedactsLogEvents` |

## Planning-Doc Coverage

Every in-phase field/endpoint/behavior from the task's cited planning-doc sections, accounted for:

| Planning item | Status | Evidence / owning task |
|---------------|--------|------------------------|
| Control owns observability aggregation; executors must not query ClickHouse. | implemented | Task 20 changes `cmd/control/main.go`; Egress direct log transport is owned by `docs/tasks/p1/27-egress-log-events-nats-transport.md`. |
| `log_events` row fields: timestamp, service, level, message, request_id, tenant_id, trace_id, worker_id, error_code, extra. | implemented | `internal/logging/logging.go:20`, `internal/logging/logging.go:98` |
| ClickHouse writes are async, bounded, and request transport does not block on ClickHouse. | implemented | `internal/control/telemetry_events.go:55`, `internal/control/telemetry_events.go:177`, `internal/control/telemetry_events.go:200` |
| Structured log fields from Section 23. | implemented | `internal/logging/logging.go:98`, `internal/logging/logging_test.go:50` |
| Authorization/cookie/credential/private-key/API-key-like attributes are redacted before ClickHouse. | implemented | `internal/logging/logging.go:136`, `internal/logging/logging.go:168`, `internal/logging/logging_test.go:68` |
| Log search/read APIs. | out of scope | Existing P1 telemetry read tasks own read surfaces; task 20 does not add routes. |
| Egress log delivery to ClickHouse. | out of scope | Owned by `docs/tasks/p1/27-egress-log-events-nats-transport.md`. |

## Verification

```sh
go test ./internal/logging ./internal/control ./cmd/control
go test ./internal/... ./cmd/... -count=1
make check
```

Result: all passed. The first broad test attempt hit a timing-sensitive existing
`TestDispatcherRateLimitRetryAfter` failure; the isolated test and fresh package/full reruns passed, and this diff does
not touch dispatcher or rate-limit code.

- Postgres-backed tests: not exercised (diff does not touch Postgres surfaces or migrations).
- Live compose verification: skipped; diff does not touch request transport, compose config, or live ClickHouse schema.

## Reviewer Start Points

- `internal/logging/logging.go`
- `internal/control/telemetry_events.go`
- `internal/control/request_metadata.go`
- `cmd/control/main.go`
- `docs/tasks/p1/27-egress-log-events-nats-transport.md`

## Remaining Work

- Egress log transport over NATS to Control-backed ClickHouse was completed by
  `docs/tasks/p1/27-egress-log-events-nats-transport.md`.

## Blockers

- None.
