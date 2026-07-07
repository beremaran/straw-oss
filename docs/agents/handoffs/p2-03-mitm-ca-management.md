# Handoff

Task: `docs/tasks/p2/03-mitm-ca-management.md`

## Changed

- Added MITM CA static config fields/defaulting/validation, including `STRAW_MITM_CERT_VALIDITY_DAYS`, in `internal/config/config.go`.
- Added authenticated `GET /api/v1/mitm/ca.pem` serving only the operator-configured public CA certificate, gated by tenant data-plane auth and an enabled MITM-capable routing rule.
- Wired the CA download route into Control with the real `ConfigCache`.
- Added focused MITM CA config and authorization tests.
- Added `scripts/dev-mitm-ca.sh`, an offline dev/test-only helper that prints its non-production limitation.
- Updated `docs/planning/24-static-configuration.md` with the canonical MITM CA keys.

## Acceptance Criteria Verdicts

Filled from the independent verifier's report.

| Criterion | Verdict | Implementation (file:line) | Proving test |
|-----------|---------|----------------------------|--------------|
| Operators can configure MITM CA material without Straw generating production CA keys. | VERIFIED | `internal/config/config.go:97`, `internal/config/config.go:417`, `internal/config/config.go:556`, `scripts/dev-mitm-ca.sh:16` | `TestLoadControlDefaultsMITMPort`, `TestLoadControlMITMValidityDaysEnv`, focused MITM command |
| Authorized tenants can download only the public CA certificate. | VERIFIED | `internal/control/mitm_ca_handler.go:28`, `internal/control/mitm_ca_handler.go:35`, `internal/control/mitm_ca_handler.go:41`, `internal/control/mitm_ca_handler.go:53`, `cmd/control/main.go:1015` | `TestMITMCAHandlerServesPublicCertificateOnly`, `TestMITMCAHandlerRejectsUnauthorizedRoles`, `TestMITMCAHandlerRejectsInactiveTenant`, `TestMITMCAHandlerRequiresMITMAllowedRoute` |
| CA secrets are never exposed. | VERIFIED | `internal/control/mitm_ca_handler.go:14`, `internal/control/mitm_ca_handler.go:41` | `TestMITMCAHandlerServesPublicCertificateOnly` |

## Planning-Doc Coverage

| Planning item | Status | Evidence / owning task |
|---------------|--------|------------------------|
| Operators provide CA material through static config. | implemented | `internal/config/config.go:97`, `internal/config/config.go:99`, `internal/config/config.go:100`, `internal/config/config.go:556`; documented at `docs/planning/24-static-configuration.md:17` |
| Straw may provide offline helper scripts for dev/test CA material. | implemented | `scripts/dev-mitm-ca.sh:16` |
| Control exposes public CA at `/api/v1/mitm/ca.pem`. | implemented | `cmd/control/main.go:1020`, `internal/control/mitm_ca_handler.go:20` |
| Endpoint requires authenticated users allowed to use MITM. | implemented | `internal/control/mitm_ca_handler.go:28`, `internal/control/mitm_ca_handler.go:35`, `internal/control/mitm_ca_handler.go:53` |
| Tenant admins configure and rotate the CA. | out of scope | Mutable configure/rotate API is owned by `docs/tasks/p2/18-mitm-ca-configure-rotate-api.md`; this task added no mutable CA endpoint, so no non-admin mutable CA path exists. |
| Public API base path is `/api/v1`. | implemented | `cmd/control/main.go:1020` |
| P2 may add MITM CA distribution API. | implemented | `cmd/control/main.go:1020` |
| `STRAW_MITM_CERT_VALIDITY_DAYS` is the canonical environment variable. | implemented | `internal/config/config.go:422`, `docs/planning/24-static-configuration.md:21` |
| Leaf certificate cache/storage policy. | out of scope | Owned by `docs/tasks/p2/04-mitm-leaf-cert-cache.md`. |
| HTTP/2 MITM ALPN. | out of scope | Owned by `docs/tasks/p2/16-ingress-http2-and-mitm-alpn.md`. |

## Verification

```sh
go test ./internal/config ./internal/control ./cmd/control -run 'TestLoadControl.*MITM|TestMITMCA|TestBuildMITM|TestGenerateMITM|TestMITM'
make check
```

Result:

- Focused MITM CA tests: passed.
- `make check`: passed (`go test ./...` plus `golangci-lint run --max-issues-per-linter 0 --max-same-issues 0`, 0 issues).
- Postgres-backed tests: not exercised; diff does not touch Postgres files or migrations.
- Live compose verification: skipped because the compose Control config does not enable MITM or provide CA cert/key material. The handler and route wiring are covered by unit tests; task 02 already covers the local MITM TLS listener path.

## Reviewer Start Points

- `internal/control/mitm_ca_handler.go`
- `cmd/control/main.go`
- `internal/config/config.go`
- `internal/control/mitm_ca_handler_test.go`

## Remaining Work

- None.

## Blockers

- None.
