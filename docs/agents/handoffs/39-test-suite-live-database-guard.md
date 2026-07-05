# Handoff

Task: `docs/tasks/p0/39-test-suite-live-database-guard.md`

## Changed

- `internal/control/postgres_store_test.go`: added `checkTestDatabaseDSN`, called by `newIdentityTestPool`
  before any connection is opened. The guard rejects (with `t.Fatalf`, not skip) any
  `STRAW_TEST_POSTGRES_DSN` whose database name does not end in `_test`. Chosen mechanism: DSN database-name
  suffix convention (the task's preferred option); no live-data heuristic was needed as a supplement because
  the guard fires before connecting, so the harness can never touch a non-`_test` database at all. Empty
  `STRAW_TEST_POSTGRES_DSN` still skips, as before.
- `internal/control/postgres_test_guard_test.go`: new pure unit test covering URL-form and keyword-form DSNs,
  the compose live database (rejected), `straw_test` (accepted), a missing database name (rejected — pgx
  defaults it to the user name), and an unparseable DSN (rejected).
- `deploy/docker/postgres-init.sql` + `docker-compose.yml`: `straw_test` is created via the Postgres image's
  `/docker-entrypoint-initdb.d` init mechanism on first volume initialization. On a pre-existing volume the
  init does not rerun; the documented fallback is
  `docker compose exec postgres createdb -U postgres straw_test` (verified working).
- `deploy/docker/README.md`: new "Running the Postgres-backed tests" section with the sanctioned
  `straw_test` DSN and the `createdb` fallback.
- `AGENTS.md` (and its `CLAUDE.md` symlink): Verification section now documents the `_test` suffix
  convention and points at the README section. No living doc still suggests the live `straw` DSN for tests
  (the remaining mentions are in historical handoff records, left as-is).

## Verification

```sh
# Guard rejects the live compose database, before opening any connection:
STRAW_TEST_POSTGRES_DSN='postgres://postgres:postgres@localhost:5432/straw?sslmode=disable' go test ./internal/control/
#   FAIL: refusing to run Postgres-backed tests: STRAW_TEST_POSTGRES_DSN targets database "straw"; ...

# Sanctioned path runs the Postgres-backed tests with zero skips (all Postgres* tests PASS):
STRAW_TEST_POSTGRES_DSN='postgres://postgres:postgres@localhost:5432/straw_test?sslmode=disable' go test ./...

make check
STRAW_TEST_POSTGRES_DSN='postgres://postgres:postgres@localhost:5432/straw_test?sslmode=disable' make check
```

Result: guard rejection confirmed against the compose `straw` database; full suite green against
`straw_test` with 0 skipped tests in `internal/control` (Postgres and Redis backends both up);
`make check` (gofmt + tests + `golangci-lint --max-issues-per-linter 0 --max-same-issues 0`) = 0 issues,
both with and without the test DSN set.

Note: one full-suite run showed Redis-backed tests skipping on ping timeout immediately after
`docker compose up -d redis`; reruns (and a stash-verified HEAD run) showed 0 skips — transient container
startup, unrelated to this change.

## Reviewer Start Points

- `internal/control/postgres_store_test.go` (`checkTestDatabaseDSN`, `newIdentityTestPool`)
- `internal/control/postgres_test_guard_test.go`
- `deploy/docker/postgres-init.sql`

## Remaining Work

- None. Nothing in this task is faked, stubbed, or deferred.

## Blockers

- None.
