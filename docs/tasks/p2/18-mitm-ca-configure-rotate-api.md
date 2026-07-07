# 18 - MITM CA Configure and Rotate API

Status: not started

## Objective

Add the tenant_admin-only MITM CA configure/rotate API promised by the P2 MITM planning docs, without generating
production CA private keys inside Straw or exposing CA secrets.

## Context (gap being closed)

Task 03 implemented operator-provided static CA config plus authenticated public CA download, but did not add a mutable
configure/rotate endpoint. The planning docs still require tenant admin rights for CA configure/rotate:
`docs/planning/17-mitm-design-p2.md` states that tenant admins configure and rotate the CA, and
`docs/planning/07-public-api-surface.md` states that tenant admin rights are required to rotate or configure the CA.
Current code only registers `GET /api/v1/mitm/ca.pem` (`cmd/control/main.go:1020`) and the handler only returns the
configured public cert (`internal/control/mitm_ca_handler.go:41`).

## Required Planning Docs

- `docs/planning/17-mitm-design-p2.md`
- `docs/planning/07-public-api-surface.md`
- `docs/planning/24-static-configuration.md`
- `docs/planning/c-mitm-leaf-certificate-design.md`

## Prerequisites

- Task 03 completed (public CA config/download baseline).
- Task 04 completed if rotation must invalidate or migrate cached leaf certificates.

## Out of Scope

- Do not generate production CA private keys inside Straw.
- Do not return CA private key material from any API.
- Do not change the task 01 private-key storage policy.

## Expected Files

- Create or modify: MITM CA config/rotation handler and route wiring.
- Create or modify: durable CA metadata/secret storage only if static file paths are insufficient for the selected API.
- Test: MITM CA configure/rotate authorization, redaction, and cache-invalidation tests.

## Steps

- [ ] Read all required planning docs.
- [ ] Define the minimal request/response contract for CA configure/rotate under `/api/v1`.
- [ ] Enforce `tenant_admin` for configure/rotate and reject requester, viewer, operator, and platform data-plane keys.
- [ ] Accept only operator-provided CA material or references; never generate production CA keys.
- [ ] Ensure responses, logs, errors, telemetry, and audit rows never include private key material.
- [ ] If leaf caching exists, invalidate or version cached leaves on CA rotation.
- [ ] Add tests for tenant_admin success, non-admin denial, secret redaction, invalid CA material, and cache/version behavior.
- [ ] Run focused MITM CA configure/rotate tests.
- [ ] Run `make check`.
- [ ] Write a handoff note.

## Tests

- Focused MITM CA configure/rotate tests.
- `make check`

## Acceptance Criteria

- A tenant_admin can configure or rotate MITM CA material through the documented API without Straw generating a
  production CA private key.
- Requester, viewer, operator, and platform data-plane keys cannot configure or rotate MITM CA material.
- Public responses, logs, telemetry, and audit records never expose CA private key material.
- CA rotation updates or invalidates any cached leaf certificate state that depends on the prior CA.

## Handoff Notes

- Document the API path, request/response schema, secret redaction behavior, and any cache invalidation/versioning.

## Stop Conditions

- Stop if the selected storage model would require a new dependency.
- Stop if task 04 has not defined the cache invalidation/versioning point needed for safe rotation.
- Stop if a deferral would have no owning task file.
