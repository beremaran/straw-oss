# Handoff

Task: `docs/tasks/p2/04-mitm-authenticated-connect-bootstrap.md`

## Changed

- Replaced the runtime MITM listener with explicit-proxy CONNECT bootstrap: port 8083 now runs plain HTTP CONNECT, authenticates first, then starts inner server-side TLS on the hijacked connection.
- Added `MITMConnectHandler` to reuse CONNECT auth/target/hijack/`200 Connection Established` behavior and pass the CONNECT identity into decoded MITM dispatch.
- Added `MITMLeafLookup`, called from the inner TLS `GetCertificate` path with authenticated identity, normalized SNI, and normalized CONNECT authority.
- Kept decoded MITM request mapping/header stripping/`ingress_type=mitm` in the existing MITM handler, with CONNECT identity used before any inner-request proxy auth.
- Added focused socket-level tests for CONNECT bootstrap, auth failure before leaf lookup, SNI mismatch before leaf lookup, decoded MITM dispatch, and direct TLS path removal.

## Acceptance Criteria Verdicts

Filled from the independent verifier's report.

| Criterion | Verdict | Implementation (file:line) | Proving test |
|-----------|---------|----------------------------|--------------|
| MITM port 8083 accepts explicit-proxy CONNECT, authenticates CONNECT, writes `200 Connection Established`, then serves inner HTTPS through decoded MITM dispatch. | VERIFIED | `cmd/control/main.go:310`, `internal/control/mitm_connect_handler.go:32`, `internal/control/mitm_connect_handler.go:38`, `internal/control/mitm_connect_handler.go:62`, `internal/control/mitm_connect_handler.go:100`, `internal/control/mitm_handler.go:141` | `TestMITMConnectAuthenticatesBeforeLeafLookupAndServesInnerRequest`, `TestConfigureMITMServerUsesAuthenticatedConnectBootstrap` |
| Leaf lookup/generation hook receives authenticated tenant identity plus normalized SNI/CONNECT authority before certificate selection. | VERIFIED | `internal/control/mitm_connect_handler.go:67`, `internal/control/mitm_connect_handler.go:75`, `internal/control/mitm_connect_handler.go:82`, `cmd/control/main.go:353` | `TestMITMConnectAuthenticatesBeforeLeafLookupAndServesInnerRequest` |
| Inner HTTPS requests do not need `Proxy-Authorization`; they use CONNECT-authenticated identity. | VERIFIED | `internal/control/mitm_connect_handler.go:104`, `internal/control/mitm_handler.go:41`, `internal/control/mitm_handler.go:45` | `TestMITMConnectAuthenticatesBeforeLeafLookupAndServesInnerRequest` |
| The old direct TLS `GetCertificate` MITM path is removed or fails closed and cannot write tenant-scoped cache entries without tenant identity. | VERIFIED | `cmd/control/main.go:323`, `cmd/control/main.go:326`, `cmd/control/main.go:345`, `cmd/control/main.go:353` | `TestConfigureMITMServerUsesAuthenticatedConnectBootstrap` |
| No encrypted leaf cache, Redis lock, singleflight, or flood-control behavior is implemented in this task. | VERIFIED | `internal/control/mitm_connect_handler.go:21`, `internal/control/mitm_connect_handler.go:82`, `cmd/control/main.go:353` | Diff review; `rg` found no cache/lock/singleflight/flood-control calls in changed MITM wiring |

## Planning-Doc Coverage

| Planning item | Status | Evidence / owning task |
|---------------|--------|------------------------|
| Leaf generation/cache lookup require authenticated tenant identity before any cache key, KMS AAD, or per-tenant flood limit. | implemented for runtime prerequisite | `internal/control/mitm_connect_handler.go:38`, `internal/control/mitm_connect_handler.go:70`, `internal/control/mitm_connect_handler.go:82`; cache keys/KMS AAD/flood limits remain owned by `docs/tasks/p2/20-mitm-leaf-cert-cache.md`. |
| Explicit-proxy MITM authenticates CONNECT first, then performs inner server-side TLS handshake for the CONNECT authority. | implemented | `internal/control/mitm_connect_handler.go:32`, `internal/control/mitm_connect_handler.go:38`, `internal/control/mitm_connect_handler.go:62`, `internal/control/mitm_connect_handler.go:86` |
| SNI remains host-validation/cache-scoping input, not tenant identity source. | implemented | `internal/control/mitm_connect_handler.go:75`, `internal/control/mitm_connect_handler.go:77`, `internal/control/mitm_connect_handler.go:82` |
| Direct TLS `GetCertificate` path must be replaced or fail closed before tenant-scoped cache storage ships. | implemented | `cmd/control/main.go:323`, `cmd/control/main.go:326`, `cmd/control/main.go:353` |
| Inbound MITM TLS uses Go server-side TLS boundary. | implemented | `internal/control/mitm_connect_handler.go:67`, `internal/control/mitm_connect_handler.go:86` |
| MITM uses decoded internal request model and preserves `ingress_type=mitm`. | already existed / preserved | `internal/control/mitm_handler.go:136`, `internal/control/mitm_handler.go:141`; exercised through `TestMITMConnectAuthenticatesBeforeLeafLookupAndServesInnerRequest`. |
| Routing can match `ingress_type=mitm`. | already existed | `internal/control/config_admin_handlers.go:397`, `internal/control/routing_test.go:194` |
| Proxy-Authorization is the proxy auth boundary and must be stripped from forwarded headers. | already existed / preserved | CONNECT auth: `internal/control/connect_handler.go:85`; inner identity: `internal/control/mitm_handler.go:41`; stripping via `internal/control/proxy_handler.go:180`; existing `TestMITMHandlerMapsDecodedTLSRequest`. |
| SNI vs Host mismatch and CONNECT target host/port are normalized/validated. | implemented / preserved | CONNECT target: `internal/control/connect_handler.go:115`; mismatch before leaf: `internal/control/mitm_connect_handler.go:77`; decoded request mismatch: `internal/control/mitm_handler.go:116`; `TestMITMConnectRejectsSNIMismatchBeforeLeafLookup`. |
| MITM port purpose is 8083. | implemented through existing config/default and runtime binding | `cmd/control/main.go:310`; verifier also cited `internal/config/config.go:423` defaulting enabled MITM to 8083. |
| Encrypted leaf cache storage, Redis locks, local singleflight, TTLs, and flood controls. | completed later | Closed by `docs/tasks/p2/20-mitm-leaf-cert-cache.md`. |
| MITM HTTP/2 ALPN. | out of scope | Owned by `docs/tasks/p2/16-ingress-http2-and-mitm-alpn.md`; this task keeps `http/1.1` only at `internal/control/mitm_connect_handler.go:69`. |
| Tenant-admin CA configure/rotate APIs. | out of scope | Owned by `docs/tasks/p2/18-mitm-ca-configure-rotate-api.md`. |

## Verification

```sh
go test ./internal/control ./cmd/control -run 'TestMITM|TestConnect|TestConfigureMITM'
make check
```

Result:

- Focused MITM/CONNECT tests: passed.
- Independent verifier reran the focused MITM/CONNECT test command and returned PASS for all acceptance criteria.
- `make check`: passed (`go test ./...` plus `golangci-lint run --max-issues-per-linter 0 --max-same-issues 0`, 0 issues).
- Postgres-backed tests: not exercised; diff does not touch Postgres files or migrations.
- Live compose verification: skipped because the current compose deployment does not enable MITM in `deploy/docker/control.json` and `docker-compose.yml` does not publish port 8083. The runtime listener shape is covered by socket-level tests in `cmd/control` and `internal/control`.

## Reviewer Start Points

- `internal/control/mitm_connect_handler.go`
- `internal/control/mitm_handler.go`
- `cmd/control/main.go`
- `internal/control/mitm_handler_test.go`
- `cmd/control/main_test.go`

## Remaining Work

- None for this task. Encrypted leaf cache storage, Redis locks, local singleflight, TTLs, and flood controls were completed by `docs/tasks/p2/20-mitm-leaf-cert-cache.md`.

## Blockers

- None.
