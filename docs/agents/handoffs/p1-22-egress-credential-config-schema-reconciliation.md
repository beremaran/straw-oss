# Handoff

Task: `docs/tasks/p1/22-egress-credential-config-schema-reconciliation.md`

## Changed

- `docs/planning/24-static-configuration.md`: removed the nested `egress.credential.*` table rows and nested
  `credential:` example block; documented flat `egress.worker_id`, `egress.credential_id`, and
  `egress.private_key_ed25519_env` as canonical.
- `internal/config/config.go`: kept flat `EgressConfig` fields and added a narrow decode path for the retired
  `credential` object so nested-only configs fail required-field validation instead of silently loading.
- `internal/config/config_test.go`: added `TestLoadEgressCredentialKeysAreFlat` for flat round-trip and nested-shape
  rejection.
- `docs/agents/handoffs/35-worker-registration-replay-and-identity-key.md`: marked the old gap closed by this task.

## Acceptance Criteria Verdicts

Filled from the independent verifier's report (straw-task-runner workflow step 12), not from the implementer's
self-assessment.

| Criterion | Verdict | Implementation (file:line) | Proving test |
|-----------|---------|----------------------------|--------------|
| No nested credential key-table row or example block remains canonical | VERIFIED | `docs/planning/24-static-configuration.md:77`, `docs/planning/24-static-configuration.md:108`, `docs/planning/24-static-configuration.md:217` | `rg -n "egress\\.credential|credential:" docs/planning/24-static-configuration.md` only finds the historical reconciled mention |
| Old "has previously unowned" sentence is removed; surviving mention names this task | VERIFIED | `docs/planning/24-static-configuration.md:108`, `docs/planning/24-static-configuration.md:110` | grep found no "has previously unowned" / "Reconciling..." sentence in the planning doc |
| Flat-key egress config round-trips `WorkerID`, `CredentialID`, and `PrivateKeyEd25519Env` | VERIFIED | `internal/config/config_test.go:348`, `internal/config/config_test.go:351`, `internal/config/config_test.go:364` | `go test ./internal/config` |
| Nested `egress.credential.*` form fails with missing required field validation | VERIFIED | `internal/config/config_test.go:374`, `internal/config/config_test.go:386`, `internal/config/config.go:167` | `go test ./internal/config` |
| `deploy/docker/egress.json` uses only flat keys and no nested `credential` object | VERIFIED | `deploy/docker/egress.json:4`, `deploy/docker/egress.json:5`, `deploy/docker/egress.json:6` | `make check` |
| `make check` passes | VERIFIED | command criterion | `make check` |

## Planning-Doc Coverage

Every in-phase field/endpoint/behavior from the task's cited planning-doc sections, accounted for:

| Planning item | Status | Evidence / owning task |
|---------------|--------|------------------------|
| `egress.worker_id` flat worker identity key | already existed | `internal/config/config.go:145`, `deploy/docker/egress.json:4`, `docs/planning/24-static-configuration.md:70` |
| `egress.credential_id` flat credential identity key | implemented | `internal/config/config.go:148`, `deploy/docker/egress.json:5`, `docs/planning/24-static-configuration.md:77` |
| `egress.private_key_ed25519_env` flat private-key env-var key | already existed | `internal/config/config.go:154`, `deploy/docker/egress.json:6`, `docs/planning/24-static-configuration.md:78` |
| Retired nested `egress.credential.*` shape | implemented | Removed from canonical table/example; nested-only config fails in `internal/config/config_test.go:374` |

## Verification

```sh
go test ./internal/config
make check
```

Result:

- `go test ./internal/config`: pass.
- `make check`: pass (`go test ./...`; `golangci-lint run --max-issues-per-linter 0 --max-same-issues 0` reported
  `0 issues`).
- Postgres-backed tests: not exercised (diff does not touch Postgres surfaces or migrations).
- Live compose verification: skipped because this is a static config-schema/documentation test; no runtime request
  path changed.

## Reviewer Start Points

- `docs/planning/24-static-configuration.md`
- `internal/config/config.go`
- `internal/config/config_test.go`

## Remaining Work

- None.

## Blockers

- None.
