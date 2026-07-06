# Handoff

Task: `docs/tasks/p1/26-upstream-connection-pooling-implementation.md`

## Changed

- Added `egress.upstream_connection_pool.*` config fields, defaults, and validation in `internal/config/config.go`.
- Added optional direct-local upstream pooling in `internal/egress/executor.go`, keyed by tenant, resolution mode, scheme, host, port, validated dial IP, and fingerprint profile.
- Wired `cmd/egress/main.go` to pass static pooling config into the built Egress binary.
- Added worker shutdown cleanup through `Worker.Serve` so pooled idle connections close during drain/shutdown.
- Added focused config and executor tests for disabled defaults, exact-key reuse boundaries, DNS rebinding stale discard, validated-IP dialing, eviction/error cleanup, TLS failure non-reuse, worker cleanup, and goroutine bounds.

## Acceptance Criteria Verdicts

Filled from the independent verifier's report (straw-task-runner workflow step 12), not from the implementer's self-assessment.

| Criterion | Verdict | Implementation (file:line) | Proving test |
|-----------|---------|----------------------------|--------------|
| Default config leaves keep-alives disabled and outbound HTTP/2 disabled. | VERIFIED | `internal/egress/executor.go:124`, `internal/egress/executor.go:132`, `internal/egress/executor.go:133`, `internal/config/config.go:172` | `TestP0TransportDefaults`; `TestLoadEgressUpstreamConnectionPoolDefaultsAndValidation` |
| Enabled config reuses only exact pool key; cross-tenant, cross-host, cross-port, cross-IP, and cross-fingerprint reuse is rejected. | VERIFIED | `internal/egress/executor.go:321`, `internal/egress/executor.go:524`, `internal/egress/executor.go:578` | `TestExecutorUpstreamConnectionPoolReusesExactKey`; `TestExecutorUpstreamConnectionPoolKeyIncludesIsolationFields` |
| Every request resolves and validates destination policy before reuse; DNS rebinding discards stale pooled IP. | VERIFIED | `internal/egress/executor.go:315`, `internal/egress/executor.go:365`, `internal/egress/executor.go:331`, `internal/egress/executor.go:545` | `TestExecutorUpstreamConnectionPoolValidatesBeforeReuse` |
| Enabled transport dials only validated IPs and does not perform an independent second hostname resolution. | VERIFIED | `internal/egress/executor.go:537`, `internal/egress/executor.go:362` | `TestExecutorUpstreamConnectionPoolReusesExactKey` |
| Idle timeout, max lifetime, protocol/TLS/body errors, and worker shutdown close pooled connections without leaking goroutines. | VERIFIED | `internal/egress/executor.go:140`, `internal/egress/executor.go:529`, `internal/egress/executor.go:568`, `internal/egress/loop.go:100`, `internal/egress/loop.go:145` | `TestExecutorUpstreamConnectionPoolEviction`; `TestExecutorUpstreamConnectionPoolTLSFailureIsNotReused`; `TestWorkerShutdownClosesExecutorPool` |

## Planning-Doc Coverage

| Planning item | Status | Evidence / owning task |
|---------------|--------|------------------------|
| Appendix B feature flag `egress.upstream_connection_pool.enabled`, disabled by default. | implemented | `internal/config/config.go:172`, `internal/config/config.go:535` |
| Appendix B config defaults: max idle per tenant/host = 2, idle timeout = 30000ms, max lifetime = 300000ms. | implemented | `internal/config/config.go:521` |
| Appendix B/P1 exact pool key: tenant, resolution mode, scheme, original host, port, validated IP, fingerprint profile. | implemented | `internal/egress/executor.go:321`, `internal/egress/executor.go:472` |
| Appendix B direct-local only; no proxy-mode pooling. | implemented | `internal/egress/executor.go:182`, `internal/egress/executor.go:187`, `internal/egress/executor.go:288` |
| Section 16/27 SSRF invariant: resolve and validate before connect/reuse; no second resolver in HTTP library. | implemented | `internal/egress/executor.go:315`, `internal/egress/executor.go:365`, `internal/egress/executor.go:537` |
| Appendix B stale DNS rebinding: close pooled connection when old dial IP is absent from current validated set. | implemented | `internal/egress/executor.go:331`, `internal/egress/executor.go:545` |
| Appendix B eviction and shutdown: idle timeout, max lifetime, error cleanup, worker shutdown cleanup. | implemented | `internal/egress/executor.go:140`, `internal/egress/executor.go:524`, `internal/egress/executor.go:568`, `internal/egress/loop.go:100` |
| Section 30 required P1 upstream pooling test rows. | implemented | `internal/egress/executor_test.go:503`, `internal/egress/executor_test.go:584`, `internal/egress/executor_test.go:664`, `internal/egress/executor_test.go:732`, `internal/egress/executor_test.go:847`, `internal/egress/executor_test.go:884` |
| Section 24 Egress config shape accepts canonical pooling keys. | implemented | `internal/config/config.go:160`, `internal/config/config_test.go:348` |

## Verification

```sh
go test ./internal/config ./internal/egress ./cmd/egress
make check
```

Result:

- `go test ./internal/config ./internal/egress ./cmd/egress`: passed.
- `make check`: passed (`go test ./...` and `golangci-lint run --max-issues-per-linter 0 --max-same-issues 0`).
- Postgres-backed tests: not exercised; diff does not touch Postgres or migrations.
- Live compose verification: skipped; task is egress-local config/transport behavior covered by focused unit tests and does not require a live Control/NATS request path to prove the pooling boundary.

## Reviewer Start Points

- `internal/egress/executor.go`
- `internal/egress/executor_test.go`
- `internal/config/config.go`
- `cmd/egress/main.go`

## Remaining Work

- None.

## Blockers

- None.
