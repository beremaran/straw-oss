# 07 - BodyRef Request Body Flow

Status: done

## Objective

Implement the S3 BodyRef request-body upload flow from Control to Egress.

## Required Planning Docs

- `docs/planning/18-large-body-transport-p2.md`
- `docs/planning/13-protobuf-contract.md`
- `docs/planning/12-nats-protocol.md`

## Prerequisites

- Task 06 completed.

## Out of Scope

- Do not implement response-body BodyRef.
- Do not implement payload capture.
- Do not implement DirectStreamRef unless task 05 selected and specified it.

## Expected Files

- Create or modify: Control request-body BodyRef flow.
- Create or modify: Egress BodyRef download handling.
- Test: request-body BodyRef tests.

## Steps

- [x] Read all required planning docs.
- [x] Upload large request bodies to object storage after authentication, validation, routing, and assignment where
      possible.
- [x] Abort multipart uploads on cancellation or failure. (Bodies are fully buffered in memory, so a single
      presigned PUT is used instead of multipart; an in-flight PUT is aborted by context cancellation and a
      completed-but-unpublished object is deleted via a presigned DELETE. See handoff.)
- [x] Send `BodyRefFrame` with expected size and checksum.
- [x] Let Egress download through scoped credentials or signed URL only for the assigned request.
- [x] Verify size/checksum where available.
- [~] Clean up orphaned objects through lifecycle rules and explicit aborts. Explicit aborts implemented; the
      bucket lifecycle-rule backstop for crash-orphaned objects is owned by
      `docs/tasks/p2/21-object-storage-lifecycle-retention.md`.
- [x] Add tests for upload, cancel cleanup, checksum mismatch, expired credentials, tenant isolation, and Egress
      download.
- [x] Run focused request BodyRef tests.
- [x] Run `make check`.
- [x] Write a handoff note.

## Tests

- Focused request-body BodyRef tests.
- `make check`

## Acceptance Criteria

- Large request bodies can be passed by BodyRef without unbounded NATS frames.
- Cancellation cleans up unfinished uploads.
- BodyRef access is scoped to the tenant/request/executor assignment.

## Handoff Notes

- Document object cleanup and checksum behavior.

## Stop Conditions

- Stop before implementing response-body mode.
- Stop if a deferral would have no owning task file.
