# 42 - Executor Pool Capability Fields

Status: done

## Objective

Add the `docs/planning/26` P0 executor-pool capability fields — `allowed_ip_types`, `allowed_countries`,
`allowed_regions` — to the pool schema, config API, snapshot, and routing eligibility, so pools can constrain which
workers serve them the way the planning doc specifies.

## Context (gap being closed)

The 2026-07-05 P0 verification audit found these three fields absent from the entire codebase (zero hits across Go,
SQL, and proto) even though the `docs/planning/26` P0 Executor Pool schema names them. Task 30 built the
`/executor-pools` CRUD surface but its Out of Scope note ("do not add P1 pool-policy fields") was misread as covering
these P0 fields; its handoff flagged the gap with "no task file currently owns closing this gap." This task is that
owner. Workers already report `country`, `region`, and `ip_type` capabilities (worker credential
`allowed_capabilities`, registration), so the enforcement side has data to match against.

## Required Planning Docs

- `docs/planning/26-config-management-api-surface.md` (P0 Executor Pool schema, lines ~254-278)
- `docs/planning/10-routing-model.md` (pool eligibility and worker matching)
- `docs/planning/21-state-and-storage.md`

## Prerequisites

- Task 30 completed (pool CRUD, snapshot sourcing, pool-policy provider).

## Out of Scope

- Do not add fields beyond the three named ones (`tags`, `allow_degraded_workers`, `executor_type`, `enabled` already
  exist).
- Do not build geo/IP-type detection on the worker; use the capabilities workers already report at registration.
- Do not change the P1 rollback or fingerprint surfaces.

## Expected Files

- Add: `migrations/postgres/000X_executor_pool_capabilities.sql` (three array columns, empty = unrestricted).
- Modify: `internal/control/postgres_config_store.go` / `postgres_config_list_store.go` (persist + read the fields).
- Modify: `internal/control/config_admin_handlers.go` (accept/validate/return the fields on pool CRUD).
- Modify: snapshot assembly (`readExecutorPools`) and the routing/worker-eligibility check that matches worker
  capabilities against the pool.
- Test: pool CRUD round-trip with the fields; a routing test where a worker outside `allowed_countries` (or
  `allowed_ip_types` / `allowed_regions`) is not eligible for the pool and a matching worker is.

## Steps

- [x] Read all required planning docs.
- [x] Add the migration (idempotent; existing rows get empty arrays = unrestricted, so behavior is unchanged until a
      pool opts in).
- [x] Persist and list the fields in the Postgres pool store and snapshot read.
- [x] Accept and validate the fields in pool create/update handlers; return them in reads. Reject unknown IP types
      (valid set per `docs/planning/26`: e.g. `datacenter`, `residential`, `mobile` — confirm against the doc).
- [x] Enforce eligibility: a worker whose reported capability is outside a non-empty pool restriction is not assignable
      for that pool.
- [x] Add the tests listed in Expected Files.
- [x] Run focused tests, then `make check`.
- [x] Write a handoff note.

## Tests

- `go test ./internal/control ./cmd/...`
- `make check`

## Acceptance Criteria

- Pool CRUD accepts, persists, and returns `allowed_ip_types`, `allowed_countries`, `allowed_regions`.
- Empty restrictions preserve current behavior (verified by existing tests staying green).
- A non-empty restriction excludes non-matching workers from assignment for that pool, proven by a routing test.
- The `docs/planning/26` P0 Executor Pool schema has no remaining unimplemented field.

## Handoff Notes

- Document the migration and the eligibility-matching semantics (empty = unrestricted).
- If any capability value taxonomy is ambiguous in `docs/planning/26`, record the interpretation chosen.

## Stop Conditions

- Stop if worker capability data proves insufficient to enforce a field (report which one and why).
- Stop if a deferral would have no owning task file.
