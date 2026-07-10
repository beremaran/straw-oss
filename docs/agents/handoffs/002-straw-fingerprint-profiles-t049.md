# Handoff

Feature: `specs/002-straw-fingerprint-profiles/`  
Task: `specs/002-straw-fingerprint-profiles/tasks.md#T049`

## Changed

- Refreshed `specs/002-straw-fingerprint-profiles/evidence/live-coles.md` with the current revision, exact UTC/Perth
  timestamps, compose image identifiers, profile availability transitions, fixed Coles result, correlated ClickHouse
  row, and all declared adjacent checks.
- Updated the final feature handoff with T049's current acceptance result and explicit T050 ownership.
- Marked only T049 complete.

## Acceptance Criteria Verdicts

| Criterion | Verdict | Implementation / evidence | Proving test / check |
|-----------|---------|---------------------------|----------------------|
| Complete quickstart and adjacent checks use one runtime revision | VERIFIED | `specs/002-straw-fingerprint-profiles/evidence/live-coles.md` | exact pre/live/post command and timestamp record on `f1a2e4d4318a55e8d9f29312f04a257cb053b0c8` |
| Fixed Coles request succeeds on first attempt | VERIFIED | request `req_1783697758593758126` in `evidence/live-coles.md` | wrapper status 200; `Coles Full Cream Milk` and `8150288` assertions |
| Operational evidence is correlated and bounded | VERIFIED | one ClickHouse attempt-1 row in `evidence/live-coles.md` | requested/selected/executed `chrome_120`; status 200; empty error |
| FR-019 and SC-006 adjacent/live evidence is current | VERIFIED | `evidence/live-coles.md`; final feature handoff | protobuf, migration, focused, full Straw, post-live, lint, and diff gates pass |

## Planning-Doc Coverage

| Planning item | Status | Evidence / owning task |
|---------------|--------|------------------------|
| FR-019 complete adjacent/profile/rejection/baseline/live gate | implemented and refreshed | T049; `specs/002-straw-fingerprint-profiles/evidence/live-coles.md` |
| SC-006 one completion evidence set | implemented and refreshed | T049; runtime revision and timestamps in `evidence/live-coles.md` |
| Cleanup-report/governance reconciliation | out of T049 scope, explicitly owned | T050 |

## Verification

```sh
make check-protos
make clickhouse-migrations-check
cd straw && go test ./api/proto/straw/v1 ./sdk/egress ./internal/control ./internal/egress ./cmd/control ./cmd/egress
cd straw && go test ./internal/egress -run 'Test(ProfileRegistry|ProfileConformance|ProfilePinnedDial|ProfileConnectionIsolation|ProfileDeadline|ProfileCancellation|ProfileErrorMapping)'
cd straw && go test ./internal/control -run 'Test(FingerprintProfile|WorkerFingerprintCapability|Dispatcher.*Fingerprint)'
cd straw && go test ./internal/egress -run 'Test.*UnsupportedFingerprint.*NoUpstream'
cd straw && go test ./internal/control ./internal/egress -run 'Test.*(Baseline|DefaultFingerprint|Unprofiled)'
make check-straw
make infra-up
make check-straw
cd straw && make check
git diff --check
```

Result: all checks passed; both full Straw runs reported zero lint issues. The rebuilt local stack served the fixed
first-attempt Coles request with status 200 and both markers, and ClickHouse contained exactly one correlated row.

- Postgres-backed test harness: not separately exercised; T049 changes only evidence/task Markdown. The compose
  Postgres path was exercised by the complete live quickstart.
- Live compose verification: passed on Control
  `sha256:bce161e31195936ae391f25a6c8be81f3239723d6219a7409b76dfcfe9f2e339` and Egress
  `sha256:49450e57375b936fc5e0f7cdfb91af4b82c73b1b87ee94682b8b3e08a71eff3f`.

## Reviewer Start Points

- `specs/002-straw-fingerprint-profiles/evidence/live-coles.md`
- `straw/docs/agents/handoffs/002-straw-fingerprint-profiles-complete.md`
- `specs/002-straw-fingerprint-profiles/tasks.md#T049`

## Remaining Work

- T050 owns cleanup-report and final governance reconciliation. T049 has no unowned deferral.

## Blockers

- None. No commit was created.
