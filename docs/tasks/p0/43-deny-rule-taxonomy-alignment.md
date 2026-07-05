# 43 - Deny Rule Taxonomy Alignment

Status: done

## Objective

Align the `deny_rules` schema and config API with the `docs/planning/26` P0 Deny Rule schema: rule `type` in
`cidr | host | host_suffix | cname_suffix | metadata_ip | private_range`, `action` in `deny | allow_override`, and a
`reason` field — replacing the narrower `host|cidr|cname|ip` + `deny|allow` taxonomy shipped by task 04/20.

## Context (gap being closed)

The 2026-07-05 P0 verification audit confirmed `migrations/postgres/0001_init.sql` still constrains
`action IN ('deny','allow')` with the narrow `kind` set, and the comment at
`internal/control/config_admin_handlers.go:486` acknowledges the planning taxonomy as unimplemented with no owning
task. This task is that owner. Note the enforcement side already covers most of the intent —
`internal/control/destination_policy.go` compiles rules into `DestinationPolicyResult` and
`internal/egress/executor.go` enforces host-suffix, CNAME-suffix, and resolved-IP/private-range checks — so this task
is primarily about the config schema/API expressiveness and the compile step, not new egress enforcement.

## Required Planning Docs

- `docs/planning/26-config-management-api-surface.md` (P0 Deny Rule schema, lines ~315-328)
- `docs/planning/27-security-controls.md` (destination validation semantics)
- `docs/planning/16-egress-execution.md` (resolved-IP enforcement boundary)

## Prerequisites

- Task 20 completed (deny-rule CRUD) and task 26 completed (egress precedence enforcement).

## Out of Scope

- Do not change the egress-side precedence order established by task 26.
- Do not add redirect or MITM-related rules (P1/P2).

## Expected Files

- Add: `migrations/postgres/000X_deny_rule_taxonomy.sql` — widen the CHECK constraints, add `reason text`, and map
  existing rows (`ip` -> `cidr` with /32, `cname` -> `cname_suffix`, `allow` -> `allow_override`; confirm mappings
  against current row semantics before writing them).
- Modify: `internal/control/config_admin_handlers.go` (accept/validate the full taxonomy and `reason`).
- Modify: `internal/control/destination_policy.go` (compile `host_suffix`, `metadata_ip`, `private_range`, and
  `allow_override` into the existing `DestinationPolicyResult` fields the egress executor already enforces).
- Test: CRUD round-trip per type; a policy-resolution test per new type proving it lands in the right
  `DestinationPolicyResult` field; a migration-mapping test if row rewriting is involved.

## Steps

- [x] Read all required planning docs.
- [x] Write the migration (idempotent; existing rules keep their effective behavior — prove with a test).
- [x] Extend handler validation to the full type/action set plus `reason`; return them in reads.
- [x] Extend the destination-policy compile step so every type maps to an enforced `DestinationPolicyResult` field;
      `metadata_ip` and `private_range` should reuse the existing resolved-IP booleans/CIDR sets rather than new
      egress code.
- [x] Define and test `allow_override` precedence Control-side consistent with the egress `allowed_cidrs` override
      already implemented by task 26.
- [x] Run focused tests, then `make check`.
- [x] Write a handoff note.

## Tests

- `go test ./internal/control ./internal/egress`
- `make check`

## Acceptance Criteria

- Deny-rule CRUD accepts exactly the `docs/planning/26` P0 type/action taxonomy plus `reason`, and rejects the old
  narrow values with a clear error.
- Every accepted type demonstrably affects `DestinationPolicyResult` (test per type).
- Pre-existing rules behave identically after the migration.
- The acknowledging gap comment at `internal/control/config_admin_handlers.go:486` is removed.

## Handoff Notes

- Document the old->new value mapping and the `allow_override` precedence decision.

## Stop Conditions

- Stop if `allow_override` semantics require an open decision from `docs/planning/32-open-decisions.md`.
- Stop if a deferral would have no owning task file.
