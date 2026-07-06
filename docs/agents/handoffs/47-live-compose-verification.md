# Handoff

Task: `docs/tasks/p0/46-live-compose-verification.md`

## Changed

- `docs/agents/handoffs/42-executor-pool-capability-fields.md` — replaced the skipped live-verification note with
  the observed restricted-pool CRUD, Postgres, no-match, and matching-worker request results.
- `docs/agents/handoffs/43-deny-rule-taxonomy-alignment.md` — replaced the skipped live-verification note with the
  observed six-type API/Postgres check and live `destination_denied` request.
- `docs/agents/handoffs/44-config-audit-event-enrichment.md` — replaced the skipped live-verification note with the
  observed ClickHouse `config_audit_events` rows.
- `docs/agents/handoffs/45-redis-backed-worker-runtime-state.md` — replaced the skipped live-verification note with
  the observed Redis keys, Control restart survival, and TTL expiry.
- `docs/tasks/p0/46-live-compose-verification.md` and `docs/tasks/p0.md` — marked the verification task done.

## Acceptance Criteria Verdicts

Independent verifier (fresh agent, given the task file and diff) confirmed the documentation criteria pass.

| Criterion | Verdict | Implementation (file:line) | Proving test |
|-----------|---------|----------------------------|--------------|
| Each of the four surfaces has a recorded live observation with exact requests/effects | VERIFIED | `docs/agents/handoffs/47-live-compose-verification.md` | Live compose run below |
| None of the four prior handoffs still says live verification was skipped | VERIFIED | `docs/agents/handoffs/42-...`, `43-...`, `44-...`, `45-...` | `rg "Live compose verification: skipped" docs/agents/handoffs/{42,43,44,45}-*.md` returned no matches |
| No implementation/API files changed | VERIFIED | `git diff --name-only` docs-only | `git diff --name-only -- internal cmd api` returned no files |

## Planning-Doc Coverage

| Planning item | Status | Evidence / owning task |
|---------------|--------|------------------------|
| `docs/planning/26` executor pool `allowed_ip_types`, `allowed_countries`, `allowed_regions` | implemented | API created `pool_task47_us`; Postgres row stored `["datacenter"]`, `["US"]`, `["us-west-1"]` |
| `docs/planning/26` deny-rule taxonomy (`cidr`, `host`, `host_suffix`, `cname_suffix`, `metadata_ip`, `private_range`) | implemented | API created all six; Postgres normalized rows; live `blocked.task47.test` request returned `destination_denied` |
| `docs/planning/26` config audit change values | implemented | ClickHouse rows carried `config_version`, `old_value_json`, and `new_value_json`; whole-resource P0 writes carried empty `field_path`, matching task 44's documented P0 floor |
| `docs/planning/21` Redis worker session/heartbeat/load/cooldown runtime state with TTL | implemented | Redis `straw:worker-runtime:<worker_id>` JSON keys included session, heartbeat/load, pool/capability state and TTLs; keys expired after worker stop |
| `docs/planning/22` ClickHouse `config_audit_events` columns | implemented | Queried live `straw.config_audit_events` rows for routing/executor-pool mutations |

## Verification

Compose revision:

```sh
git rev-parse HEAD
# 75eccc5287275371b97fa4a7a41ba2d8012b43f0
```

Live compose commands/results:

```sh
STRAW_BOOTSTRAP_SYSTEM_ADMIN_KEY=sk_live_task47_admin docker compose -p straw47 -f docker-compose.yml -f /tmp/straw47-compose-17993.yml up -d --build
curl -fsS http://localhost:19090/readyz
# ready
curl -fsS -H 'Authorization: Bearer sk_live_task47_admin' http://localhost:18080/api/v1/admin/workers
# egress-local-1 runtime_state=ready
```

Task 42:

```sh
curl -H "Authorization: Bearer <tenant-admin>" -d '{"id":"pool_task47_us","allowed_ip_types":["datacenter"],"allowed_countries":["US"],"allowed_regions":["us-west-1"],"tags":["task47"],"allow_degraded_workers":false}' \
  http://localhost:18080/api/v1/config/executor-pools
```

Postgres stored:

```text
pool_task47_us | egress | ["datacenter"] | ["US"] | ["us-west-1"]
```

With the seeded fallback route disabled and only the non-matching compose worker available:

```text
POST /api/v1/requests https://example.com/ -> HTTP 503 route_unavailable
```

After provisioning a second live Go worker credential with matching `task47`/`US`/`us-west-1`/`datacenter`/`rest`
capabilities and registering that worker over NATS:

```text
POST /api/v1/requests https://example.com/ -> HTTP 200, upstream_status=200
```

Task 43:

```text
API-created deny rules:
cidr          deny           203.0.113.0/24       task47 cidr
cname_suffix  deny           internal.task47.test task47 cname suffix
host          deny           blocked.task47.test  task47 host
host_suffix   deny           denied.task47.test   task47 host suffix
metadata_ip   deny           169.254.169.254/32   task47 metadata
private_range allow_override 10.0.0.0/8           task47 private override

POST /api/v1/requests https://blocked.task47.test/ -> HTTP 403 destination_denied
```

Task 44:

```text
ClickHouse straw.config_audit_events:
resource_id=pool_task47_us action=upsert config_version=5 field_path="" old_value_json=null new_value_json={...AllowedIPTypes:["datacenter"]...}
resource_id=route_task47_us action=upsert config_version=14 field_path="" old_value_json={...IngressType:"rest"...} new_value_json={...IngressType:""...}
```

The empty `field_path` is the whole-resource P0 floor explicitly allowed by
`docs/tasks/p0/44-config-audit-event-enrichment.md`.

Task 45:

```text
Redis keys:
straw:worker-runtime:egress-local-1 ttl=59
straw:worker-runtime:egress-task47-matching ttl=45
```

The JSON values contained current session id, credential id, tenant scope, pool refs, heartbeat/load fields, and for
the matching worker the `tags`, `countries`, `regions`, `ip_types`, and `ingress_modes` capability claims.

After stopping workers and restarting Control:

```text
GET /api/v1/admin/workers -> egress-local-1 runtime_state=draining; egress-task47-matching runtime_state=unavailable
Redis keys still present with TTLs
```

TTL expiry:

```text
straw:worker-runtime:egress-task47-matching expired first; straw:worker-runtime:egress-local-1 expired after 44s
```

Final verification:

```sh
make check
```

Result:

- Postgres-backed tests: not exercised; this task changed docs only and used the live compose `straw` database only
  for manual verification, not the test harness.
- Live compose verification: completed as above.

## Reviewer Start Points

- `docs/agents/handoffs/47-live-compose-verification.md`
- `docs/agents/handoffs/42-executor-pool-capability-fields.md`
- `docs/agents/handoffs/43-deny-rule-taxonomy-alignment.md`
- `docs/agents/handoffs/44-config-audit-event-enrichment.md`
- `docs/agents/handoffs/45-redis-backed-worker-runtime-state.md`

## Remaining Work

- None. No implementation behavior was changed; the skipped live-verification gap is closed.

## Blockers

- None.
