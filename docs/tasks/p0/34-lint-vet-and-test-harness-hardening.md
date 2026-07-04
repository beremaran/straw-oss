# 34 - Lint/Vet Hardening, Test-Harness Bootstrap, and Repo Hygiene

Status: done

## Objective

Close the engineering-hygiene gaps found by the 2026-07-04 review: enable `go vet` in `make check`, fix the
`copylocks` violation it surfaces, make the Postgres-backed tests self-bootstrap their schema, and stop leaving build
artifacts in the tree.

## Context (gaps being closed)

- `.golangci.yml` sets `default: none` and never enables `govet`, so `make check` misses `go vet` findings. `go vet
  ./...` reports `internal/egress/registration.go:44: assignment copies lock value to p` — a `copylocks` violation
  from copying a `strawpb.RegisterRequest_PoolRef` (which embeds a mutex) by value. Harmless at runtime today (the
  value is freshly zeroed and the real egress binary never populates `AllowedPools`), but a real latent footgun.
- `newIdentityTestPool` in `internal/control/postgres_store_test.go` runs `TRUNCATE` before it ever applies
  migrations, so pointing `STRAW_TEST_POSTGRES_DSN` at a fresh database yields `relation "tenants" does not exist`
  instead of passing or skipping. The "run `go test ./...` to verify" instruction in tasks 18/19/25 silently assumes a
  pre-migrated database.
- A ~30 MB `control.test` build artifact sits in the repo root (untracked and gitignored, but stray clutter).

## Required Planning Docs

- `docs/planning/30-testing-matrix.md` (the suite this protects)
- `docs/planning/31-implementation-order.md` (process/quality gate)

This is a cleanup/integration task per the board rules; it changes no product behavior.

## Prerequisites

- Tasks 17, 18, 19 completed (the code and tests being hardened).

## Out of Scope

- Do not add new product features or endpoints.
- Do not broaden the linter set beyond `govet` and the fixes required to pass it.

## Expected Files

- Modify: `.golangci.yml` (add `govet` to the enabled linters).
- Modify: `internal/egress/registration.go` (make `Capabilities.AllowedPools` a `[]*strawpb.RegisterRequest_PoolRef`,
  or otherwise avoid copying the proto value, and update callers/tests).
- Modify: `internal/control/postgres_store_test.go` (apply migrations before the first `TRUNCATE` so the harness
  self-bootstraps a fresh `STRAW_TEST_POSTGRES_DSN` database; keep the skip when the DSN is unset).
- Remove: the stray `control.test` artifact (confirm `*.test` stays gitignored).
- Test: existing suites remain green; `make check` now runs `go vet`.

## Steps

- [x] Read the board rules and the affected files.
- [x] Add `govet` to `.golangci.yml` and confirm it runs under `make check`.
- [x] Fix the `copylocks` violation so `go vet ./...` is clean; update any callers/tests of the changed
      `Capabilities` field. (`Capabilities.AllowedPools` changed from `[]strawpb.RegisterRequest_PoolRef` to
      `[]*strawpb.RegisterRequest_PoolRef`; updated `registration.go`, `registration_test.go`, `worker_nats_test.go`.)
- [x] Change the Postgres test harness to apply migrations before the first `TRUNCATE` (self-bootstrap), so a fresh
      database passes with only `STRAW_TEST_POSTGRES_DSN` set; preserve the unset-DSN skip.
- [x] Delete the stray `control.test` artifact and confirm `.gitignore` still covers `*.test`. (`control.test` was
      untracked and already matched `.gitignore:18 *.test`; no `.gitignore` change needed.)
- [x] Run `go vet ./...` and confirm it is clean.
- [x] Run the Postgres suite against a fresh empty database given only `STRAW_TEST_POSTGRES_DSN`. (Verified against a
      throwaway `postgres:16-alpine` with no pre-applied schema.)
- [x] Run `make check`.
- [x] Write a handoff note.

Note: `golangci-lint`'s incremental cache produced phantom `staticcheck` SA5011 findings the first time the linter
set changed; a `golangci-lint cache clean` cleared them and the fresh run reported `0 issues`. Run `cache clean` after
any `.golangci.yml` change.

## Tests

- `go vet ./...` (clean)
- `go test ./...` and, with a fresh DB, `STRAW_TEST_POSTGRES_DSN=... go test ./internal/control`
- `make check`

## Acceptance Criteria

- `go vet ./...` reports no findings; `make check` runs and passes `go vet`.
- The Postgres-backed tests pass against a fresh empty database given only `STRAW_TEST_POSTGRES_DSN`, and still skip
  cleanly when it is unset.
- No `*.test` or other build artifacts are tracked or left in the tree.

## Handoff Notes

- Note the `copylocks` fix and any signature change to `Capabilities`.
- Note the test-harness bootstrap change and the exact fresh-DB verification command.

## Stop Conditions

- Stop if enabling `govet` surfaces findings that cannot be fixed within this task's cleanup scope without changing
  product behavior; report them for a follow-up rather than suppressing.
