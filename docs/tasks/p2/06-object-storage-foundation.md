# 06 - Object Storage Foundation

Status: done

## Objective

Add the object storage foundation for BodyRef and payload-capture body references.

## Required Planning Docs

- `docs/planning/18-large-body-transport-p2.md`
- `docs/planning/29-operational-behavior.md`
- `docs/planning/24-static-configuration.md`

## Prerequisites

- Task 05 completed.

## Out of Scope

- Do not implement request or response BodyRef flows.
- Do not implement payload capture.
- Do not allow executors to list buckets.

## Expected Files

- Create: object storage client package.
- Create or modify: config loading for object storage.
- Test: object storage foundation tests.

## Steps

- [x] Read all required planning docs.
- [x] Add object storage config and credential loading.
- [x] Implement tenant/request-scoped object key generation with high-entropy nonce.
- [x] Implement scoped signed URL or temporary credential generation with short expiry.
- [x] Require server-side encryption where supported.
- [x] Enforce retention defaults and maximums.
- [x] Define outage behavior for object storage unavailable.
- [x] Add tests for key shape, entropy, tenant scoping, expiry, SSE, retention, and outage behavior.
- [x] Run focused object storage tests.
- [x] Run `make check`.
- [x] Write a handoff note.

## Tests

- Focused object storage tests.
- `make check`

## Acceptance Criteria

- Object keys are unguessable and tenant/request scoped.
- Executors cannot list buckets.
- Retention defaults to one day and is capped at three days unless later policy says otherwise.

## Handoff Notes

- Document provider assumptions, key prefix, retention, and credential scope.

## Stop Conditions

- Stop before adding capture or BodyRef flow logic.
- Stop if object listing permission is required by the design.
- Stop if a deferral would have no owning task file.
