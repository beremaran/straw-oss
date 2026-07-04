# Handoff

Task: `docs/tasks/p0/32-request-outcome-and-worker-audit-telemetry.md`

## Changed

- `internal/control/handler.go`, `internal/control/dispatcher.go`, `internal/control/request_metadata.go`:
  `request_events` is now finalized after dispatch completes instead of enqueued pre-dispatch with hardcoded
  success fields. `RequestHandler.dispatchValidated` builds the base event (target/method/size) before calling
  the dispatcher, then calls the new `applyRequestOutcome` to fill `upstream_status`/`client_status`,
  `error_code`/`error_category`/`timeout_type`, `response_size_bytes`, and the four phase-timing columns from
  the real `SuccessResponse` or `*PipelineError`, and enqueues exactly once. `PipelineError` gained
  `RoutingMs`/`AssignmentMs`/`EgressMs`/`TotalMs` fields; `DefaultRequestDispatcher.dispatch` annotates every
  early-return error with whatever partial timing it measured before failing (`withTiming` helper), so a failed
  row still reports real elapsed time instead of zeros. `SuccessResponse` gained an unexported-from-wire
  `ResponseSizeBytes uint64 \`json:"-"\`` field (raw pre-base64 byte count) so metadata construction doesn't need
  to re-decode the response body.
- `internal/control/telemetry_events.go` (new): extracted the bounded/async/outage-tolerant queue behavior
  `RequestMetadataWriter` already had into a generic `asyncEventQueue[T]`, then built `WorkerEvent`/
  `WorkerEventWriter`/`WorkerEventSink`/`WorkerEventRecorder` and `ConfigAuditEvent`/`ConfigAuditEventWriter`/
  `ConfigAuditEventSink`/`ConfigAuditRecorder` on top of it, matching `request_events`' drop-oldest/batch/flush/
  outage semantics without duplicating that logic three times. `RequestMetadataWriter` itself now wraps
  `asyncEventQueue[RequestEvent]` (public API unchanged).
- `internal/control/request_metadata.go`: `HTTPClickHouseSink` gained `WriteWorkerEvents` and
  `WriteConfigAuditEvents`, both routed through a new generic `insertClickHouseRows[T]` helper alongside the
  existing `WriteRequestEvents`, so all three tables share one HTTP-insert code path.
- `internal/control/worker_registry.go`: `WorkerRegistry` gained an optional `WorkerEventRecorder` (wired via
  `SetEventRecorder`, nil-safe when unset). `Register`, `Heartbeat`, `SetGlobalAdmin`, `SetGlobalDrain`,
  `SetTenantAdmin`, and `SetTenantDrain` each emit one `worker_events` row (`register`/`heartbeat`/
  `disable`/`enable`/`drain`/`undrain`/`tenant_disable`/`tenant_enable`/`tenant_drain`/`tenant_undrain`) carrying
  session/health/capacity fields when a current session exists. Extracted `newRuntimeSession` out of `Register`
  to keep it under the 60-line lint limit after adding the emit call.
- `internal/control/audit.go`: added `auditStoreWithEvents`, an `AuditStore` decorator
  (`NewAuditStoreWithEvents`) that mirrors every successful `Record` into a `ConfigAuditRecorder`. This is the
  single choke point `recordAudit` already uses at every config/admin mutation site (tenant, API key, worker
  credential, routing/deny/injection/pool config, worker admin, request cancel), so wiring it there covers every
  transition without touching ~15 individual handler call sites.
- `cmd/control/main.go`: replaced the single `wireClickHouse`/`*RequestMetadataWriter` plumbing with
  `wireClickHouseWriters` returning a `*clickHouseWriters` (request/worker/config-audit, one shared HTTP sink,
  shared queue/batch/flush tuning). `workerRegistry.SetEventRecorder(...)` wires worker events;
  `buildAdminHandlers` now wraps the Postgres audit store with `NewAuditStoreWithEvents`. `SetMetrics`/
  `QueueDepth`/`Close` on `clickHouseWriters` fan out to all three writers so the existing
  `straw_clickhouse_write_queue_depth` gauge stays a single series (summed) and shutdown drains all three
  queues.
- Tests: `internal/control/request_metadata_test.go` (outcome success/failure fields, `applyRequestOutcome` unit
  tests), `internal/control/dispatcher_test.go` (`PipelineError` carries partial timing on failure),
  `internal/control/worker_events_test.go` (new: register/heartbeat/disable/drain emit the right rows, outage
  doesn't block registration), `internal/control/audit_test.go` (new: `auditStoreWithEvents` mirrors `Record`,
  nil recorder returns the store unwrapped, outage doesn't fail the audit write).

## Verification

```sh
go test ./internal/control ./cmd/... -count=1
make check
```

Result: all packages pass; `golangci-lint run --max-issues-per-linter 0 --max-same-issues 0` reports 0 issues.

## Reviewer Start Points

- `internal/control/handler.go` (`dispatchValidated`, `recordOutcome`)
- `internal/control/request_metadata.go` (`applyRequestOutcome`, `uint16OrMax`/`uint32OrMax`)
- `internal/control/dispatcher.go` (`withTiming`, the annotated early returns in `dispatch`)
- `internal/control/telemetry_events.go` (`asyncEventQueue`, `WorkerEventWriter`, `ConfigAuditEventWriter`)
- `internal/control/worker_registry.go` (`emitWorkerEvent`, `emitTransitionEvent`, the six call sites)
- `internal/control/audit.go` (`auditStoreWithEvents`)
- `cmd/control/main.go` (`wireClickHouseWriters`, `clickHouseWriters`)

## Remaining Work

- `config_audit_events` rows mirrored from `recordAudit`/`AuditStore.Record` carry `tenant_id`/`actor_type`/
  `actor_id`/`config_type`/`resource_id`/`action` but leave `field_path`, `old_value_json`, `new_value_json`, and
  `config_version` empty. Those richer fields exist only in the separate Postgres `config_audit_source` writer in
  `postgres_config_store.go` (`insertConfigAudit`/`writeTenantConfig`, used by routing/deny/injection/pool
  writes), which was not threaded through to ClickHouse in this task — doing so would mean converting ~7
  `writeTenantConfig` call sites into methods on `*PostgresConfigStore` carrying an event recorder, which felt
  like scope creep beyond "receive rows on the corresponding transitions/writes." No existing task file owns
  this specific enrichment; if the old/new-value fidelity in `config_audit_events` matters for P1 telemetry
  consumers, it needs a new task.
- `log_events` ingestion remains unbuilt, per the existing deferral to `docs/tasks/p1/20-log-events-ingestion.md`
  (unchanged by this task; task 32's Out of Scope section already named it).
- Telemetry read APIs/dashboards and payload capture remain out of scope (P1/P2), per the task's Stop Conditions.

## Blockers

- None.
