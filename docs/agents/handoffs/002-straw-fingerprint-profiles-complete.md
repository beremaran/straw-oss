# Handoff

Feature: `specs/002-straw-fingerprint-profiles/`  
Task: `specs/002-straw-fingerprint-profiles/tasks.md#T040-T046`

## Resolution Update (2026-07-10)

This handoff was complete for T040-T046. The later cleanup gate discovered two convergence issues, subsequently closed
by T047-T048. Current final evidence is in `002-straw-fingerprint-profiles-t047-t048.md`; the historical verdicts below
remain valid for their original scope.

## Changed

- Synchronized canonical protobuf, error, Egress, ClickHouse, observability, config API, security, testing, public
  request/config, and local-stack quickstart documentation with the implemented `chrome_120` capability and evidence
  contracts.
- Updated governance/security evidence, ADR status, quality-scenario verdicts, parity verification, live evidence, and
  this completion handoff.

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
markers, and one correlated requested/selected/executed `chrome_120` row.

## Reviewer Start Points

- `straw/internal/egress/profile_registry.go`
- `straw/internal/egress/profiled_transport.go`
- `straw/internal/control/destination_policy.go`
- `straw/internal/control/request_metadata.go`
- `straw/api/proto/straw/v1/registration_sign.go`
- `infra/scripts/check-clickhouse-migrations.sh`
- `specs/002-straw-fingerprint-profiles/evidence/live-coles.md`

## Remaining Work at Handoff (Historical; Resolved)

- None.

Current remaining work after T047-T048: none.

## Blockers

- None. No commit was created.
