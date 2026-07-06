# Handoff

Task: `docs/tasks/p0/46-live-compose-verification.md` (board task 47)

## Summary

Verification-only task. Drove the tasks 42-45 surfaces end-to-end through the full docker-compose stack (compose
SHA `eb0c32b2`, fresh volumes via `docker compose down -v` then
`STRAW_BOOTSTRAP_SYSTEM_ADMIN_KEY=dev-admin-key-0001 docker compose up -d --build`). No implementation code was
changed for the verification itself (`git diff` under `internal/`/`cmd/`/`api/` from this task's docs work is
empty). One unrelated, pre-existing lint failure that blocked `make check` was fixed in a separate commit (see
Blockers).

Bootstrap IDs used: tenant `22222222-2222-4222-8222-222222222222`, pool `dev-pool`, worker credential
`11111111-1111-4111-8111-111111111111`, worker `egress-local-1`.

## Changed

- `docs/tasks/p0/46-live-compose-verification.md` — status → done, steps checked with per-surface results.
- `docs/tasks/p0.md` — board row 47 → done; added rows 48/49/50; added a Notes paragraph recording the three
  gaps this verification surfaced and their new owning tasks.
- `docs/agents/handoffs/42|43|44|45-*.md` — replaced each "live compose verification: skipped" note with the
  observed result and a pointer to the owning follow-up task.
- Added: `docs/tasks/p0/48-deny-host-suffix-leading-dot-normalization.md`,
  `docs/tasks/p0/49-egress-capability-declaration-from-config.md`,
  `docs/tasks/p0/50-config-audit-field-path-population.md`.

## Live Observations (the deliverable)

### Baseline
`GET https://example.com/` via a minted requester key → `status: 200` (full REST → Control → NATS → egress →
upstream round trip works).

### Surface 42 — executor pool capability fields
- `POST /api/v1/config/executor-pools` `{"id":"dev-pool", allowed_ip_types:["datacenter","residential"],
  allowed_countries:["US","DE"], allowed_regions:["us-east-1"]}` (tenant_admin key) → persisted; Postgres
  `executor_pools` row showed `allowed_ip_types_jsonb=["datacenter","residential"]`,
  `allowed_countries_jsonb=["US","DE"]`, `allowed_regions_jsonb=["us-east-1"]`.
- Live request through the restricted pool → `status: 200` (dev worker claims empty capabilities, a subset of any
  restriction, correctly not false-excluded).
- Invalid `allowed_ip_types:["satellite"]` → HTTP 400 `invalid allowed_ip_types: [satellite]`.
- PUT update with `expected_config_version` round-tripped the restriction fields.
- **Gap:** the non-matching (exclusion) branch is not drivable live — stock egress (`cmd/egress/main.go:214`)
  declares no countries/regions/ip_types, so no live worker can be excluded. Owned by
  `docs/tasks/p0/49-egress-capability-declaration-from-config.md`. **Closed 2026-07-07 by task 49** (worker now
  declares capabilities from config; exclusion driven live — see
  `docs/agents/handoffs/49-egress-capability-declaration-from-config.md`).

### Surface 43 — deny rule taxonomy
- Created one rule of every type via the API (all HTTP 200): `cidr` `203.0.113.0/24`, `host` `denied.example.net`,
  `host_suffix` `.evil.example`, `cname_suffix` `.cdn.evil.example`, `metadata_ip` `169.254.169.254`,
  `private_range` `10.0.0.0/8`, plus a `host` `allow_override` and `reason` on each. Postgres `deny_rules` rows
  showed correct `rule_type`/`action`/`reason` and normalized columns (`normalized_cidr` for
  cidr/metadata_ip/private_range, `normalized_host` for host/host_suffix, `normalized_cname` for cname_suffix).
- `GET https://denied.example.net/` → `destination_denied` ("Deny rule matched"); `GET https://example.com/` → 200.
- `allow_override` precedence observed: `.evil.example` host_suffix deny was overridden by the allow_override rule.
- Clean no-leading-dot `host_suffix` (`blocked2.example`) → `GET https://sub.blocked2.example/` →
  `destination_denied`.
- **Defect:** `host_suffix` with a *leading-dot* value (`.blocked.example`) is accepted (200) but silently never
  matches — `normalizeHostname` (`config_admin_handlers.go:695`) trims only trailing dots and `hostMatchesSuffix`
  (`destination_policy.go:354`) prepends one → `..blocked.example`. `GET https://sub.blocked.example/` was NOT
  denied. Owned by `docs/tasks/p0/48-deny-host-suffix-leading-dot-normalization.md`. **Fixed 2026-07-07 by task
  48** (leading dots stripped at write time; live re-check confirmed `.blocked.example` now denies
  `sub.blocked.example` — see `docs/agents/handoffs/48-deny-host-suffix-leading-dot-normalization.md`).

### Surface 44 — config audit event enrichment
- Queried ClickHouse `straw.config_audit_events`. Create/upsert rows carried non-zero `config_version` and full
  `new_value_json`; API-key rows showed `"SecretHash":"[redacted]"` (redaction working).
- A PUT executor-pool update produced a row with `config_version:19`, `old_value_json` (prior whole-object) and
  `new_value_json` (new whole-object) both populated.
- **Gap:** `field_path` is empty on every row (create and update) — all `recordAudit` call sites pass `""`; the
  implementation records whole-object diffs, not the per-field `docs/planning/26` model. Owned by
  `docs/tasks/p0/50-config-audit-field-path-population.md`.

### Surface 45 — Redis-backed worker runtime state
- Redis key `straw:worker-runtime:egress-local-1` present with `session_id`, heartbeat (`heartbeat_unix`), and load
  (`active_requests`, `available_cap`, `max_concurrency`, `draining`); `TTL` = 60s.
- `docker compose restart control`: same `session_id` before/after; a live request immediately after restart →
  `status: 200` (worker routable from Redis state, no re-registration).
- `docker compose stop egress`: key TTL counted down 61→…→4→`-2` (gone) at ~65s of missed heartbeats; a subsequent
  live request failed safe → `route_unavailable` ("Rule matched but no eligible executor"). `docker compose start
  egress` re-created the key. **No defect.**

## Acceptance Criteria Verdicts

Self-verified (verification-only task; no implementation diff for a fresh agent to grade, and no subagent was
requested). Each verdict is backed by the reproducible commands/outputs above.

| Criterion | Verdict | Evidence |
|-----------|---------|----------|
| Each of the four surfaces has a recorded live observation (request + verified effect) | VERIFIED | Live Observations section above (Postgres rows, ClickHouse rows, Redis key/TTL, assignment & denial behavior) |
| None of the four handoffs still says live verification was skipped; each names this task and the result | VERIFIED | `docs/agents/handoffs/42|43|44|45-*.md` updated |
| `git diff` shows no changes under `internal/`, `cmd/`, or `api/` from this task | VERIFIED (with note) | This task's own diff is docs-only; the separate `internal/egress/runtime.go` lint-fix commit is an unrelated pre-existing failure, not part of this task's verification work (see Blockers) |

## Verification

```sh
make check
```

Result:
- `make check`: green (gofmt clean, golangci-lint `0 issues`, `go test ./...` all pass) after the separate lint fix.
- Postgres-backed tests: not separately exercised (this task changed no Postgres surfaces); Postgres state was
  observed live via `psql` against the compose `straw` database (read-only SELECTs and admin-API writes, never the
  test harness).
- Live compose verification: performed — this task *is* the live verification; commands and outputs recorded above.

## Reviewer Start Points

- `docs/tasks/p0.md` (Notes paragraph on tasks 48-50)
- `docs/tasks/p0/48-…`, `49-…`, `50-…` (the three new owning tasks)

## Remaining Work

- None owned by this task. The three defects/gaps found live are each owned by a new P0 task (48, 49, 50); the
  four source handoffs and the board Notes name those owners. No unowned deferral remains.

## Blockers

- `make check` was red on clean HEAD due to a pre-existing revive `time-naming` lint error
  (`registerBackoffMin`, introduced by commit `57440d72`, task 40), unrelated to this task. Per user direction
  (2026-07-07) it was fixed as a **separate commit** (`registerBackoffMin` → `registerBackoffFloor` in
  `internal/egress/runtime.go`) so this task's own commit stays docs-only. After that fix `make check` is green.
