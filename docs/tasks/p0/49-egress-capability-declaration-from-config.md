# 49 - Egress Worker Capability Declaration from Static Config

Status: done

## Objective

The official Go Egress Worker reads its declared capabilities (`countries`, `regions`, `ip_types`, and — where not
already sourced elsewhere — `tags`) from static config and sends them in its NATS `RegisterRequest`, so that a
worker can legitimately claim a non-empty capability set. After this task, an executor pool's
`allowed_ip_types` / `allowed_countries` / `allowed_regions` restriction (task 42) can actually exclude a live
worker whose declared capabilities are not a subset of the restriction — the exclusion branch becomes
observable end-to-end, not unit-test-only.

## Context (gap being closed)

The 2026-07-07 live compose verification (`docs/tasks/p0/46-live-compose-verification.md`, surface 42) could
verify pool-restriction *persistence* and the *matching* (positive) routing path live, but could **not** drive the
*non-matching* (exclusion) path, because the shipped Egress worker always registers with empty
countries/regions/ip_types. An empty claimed set is a subset of every restriction (`subset([], anything) == true`,
`internal/control/worker_registry.go:1096`), so the official worker is never excluded by any pool restriction.

Current-code evidence:

- `cmd/egress/main.go:214-218` — `caps := egress.Capabilities{SoftwareVersion: "dev", MaxConcurrency: ...,
  AllowedPools: pools}`: `Countries`, `Regions`, `IPTypes` are never set.
- `internal/config/config.go` — `EgressConfig` declares `AllowedPools` (`allowed_pools`) but has no
  countries/regions/ip_types/tags capability fields to load.
- `internal/egress/registration.go:35-37` and `:71-73` — the `Capabilities` struct and the `RegisterRequest`
  builder already carry and send `Countries` / `Regions` / `IpTypes`; only the config→caps population is missing.
- `docs/planning/24-static-configuration.md:82-86` — `egress.capabilities.countries`, `egress.capabilities.regions`,
  `egress.capabilities.ip_types` (and `.tags`, `.supported_ingress_modes`) are canonical static config keys with
  `[]` defaults.

Registration already *rejects* claims outside the credential's `allowed_capabilities`
(`internal/control/worker_registry.go:608-616`), so a worker that declares capabilities must have a credential
scoped to permit them — the enforcement side is built; only the worker's declaration side is missing.

## Required Planning Docs

- `docs/planning/24-static-configuration.md` — the `egress.capabilities.*` key table (lines ~80-86) and the Egress
  Config Example (`capabilities:` block, lines ~235-238).
- `docs/planning/26-config-management-api-surface.md` — Executor Pool `allowed_ip_types`/`allowed_countries`/
  `allowed_regions` semantics (the restriction side this declaration feeds).

## Prerequisites

- Task 42 done (pool-side restriction fields + routing enforcement). Done.
- Note the coupling with `docs/tasks/p1/22-egress-credential-config-schema-reconciliation.md`: task 22 canonicalizes
  the flat `egress.*` identity keys and explicitly excludes `egress.capabilities.*`. Prefer the flat/canonical
  config shape task 22 establishes for the new capability keys; if task 22 is not yet done, keep the new keys under
  a single `egress.capabilities.*` object matching `docs/planning/24` and cross-note the two tasks.

## Out of Scope

- Do not change the pool-side restriction fields, routing `poolAllows`, or the credential capability-scope
  rejection — those are built (task 42) and correct.
- Do not add per-request or dynamic capability negotiation; capabilities are static config declared at registration.
- Do not change `max_concurrency`/`supported_ingress_modes` sourcing if they are already wired elsewhere; add only
  the missing declaration fields, and state in the handoff which were already present.

## Expected Files

- Modify: `internal/config/config.go` — add the capability fields to `EgressConfig` (countries/regions/ip_types,
  plus tags if not already sourced), with `[]` defaults and validation consistent with existing egress config.
- Modify: `cmd/egress/main.go` — populate `egress.Capabilities.Countries/Regions/IPTypes` (and tags) from the
  loaded config so the built `cmd/egress` binary sends them at registration.
- Modify: `deploy/docker/egress.json` and `deploy/docker/README.md` — optionally declare a dev capability set so the
  compose worker can exercise a pool restriction; document it.
- Test: `internal/config/config_test.go` (capability keys load and round-trip; empty default), and an
  `internal/egress` registration test asserting the declared capabilities reach the `RegisterRequest`.

## Steps

- [x] Read the required planning doc sections listed above.
- [x] Add capability fields to `EgressConfig` in `internal/config/config.go` with `[]` defaults; add a config test
      proving they load and round-trip and default to empty when absent.
- [x] In `cmd/egress/main.go`, populate `egress.Capabilities` from the loaded config so the **built `cmd/egress`
      binary** sends countries/regions/ip_types (and tags) in its `RegisterRequest`.
- [x] Add an `internal/egress` test asserting the registration payload carries the configured capabilities.
- [x] Run focused tests (`go test ./internal/config ./internal/egress ./cmd/egress`), then `make check`.
- [x] If the compose stack is available, close the surface-42 exclusion gap live: scope the dev worker credential to
      permit `ip_types` (e.g. `["residential"]`) via the admin API, declare that capability in `egress.json`,
      restart egress, create a pool restricting `allowed_ip_types` to a disjoint value (e.g. `["datacenter"]`),
      and confirm a live request routes to `route_unavailable`; then loosen the restriction and confirm it
      succeeds. Record commands and output.
- [x] Update `docs/tasks/p0/46-live-compose-verification.md`'s surface-42 note and the task 42 handoff to point at
      this task as the owner of the previously un-drivable exclusion check.
- [x] Write a handoff note.

## Tests

- `go test ./internal/config ./internal/egress ./cmd/egress`
- `make check`

## Acceptance Criteria

- `EgressConfig` loads `countries`/`regions`/`ip_types` (and tags) from static config, defaulting to empty, proven
  by a config test.
- The built `cmd/egress` binary sends the configured capabilities in its `RegisterRequest`, proven by an
  `internal/egress`/`cmd/egress` test (not merely a struct field existing).
- With a worker declaring a capability outside a pool's non-empty restriction, the worker is not assignable for that
  pool — demonstrated live per the Steps if compose is available, otherwise proven by an existing/added routing test
  and recorded as live-pending in the handoff with this task named as owner.
- `grep` of `cmd/egress/main.go` shows `Capabilities.Countries`/`Regions`/`IPTypes` populated from config, not
  hardcoded empty.

## Handoff Notes

- Record the config key shape chosen and its relationship to `docs/tasks/p1/22-...` (flat vs nested).
- Record which capability fields were already wired (`max_concurrency`, `supported_ingress_modes`) vs added here.
- Record the live exclusion-path result if run; if compose was unavailable, say so explicitly and keep this task as
  the named owner in the surface-42 notes.

## Stop Conditions

- Stop if the config key shape conflicts with a committed decision in `docs/tasks/p1/22-...` or
  `docs/planning/32-open-decisions.md`.
- Stop if credential capability scoping would need to change to permit the declaration (that is a separate concern —
  ask before expanding).
- Stop if a deferral would have no owning task file.
