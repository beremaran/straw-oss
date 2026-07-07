# 11 - Payload Capture Storage

Status: done

## Objective

Persist payload capture metadata in ClickHouse and store large captured bodies by object reference.

## Required Planning Docs

- `docs/planning/19-payload-capture-p2.md`
- `docs/planning/22-canonical-clickhouse-schema.md`
- `docs/planning/21-state-and-storage.md`

## Prerequisites

- Task 06 completed.
- Task 10 completed.
- P0 task 14 completed.

## Out of Scope

- Do not change capture policy.
- Do not store unlimited bodies in ClickHouse.
- Do not create tenant-facing read APIs unless already owned by telemetry tasks.

## Expected Files

- Create or modify: ClickHouse payload capture writer.
- Create or modify: object-storage body reference writer.
- Test: payload capture storage tests.

## Steps

- [x] Read all required planning docs.
- [x] Implement writes to the canonical `payload_capture_events` table.
- [x] Enforce table TTL and retention behavior.
- [x] Store large captured bodies by object reference.
- [x] Link body refs to tenant/request/capture event metadata.
- [x] Ensure secret and sensitive fields follow storage redaction rules.
- [x] Add tests for ClickHouse rows, TTL/retention metadata, large body refs, object cleanup, redaction, and outage
      behavior.
- [x] Run focused capture storage tests.
- [x] Run `make check`.
- [x] Write a handoff note.

## Tests

- Focused payload capture storage tests.
- `make check`

## Acceptance Criteria

- Capture metadata lands in ClickHouse using the canonical schema.
- Large bodies are stored by reference, not inline without bounds.
- Storage respects tenant isolation and redaction rules.

## Handoff Notes

- Document retention and object-reference cleanup behavior.

## Stop Conditions

- Stop if the ClickHouse schema does not define the required table.
- Stop if a deferral would have no owning task file.
