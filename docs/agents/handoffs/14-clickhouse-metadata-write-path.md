# Handoff

Task: `docs/tasks/p0/14-clickhouse-metadata-write-path.md`

## Changed

- `internal/control/request_metadata.go`: added the canonical `request_events` record, target URL sanitization, sensitive-header redaction helper, bounded async queue writer, and a ClickHouse HTTP sink.
- `internal/control/handler.go`: added optional request-metadata recording on the accepted request path without blocking transport.
- `internal/control/request_metadata_test.go`: added tests for sanitization, header redaction, request-event construction, async write success, outage retention, bounded queue drop, and handler wiring.
- `docs/tasks/p0/14-clickhouse-metadata-write-path.md`: marked complete.
- `docs/tasks/p0.md`: marked task 14 complete.

## Verification

```sh
go test ./internal/control -run 'Test(SanitizeTargetURLDropsQuery|RedactSensitiveHeaderValue|BuildRequestEventRecordsActorAndSanitizedTarget|RequestMetadataWriterFlushSuccess|RequestMetadataWriterOutageKeepsQueuedEvents|RequestMetadataWriterDropsOldestWhenFull|RequestHandlerQueuesSanitizedMetadata)$'
make check
```

Result:

- Passed.

## Reviewer Start Points

- `/Users/beremaran/projects/straw/internal/control/request_metadata.go`
- `/Users/beremaran/projects/straw/internal/control/request_metadata_test.go`
- `/Users/beremaran/projects/straw/internal/control/handler.go`

## Remaining Work

- None for task 14.

## Blockers

- None.
