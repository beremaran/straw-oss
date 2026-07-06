# Handoff

Task: `docs/tasks/p1/27-egress-log-events-nats-transport.md`

## Changed

- Added protobuf `LogEvent` and `Envelope.log_event`, regenerated Go bindings.
- Added `straw.v1.control.logs` subject helper and documented payload/ACLs.
- Added `logging.NATSLogPublisher`: bounded queue, non-blocking enqueue, drop-oldest on overflow, publish-error drop counting.
- Wired Egress to tee structured logs to NATS after connecting; no ClickHouse config or writer was added to Egress.
- Wired Control to subscribe to log telemetry and enqueue rows through the existing `LogEventWriter`.
- Strengthened shared logging redaction for sensitive values: NATS subjects, inboxes, private keys, signed URL markers, and URLs with userinfo.

## Acceptance Criteria Verdicts

Filled from the independent verifier's report (straw-task-runner workflow step 12), not from the implementer's self-assessment.

| Criterion | Verdict | Implementation (file:line) | Proving test |
|-----------|---------|----------------------------|--------------|
| Egress `slog` record publishes over protobuf NATS path, Control receives it, and enqueues `log_events` row | VERIFIED | `cmd/egress/main.go:116`, `internal/logging/nats_publisher.go:106`, `internal/control/log_nats.go:27`, `internal/control/log_nats.go:55`, `api/proto/straw/v1/straw.proto:124` | `TestNATSLogPublisherPublishesRedactedProtobuf`, `TestHandleLogEventEnqueuesClickHouseRow`, `TestLogEventSubscriptionReceivesNATSTelemetry` |
| Egress has no ClickHouse config surface and no direct ClickHouse writer | VERIFIED | `internal/config/config.go:144`, `cmd/egress/main.go:93`; grep found no ClickHouse/writer use under `cmd/egress` or `internal/egress` | `go test ./api/... ./internal/natsx ./internal/logging ./internal/control ./internal/egress ./cmd/...`; grep evidence |
| NATS outage, missing Control subscribers, or full Egress log queue never blocks/fails logging caller; bounded observable drops | VERIFIED | `internal/logging/nats_publisher.go:48`, `internal/logging/nats_publisher.go:63`, `internal/logging/nats_publisher.go:71`, `internal/logging/nats_publisher.go:119` | `TestNATSLogPublisherOverflowDoesNotBlock`, `TestNATSLogPublisherPublishErrorIsObservableDrop`, `TestNATSLogPublisherNoSubscriberDoesNotBlockOrDrop` |
| Redaction invariants hold before Egress records can reach ClickHouse | VERIFIED | `internal/logging/logging.go:137`, `internal/logging/logging.go:174`, `internal/logging/logging.go:186`, `internal/logging/nats_publisher.go:106` | `TestTeeHandlerMapsAndRedactsLogEvents`, `TestNATSLogPublisherPublishesRedactedProtobuf` |
| `docs/planning/12-nats-protocol.md` documents subject, payload, and ACLs | VERIFIED | `docs/planning/12-nats-protocol.md:43`, `docs/planning/12-nats-protocol.md:63`, `docs/planning/12-nats-protocol.md:194` | `TestSubjects`; doc evidence |

## Planning-Doc Coverage

Every in-phase field/endpoint/behavior from the task's cited planning-doc sections, accounted for:

| Planning item | Status | Evidence / owning task |
|---------------|--------|------------------------|
| Control owns observability aggregation; executors do not query ClickHouse | implemented | `cmd/egress/main.go:116` publishes only over NATS; `internal/control/log_nats.go:55` enqueues through Control writer |
| NATS uses binary protobuf `Envelope`, not JSON | implemented | `api/proto/straw/v1/straw.proto:124`, `internal/logging/nats_publisher.go:106` |
| Canonical Egress-to-Control log subject | implemented | `internal/natsx/natsx.go:43`, `docs/planning/12-nats-protocol.md:63` |
| Subject ACLs include worker publish and Control subscribe | implemented | `docs/planning/12-nats-protocol.md:194` |
| `log_events` row shape: timestamp, service, level, message, request_id, tenant_id, trace_id, worker_id, error_code, extra | implemented | `api/proto/straw/v1/straw.proto:128`, `internal/control/log_nats.go:64` |
| Bounded async writes with drop behavior | implemented | `internal/logging/nats_publisher.go:48`, `internal/logging/nats_publisher.go:63`, existing Control writer in `internal/control/telemetry_events.go` |
| Egress logs carry service/timestamp/level and optional request/tenant/trace/error/worker fields | implemented | `internal/logging/logging.go:70`, `internal/logging/nats_publisher.go:132` |
| Redact secret material, NATS subjects, credentials, private keys, signed URLs, upstream proxy credentials | implemented | `internal/logging/logging.go:137`, `internal/logging/logging.go:186` |
| ClickHouse remains Control-owned operational analytics, not source of truth | already existed | `internal/control/request_metadata.go:403`; no Egress ClickHouse grep hits |

## Verification

```sh
go test ./api/... ./internal/natsx ./internal/logging ./internal/control ./internal/egress ./cmd/...
make check
```

Result:

- Focused tests: passed.
- `make check`: passed.
- Postgres-backed tests: not exercised (diff does not touch Postgres surfaces or migrations).
- Live compose verification: skipped because this task's runtime path is covered by fake-NATS unit tests and does not change request execution; no compose-only behavior was required.

## Reviewer Start Points

- `internal/logging/nats_publisher.go`
- `internal/control/log_nats.go`
- `cmd/egress/main.go`
- `cmd/control/main.go`
- `api/proto/straw/v1/straw.proto`
- `docs/planning/12-nats-protocol.md`

## Remaining Work

- None.

## Blockers

- None.
