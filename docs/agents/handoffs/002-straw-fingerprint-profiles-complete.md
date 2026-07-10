# Handoff

Feature: `specs/002-straw-fingerprint-profiles/`  
Task: `specs/002-straw-fingerprint-profiles/tasks.md#T040-T050`

## Resolution Update (2026-07-10)

T047-T048 closed the first convergence issues, TD001-TD003 closed the later runtime tech-debt findings, and T049
refreshed the complete live/adjacent evidence on runtime commit `f1a2e4d4318a55e8d9f29312f04a257cb053b0c8`.
T050 reconciled the cleanup report with those completed tasks and reran cleanup, SpecKit analysis, Architecture Guard
review/verification, and convergence. No finding, follow-up task, stale next-step instruction, or unowned deferral
remains. Current live evidence is in `specs/002-straw-fingerprint-profiles/evidence/live-coles.md`.

## Changed

- Synchronized canonical protobuf, error, Egress, ClickHouse, observability, config API, security, testing, public
  request/config, and local-stack quickstart documentation with the implemented `chrome_120` capability and evidence
  contracts.
- Updated governance/security evidence, ADR status, quality-scenario verdicts, parity verification, live evidence, and
  this completion handoff.
- Refreshed the complete quickstart after TD001-TD003: profile availability transitions, first-attempt Coles status
  200 and both markers, exactly one correlated ClickHouse row, and pre/post-live full checks all passed on `f1a2e4d`.
- Regenerated `specs/002-straw-fingerprint-profiles/tech-debt-report.md` as a zero-finding report and recorded the
  clean T050 cleanup, analysis, architecture, verification, and convergence gates.

## Acceptance Criteria Verdicts

| Criterion | Verdict | Implementation / evidence | Proving test / check |
|-----------|---------|---------------------------|----------------------|
| T040 canonical documentation synchronized | VERIFIED | `straw/docs/planning/13-protobuf-contract.md`, `14-canonical-error-registry.md`, `16-egress-execution.md`, `22-canonical-clickhouse-schema.md`, `23-observability.md`, `26-config-management-api-surface.md`, `27-security-controls.md`, `30-testing-matrix.md`, `straw/docs/public/api/{requests,config}.md`, `straw/docs/public/quickstart.md` | `git diff --check`; focused/full checks |
| T041 governance/security evidence re-reviewed | VERIFIED | `docs/security/002-straw-fingerprint-profiles.md`, ADR, `security/`, `evidence/live-coles.md` | focused/full Straw checks; redaction/conformance tests |
| T042 adjacent verification on one revision | VERIFIED | `specs/002-straw-fingerprint-profiles/evidence/live-coles.md` | exact command record below |
| T043 first-attempt Coles request | VERIFIED | `evidence/live-coles.md`: clean rebuild, fresh requester key, status 200, both product markers, one correlated row | `make infra-up`; fixed first-attempt request; ClickHouse query |
| T044 agent-surface parity | VERIFIED | `parity/agent-surface-parity.md` | `cmp -s AGENTS.md CLAUDE.md`; `cmp -s straw/AGENTS.md straw/CLAUDE.md` |
| T045 SpecKit/Architecture Guard analysis | VERIFIED with no blocking finding | analysis and architecture review/verification recorded in this handoff; no architecture constitution file exists in `.specify/memory/` | prerequisites, `git diff --check`, full Straw/lint gates |
| T046 final handoff and zero unowned code deferrals | VERIFIED | this file; live acceptance is now proven and no deferrals remain | task/evidence cross-check |
| T049 refreshed completion evidence after TD001-TD003 | VERIFIED | `specs/002-straw-fingerprint-profiles/evidence/live-coles.md`; `002-straw-fingerprint-profiles-t049.md` | complete quickstart, first-attempt Coles request, correlated ClickHouse query, pre/post-live full gates |
| T050 cleanup/governance reconciliation | VERIFIED | `specs/002-straw-fingerprint-profiles/tech-debt-report.md`; this handoff | zero cleanup/analyze/architecture findings; convergence appended no task; focused and full gates pass |

## Verification

Commands run from the repository revision `482df5095cf1644eb90c92ba44b0320969bfab1b`:

```sh
make check-protos                                      # PASS
make clickhouse-migrations-check                       # PASS
go test ./api/proto/straw/v1 ./sdk/egress ./internal/control ./internal/egress ./cmd/control ./cmd/egress  # PASS
go test ./internal/egress -run 'Test(ProfileRegistry|ProfileConformance|ProfilePinnedDial|ProfileConnectionIsolation|ProfileDeadline|ProfileCancellation|ProfileErrorMapping)' # PASS
go test ./internal/control -run 'Test(FingerprintProfile|WorkerFingerprintCapability|Dispatcher.*Fingerprint)' # PASS
go test ./internal/egress -run 'Test.*UnsupportedFingerprint.*NoUpstream' # PASS
go test ./internal/control ./internal/egress -run 'Test.*(Baseline|DefaultFingerprint|Unprofiled)' # PASS
make check-straw                                      # PASS: all Straw tests and golangci-lint
git diff --check                                      # PASS
make infra-up                                         # PASS: clean rebuild with known bootstrap key
```

`make check-straw` invokes `cd straw && make check`; the exact output and timestamps are captured in
`specs/002-straw-fingerprint-profiles/evidence/live-coles.md`. The live request returned status 200, both fixed product
markers, and one correlated requested/selected/executed `chrome_120` row. T049 refreshed this result on
`f1a2e4d4318a55e8d9f29312f04a257cb053b0c8` with request `req_1783697758593758126`.

T049's current evidence set ran from 2026-07-10T15:29:49Z through 2026-07-10T15:36:48Z
(2026-07-10T23:29:49+0800 through 2026-07-10T23:36:48+0800). The live request completed at
2026-07-10T15:35:59Z with status 200, both markers, and exactly one attempt-1 ClickHouse row. Compose images were
Control `sha256:bce161e31195936ae391f25a6c8be81f3239723d6219a7409b76dfcfe9f2e339`, Egress
`sha256:49450e57375b936fc5e0f7cdfb91af4b82c73b1b87ee94682b8b3e08a71eff3f`, and ClickHouse
`sha256:d4006648d4666e2d4e087657609789931ef7f3db9838b5e5d7a0eacc2b15f8e9`. Exact commands and timestamps are in
`specs/002-straw-fingerprint-profiles/evidence/live-coles.md`.

## Reviewer Start Points

- `straw/internal/egress/profile_registry.go`
- `straw/internal/egress/profiled_transport.go`
- `straw/internal/control/destination_policy.go`
- `straw/internal/control/request_metadata.go`
- `straw/api/proto/straw/v1/registration_sign.go`
- `infra/scripts/check-clickhouse-migrations.sh`
- `specs/002-straw-fingerprint-profiles/evidence/live-coles.md`

## Final Analysis and Architecture Gates

- `/speckit-cleanup-run`: zero critical, large, medium, or small findings; TD001-TD003 are verified resolved.
- `/speckit-analyze`: 19/19 functional requirements and 8/8 success criteria have task and implementation evidence;
  no ambiguity, duplication, inconsistency, constitution conflict, or unmapped implementation task was found.
- Architecture Guard review: task synchronization is `Synced`, boundary integrity is `Strong`, and architectural
  risk is `LOW`; no refactor task was generated.
- Architecture Guard verification: all tasks completed before T050 have implementation/evidence support, requirement
  coverage is 100%, and no ghost task, orphaned code, missing contract, or boundary bypass was found.
- The repository does not install `.specify/memory/architecture_constitution.md`, so the architecture gates used the
  project constitution, plan/contracts, security artifacts, and framework-agnostic Guard principles. The installed
  optional Sonar rules have no Go rules.
- `/speckit-converge`: converged with zero actionable finding and no appended task.

## Remaining Work at Handoff (Historical; Resolved)

- None.

Current remaining work: None. No feature gap or deferral is unowned.

## Blockers

- None. No commit was created for T049.
