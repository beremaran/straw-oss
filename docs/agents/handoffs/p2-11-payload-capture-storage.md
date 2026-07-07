# Handoff

Task: `docs/tasks/p2/11-payload-capture-storage.md`

## Changed

- `deploy/docker/clickhouse-schema.sql` now creates canonical `payload_capture_events` with body-reference columns and a 7-day TTL.
- `internal/control/payload_capture_storage.go` adds the ClickHouse writer and object-reference capture store.
- `cmd/control/main.go` wires the payload capture ClickHouse writer and REST capture store when ClickHouse and object storage are configured.
- `internal/control/handler.go` tees successful REST captures into storage after dispatch without changing the client response.
- `internal/control/body_ref_store.go` / `internal/objectstore/objectstore.go` keep DELETE cleanup on typed presigned URLs.
- Tests cover ClickHouse row format, schema retention refs, body refs, cleanup, redaction, outage behavior, and the REST handler storage path.

## Acceptance Criteria Verdicts

Filled from the independent verifier's report.

| Criterion | Verdict | Implementation (file:line) | Proving test |
|-----------|---------|----------------------------|--------------|
| Capture metadata lands in ClickHouse using the canonical schema. | VERIFIED | `internal/control/payload_capture_storage.go:20`, `internal/control/payload_capture_storage.go:41`, `cmd/control/main.go:939`, `cmd/control/main.go:1137`, `deploy/docker/clickhouse-schema.sql:102` | `internal/control/payload_capture_storage_test.go:268`, `internal/control/payload_capture_storage_test.go:324` |
| Large bodies are stored by reference, not inline without bounds. | VERIFIED | `internal/control/payload_capture_storage.go:116`, `internal/control/payload_capture_storage.go:193`, `internal/control/payload_capture_storage.go:213`, `internal/control/payload_capture_storage.go:227`, `deploy/docker/clickhouse-schema.sql:111` | `internal/control/payload_capture_storage_test.go:91`, `internal/control/handler_test.go:780` |
| Storage respects tenant isolation and redaction rules. | VERIFIED | `internal/control/payload_capture_storage.go:172`, `internal/control/body_ref_store.go:77`, `internal/control/payload_capture.go:248`, `internal/control/payload_capture.go:269`, `internal/control/handler.go:185` | `internal/control/payload_capture_storage_test.go:120`, `internal/control/payload_capture_storage_test.go:227`, `internal/control/handler_test.go:817` |

## Planning-Doc Coverage

| Planning item | Status | Evidence / owning task |
|---------------|--------|------------------------|
| Payload capture is off by default and tenant-admin policy gated. | already existed | `internal/control/payload_capture.go:43`, `internal/control/request.go:111` |
| Capture must tee and not mutate forwarded request/response bytes. | implemented | `internal/control/handler.go:165`, `internal/control/handler.go:185` |
| Capture decisions `none`, `metadata_only`, `headers`, `body_truncated`, `body_full`. | already existed | `internal/control/payload_capture.go:18` |
| Full body capture remains bounded by capture limits. | already existed | `internal/control/payload_capture.go:184`, `internal/control/payload_capture.go:194` |
| Baseline P2 supports header redaction and raw-body truncation. | already existed | `internal/control/payload_capture.go:176`, `internal/control/payload_capture.go:188`, `internal/control/payload_capture.go:248` |
| Metadata lands in ClickHouse; captured bodies use object storage refs when beyond safe inline storage. | implemented | `internal/control/payload_capture_storage.go:41`, `internal/control/payload_capture_storage.go:208`, `internal/control/payload_capture_storage.go:222` |
| ClickHouse uses database `straw` and table namespace. | implemented | `deploy/docker/clickhouse-schema.sql:102`, `internal/control/request_metadata.go:371` |
| Control writes ClickHouse asynchronously through bounded queues and does not block transport on ClickHouse. | implemented | `internal/control/payload_capture_storage.go:51`, `internal/control/telemetry_events.go:169` |
| `payload_capture_events.captured_at` DateTime64. | implemented | `deploy/docker/clickhouse-schema.sql:104`, `internal/control/payload_capture_storage.go:21` |
| `payload_capture_events.request_id` String. | implemented | `deploy/docker/clickhouse-schema.sql:105`, `internal/control/payload_capture_storage.go:22` |
| `payload_capture_events.tenant_id` LowCardinality(String). | implemented | `deploy/docker/clickhouse-schema.sql:106`, `internal/control/payload_capture_storage.go:23` |
| `payload_capture_events.capture_scope` LowCardinality(String). | implemented | `deploy/docker/clickhouse-schema.sql:107`, `internal/control/payload_capture_storage.go:24` |
| `payload_capture_events.capture_decision` LowCardinality(String). | implemented | `deploy/docker/clickhouse-schema.sql:108`, `internal/control/payload_capture_storage.go:25` |
| `payload_capture_events.request_headers` String. | implemented | `deploy/docker/clickhouse-schema.sql:109`, `internal/control/payload_capture_storage.go:26` |
| `payload_capture_events.response_headers` String. | implemented | `deploy/docker/clickhouse-schema.sql:110`, `internal/control/payload_capture_storage.go:27` |
| `payload_capture_events.request_body_ref` String. | implemented | `deploy/docker/clickhouse-schema.sql:111`, `internal/control/payload_capture_storage.go:28` |
| `payload_capture_events.response_body_ref` String. | implemented | `deploy/docker/clickhouse-schema.sql:112`, `internal/control/payload_capture_storage.go:29` |
| `payload_capture_events.redacted_fields` Array(String). | implemented | `deploy/docker/clickhouse-schema.sql:113`, `internal/control/payload_capture_storage.go:30` |
| `payload_capture_events.truncated` UInt8. | implemented | `deploy/docker/clickhouse-schema.sql:114`, `internal/control/payload_capture_storage.go:31` |
| Table partition/order/TTL: partition by month, order by tenant/time/request, TTL 7 days. | implemented | `deploy/docker/clickhouse-schema.sql:116`, `deploy/docker/clickhouse-schema.sql:117`, `deploy/docker/clickhouse-schema.sql:118` |
| ClickHouse is append-heavy operational data, not config source of truth. | implemented | Capture storage only writes operational rows in `internal/control/payload_capture_storage.go:41`; config policy remains Postgres-backed in `internal/control/handler.go:58`. |
| Metadata redaction boundary: sensitive headers are not stored raw. | implemented | `internal/control/payload_capture.go:248`, `internal/control/payload_capture.go:269`, `internal/control/payload_capture_storage_test.go:227` |
| Tenant-facing telemetry exposure rules. | out of scope | No tenant-facing payload read API was added, matching task out-of-scope. |

## Verification

```sh
go test ./internal/control -run 'TestHandlerPayloadCaptureStoresSuccessfulRESTResponse|TestPayloadCapture(Store|Schema|HTTPClickHouseSinkPayloadCapture)'
make check
```

Result:

- Postgres-backed tests: not exercised; diff does not touch Postgres migrations or `postgres_*` files.
- Live compose verification: skipped because the local compose config has no object-storage service/config, so Control correctly would not wire `PayloadCaptureStore`; a live request would not verify payload capture storage.
- Retention/object-reference cleanup: table TTL is pinned by `TestPayloadCaptureSchemaRetentionAndRefs`; explicit partial-upload cleanup is covered by `TestPayloadCaptureStoreResponseFailureCleansUpRequest`. Bucket lifecycle cleanup remains owned by `docs/tasks/p2/21-object-storage-lifecycle-retention.md`.

## Reviewer Start Points

- `internal/control/handler.go`
- `internal/control/payload_capture_storage.go`
- `deploy/docker/clickhouse-schema.sql`

## Remaining Work

- None.

## Blockers

- None.
