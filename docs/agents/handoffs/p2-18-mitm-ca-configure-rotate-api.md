# Handoff

Task: `docs/tasks/p2/18-mitm-ca-configure-rotate-api.md`

## Changed

- `internal/control/mitm_ca_handler.go`: added `PUT /api/v1/mitm/ca` for tenant-admin CA rotation, PEM pair validation, configured cert/key file writes, public fingerprint/version response, and redacted audit recording.
- `cmd/control/main.go`: wired the rotate route and changed MITM leaf hooks to reload CA files and rebuild CA-versioned leaf cache hooks when the configured CA changes.
- `internal/control/mitm_ca_handler_test.go`, `cmd/control/main_test.go`: added rotation authorization, redaction, invalid material, platform-key denial, and CA-version reload tests.

## Acceptance Criteria Verdicts

Filled from the independent verifier's report.

| Criterion | Verdict | Implementation (file:line) | Proving test |
|-----------|---------|----------------------------|--------------|
| A tenant_admin can configure or rotate MITM CA material through the documented API without Straw generating a production CA private key. | VERIFIED | `cmd/control/main.go:1202`, `cmd/control/main.go:1215`, `internal/control/mitm_ca_handler.go:85`, `internal/control/mitm_ca_handler.go:109`, `internal/control/mitm_ca_handler.go:116` | `TestMITMCAHandlerRotatesCAForTenantAdmin` |
| Requester, viewer, operator, and platform data-plane keys cannot configure or rotate MITM CA material. | VERIFIED | `internal/control/mitm_ca_handler.go:93` | `TestMITMCAHandlerRotateRejectsNonAdmins`, `TestMITMCAHandlerRotateRejectsPlatformKey` |
| Public responses, logs, telemetry, and audit records never expose CA private key material. | VERIFIED | `internal/control/mitm_ca_handler.go:42`, `internal/control/mitm_ca_handler.go:123`, `internal/control/mitm_ca_handler.go:125` | `TestMITMCAHandlerRotatesCAForTenantAdmin`, `TestMITMCAHandlerRotateRejectsInvalidMaterial` |
| CA rotation updates or invalidates any cached leaf certificate state that depends on the prior CA. | VERIFIED | `cmd/control/main.go:382`, `cmd/control/main.go:459`, `cmd/control/main.go:465`, `cmd/control/main.go:474`, `cmd/control/main.go:505` | `TestMITMLeafFileHooksReloadsAfterCARotation` |

## Planning-Doc Coverage

| Planning item | Status | Evidence / owning task |
|---------------|--------|------------------------|
| `/api/v1` MITM CA configure/rotate surface requiring tenant_admin rights (`docs/planning/07-public-api-surface.md`, `17-mitm-design-p2.md`). | implemented | `cmd/control/main.go:1215`, `internal/control/mitm_ca_handler.go:93` |
| Operator-provided CA material only; no production CA key generation in Straw (`docs/planning/17-mitm-design-p2.md`). | implemented | `internal/control/mitm_ca_handler.go:37`, `internal/control/mitm_ca_handler.go:109`, `internal/control/mitm_ca_handler.go:229` |
| Static CA config paths are canonical (`control.server.mitm_ca_cert_file`, `control.server.mitm_ca_key_file`) (`docs/planning/24-static-configuration.md`). | implemented | `cmd/control/main.go:1210`, `cmd/control/main.go:1211`, `internal/control/mitm_ca_handler.go:224` |
| Public CA download remains authenticated and returns only the public certificate. | already existed, preserved | `internal/control/mitm_ca_handler.go:59`, `internal/control/mitm_ca_handler.go:73` |
| Leaf cache must be invalidated or versioned on CA rotation (`docs/planning/c-mitm-leaf-certificate-design.md`). | implemented | `cmd/control/main.go:459`, `cmd/control/main.go:465`, `cmd/control/main.go:474`, `cmd/control/main.go:505` |
| Plaintext private keys must not be written to logs, ClickHouse, audit rows, or error responses. | implemented | `internal/control/mitm_ca_handler.go:103`, `internal/control/mitm_ca_handler.go:110`, `internal/control/mitm_ca_handler.go:123`, `internal/control/mitm_ca_handler.go:125` |

## Verification

```sh
go test ./internal/control -run 'TestMITMCAHandler|TestMITMLeafCache'
go test ./cmd/control -run 'TestMITM|TestBuildMITM'
golangci-lint run --max-issues-per-linter 0 --max-same-issues 0 ./cmd/control ./internal/control
make check
```

Result: all passed.

- Postgres-backed tests: not exercised; diff does not touch Postgres stores, migrations, or Postgres-backed test surfaces.
- Live compose verification: skipped. `deploy/docker/docker-compose.yml` is absent, and `deploy/production/compose.yml` cannot be inspected without required environment such as `POSTGRES_PASSWORD`; no usable local compose stack was available in this run.

## Reviewer Start Points

- `internal/control/mitm_ca_handler.go`
- `cmd/control/main.go`

## Remaining Work

- None.

## Blockers

- None.
