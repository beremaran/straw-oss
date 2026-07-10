# Handoff

Feature: `specs/002-straw-fingerprint-profiles/`
Tasks: `specs/002-straw-fingerprint-profiles/tasks.md#T015-T016`
Revision under test: `4820efa` on `main` plus the uncommitted T015-T016 diff
Verified: `2026-07-10T12:19:27Z`

## Resolution Update (2026-07-10)

The later-behavior and open-task entries below are historical snapshots. T024/T026/T029-T030/T033 closed runtime and
rejection evidence, and T042-T046 closed full checks, live acceptance, reviews, and completion. Final evidence:
`002-straw-fingerprint-profiles-complete.md` and
`specs/002-straw-fingerprint-profiles/evidence/live-coles.md`.

## Changed

- Added requested, selected, and executed fingerprint-profile fields to the shared `RequestEvent` JSON/ClickHouse row.
- Added a redaction-focused JSON test proving profile fields survive while sensitive query, header, and body values do not.
- Added the three empty-default `LowCardinality(String)` columns to the clean ClickHouse schema.
- Added an idempotent, startup-race-tolerant ClickHouse migration and a Docker-backed clean-schema/existing-volume check.
- Wired a one-shot Compose migration before Control startup and exposed `make clickhouse-migrations-check`.

## Acceptance Criteria Verdicts

`/speckit-analyze` found no artifact inconsistency, unmapped requirement, ordering conflict, or constitution violation. Its scoped T015-T016 verdicts are:

| Criterion | Verdict | Implementation (file:line) | Proving test / check |
|-----------|---------|----------------------------|----------------------|
| Shared request-event shape exposes requested/selected/executed profile evidence | VERIFIED | `straw/internal/control/request_metadata.go:31` | `TestRequestEventProfileEvidenceAndRedaction` |
| Evidence JSON preserves all three exact profile values without sensitive query/header/body content | VERIFIED | `straw/internal/control/request_metadata_test.go:255` | `go test ./internal/control -run TestRequestEventProfileEvidenceAndRedaction` |
| Clean ClickHouse schemas contain exact empty-default `LowCardinality(String)` columns | VERIFIED | `infra/clickhouse-schema.sql:22` | `make clickhouse-migrations-check` clean-schema case |
| Existing ClickHouse volumes gain the columns idempotently | VERIFIED | `infra/scripts/migrate-clickhouse.sh:14` | `make clickhouse-migrations-check` existing-volume case and repeated migration |
| Control cannot start before the one-shot schema migration succeeds | VERIFIED | `infra/docker-compose.yml:67` and `infra/docker-compose.yml:116` | `docker compose config -q`; live `clickhouse-migrate` exit 0 |
| Repository exposes the declared migration gate | VERIFIED | `Makefile:33` | `make clickhouse-migrations-check` |

## Planning-Doc Coverage

| Planning item | Status | Evidence / owning task |
|---------------|--------|------------------------|
| Add bounded requested/selected/executed request-event fields | implemented | `straw/internal/control/request_metadata.go:45` |
| Clean-volume canonical schema | implemented | `infra/clickhouse-schema.sql:22` |
| Existing-volume-safe idempotent ALTER path | implemented | `infra/scripts/migrate-clickhouse.sh:14` |
| Clean and persisted-volume verification | implemented | `infra/scripts/check-clickhouse-migrations.sh:97` |
| Run migration before Control emits new JSON fields | implemented | `infra/docker-compose.yml:67` and `infra/docker-compose.yml:116` |
| Populate named success/failure evidence on runtime paths | out of this slice | T024 |
| Hash-plus-byte-length projection for unsafe requested values | out of this slice | T026 and T030 |
| Unsupported rejection event population | out of this slice | T029 and T033 |

## Verification

```sh
# T015 red phase: expected failures observed
cd straw && go test ./internal/control -run TestRequestEventProfileEvidenceAndRedaction
# failed: RequestEvent fields absent
./infra/scripts/check-clickhouse-migrations.sh
# failed: canonical schema column absent

# T016 focused and adjacent gates
cd straw && go test ./internal/control -run 'Test(RequestEventProfileEvidenceAndRedaction|BuildRequestEventRecordsActorAndSanitizedTarget|HTTPClickHouseSinkRequestFormat)'
bash -n infra/scripts/migrate-clickhouse.sh infra/scripts/check-clickhouse-migrations.sh
docker compose -f infra/docker-compose.yml config -q
make clickhouse-migrations-check
make check-straw

# Existing running Compose volume
docker compose -f infra/docker-compose.yml up --no-deps --force-recreate --abort-on-container-exit --exit-code-from clickhouse-migrate clickhouse-migrate
docker compose -f infra/docker-compose.yml exec -T clickhouse clickhouse-client --query "SELECT name, type, default_kind, default_expression FROM system.columns WHERE database = 'straw' AND table = 'request_events' AND name LIKE '%fingerprint_profile' ORDER BY name"
```

Results:

- Focused Go tests: PASS.
- Shell syntax and Compose configuration: PASS.
- Clean-schema, legacy persisted-volume, and repeated migration checks: PASS.
- Existing running Compose volume migration: PASS; one-shot service exited 0 and all three exact columns/defaults were observed.
- `make check-straw`: PASS; all Go tests and `golangci-lint` completed with 0 issues.
- Postgres-backed tests: not exercised; this diff does not touch Postgres surfaces.
- Live request verification: not run in this historical slice; resolved by T043 with final evidence in
  `specs/002-straw-fingerprint-profiles/evidence/live-coles.md`.

## Reviewer Start Points

- `infra/scripts/check-clickhouse-migrations.sh`
- `infra/scripts/migrate-clickhouse.sh`
- `straw/internal/control/request_metadata_test.go`
- `infra/docker-compose.yml`

## Remaining Work at Handoff (Historical; Resolved)

- T024 owns success/transport-failure runtime population of the shared fields.
- T026 and T030 own attacker-controlled requested-value projection and validation.
- T029 and T033 own unsupported-rejection event population.
- T042-T046 own full-feature gates, the live Coles run, final reviews, and completion evidence.

No work required by T015-T016 was stubbed or deferred. Every later behavior above was closed by its named task; final
evidence is in `002-straw-fingerprint-profiles-complete.md`.

## Blockers

- None. Changes are intentionally uncommitted for user review.
