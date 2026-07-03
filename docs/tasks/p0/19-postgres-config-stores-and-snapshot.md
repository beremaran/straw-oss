# 19 - Postgres Config Stores and Snapshot Assembly

Status: done

## Objective

Implement Postgres-backed durable config stores and tenant snapshot assembly so Control can load immutable request-time
snapshots from Postgres instead of in-memory config fakes.

## Required Planning Docs

- `docs/planning/21-state-and-storage.md`
- `docs/planning/25-dynamic-configuration.md`
- `docs/planning/10-routing-model.md`
- `docs/planning/20-rate-limits-and-quotas.md`
- `docs/planning/24-static-configuration.md`

## Prerequisites

- Task 05 completed.
- Task 09 completed.
- Task 18 completed.

## Out of Scope

- Do not implement config-management HTTP handlers (task 20).
- Do not implement Redis pub/sub invalidation or periodic version polling (task 21).
- Do not wire request dispatch to consume the full snapshot (task 24).
- Do not add P1 rollback.

## Expected Files

- Create or modify: `internal/control` Postgres-backed config stores.
- Modify: `internal/config/snapshot.go`
- Modify: `cmd/control/main.go`
- Test: focused Postgres config-store and snapshot tests.

## Steps

- [x] Read all required planning docs.
- [x] Extend `internal/config.TenantSnapshot` to carry P0 routing rules, executor pools, deny rules, injection
      policies, fingerprint profiles, rate-limit configs, quota configs, worker admin state, tenant worker overrides,
      and the captured `tenant_config_version`.
- [x] Implement Postgres stores for routing rules and executor pools, including priority ordering, soft deletion,
      tenant scoping, and idempotent stable IDs where Section 26 allows them.
- [x] Implement Postgres stores for deny rules, injection policies, and seeded built-in fingerprint profiles; do not
      add a P0 fingerprint-profile write path.
- [x] Implement Postgres stores for rate-limit configs and quota configs, including tenant `rate_limit_ceiling`
      validation and platform-managed quota writes.
- [x] Implement durable worker admin disable/drain state and tenant worker override stores backed by Postgres.
      (Durable disable only — `worker_admin_state`/`tenant_worker_admin_state` are disable-only per Section 21;
      drain remains runtime-only, so drain is intentionally not persisted.)
- [x] Implement a Postgres `SnapshotStore` that assembles a full immutable tenant snapshot keyed by
      `(tenant_id, tenant_config_version)`.
- [x] Ensure config writes increment `tenant_config_versions` and append `config_audit_source` records in the same
      transaction, with secret fields redacted.
- [x] Wire `cmd/control/main.go` to use the Postgres `SnapshotStore` instead of `NewInMemorySnapshotStore`.
- [x] Add tests for transactional version increments, snapshot immutability, deleted resources excluded from routing,
      audit redaction, seeded fingerprint profiles, and quota/rate-limit config loading.
- [x] Run focused config-store and snapshot tests.
- [x] Run `make check`.
- [x] Write a handoff note.

## Tests

- Focused Postgres config-store tests.
- Focused snapshot assembly tests.
- `go test ./internal/config ./internal/control ./cmd/...`
- `make check`

## Acceptance Criteria

- Runtime Control snapshots are assembled from Postgres-backed stores, not in-memory config data.
- Every successful mutable config write increments the tenant config version and records an audit source row in the
  same transaction.
- P0 fingerprint profiles are seeded/read-only through the public config surface.
- Tenant snapshots contain the policy data needed by routing, admission, destination-policy resolution, and dispatch
  tasks.

## Handoff Notes

- Document the snapshot fields added and the config-version transaction boundary.
- Note that HTTP APIs are deferred to `docs/tasks/p0/20-config-admin-apis.md` and Redis invalidation is deferred to
  `docs/tasks/p0/21-redis-wiring-and-config-invalidation.md`.

## Stop Conditions

- Stop before adding P1 rollback.
- Stop if a config resource in Section 21 would have no store or owning task.
- Stop if a deferral would have no owning task file.
