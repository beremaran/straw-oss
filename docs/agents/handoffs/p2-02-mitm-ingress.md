# Handoff

Task: `docs/tasks/p2/02-mitm-ingress.md`

## Changed

- Added MITM static config fields and validation for `mitm_enabled`, `mitm_port`, and operator CA cert/key paths in `internal/config/config.go`.
- Added `internal/control/mitm_handler.go`, which authenticates with proxy credentials, validates TLS, rejects Host/SNI mismatches, maps origin-form HTTPS requests into `ValidatedRequest`, strips proxy/internal headers, and dispatches through the existing request pipeline with `ingress_type=mitm`.
- Wired Control to build the MITM handler and start a server-side `crypto/tls` listener on port 8083 only when MITM is enabled in `cmd/control/main.go`.
- Added focused tests for MITM mapping, TLS requirement, SNI mismatch, denied destination policy, raw response dispatch, listener TLS handshake/shutdown, generated leaf certs, and config/handler gating.

## Acceptance Criteria Verdicts

Filled from independent verifier report `019f3ac9-ec63-7f70-9055-b976d4b0b909`.

| Criterion | Verdict | Implementation (file:line) | Proving test |
|-----------|---------|----------------------------|--------------|
| MITM HTTPS requests enter the existing dispatch pipeline as decoded requests. | VERIFIED | `internal/control/mitm_handler.go:53`, `cmd/control/main.go:959`, `cmd/control/main.go:1052` | `TestMITMHandlerMapsDecodedTLSRequest`, `TestMITMRawDispatcherWritesDecodedResponse`, `make check` |
| The server TLS implementation is not confused with outbound TLS fingerprinting. | VERIFIED | `cmd/control/main.go:330`; no outbound fingerprint profile fields touched by MITM handler | `TestGenerateMITMLeafSignsServerCertificate`, `make check` |
| Port 8083 is mapped only when MITM is enabled. | VERIFIED | `internal/config/config.go:95`, `internal/config/config.go:411`, `internal/config/config.go:537`, `cmd/control/main.go:287` | `TestBuildMITMHandlerOnlyWhenEnabled`, `TestLoadControlDefaultsMITMPort`, `TestLoadControl` |

## Planning-Doc Coverage

| Planning item | Status | Evidence / owning task |
|---------------|--------|------------------------|
| MITM uses the same decoded internal request model as REST and HTTP proxy. | implemented | `internal/control/mitm_handler.go:97`, `internal/control/mitm_handler.go:112` |
| Inbound TLS termination is server-side TLS using Go `crypto/tls` or another server-capable TLS implementation. | implemented | `cmd/control/main.go:330`, `cmd/control/main.go:308`; `TestMITMServerTerminatesTLSAndShutsDown` |
| MITM must not claim to change client JA3/JA4 fingerprinting. | implemented | MITM code does not touch fingerprint fields; `cmd/control/main.go:335` limits ALPN to HTTP/1.1 |
| Operators provide CA material through static config. | implemented | `internal/config/config.go:95`, `cmd/control/main.go:335` |
| Generated per-SNI certificate is a leaf certificate. | implemented | `cmd/control/main.go:397`, `TestGenerateMITMLeafSignsServerCertificate` |
| Leaf cert cache/storage policy. | out of scope | Owned by `docs/tasks/p2/20-mitm-leaf-cert-cache.md`; this task generates uncached leaves. |
| Routing match condition supports `ingress_type=mitm`. | already existed / implemented path | `internal/control/request.go:28`; MITM sets it at `internal/control/mitm_handler.go:117` |
| Port 8083 is MITM proxy and unused ports are not mapped before enabled. | implemented | `internal/config/config.go:411`, `cmd/control/main.go:287` |
| Destination deny normalization includes SNI vs Host mismatches and private/metadata denial. | implemented | `internal/control/mitm_handler.go:92`, `internal/control/mitm_handler.go:123`, `internal/control/mitm_handler_test.go:124` |
| Header stripping removes `Proxy-Authorization`, `X-Straw-*`, and hop-by-hop headers. | implemented | Reuses `proxyHeaders`; proved by `TestMITMHandlerMapsDecodedTLSRequest` |
| HTTP/2 ALPN for MITM. | out of scope | Owned by `docs/tasks/p2/16-ingress-http2-and-mitm-alpn.md`. |

## Verification

```sh
go test ./internal/control ./cmd/control -run 'TestMITM|TestGenerateMITM|TestBuildMITM'
make check
```

Result:

- Focused tests: passed.
- `make check`: passed.
- Postgres-backed tests: not required for this diff; no Postgres stores or migrations changed.
- Live compose verification: skipped because the compose Control config does not enable MITM or provide CA cert/key material. The real TLS listener and shutdown path are covered by `TestMITMServerTerminatesTLSAndShutsDown`.

## Reviewer Start Points

- `internal/control/mitm_handler.go`
- `cmd/control/main.go`
- `internal/control/mitm_handler_test.go`
- `cmd/control/main_test.go`

## Remaining Work

- Certificate cache/storage is owned by `docs/tasks/p2/20-mitm-leaf-cert-cache.md`.
- MITM HTTP/2 ALPN is owned by `docs/tasks/p2/16-ingress-http2-and-mitm-alpn.md`.

## Blockers

- None.
