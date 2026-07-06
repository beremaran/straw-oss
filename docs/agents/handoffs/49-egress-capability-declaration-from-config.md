# Handoff

Task: `docs/tasks/p0/49-egress-capability-declaration-from-config.md`

## Summary

The official Go Egress Worker now reads its declared capabilities from static config
(`egress.capabilities.{tags,countries,regions,ip_types,supported_ingress_modes,max_concurrency}`, the nested object
shape from `docs/planning/24-static-configuration.md`) and sends them in its NATS `RegisterRequest`. The task-42
pool-restriction exclusion branch was driven live end-to-end through the compose stack for the first time: a pool
restricted to `allowed_ip_types: ["residential"]` excluded the worker declaring `["datacenter"]`
(`route_unavailable`), and loosening to `["datacenter"]` restored 200.

## Changed

- `internal/config/config.go` — added `EgressCapabilities` struct and `EgressConfig.Capabilities`
  (`capabilities` JSON key, strict-decode compatible); `supported_ingress_modes` defaults to `["rest"]` in
  `validate()` per planning/24; all lists default empty.
- `cmd/egress/main.go` — new `buildCapabilities(cfg)` maps config onto `egress.Capabilities`
  (tags/countries/regions/ip_types/supported_ingress_modes; `max_concurrency` falls back to the existing
  `defaultConcurrency` when unset); `runWorker` uses it, replacing the hardcoded-empty literal.
- `cmd/egress/main_test.go` — new: `TestBuildCapabilitiesFromConfig`, `TestBuildCapabilitiesDefaultsMaxConcurrency`.
- `internal/config/config_test.go` — `TestLoadEgressParsesCapabilities` (round-trip),
  `TestLoadEgressCapabilitiesDefaults` (empty defaults, `["rest"]` ingress-mode default).
- `internal/egress/registration_test.go` — `TestBuildRegisterRequestCarriesCapabilities` (claims reach the proto
  payload); replaced repeated string literals with `testIPType` for goconst.
- `deploy/docker/egress.json` — dev worker declares `capabilities.ip_types: ["datacenter"]`.
- `deploy/docker/README.md` — documents the dev capability set and how to observe pool exclusion.
- `docs/tasks/p0/46-live-compose-verification.md`, `docs/agents/handoffs/42-executor-pool-capability-fields.md`,
  `docs/agents/handoffs/46-live-compose-verification.md` — surface-42 "exclusion not drivable" notes marked closed
  by this task.
- `docs/tasks/p1/22-egress-credential-config-schema-reconciliation.md` — cross-note that the nested
  `egress.capabilities.*` shape added here is the planning-doc shape and outside task 22's flat-key scope.

## Acceptance Criteria Verdicts

From the independent verifier (fresh agent, task file + diff only):

| Criterion | Verdict | Implementation (file:line) | Proving test |
|-----------|---------|----------------------------|--------------|
| Config loads countries/regions/ip_types (+tags), empty defaults | VERIFIED | `internal/config/config.go:173,180-187,494-496` | `TestLoadEgressParsesCapabilities`, `TestLoadEgressCapabilitiesDefaults` |
| Built binary sends configured capabilities in RegisterRequest | VERIFIED | `cmd/egress/main.go:200-236` → `internal/egress/runtime.go:28` → `registration.go:70-75` | `TestBuildCapabilitiesFromConfig` + `TestBuildRegisterRequestCarriesCapabilities` |
| Non-subset worker not assignable for restricted pool | VERIFIED (unit + live) | `internal/control/worker_registry.go` `poolAllows`/`subset` (task 42, unchanged) | `TestDispatcherRoutePoolCapabilityRestriction`; live run below |
| main.go populates Countries/Regions/IPTypes from config, not hardcoded | VERIFIED | `cmd/egress/main.go:216-218` | grep + `TestBuildCapabilitiesFromConfig` |

The verifier's only defects were the then-missing handoff file (this file) and the pending status flips — both
resolved in this run; no code defects.

## Planning-Doc Coverage

| Planning item | Status | Evidence / owning task |
|---------------|--------|------------------------|
| `egress.capabilities.tags` (`[]`) | implemented | `config.go` `EgressCapabilities.Tags` |
| `egress.capabilities.countries` (`[]`) | implemented | `EgressCapabilities.Countries` |
| `egress.capabilities.regions` (`[]`) | implemented | `EgressCapabilities.Regions` |
| `egress.capabilities.ip_types` (`[]`) | implemented | `EgressCapabilities.IPTypes` |
| `egress.capabilities.supported_ingress_modes` (`["rest"]`) | implemented (was never sent before) | `EgressCapabilities.SupportedIngressModes` + default in `validate()` |
| `egress.capabilities.max_concurrency` | implemented (was hardcoded `defaultConcurrency=4`) | `EgressCapabilities.MaxConcurrency`, fallback in `buildCapabilities` |
| `egress.capabilities.pool_ids` | already existed | flat `egress.allowed_pools` (`config.go` `AllowedPools`, `cmd/egress/main.go` pool refs); richer (tenant,pool) pairs |
| planning/26 pool `allowed_ip_types`/`allowed_countries`/`allowed_regions` semantics | already existed | task 42 (restriction side), unchanged per Out of Scope |

Config key shape: nested `egress.capabilities.*` object exactly as planning/24, because
`docs/tasks/p1/22-...` (flat-key reconciliation) is not started and explicitly excludes `egress.capabilities.*`;
cross-noted in that task file.

Already wired vs added: `max_concurrency` was previously *sent* but hardcoded (now config-sourced with the same
fallback); `supported_ingress_modes` was previously **not sent at all** (now sent, default `["rest"]`);
tags/countries/regions/ip_types added here.

## Verification

```sh
go test ./internal/config ./internal/egress ./cmd/egress   # ok
make check                                                  # fmt + go test ./... + golangci-lint, 0 issues
```

- Postgres-backed tests: not exercised — the diff touches no `postgres_*` files or `migrations/`.
- Live compose verification: **ran 2026-07-07** against the running stack (rebuilt `egress` with
  `docker compose up -d --build egress`; worker re-registered with declared `ip_types: ["datacenter"]`):
  1. Baseline `GET https://example.com/` via minted requester key → `status: 200`.
  2. `POST /api/v1/config/executor-pools` `{"id":"dev-pool","allowed_ip_types":["residential"]}` (tenant_admin) →
     persisted (`config_version:1`); same request → `{"code":"route_unavailable","message":"Rule matched but no
     eligible executor"}` — **the exclusion branch, live, for the first time**.
  3. `PUT /api/v1/config/executor-pools/dev-pool` `{"allowed_ip_types":["datacenter"],"expected_config_version":1}`
     → same request → `status: 200`.
  The `dev-pool` row was left at `allowed_ip_types: ["datacenter"]`, which admits the compose worker.

## Reviewer Start Points

- `cmd/egress/main.go` (`buildCapabilities`)
- `internal/config/config.go` (`EgressCapabilities`)

## Remaining Work

- None. Nothing in the task is faked, stubbed, or deferred; the previously-open surface-42 exclusion gap is closed
  and the earlier notes (task 42 handoff, task 47 task file + handoff) were updated in this run.

## Blockers

- None.
