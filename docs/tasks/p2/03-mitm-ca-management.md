# 03 - MITM CA Management

Status: done

## Objective

Add operator-provided MITM CA configuration, optional local dev CA helpers, and the authenticated CA download endpoint.

## Required Planning Docs

- `docs/planning/17-mitm-design-p2.md`
- `docs/planning/07-public-api-surface.md`
- `docs/planning/24-static-configuration.md`

## Prerequisites

- Task 01 completed.

## Out of Scope

- Do not generate production CA keys inside Straw.
- Do not implement leaf certificate cache.
- Do not let non-admin users rotate/configure the CA.

## Expected Files

- Create or modify: MITM CA config loading and API handler.
- Optional: dev/test CA helper scripts.
- Test: CA management tests.

## Steps

- [x] Read all required planning docs.
- [x] Add static config and environment handling for operator-provided CA paths and `STRAW_MITM_CERT_VALIDITY_DAYS`.
- [x] Add optional offline dev/test CA helper scripts that are clearly not production CA generation.
- [x] Implement `GET /api/v1/mitm/ca.pem` for authenticated tenants allowed to use MITM.
- [x] Enforce tenant_admin-only rotate/configure behavior if rotation endpoints are added.
- [x] Ensure CA private key material is never logged or returned.
- [x] Add tests for config loading, CA download authorization, tenant access, rotation permissions, and secret redaction.
- [x] Run focused MITM CA tests.
- [x] Run `make check`.
- [x] Write a handoff note.

## Tests

- Focused MITM CA management tests.
- `make check`

## Acceptance Criteria

- Operators can configure MITM CA material without Straw generating production CA keys.
- Authorized tenants can download only the public CA certificate.
- CA secrets are never exposed.

## Handoff Notes

- Document config keys and dev-helper limitations.

## Stop Conditions

- Stop before generating production CA private keys.
- Stop if a deferral would have no owning task file.
