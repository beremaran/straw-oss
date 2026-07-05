# 39 - Test Suite Live-Database Guard

Status: not started

## Objective

Stop the Postgres-backed test harness from destroying a live deployment's data when
`STRAW_TEST_POSTGRES_DSN` points at a database that is in real use (e.g. the docker-compose `straw` database).

## Context (gap being closed)

The 2026-07-05 live end-to-end verification hit this twice in one session:

1. Running `go test ./...` with `STRAW_TEST_POSTGRES_DSN` pointed at the compose stack's `straw` database
   truncated `tenants`, `worker_credentials`, and `api_keys` mid-session, wiping the running stack's seeded
   state (the harness truncates identity tables between tests — see `internal/control/postgres_store_test.go`).
2. The reverse direction: test fixtures leaked into the shared database. A test-seeded platform `system_admin`
   key (prefix `boot`, fixed ID `00000000-0000-0000-0000-000000000030`) survived the run, and because
   `BootstrapFromEnv` is a no-op when any active platform system_admin exists, it silently blocked the
   `STRAW_BOOTSTRAP_SYSTEM_ADMIN_KEY` bootstrap on every subsequent Control startup until manually deleted.

Both failure modes are silent. The docs currently even suggest the compose DSN as the test DSN.

## Required Planning Docs

- `docs/planning/21-state-and-storage.md` (Postgres role and schema ownership)
- `docs/planning/30-testing-matrix.md` (test-tier definitions)

## Prerequisites

- Task 18 completed (Postgres foundation and the test harness this task hardens).

## Out of Scope

- Do not introduce testcontainers or any new dependency; the guard is plain SQL + convention.
- Do not change what the harness truncates or how migrations apply.

## Expected Files

- Modify: `internal/control/postgres_store_test.go` (harness guard).
- Modify: `deploy/docker/README.md` and/or `AGENTS.md` (document the dedicated test database convention).
- Possibly modify: `docker-compose.yml` (create a separate `straw_test` database for local test runs).

## Steps

- [ ] Read the required planning docs.
- [ ] Pick the guard mechanism. Preferred: the harness refuses to run unless the target database is provably a
      test database — e.g. the DSN's database name ends in `_test`, or a `straw_test_marker` table exists.
      A live-data heuristic (refuse when unexpected rows exist) is acceptable only as a supplement, not the
      primary guard.
- [ ] Make the harness fail with a clear, actionable error (not skip) when the guard rejects the DSN, so a
      misconfigured CI run is loud instead of silently green-with-skips.
- [ ] Provide the sanctioned local path: a `straw_test` database in compose (created via the Postgres image's
      init mechanism or documented `createdb`), and update every doc that previously suggested the live DSN.
- [ ] Verify: point the harness at the compose `straw` database and confirm it refuses; point it at the test
      database and confirm the full suite runs with zero skips.
- [ ] Run `make check`.
- [ ] Write a handoff note.

## Tests

- `STRAW_TEST_POSTGRES_DSN=<live-db> go test ./internal/control/` fails fast with the guard error.
- `STRAW_TEST_POSTGRES_DSN=<test-db> go test ./...` runs the Postgres-backed tests with no skips.
- `make check`

## Acceptance Criteria

- The harness can no longer truncate a database that is not explicitly designated for tests.
- Test fixtures can no longer leak into the compose database (the `boot` admin-key incident is impossible).
- The dedicated test-database workflow is documented where the old DSN advice lived.

## Handoff Notes

- Record the chosen guard mechanism and the test-database naming convention.

## Stop Conditions

- Stop if the guard requires a new dependency.
- Stop if a deferral would have no owning task file.
