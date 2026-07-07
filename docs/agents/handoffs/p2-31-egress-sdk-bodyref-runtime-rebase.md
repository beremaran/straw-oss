# Handoff

Task: `docs/tasks/p2/31-egress-sdk-bodyref-runtime-rebase.md`

## Changed

- Added `sdk/egress.HTTPBodyRefResolver`, which downloads S3 request BodyRefs and maps missing URL, expired URL,
  non-2xx fetch, size mismatch, and checksum mismatch to `body_ref_unavailable` error facts.
- Kept the official worker's HTTP client behavior inside `internal/egress` by constructing the SDK resolver with
  `ExecutorOptions.BodyRefHTTPClient` and `ExecutorOptions.Now`.
- Added SDK tests for successful BodyRef download/verification, unavailable-object mapping, expiry, checksum mismatch,
  size mismatch, and object-key scope validation.
- Updated the task 27 handoff note now that task 31's BodyRef runtime gap is closed.

## Acceptance Criteria Verdicts

Filled from the independent verifier's report (straw-task-runner workflow step 12), not from the implementer's
self-assessment.

| Criterion | Verdict | Implementation (file:line) | Proving test |
|-----------|---------|----------------------------|--------------|
| `internal/egress` no longer owns request-body BodyRef stream protocol except through the SDK-facing official body-ref hook. | VERIFIED | `sdk/egress/assignment.go:631`, `internal/egress/registration.go:92`, `internal/egress/executor.go:455` | `go test ./sdk/... ./internal/egress ./cmd/egress`; `make check` |
| SDK tests prove BodyRef scope validation, checksum mismatch, expiry, and unavailable-object error mapping. | VERIFIED | `sdk/egress/bodyref.go:22`, `sdk/egress/bodyref_test.go:15`, `sdk/egress/bodyref_test.go:34`, `sdk/egress/bodyref_test.go:86` | `go test ./sdk/... ./internal/egress ./cmd/egress` |
| `grep -R "\"github.com/beremaran/straw/v2/internal/" sdk/egress` returns no matches. | VERIFIED | `sdk/egress/bodyref.go:3`, `sdk/egress/types.go:3` | `grep -R "\"github.com/beremaran/straw/v2/internal/" sdk/egress` |
| Existing official executor tests still pass. | VERIFIED | `internal/egress/executor.go:455`, `internal/egress/worker.go:15` | `go test ./sdk/... ./internal/egress ./cmd/egress`; `make check` |

## Planning-Doc Coverage

Every in-phase field/endpoint/behavior from the task's cited planning-doc sections, accounted for:

| Planning item | Status | Evidence / owning task |
|---------------|--------|------------------------|
| Egress SDK owns worker protocol machinery with a public execution seam. | implemented | `sdk/egress/types.go:49`, `sdk/egress/assignment.go:631`, `sdk/egress/bodyref.go:22` |
| BodyRef frames are accepted only on the Control-to-executor stream and sequenced by the SDK runtime. | already existed | `sdk/egress/stream.go:72`, `sdk/egress/assignment.go:621` |
| Request BodyRef object keys are scoped to tenant, request, and request direction. | implemented | `sdk/egress/assignment.go:636`, `sdk/egress/assignment.go:655`, `sdk/egress/bodyref_test.go:86` |
| Executor downloads request BodyRefs from short-lived S3 signed URLs or scoped credentials. | implemented | `sdk/egress/bodyref.go:29`, `sdk/egress/bodyref.go:60` |
| Executor verifies expected size/checksum when available. | implemented | `sdk/egress/bodyref.go:100`, `sdk/egress/bodyref_test.go:15`, `sdk/egress/bodyref_test.go:62` |
| Expired BodyRefs map to unavailable-object style failure. | implemented | `sdk/egress/bodyref.go:53`, `sdk/egress/bodyref_test.go:58` |
| Official outbound execution stays in the worker while protocol hooks move to SDK. | implemented | `internal/egress/executor.go:455`; outbound executor behavior remains in `internal/egress/executor.go` |
| Raw tunnel, decoded stream, object-storage server behavior, lifecycle rules, and final live SDK conformance are outside this task. | out of scope | Raw tunnel: `docs/tasks/p2/27-egress-sdk-raw-tunnel-runtime-rebase.md`; decoded stream: `docs/tasks/p2/26-egress-sdk-decoded-stream-runtime-rebase.md`; storage/lifecycle: `docs/tasks/p2/06-object-storage-foundation.md`, `docs/tasks/p2/07-bodyref-request-body-flow.md`, `docs/tasks/p2/08-bodyref-response-body-flow.md`, `docs/tasks/p2/21-object-storage-lifecycle-retention.md`; final conformance/live proof: `docs/tasks/p2/24-egress-sdk-conformance-and-live-verification.md` |

## Verification

```sh
go test ./sdk/... ./internal/egress ./cmd/egress
grep -R "\"github.com/beremaran/straw/v2/internal/" sdk/egress
make check
```

Result:

- Focused tests: passed.
- SDK internal-import check: no matches.
- `make check`: passed.
- Postgres-backed tests: not exercised (diff does not touch Postgres surfaces or migrations).
- Live compose verification: skipped because this task is the SDK BodyRef runtime slice; final SDK/live compose proof
  is explicitly owned by `docs/tasks/p2/24-egress-sdk-conformance-and-live-verification.md`.

## Reviewer Start Points

- `sdk/egress/bodyref.go`
- `sdk/egress/bodyref_test.go`
- `internal/egress/executor.go`

## Remaining Work

- None.

Task 28 can proceed; no temporary BodyRef compatibility wrappers were introduced.

## Blockers

- None.
