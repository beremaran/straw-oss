# Handoff

Task: `docs/tasks/p2/07-bodyref-request-body-flow.md`

## Changed

- `internal/control/body_ref_store.go` (new): `RequestBodyRefStore` interface + `S3RequestBodyRefStore`. Uploads a
  buffered request body via a scoped presigned PUT (SSE header from task 06), returns a `BodyRefFrame` carrying the
  scoped object key, a presigned GET URL, `ExpiresUnixMs`, expected size, and sha256. `DeleteRequestBody` removes an
  object via a presigned DELETE for explicit cleanup.
- `internal/control/body_ref_store_test.go` (new): scoped-upload and cancel-stops-upload tests.
- `internal/control/dispatcher.go`: `sendRequestStart` now uploads large request bodies (via `sendRequestBody`) after
  auth/validate/route/assign, publishes a single `StreamFrame_BodyRef` for the S3 path, and deletes the uploaded
  object on flush/publish failure (`deleteUploadedRequestBody`). `requestStreamError` carries the pipeline error so a
  BodyRef-unavailable selection surfaces the correct code.
- `internal/egress/loop.go`: `readRequestBody` accepts a `StreamFrame_BodyRef`, enforces the
  `tenant/<id>/request/<id>/request/` object-key prefix (executor-assignment scope), downloads via the signed URL, and
  fails with `body_ref_unavailable` / `body_ref_scope_mismatch` frames.
- `internal/egress/executor.go`: `downloadBodyRef` validates the signed URL/expiry, fetches, and verifies size + sha256.
- `internal/natsx/stream.go`: `StreamValidatorOptions{AllowBodyRef}` gates BodyRef acceptance to the P2 request receive
  path only; P0/response validators still reject BodyRef.
- `internal/objectstore/objectstore.go`: added `PresignDelete` (single-key DELETE) for explicit cleanup.
- `cmd/control/main.go`: `buildRequestBodyRefStore` constructs the real `objectstore.Client` +
  `S3RequestBodyRefStore` and wires it into the dispatcher as `BodyObjectStore` (nil when object storage disabled).
- `internal/control/lifecycle.go` / `errors_test.go`: `ERROR_CODE_BODY_REF_UNAVAILABLE` added to the executor-emittable
  set.

## Acceptance Criteria Verdicts

From the independent verifier (workflow step 12), evidence read from disk.

| Criterion | Verdict | Implementation (file:line) | Proving test |
|-----------|---------|----------------------------|--------------|
| Large request bodies passed by BodyRef without unbounded NATS frames | VERIFIED | `internal/control/dispatcher.go:706-758`; `internal/natsx/stream.go:188-194` | `TestDispatcherSendsLargeRequestBodyRef` (`dispatcher_test.go:269`) |
| Cancellation cleans up unfinished uploads | VERIFIED | `internal/control/body_ref_store.go:108-132`; `dispatcher.go:696-701,747-750,760-769` | `TestS3RequestBodyRefStoreStopsUploadOnCancellation`, `TestDispatcherCleansUploadedRequestBodyRefOnPublishFailure` |
| BodyRef access scoped to tenant/request/executor assignment | VERIFIED | `internal/egress/loop.go:674-703`; `internal/objectstore/objectstore.go:209-226,243-311` | `TestReadRequestBodyRejectsBodyRefTenantRequestMismatch` (`loop_test.go:340`) |

Wiring confirmed: `cmd/control/main.go:1086` → `buildRequestBodyRefStore` builds a real `objectstore.New(...)` client
and passes `NewS3RequestBodyRefStore` to the dispatcher.

## Planning-Doc Coverage

`docs/planning/18-large-body-transport-p2.md` — S3 Request Body Flow, Security, Retention:

| Planning item | Status | Evidence / owning task |
|---------------|--------|------------------------|
| Upload after auth/validate/route/assign | implemented | upload runs inside `sendRequestStart`, post-assignment (`dispatcher.go:706`) |
| Object key `tenant/<t>/request/<r>/<dir>/<nonce>` | already existed | `objectstore.ObjectKey` (task 06) |
| High-entropy nonce | already existed | `objectstore.go:218-225` (task 06) |
| Upload via multipart | implemented (single PUT) | Bodies are fully buffered `[]byte`, so a single presigned PUT is used; multipart offers nothing for in-memory bodies and is not implemented. In-flight abort = context-cancelled PUT (`body_ref_store.go:108`) |
| Send `BodyRefFrame` on completion | implemented | `dispatcher.go:735-746` |
| Executor downloads via signed URL | implemented | `executor.go:downloadBodyRef` |
| Executor verifies size/checksum | implemented | `executor.go:516-525` |
| Delete/abort unfinished on cancellation | implemented | `deleteUploadedRequestBody` + context-cancel PUT |
| Lifecycle rules clean orphaned objects | out of scope | `docs/tasks/p2/21-object-storage-lifecycle-retention.md` |
| Object storage security (unguessable, tenant-scoped, SSE, short expiry, no listing, request/tenant/executor-bound) | implemented / already existed | task 06 (`PresignPut` SSE, `clampExpiry`, no list op) + executor scope check (`loop.go:674-703`) |
| Retention default 1 / max 3 days | already existed | `config.go` + `objectstore.go` validation (task 06) |

## Verification

```sh
make check
```

Result: green (`go test ./...` all pass; `golangci-lint` 0 issues).

- Postgres-backed tests: not exercised — diff touches no `postgres_*` files or `migrations/`.
- Live compose verification: skipped — object storage is not part of the `deploy/docker` compose stack, so the S3
  BodyRef path cannot be driven end-to-end there. The flow is instead proven by a full in-process round trip through a
  real `egress.Worker`/`Executor` downloading from a live httptest object server
  (`TestDispatcherControlNATSEgressRoundTripLargeRequestBodyRef`). Standing up object storage in compose is part of
  task 21.

## Reviewer Start Points

- `internal/control/dispatcher.go` (`sendRequestBody`, `deleteUploadedRequestBody`)
- `internal/control/body_ref_store.go`
- `internal/egress/loop.go` (`acceptBodyRef`, `bodyRefObjectKeyScoped`)
- `internal/egress/executor.go` (`downloadBodyRef`, `verifyBodyRef`)

## Remaining Work

- Bucket-level lifecycle-rule backstop that expires objects orphaned by a Control crash (planning doc 18 step 9) was
  completed by `docs/tasks/p2/21-object-storage-lifecycle-retention.md`. Explicit per-object DELETE/abort cleanup is
  implemented in this task.
- Object-storage lifecycle behavior for compose/production was documented by task 21.

## Blockers

- None. Work committed (all local checks ran; no `--no-verify`).

## Object Cleanup and Checksum Behavior (task-requested note)

- Cleanup: an in-flight upload is aborted by cancelling the request context (the PUT stops). An object that was
  successfully uploaded but whose request stream then fails to publish/flush is deleted via a presigned DELETE
  (`deleteUploadedRequestBody`, best-effort, 1s timeout, `context.WithoutCancel`). On the happy path the object is left
  for retention/lifecycle to expire (task 21).
- Checksum/size: Control computes sha256 and byte length at upload and puts them on the `BodyRefFrame`. The executor
  recomputes both after download and fails with `body_ref_size_mismatch` / `body_ref_checksum_mismatch`
  (`ERROR_CODE_BODY_REF_UNAVAILABLE`) on any mismatch. Expiry and tenant/request scope are checked before download.
