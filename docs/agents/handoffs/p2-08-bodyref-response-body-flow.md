# Handoff

Task: `docs/tasks/p2/08-bodyref-response-body-flow.md`

Implements the resolved `P2 BodyRef Response-Body Mode`
(`docs/planning/32-open-decisions.md`, resolved 2026-07-07): the executor streams the response body through
Control on the synchronous transport path, and Control tees the completed body to object storage. The teed object
backs the REST download reference. Only the object-storage tee mode ships; the rejected executor-writes-object mode
is not implemented.

## Changed

- `internal/control/dispatcher.go` — response path no longer errors when a buffered response exceeds the inline
  threshold with object storage enabled. `acceptResponseData` sets `dispatchResult.useBodyRef` and keeps streaming;
  after a successful stream (`finalizeDispatch`), `teeResponseBody` uploads the completed body and records the
  BodyRef. DirectStreamRef response transport still maps to `body_ref_unavailable` (only one mode ships).
- `internal/control/body_ref_store.go` — `S3RequestBodyRefStore` now implements `ResponseBodyRefStore`
  (`UploadResponseBody`/`DeleteResponseBody`) via a shared `uploadBody`/`deleteBody`, using object direction
  `response`. New `ResponseBodyRefStore` interface.
- `internal/control/handler.go` — `ResponseBody` gains `body_ref` mode with a `ResponseBodyRef` download reference
  (object key, signed URL, expiry, size, sha256).
- `cmd/control/main.go` — `buildBodyRefStore` (renamed from `buildRequestBodyRefStore`) returns the concrete store;
  wired into the dispatcher as both `BodyObjectStore` and `ResponseObjectStore` with a typed-nil guard.
- Tests: `dispatcher_test.go` (tee, outage, cancellation-no-orphan), `body_ref_store_test.go`
  (response-direction scoped upload + size/checksum + SSE + short-lived signed URL).

## Acceptance Criteria Verdicts

From the independent verifier (`.llm-docs/skills/verify-straw-task`), given only the task file and the staged diff.

| Criterion | Verdict | Implementation (file:line) | Proving test |
|-----------|---------|----------------------------|--------------|
| Only one response BodyRef mode ships (object-storage tee; DirectStreamRef response → `body_ref_unavailable`) | VERIFIED | `internal/control/dispatcher.go:1161` (S3 sets useBodyRef; DirectStreamRef + default → body_ref_unavailable) | `TestDispatcherResponseBodyRefUnavailableWhenSelectedBackendUnwired`, `TestDispatcherTeesLargeResponseBodyToObjectStorage` |
| Required open-decision acceptance tests pass (cancellation cleanup, checksum/size, retention, outage, too-large) | VERIFIED | tee `dispatcher.go:277-289`; cancel-guarded by success-only finalize `dispatcher.go:247` | `TestDispatcherDoesNotTeeResponseWhenCancelled`, `TestDispatcherTeesLargeResponseBodyToObjectStorage`, `TestS3RequestBodyRefStoreUploadsScopedResponseBody`, `TestDispatcherResponseTeeObjectStorageOutage`, `TestDispatcherResponseBodyTooLarge` |
| Object-storage outage behavior is explicit | VERIFIED | `dispatcher.go:283` upload error → `bodyRefUnavailableError(Response, S3BodyRef)` (HTTP 502), matches planning/29 outage row | `TestDispatcherResponseTeeObjectStorageOutage` |

## Planning-Doc Coverage

| Planning item (doc 18 / 29) | Status | Evidence / owning task |
|---------------|--------|------------------------|
| Response mode = executor streams through Control while teeing to object storage | implemented | `dispatcher.go` `acceptResponseData`/`teeResponseBody` |
| Teed object backs REST download reference | implemented | `handler.go` `ResponseBody.body_ref` + `dispatcher.go` `responseBodyFromDispatch` |
| Reject executor-writes-object/read-after-completion mode | out of scope (rejected) | not implemented; DirectStreamRef response → body_ref_unavailable |
| Response size thresholds + `body_too_large` (no transport enabled) | already existed + wired | `SelectBodyTransport` (task 05) via `acceptResponseData`; `TestDispatcherResponseBodyTooLarge` |
| Size/checksum verification | implemented | `body_ref_store.go` `bodyRefFrame` (sha256 + size); surfaced in `ResponseBodyRef` |
| Object key tenant/request/direction scoping + high-entropy nonce | already existed | `objectstore.ObjectKey` (task 06); response direction used here |
| Server-side encryption on upload; unguessable keys; short-lived signed URL | already existed | `objectstore` (task 06); asserted in `TestS3RequestBodyRefStoreUploadsScopedResponseBody` |
| Retention (1 day default, 3 max) | already existed | `objectstore` retention (task 06), wired via `main.go RetentionDays` |
| Object-storage-unavailable outage → BodyRef fails | implemented | `teeResponseBody` maps `objectstore.ErrUnavailable` → `body_ref_unavailable` |
| Bucket lifecycle backstop for crash-orphaned objects | out of scope | owned by `docs/tasks/p2/21-object-storage-lifecycle-retention.md` |

## Verification

```sh
make check
```

Result: PASS (gofmt, `go test ./...`, `golangci-lint` — 0 issues).

- Postgres-backed tests: not exercised — diff touches no `postgres_*` files or `migrations/`.
- Live compose verification: skipped. The response tee is proven end-to-end by the live in-process harness
  (`newLiveDispatchHarness` runs a real egress worker + real NATS + real upstream through Control); the object
  store is the one seam covered by a test double. Driving the full docker-compose stack with a real MinIO/S3
  bucket was not run.

## Reviewer Start Points

- `internal/control/dispatcher.go` — `acceptResponseData`, `finalizeDispatch`, `teeResponseBody`.
- `internal/control/body_ref_store.go` — `UploadResponseBody`/`uploadBody`.
- `internal/control/handler.go` — `ResponseBody` / `ResponseBodyRef`.

## Remaining Work

- None faked or stubbed in the production path: the built `cmd/control` binary constructs the real
  `objectstore.Client`-backed store and passes it as `ResponseObjectStore`.
- **Known simplification (not a deferral):** Control buffers the full response body in memory before the single
  presigned PUT, rather than a concurrent multipart stream-tee. This matches the buffered REST response contract
  and the resolved decision (Control writes the object). The request-side flow (task 07) buffers identically; a
  multipart upgrade, if ever pursued, would be a shared enhancement to the objectstore foundation and is not
  required by this task.
- Bucket-level lifecycle retention for objects orphaned by a Control crash is owned by
  `docs/tasks/p2/21-object-storage-lifecycle-retention.md` (shared by tasks 07/08/11). This task creates no
  orphans on the normal or cancellation paths (the tee runs only after successful stream completion).

## Blockers

- None. Work is committed (see task-runner commit).
