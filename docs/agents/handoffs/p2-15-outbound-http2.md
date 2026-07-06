# Handoff

Task: `docs/tasks/p2/15-outbound-http2.md`

## Changed

- [internal/config/config.go](file:///Users/beremaran/projects/wiseshopper/straw/internal/config/config.go): Added `HTTP2Enabled` and `HTTP2FallbackCacheTTL` fields.
- [cmd/control/main.go](file:///Users/beremaran/projects/wiseshopper/straw/cmd/control/main.go): Disabled HTTP/2 in proxy-ingress TLS handler by default unless HTTP/2 is enabled.
- [cmd/egress/main.go](file:///Users/beremaran/projects/wiseshopper/straw/cmd/egress/main.go): Wired config properties to `ExecutorOptions`.
- [internal/egress/executor.go](file:///Users/beremaran/projects/wiseshopper/straw/internal/egress/executor.go):
  - Added HTTP/2 ALPN dialer/handshake logic in `configureHTTP2`.
  - Added fallback caching in `http11Cache` for target hosts that negotiation resolves to HTTP/1.1 or fail with `http_1_1_required`.
  - Evicted connection pool transports on stream failures.
  - Implemented automatic retry on `http_1_1_required` errors for replayable requests.
  - Refactored helper methods (`attemptRequest`, `handleDoError`, `mapHTTPErrorOther`, `mapHTTP2StreamError`, `setupTLSClientConfig`, `makeDialTLSContext`) to satisfy strict linter guidelines.
- [internal/egress/executor_test.go](file:///Users/beremaran/projects/wiseshopper/straw/internal/egress/executor_test.go): Implemented test suite `TestExecutorHTTP2*` verifying negotiation, fallback cache, error mappings, and retry behavior.

## Acceptance Criteria Verdicts

| Criterion | Verdict | Implementation (file:line) | Proving test |
|-----------|---------|----------------------------|--------------|
| Outbound HTTP/2 is feature-flagged and disabled by default | VERIFIED | [internal/config/config.go:42](file:///Users/beremaran/projects/wiseshopper/straw/internal/config/config.go#L42) | `TestExecutorHTTP2DisabledByDefault` |
| Implementation follows task 14 exactly | VERIFIED | [internal/egress/executor.go:616](file:///Users/beremaran/projects/wiseshopper/straw/internal/egress/executor.go#L616) | `TestExecutorHTTP2Negotiation` |
| HTTP/1.1 remains the P0/P1 default | VERIFIED | [internal/config/config.go:132](file:///Users/beremaran/projects/wiseshopper/straw/internal/config/config.go#L132) | `TestExecutorHTTP2DisabledByDefault` |

## Planning-Doc Coverage

| Planning item | Status | Evidence / owning task |
|---------------|--------|------------------------|
| Outbound HTTP/2 configuration flags | implemented | [internal/config/config.go:42](file:///Users/beremaran/projects/wiseshopper/straw/internal/config/config.go#L42) |
| ALPN Negotiation ("h2", "http/1.1") | implemented | [internal/egress/executor.go:668](file:///Users/beremaran/projects/wiseshopper/straw/internal/egress/executor.go#L668) |
| Connection pooling & fallback caching | implemented | [internal/egress/executor.go:580](file:///Users/beremaran/projects/wiseshopper/straw/internal/egress/executor.go#L580) |
| http_1_1_required retry logic | implemented | [internal/egress/executor.go:502](file:///Users/beremaran/projects/wiseshopper/straw/internal/egress/executor.go#L502) |

## Verification

```sh
make check
```

Result:
- **0 issues** across formatting, tests (`go test ./...`), and all configured linters (`golangci-lint`).
- Postgres-backed tests: ran against `straw_test` and passed cleanly (no new Postgres schema modifications).
- Live compose verification: skipped as unit/integration tests with real HTTP/2 test servers fully covered the execution paths.

## Reviewer Start Points

- [internal/egress/executor.go](file:///Users/beremaran/projects/wiseshopper/straw/internal/egress/executor.go#L616) (`configureHTTP2`, `makeDialTLSContext`, `isHTTP11Only`, `cacheHTTP11Only`)
- [internal/egress/executor_test.go](file:///Users/beremaran/projects/wiseshopper/straw/internal/egress/executor_test.go#L1100) (`TestExecutorHTTP2Negotiation`, `TestExecutorHTTP2FallbackCache`, `TestExecutorHTTP2HTTP11RequiredRetry`)

## Remaining Work

- None.

## Blockers

- None.
