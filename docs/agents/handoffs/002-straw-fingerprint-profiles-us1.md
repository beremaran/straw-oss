# Handoff

Feature: `specs/002-straw-fingerprint-profiles/`  
Task: `specs/002-straw-fingerprint-profiles/tasks.md#T017-T025`

## Resolution Update (2026-07-10)

The environment-gated/open T043 and later-story entries below are historical snapshots of the bounded US1 run.
T026-T034 closed US2, T035-T039 closed US3, and T043 closed the clean-stack Coles run. Final evidence is in
`002-straw-fingerprint-profiles-complete.md` and `specs/002-straw-fingerprint-profiles/evidence/live-coles.md`.

## Changed

- Control now applies exact named-profile capability filtering after ordinary tenant, route, sticky, health, pool, and capacity eligibility; buffered, raw, and tunnel preparation paths carry the request intent.
- Egress resolves the local executable profile before DNS, emits the executed profile in `OutboundStartFrame`, and executes named requests through a request-scoped tls-client/fhttp adapter with validated-IP dialing, original-host SNI/certificate verification, no redirects/HTTP3/reuse, live 32 KiB streaming, trailers, deadlines, cancellation, and canonical errors.
- Control propagates selected/executed values through both response paths and rejects non-empty selected/executed drift.
- Request metadata now records bounded requested/selected/executed profile evidence with unsafe-value hashing.
- Added local TLS/HTTP2 wire observation against `testdata/chrome_120_v1_15_1.json`, baseline-difference assertions, and focused US1 tests.

## Acceptance Criteria Verdicts

Fresh verifier: focused package tests, adjacent package tests, protobuf checks, and `make check-straw` passed on the final worktree.

| Criterion | Verdict | Implementation (file:line) | Proving test |
|-----------|---------|----------------------------|--------------|
| Exact `chrome_120` selection follows ordinary eligibility and works in buffered/raw dispatch | VERIFIED | `internal/control/routing.go:141`, `internal/control/dispatcher.go:516` | `TestDispatcherNamedFingerprintUsesCapableSessionAfterOrdinaryEligibility`, `TestDispatcherNamedFingerprintBufferedRoundTrip`, `TestDispatcherNamedFingerprintRawRoundTrip` |
| Named transport uses the validated IP and original host binding while preserving streaming and failure semantics | VERIFIED | `internal/egress/profiled_transport.go:35`, `internal/egress/executor.go:406` | `TestProfilePinnedDialPreservesOriginalHostAndCertificate`, `TestProfileConformanceStreamsBodyAndLateTrailers`, `TestProfileConnectionIsolationDoesNotFollowRedirects`, `TestProfileDeadlineDuringPinnedDial`, `TestProfileCancellationStopsPinnedDial`, `TestProfileErrorMappingUsesFHTTPStreamError` |
| Executed profile is emitted after local resolution and Control rejects drift | VERIFIED | `internal/egress/executor.go:1600`, `internal/control/lifecycle.go:82` | `TestProfileConformanceEmitsExecutedAfterLocalResolutionBeforeDNS`, `TestSDKDecodedRuntimePreservesExecutedFingerprintOnWire`, `TestFingerprintProfileExecutedMismatchRejected` |
| Runtime wire behavior equals the committed fixture and differs from baseline | VERIFIED | `internal/egress/profile_conformance_test.go:43` | `TestProfileConformanceChrome120MatchesGoldenOnLocalWireAndDiffersFromBaseline` |
| Requested/selected/executed evidence is correlated and redacted | VERIFIED | `internal/control/request_metadata.go:182`, `internal/control/request_metadata.go:283` | `TestRequestEventProfileEvidenceAndRedaction`, `TestRequestEventCarriesProfileEvidenceOnTransportFailure` |
| Static/adjacent US1 verification is complete | VERIFIED | `specs/002-straw-fingerprint-profiles/quickstart.md` | Commands below |
| Unified-stack Coles first-attempt live acceptance | HISTORICAL OPEN — resolved by T043 | `specs/002-straw-fingerprint-profiles/evidence/live-coles.md` | Not run in this bounded local slice; T043 later recorded the clean-stack pass |

## Planning-Doc Coverage

| Planning item | Status | Evidence / owning task |
|---------------|--------|------------------------|
| Exact catalog and `chrome_120` executable binding | implemented | `internal/egress/profile_registry.go:8`, existing committed fixture; T020/T021 |
| Ordinary routing before exact session capability filtering | implemented | `internal/control/routing.go:147`; T020 |
| Pinned profiled transport, original host/SNI, response adapter, cleanup, and canonical errors | implemented | `internal/egress/profiled_transport.go:35`; T021 |
| Wire execution evidence and selected/executed invariant | implemented | `internal/egress/executor.go:406`, `internal/control/dispatcher.go:1371`, `internal/control/lifecycle.go:82`; T023 |
| Request-event evidence and redaction | implemented | `internal/control/request_metadata.go:241`; T024 |
| Local conformance observer and normalized golden comparison | implemented | `internal/egress/profile_conformance_test.go:43`; T022 |
| Live Coles acceptance | historical open state; resolved | T043; final evidence in `specs/002-straw-fingerprint-profiles/evidence/live-coles.md` |

## Verification

```sh
go test ./internal/egress -run 'Test(ProfileRegistry|ProfileConformance|ProfilePinnedDial|ProfileConnectionIsolation|ProfileDeadline|ProfileCancellation|ProfileErrorMapping)' -count=1
go test ./internal/control -run 'Test(FingerprintProfile|WorkerFingerprintCapability|Dispatcher.*Fingerprint)' -count=1
go test ./internal/egress -run 'Test.*UnsupportedFingerprint.*NoUpstream' -count=1
go test ./api/proto/straw/v1 ./sdk/egress ./internal/control ./internal/egress ./cmd/control ./cmd/egress -count=1
make check-protos
make check-straw
cd straw && make check
```

Result:

- Focused Egress and Control tests: passed.
- Unsupported-profile no-upstream selector: passed with no tests to run; that observer is owned by US2/T028.
- Adjacent package tests: passed.
- Protobuf lint/build: passed.
- Straw check: passed (`go test ./...`, formatting, and `golangci-lint`; lint reported 0 issues).
- Postgres-backed tests: not exercised; this slice does not change Postgres surfaces or migrations.
- Live compose verification: not run in this historical slice; T043 later closed the gate in
  `specs/002-straw-fingerprint-profiles/evidence/live-coles.md`.

## Reviewer Start Points

- `internal/egress/profiled_transport.go`
- `internal/egress/profile_conformance_test.go`
- `internal/control/routing.go`
- `internal/control/dispatcher.go`
- `internal/control/request_metadata.go`

## Remaining Work at Handoff (Historical; Resolved)

- T017–T025 are implemented and independently verified.
- Later US2/US3 and completion tasks remain outside this selected US1 slice.
- The live Coles acceptance was open and owned by T043 at handoff; T043 later closed it with the final live evidence.

## Blockers

- None for the local US1 slice. Changes are uncommitted in the shared worktree.
