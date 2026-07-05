# Handoff

Task: `docs/tasks/p0/20-config-admin-apis.md`

## Changed

- `internal/control/config_admin_handlers.go` — New handlers implementing the P0 config
  admin API surface under `/api/v1/config`:
  - Routing rules (`GET/POST/PUT/DELETE .../routing-rules[/{id}]`): `tenant_admin`/`operator`
    write, `tenant_admin`/`operator`/`viewer` read. Client-supplied stable `id` (docs/planning/26).
  - Deny rules (`GET/POST/PUT/DELETE .../deny-rules[/{id}]`): `tenant_admin`-only write,
    `tenant_admin`/`operator`/`viewer` read. Server-generated ID. `normalizeDenyRule` validates
    and normalizes the value per type (lowercase + trailing-dot-trim for `host`/`cname`,
    `netip.ParsePrefix`/`ParseAddr` for `cidr`/`ip`) per docs/planning/27's normalization intent.
  - Injection policies (`GET/POST/PUT/DELETE .../injection-policies[/{id}]`): `tenant_admin`/
    `operator` write (operator restricted to non-sensitive operations), same roles plus `viewer`
    read. Server-generated ID. `validateInjectionOperations` enforces docs/planning/27's header
    safety rules: `Host`/`Content-Length`/`Transfer-Encoding`/`Connection`/`Proxy-Authorization`/
    `X-Straw-*` always denied; `Authorization`/`Cookie` set-or-append require `tenant_admin`;
    op count bounded by `max_operations` (32).
  - Fingerprint profiles (`GET .../fingerprint-profiles`): read-only, `tenant_admin`/`operator`/
    `viewer`. No write handler exists — P0 has none (docs/planning/26).
  - All four resources are tenant-scoped: `authorizeConfig` (wrapping the existing
    `requireTenantRole`) rejects platform-scoped keys, and every store call is scoped by
    `identity.TenantID`, so tenant isolation is automatic (no other-tenant existence leak).
  - Every write increments the tenant config version and calls `ConfigCache.PublishInvalidation`
    (see below) after commit, matching the API-key/worker-credential invalidation pattern already
    in `admin_handlers.go`.
  - Version conflicts return HTTP 409 with `details.current_config_version` (looked up via a
    follow-up `Get` after the store reports `ErrConfigResourceVersionConflict`).
- `internal/control/postgres_config_store.go` — `UpsertRoutingRule`/`UpsertDenyRule`/
  `UpsertInjectionPolicy` now take `expectedVersion uint64`, perform optimistic concurrency
  against each row's own `config_version` (0 for missing/soft-deleted, matching what a GET/List
  would show), and return the saved resource record plus the bumped tenant version (for
  invalidation) instead of just the tenant version. New `ErrConfigResourceVersionConflict`. New
  `RoutingRuleRecord`/`DenyRuleRecord`/`InjectionPolicyRecord`/`FingerprintProfileRecord` carry
  the per-resource `CreatedAt`/`ConfigVersion` the admin API needs on top of the config-layer
  snapshot types.
- `internal/control/postgres_config_list_store.go` — New: `GetRoutingRule`/`ListRoutingRules`,
  `GetDenyRule`/`ListDenyRules`, `GetInjectionPolicy`/`ListInjectionPolicies`,
  `ListFingerprintProfiles`, all on `PostgresConfigStore`. Pagination defaults to 50/max 200,
  sorted `created_at DESC, id ASC` (docs/planning/26 "Shared Config API Contract").
- `internal/control/config_resource_store.go` — New `RoutingRuleStore`/`DenyRuleStore`/
  `InjectionPolicyStore`/`FingerprintProfileStore` interfaces (matched structurally by
  `PostgresConfigStore`) plus `InMemory*` test doubles used only by
  `config_admin_handlers_test.go` — never wired into the binary.
- `internal/control/config_cache.go` — New `ConfigCache.PublishInvalidation(ctx, tenantID,
  version)`: advances the locally cached version and calls the injected `InvalidationPublisher`
  (nil-safe). The routing/deny/injection writes bump the tenant version transactionally inside
  `PostgresConfigStore` (bypassing `ConfigCache.Save`), so this is the seam that still notifies
  the publisher after those writes — satisfying "successful writes publish invalidation through
  the interface, even if the concrete Redis publisher lands in task 21."
- `internal/control/admin_handlers.go` — `AdminHandlers` gained `RoutingRules`, `DenyRules`,
  `InjectionPolicies`, `FingerprintProfiles` fields (all optional/nil-checked, matching the
  existing `ConfigWrites`/`WorkerAdmin` pattern). Replaced 9 literal `"reason"` detail-key
  occurrences with the new `errorDetailReasonKey` const (goconst).
- `internal/control/errors.go` — New `errorDetailReasonKey = "reason"` const.
- `cmd/control/main.go` — `buildAdminHandlers` wires `PostgresConfigStore` as all four new
  store fields; `serveAdminRoutes` registers the 13 new routes under `/api/v1/config/*`.
- Tests: `internal/control/config_admin_handlers_test.go` (new — role access, tenant isolation,
  pagination defaults, conflict-with-current-version, soft delete + re-delete 404, deny-rule
  CIDR validation + normalization, injection safety rules incl. operator-vs-tenant_admin sensitive
  header restriction, fingerprint read-only, invalidation-publisher call), plus updated call
  sites in `postgres_config_store_test.go` and `admin_handlers_test.go` for the new store
  signatures/fields.

## Verification

```sh
make check
```

Result: **pass** — `gofmt`/`gofumpt` clean, `go test ./...` pass (Postgres/Redis integration
tests skip without their DSN env vars), `golangci-lint run --max-issues-per-linter 0
--max-same-issues 0` reports **0 issues**.

Additionally verified against a real Postgres (no docker-compose file exists yet — that's task
25's scope — so a one-off `postgres:16` container was used, migrations applied by hand, then
removed after):

- All pre-existing Postgres integration tests still pass unchanged in behavior.
- An ad hoc smoke test (written, run, then deleted — not part of the committed diff) exercised
  `UpsertRoutingRule`/`GetRoutingRule`/`ListRoutingRules`/`DeleteRoutingRule` end to end including
  the stale-version-conflict path, plus `UpsertDenyRule`/`ListDenyRules` with real CIDR
  normalization and `UpsertInjectionPolicy`/`GetInjectionPolicy`/`ListFingerprintProfiles` — all
  passed against live Postgres, confirming the new SQL (`RETURNING created_at`, version-aware
  `ON CONFLICT`) is correct, not just type-checked.

## Reviewer Start Points

- `internal/control/config_admin_handlers.go` — HTTP handlers, RBAC, validation/normalization.
- `internal/control/postgres_config_store.go` — CAS-aware `Upsert*` (`checkResourceVersion`).
- `internal/control/postgres_config_list_store.go` — List/Get read paths.
- `internal/control/config_cache.go` — `PublishInvalidation`.
- `cmd/control/main.go` — binary wiring and route registration.

## Remaining Work

- Redis-backed invalidation publisher implementation is deferred to
  `docs/tasks/p0/21-redis-wiring-and-config-invalidation.md` — `ConfigCache` is constructed with
  a `nil` publisher in `cmd/control/main.go` (unchanged from task 19); `PublishInvalidation` is
  nil-safe and will start actually publishing once that task wires a concrete publisher.
- Request dispatch consuming these resources at request time is
  `docs/tasks/p0/24-control-request-dispatch-pipeline.md`'s scope, not this task's.
- **Schema gap discovered, not owned by any existing task**: docs/planning/26's deny-rule schema
  documents `type: cidr | host | host_suffix | cname_suffix | metadata_ip | private_range` and
  `action: deny | allow_override`, but the `deny_rules` table (migrations/postgres/0001_init.sql,
  from task 04) only has CHECK constraints for `rule_type IN ('host','cidr','cname','ip')` and
  `action IN ('deny','allow')`, and has no `reason` column at all. This handler implements the
  4 DB-backed types/2 actions and does not persist `reason`. Extending the schema to the full
  Section 26 taxonomy (plus a `reason` column) is real follow-on work with no owning task file
  today — flagging per the Stop Conditions rather than silently narrowing or widening the
  contract. **[Update 2026-07-05: now owned by
  `docs/tasks/p0/43-deny-rule-taxonomy-alignment.md`.]**
- No InMemory/fake/stub is used by the running binary — grepped the diff for
  `InMemory|stub|fake|synthetic|TODO`; the one hit is a code comment documenting that the
  InMemory doubles in `config_resource_store.go` are handler-test-only.

## Blockers

- None.
