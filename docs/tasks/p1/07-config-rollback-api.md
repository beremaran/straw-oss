# 07 - Config Rollback API

Status: not started

## Objective

Implement `POST /api/v1/config/rollback` so tenant admins can create a new config version from audit-source history
without restoring redacted secrets.

## Required Planning Docs

- `docs/planning/26-config-management-api-surface.md`
- `docs/planning/21-state-and-storage.md`
- `docs/planning/25-dynamic-configuration.md`

## Prerequisites

- P0 task 20 completed.

## Out of Scope

- Do not restore fields redacted as secrets.
- Do not reuse an old config version number.
- Do not add generic point-in-time database restore.

## Expected Files

- Create or modify: config rollback handler/store code.
- Test: rollback API tests.

## Steps

- [ ] Read all required planning docs.
- [ ] Implement the rollback request schema with `expected_config_version`, `target_config_version`, and `reason`.
- [ ] Reconstruct rollback-safe fields from `config_audit_source`.
- [ ] Reject rollback when secret fields would be required but redacted.
- [ ] Write rollback as a new config version in one transaction with audit source rows.
- [ ] Publish config invalidation after commit.
- [ ] Add tests for successful rollback, conflict, secret-redaction rejection, audit rows, and cache invalidation.
- [ ] Run focused rollback tests.
- [ ] Run `make check`.
- [ ] Write a handoff note.

## Tests

- Focused rollback API tests.
- `make check`

## Acceptance Criteria

- Rollback creates a new tenant config version.
- Secret values are never restored from redacted audit records.
- Version conflicts return canonical `conflict`.

## Handoff Notes

- Document which resources are rollback-safe.

## Stop Conditions

- Stop if rollback would require unrecoverable secret values.
- Stop if a deferral would have no owning task file.
