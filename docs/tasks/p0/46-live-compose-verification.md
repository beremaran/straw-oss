# 46 - Live Compose Verification of Tasks 42-45 Surfaces

Status: not started

## Objective

Drive the surfaces built by tasks 42 (executor pool capability fields), 43 (deny rule taxonomy), 44 (config audit
event enrichment), and 45 (Redis-backed worker runtime state) end-to-end through the live docker-compose stack,
observing real effects in Postgres, Redis, ClickHouse, and assignment behavior — closing the "live compose
verification: skipped" note each of those four handoffs carried.

## Context (gap being closed)

The 2026-07-06 handoff sweep found four consecutive done tasks whose handoffs skipped live verification and said
so honestly:

- `docs/agents/handoffs/42-executor-pool-capability-fields.md` — "a live pool-restriction CRUD + assignment check
  has not been driven end-to-end ... it should be done as a deliberate, user-approved step".
- `docs/agents/handoffs/43-deny-rule-taxonomy-alignment.md` — "Live compose verification: skipped".
- `docs/agents/handoffs/44-config-audit-event-enrichment.md` — "Live compose verification: skipped".
- `docs/agents/handoffs/45-redis-backed-worker-runtime-state.md` — "Live compose verification: skipped because
  the full ... compose stack was not running".

The user approved this task on 2026-07-06. All four surfaces have unit and Postgres-integration coverage; what is
unverified is the built binaries exercising them against the real compose backends. This is a verification-only
task: it changes no implementation code. If a live run exposes a real defect, fixing it is a stop condition (ask,
or task it), not silent scope growth.

## Required Planning Docs

- `docs/planning/26-config-management-api-surface.md` (executor pool fields; deny-rule taxonomy; config audit list)
- `docs/planning/21-state-and-storage.md` (Redis worker session/heartbeat/load keys)
- `docs/planning/22-canonical-clickhouse-schema.md` (`config_audit_events` columns)

## Prerequisites

- Tasks 42, 43, 44, 45 done (they built the surfaces under verification).

## Out of Scope

- No implementation-code changes; docs and (if needed) compose/README fixes only.
- No load or performance claims (owned by `docs/tasks/p1/18-load-and-backpressure-testing.md`).
- No new automated test harness; this is a documented manual/live verification run. Automating it, if wanted,
  needs its own task.

## Expected Files

- Modify: `docs/agents/handoffs/42-executor-pool-capability-fields.md`, `43-...`, `44-...`, `45-...` (replace each
  "live compose verification: skipped" note with the observed result and a pointer here).
- Add: handoff note for this task recording every command run and observation.

## Steps

- [ ] Read all required planning docs.
- [ ] Bring up the full compose stack (`deploy/docker`, see its README) and confirm Control `/readyz` is ready and
      the egress worker registers.
- [ ] Task 42 surface: create an executor pool with `allowed_ip_types`/`allowed_countries`/`allowed_regions` via
      the config API; verify the row in Postgres; verify a non-matching worker is not assignable and a matching
      one is (live request through the stack).
- [ ] Task 43 surface: create deny rules across the full Section 26 taxonomy via the API; verify Postgres rows and
      that a live request to a denied destination is rejected with the mapped error code.
- [ ] Task 44 surface: perform a config mutation and verify the ClickHouse `config_audit_events` row carries
      `field_path`, `old_value_json`, `new_value_json`, and `config_version`.
- [ ] Task 45 surface: verify worker session/heartbeat/load state appears under the canonical Redis keys, expires
      on TTL after stopping the worker, and survives a Control restart (re-registration not required for state).
- [ ] Update the four handoffs' skipped-verification notes with the observed results.
- [ ] Run `make check` (no code should have changed).
- [ ] Write a handoff note.

## Tests

- Live compose runs per the Steps (documented commands + observed output in the handoff).
- `make check`

## Acceptance Criteria

- Each of the four surfaces has a recorded live observation in this task's handoff: the exact request(s) sent and
  the verified effect (Postgres row / Redis key / ClickHouse row / assignment or denial behavior).
- None of the four handoffs still says live verification was skipped; each names this task and the result.
- `git diff` shows no changes under `internal/`, `cmd/`, or `api/` from this task.

## Handoff Notes

- Record the compose stack revision (git SHA), every command run, and each observation with enough detail to
  replay.
- Record any defect found live and the stop-condition outcome (user decision or new owning task).

## Stop Conditions

- Stop if a live run exposes a real defect in any surface — fixing implementation code is outside this task; ask
  the user or create the owning task before continuing.
- Stop if the compose stack cannot reach ready state for reasons outside these four surfaces.
- Stop if a deferral would have no owning task file.
