# Handoff

Task: `docs/tasks/p1/10-cli.md`

## Changed

- Added `cmd/straw` as the CLI entrypoint wired to `internal/cli.Run`.
- Added `internal/cli` with SDK-backed request submission, raw JSON config/admin commands for the documented surfaces, operational `healthz`/`readyz`/`metrics` reads, env/default handling, and stderr secret redaction.
- Added focused CLI tests for request command parsing, SDK request submission, env API-key/base-URL loading, config write routing, admin actions, undocumented resource rejection, and error redaction.

## Command Set

- `straw request --method GET --url https://example.com [--header 'Name: value'] [--body-file path]`
- `straw request --json request.json`
- `straw config list [--limit n] [--offset n] <resource>`
- `straw config get tenants <id>|quotas|rate-limits|fingerprint-profiles|changes`
- `straw config create --json body.json <resource>`
- `straw config update --json body.json <resource-path>`
- `straw config delete <resource-path>`
- `straw config revoke platform-api-keys|api-keys|worker-credentials <id>`
- `straw config rollback --json body.json`
- `straw admin workers`
- `straw admin worker <worker_id> disable|enable|drain|undrain|tenant-disable|tenant-enable|tenant-drain|tenant-undrain`
- `straw admin cancel <request_id>`
- `straw healthz`
- `straw readyz`
- `straw metrics`

Environment variables:

- `STRAW_BASE_URL`: Control API base URL; defaults to `http://localhost:8080`.
- `STRAW_API_KEY`: Bearer token for authenticated request, config, and admin calls.

## Acceptance Criteria Verdicts

Filled from the independent verifier's report.

| Criterion | Verdict | Implementation (file:line) | Proving test |
|-----------|---------|----------------------------|--------------|
| CLI commands cover the documented minimal API surface. | VERIFIED | `cmd/straw/main.go:11`; `internal/cli/cli.go:78`; `internal/cli/cli.go:149`; `internal/cli/cli.go:162`; `internal/cli/cli.go:620`; `internal/cli/cli.go:659` | `TestRequestCommandUsesSDKAndAPIKey`, `TestConfigUpdateAllowsDocumentedResourcePath`, `TestConfigRejectsUndocumentedResourcePath`, `TestAdminCommands` |
| The command set is documented in the task handoff. | VERIFIED | `docs/agents/handoffs/p1-10-cli.md:10` | Handoff command inventory |
| Secrets are not printed except one-time key create responses when explicitly requested. | VERIFIED | `internal/cli/cli.go:56`; `internal/cli/cli.go:579`; `internal/cli/cli.go:167`; `internal/cli/cli.go:239` | `TestRunRedactsSecretsFromErrors` |

## Planning-Doc Coverage

| Planning item | Status | Evidence / owning task |
|---------------|--------|------------------------|
| Public base path `/api/v1`. | implemented | `internal/cli/cli.go:203`, `internal/cli/cli.go:264`, `internal/cli/cli.go:365` |
| `POST /api/v1/requests` synchronous REST transport. | implemented | `internal/cli/cli.go:149`, `internal/cli/cli.go:738`, `internal/cli/cli_test.go:24` |
| Request schema fields `method`, `url`, `headers`, `body.inline_base64`, `routing`, `fingerprint_profile`, `timeout_ms`, `replayable`, `capture_hint`. | implemented | Full schema can be passed through `--json` at `internal/cli/cli.go:120` and decoded at `internal/cli/cli.go:741`; common flags are at `internal/cli/cli.go:121` through `internal/cli/cli.go:126`; headers/body at `internal/cli/cli.go:128` and `internal/cli/cli.go:759` |
| `/healthz`, `/readyz`, `/metrics`. | implemented | `internal/cli/cli.go:85`, `internal/cli/cli.go:99` |
| Config list pagination `limit` and `offset`. | implemented | `internal/cli/cli.go:185`, `internal/cli/cli.go:187`, `internal/cli/cli.go:205` |
| Tenants create/list/get/update/delete. | implemented | `internal/cli/cli.go:620`, `internal/cli/cli.go:635`, `internal/cli/cli.go:517`, `internal/cli/cli.go:670` |
| Platform API keys create/list/revoke. | implemented | `internal/cli/cli.go:621`, `internal/cli/cli.go:637`, `internal/cli/cli.go:653` |
| Tenant API keys create/list/revoke. | implemented | `internal/cli/cli.go:622`, `internal/cli/cli.go:638`, `internal/cli/cli.go:654` |
| Worker credentials create/list/revoke. | implemented | `internal/cli/cli.go:623`, `internal/cli/cli.go:639`, `internal/cli/cli.go:655` |
| Executor pools create/list/update/delete. | implemented | `internal/cli/cli.go:624`, `internal/cli/cli.go:640`, `internal/cli/cli.go:670` |
| Routing rules create/list/update/delete. | implemented | `internal/cli/cli.go:625`, `internal/cli/cli.go:641`, `internal/cli/cli.go:670` |
| Fingerprint profiles list. | implemented | `internal/cli/cli.go:626`, `internal/cli/cli.go:512` |
| Injection policies create/list/update/delete. | implemented | `internal/cli/cli.go:627`, `internal/cli/cli.go:642`, `internal/cli/cli.go:670` |
| Quotas get and tenant quota update. | implemented | `internal/cli/cli.go:628`, `internal/cli/cli.go:512`, `internal/cli/cli.go:678` |
| Rate limits get/update. | implemented | `internal/cli/cli.go:630`, `internal/cli/cli.go:647`, `internal/cli/cli.go:512` |
| Deny rules create/list/update/delete. | implemented | `internal/cli/cli.go:631`, `internal/cli/cli.go:643`, `internal/cli/cli.go:670` |
| Config changes list. | implemented | `internal/cli/cli.go:632`, `internal/cli/cli.go:512` |
| Config rollback. | implemented | `internal/cli/cli.go:175`, `internal/cli/cli.go:311` |
| Runtime workers list. | implemented | `internal/cli/cli.go:340`, `internal/cli/cli.go:351` |
| Runtime worker global and tenant actions. | implemented | `internal/cli/cli.go:342`, `internal/cli/cli.go:368`, `internal/cli/cli.go:659` |
| Runtime request cancel. | implemented | `internal/cli/cli.go:344`, `internal/cli/cli.go:390` |
| REST streaming variant `/api/v1/requests:stream`. | out of scope | Server endpoint owned by `docs/tasks/p1/06-rest-streaming-endpoint.md`; CLI stream client support after task 06 is owned by `docs/tasks/p1/28-sdk-cli-rest-streaming-client.md`. |
| Telemetry read APIs. | out of scope | Owned by `docs/tasks/p1/11-telemetry-schema-and-query-limits-spec.md` and `docs/tasks/p1/12-telemetry-read-apis.md`. |
| MITM CA distribution and payload-capture APIs. | out of scope | P2 scope owned by `docs/tasks/p2/01-mitm-leaf-certificate-design.md` and `docs/tasks/p2/09-payload-capture-policy.md`. |

## Verification

```sh
go test ./internal/cli ./cmd/straw -count=1 -timeout=30s
make check
```

Result:

- Focused CLI tests: passed.
- `make check`: passed.
- Postgres-backed tests: not exercised; diff does not touch Postgres files or migrations.
- Live compose verification: skipped; diff does not touch Control/Egress runtime request path, only a client CLI over existing HTTP APIs.

## Reviewer Start Points

- `cmd/straw/main.go`
- `internal/cli/cli.go`
- `internal/cli/cli_test.go`

## Remaining Work

- None.

## Blockers

- None.
