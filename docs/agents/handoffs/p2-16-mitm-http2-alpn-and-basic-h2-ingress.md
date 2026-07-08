# Handoff

Task: `docs/tasks/p2/16-ingress-http2-and-mitm-alpn.md`

## Changed

- [internal/control/mitm_connect_handler.go](file:///Users/beremaran/projects/wiseshopper/straw/internal/control/mitm_connect_handler.go): Added policy-gated MITM inner TLS ALPN and h2 server setup.
- [internal/control/mitm_handler.go](file:///Users/beremaran/projects/wiseshopper/straw/internal/control/mitm_handler.go): Added tenant MITM routing-policy check used by ALPN gating.
- [cmd/control/main.go](file:///Users/beremaran/projects/wiseshopper/straw/cmd/control/main.go): Wired `control.http2.enabled` into the built Control MITM CONNECT bootstrap.
- [cmd/control/main_test.go](file:///Users/beremaran/projects/wiseshopper/straw/cmd/control/main_test.go): Added MITM ALPN tests for disabled config, denied tenant policy, enabled h2, concurrent h2 requests, and HTTP/1.1 compatibility.
- [docs/tasks/p2/25-ingress-http2-stream-semantics.md](file:///Users/beremaran/projects/wiseshopper/straw/docs/tasks/p2/25-ingress-http2-stream-semantics.md): Added owner for the remaining ingress HTTP/2 stream semantics split out of the original task 16 scope.

## Acceptance Criteria Verdicts

Filled from the independent verifier's report plus the task split approved after the no-go:

| Criterion | Verdict | Implementation (file:line) | Proving test |
|-----------|---------|----------------------------|--------------|
| MITM ALPN behavior is implemented only where task 14 specifies it and only when both `control.http2.enabled` and tenant MITM routing policy allow it. | VERIFIED | [cmd/control/main.go:387](file:///Users/beremaran/projects/wiseshopper/straw/cmd/control/main.go#L387), [internal/control/mitm_connect_handler.go:168](file:///Users/beremaran/projects/wiseshopper/straw/internal/control/mitm_connect_handler.go#L168), [internal/control/mitm_handler.go:40](file:///Users/beremaran/projects/wiseshopper/straw/internal/control/mitm_handler.go#L40) | `TestConfigureMITMServerHTTP2ALPN` |
| A basic HTTP/2 MITM request is translated through the normal decoded MITM handler path and concurrent h2 streams each receive a response. | VERIFIED | [internal/control/mitm_connect_handler.go:150](file:///Users/beremaran/projects/wiseshopper/straw/internal/control/mitm_connect_handler.go#L150), [cmd/control/main_test.go:294](file:///Users/beremaran/projects/wiseshopper/straw/cmd/control/main_test.go#L294) | `TestConfigureMITMServerHTTP2ALPN` |
| HTTP/1.1 ingress remains compatible. | VERIFIED | [internal/control/mitm_connect_handler.go:155](file:///Users/beremaran/projects/wiseshopper/straw/internal/control/mitm_connect_handler.go#L155) | `TestConfigureMITMServerUsesAuthenticatedConnectBootstrap`, `TestConfigureMITMServerHTTP2ALPN` |
| Full ingress HTTP/2 stream semantics remain owned. | VERIFIED | [docs/tasks/p2/25-ingress-http2-stream-semantics.md](file:///Users/beremaran/projects/wiseshopper/straw/docs/tasks/p2/25-ingress-http2-stream-semantics.md) | Board row in `docs/tasks/p2.md` |

## Planning-Doc Coverage

| Planning item | Status | Evidence / owning task |
|---------------|--------|------------------------|
| `control.http2.enabled` controls whether Control offers h2 on public entrypoints. | implemented | [cmd/control/main.go:387](file:///Users/beremaran/projects/wiseshopper/straw/cmd/control/main.go#L387) |
| MITM inbound ALPN offers `h2` and `http/1.1` only when Control HTTP/2 is enabled and tenant policy permits HTTP/2. | implemented | [internal/control/mitm_connect_handler.go:168](file:///Users/beremaran/projects/wiseshopper/straw/internal/control/mitm_connect_handler.go#L168), [internal/control/mitm_handler.go:40](file:///Users/beremaran/projects/wiseshopper/straw/internal/control/mitm_handler.go#L40) |
| If the client selects h2, Control accepts it and routes requests as multiplexed streams. | implemented for basic decoded MITM requests | [cmd/control/main_test.go:294](file:///Users/beremaran/projects/wiseshopper/straw/cmd/control/main_test.go#L294) |
| If h2 is unavailable or disabled, Control falls back to HTTP/1.1. | implemented | [cmd/control/main_test.go:273](file:///Users/beremaran/projects/wiseshopper/straw/cmd/control/main_test.go#L273) |
| Stream cancellation mapping, NATS-credit flow control, pseudo-header/trailer edge behavior, and connection-level fanout. | out of scope | Owned by [docs/tasks/p2/25-ingress-http2-stream-semantics.md](file:///Users/beremaran/projects/wiseshopper/straw/docs/tasks/p2/25-ingress-http2-stream-semantics.md). |

## Verification

```sh
go test ./cmd/control -run TestConfigureMITMServerHTTP2ALPN -count=1
make check
```

Result:

- `go test ./cmd/control -run TestConfigureMITMServerHTTP2ALPN -count=1`: passed.
- `make check`: passed (`go test ./...`, `golangci-lint run --max-issues-per-linter 0 --max-same-issues 0`).
- Postgres-backed tests: not exercised (diff does not touch Postgres schema/store surfaces).
- Live compose verification: skipped because this task only wires MITM ALPN/basic h2 handler behavior; live full stream semantics are owned by task 25.

## Reviewer Start Points

- [internal/control/mitm_connect_handler.go](file:///Users/beremaran/projects/wiseshopper/straw/internal/control/mitm_connect_handler.go)
- [cmd/control/main_test.go](file:///Users/beremaran/projects/wiseshopper/straw/cmd/control/main_test.go)

## Remaining Work

- Full ingress HTTP/2 stream semantics were completed by [docs/tasks/p2/25-ingress-http2-stream-semantics.md](file:///Users/beremaran/projects/wiseshopper/straw/docs/tasks/p2/25-ingress-http2-stream-semantics.md).

## Blockers

- None.
