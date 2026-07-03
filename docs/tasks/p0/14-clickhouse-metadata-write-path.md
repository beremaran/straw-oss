# 14 - ClickHouse Metadata Write Path

Status: not started

## Objective

Implement asynchronous ClickHouse request metadata writes with redaction, sanitization, bounded queueing, and outage behavior.

## Required Planning Docs

- `docs/planning/22-canonical-clickhouse-schema.md`
- `docs/planning/23-observability.md`
- `docs/planning/27-security-controls.md`
- `docs/planning/30-testing-matrix.md`

## Prerequisites

- Task 06 completed.
- Task 10 completed.
- Task 12 completed.

## Out of Scope

- Do not fail request transport because ClickHouse writes fail.
- Do not implement telemetry read APIs or dashboards.
- Do not implement payload capture.

## Expected Files

- Create or modify: `internal/control`
- Create or modify: ClickHouse writer package if package boundaries require it.
- Test: metadata writer, redaction, and outage tests.

## Steps

- [ ] Read all required planning docs.
- [ ] Define the P0 metadata record from the canonical schema.
- [ ] Sanitize target URLs and drop query by default.
- [ ] Redact auth, cookie, injection secret values, and API key material.
- [ ] Implement async bounded queue writes.
- [ ] Handle ClickHouse outage without failing request transport.
- [ ] Add tests for async write success, outage, bounded queue drop, sanitized target URL, redacted headers, and actor API key audit source.
- [ ] Run focused metadata writer tests.
- [ ] Run `make check`.
- [ ] Write a handoff note.

## Tests

- Focused ClickHouse writer tests.
- `go test ./...`
- `make check`

## Acceptance Criteria

- ClickHouse write failure does not fail request transport.
- Metadata is sanitized and redacted before persistence.
- Queue bounds and drop behavior are tested.
- Tests cover the ClickHouse, redaction, and audit rows in `docs/planning/30-testing-matrix.md`.

## Handoff Notes

- Document queue size and drop policy.
- List fields intentionally omitted from metadata.

## Stop Conditions

- Stop before adding telemetry read APIs.
- Stop before storing payload capture data.
- Stop if a deferral would have no owning task file.
