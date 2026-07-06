# Handoff

Task: `docs/tasks/p1/02-http-proxy-ingress.md`

## Changed

- Added `internal/control/proxy_handler.go` for proxy auth, absolute-form target validation, header stripping, decoded request mapping, raw HTTP success/error rendering, and request metadata recording through the existing request pipeline.
- Wired an optional Control proxy listener on port `8081` with ordered raw-header capture so forwarded headers preserve wire order before `net/http` normalizes them.
- Added `control.server.proxy_enabled` and `control.server.proxy_port`, defaulting enabled proxy config to `8081` and rejecting other enabled ports.
- Added focused proxy/config/control tests for auth failures, revoked/unauthorized keys, header stripping/order, malformed requests, route-no-match `421`, raw response passthrough, and listener gating.

## Acceptance Criteria Verdicts

Filled from the independent verifier's report (straw-task-runner workflow step 12), not from the implementer's
self-assessment.

| Criterion | Verdict | Implementation (file:line) | Proving test |
|-----------|---------|----------------------------|--------------|
| HTTP forward proxy requests can use the P0 dispatch pipeline without the REST JSON envelope. | VERIFIED | `internal/control/proxy_handler.go:74`, `cmd/control/main.go:646`, `cmd/control/main.go:665` | `TestProxyHandlerMapsRequestAndWritesRawResponse` |
| `Proxy-Authorization` and internal routing headers are never forwarded outbound. | VERIFIED | `internal/control/proxy_handler.go:276`, `internal/control/proxy_handler.go:281` | `TestProxyHandlerStripsProxyAndInternalHeaders`, `TestProxyHeadersFromRawPreserveOrderAndStrip` |
| Port 8081 is only exposed when the listener is enabled. | VERIFIED | `cmd/control/main.go:239`, `cmd/control/main.go:246`, `internal/config/config.go:311`, `internal/config/config.go:426` | `TestBuildProxyHandlerOnlyWhenEnabled`, `TestLoadControl`, `TestLoadControlDefaultsProxyPort` |

## Planning-Doc Coverage

Every in-phase field/endpoint/behavior from the task's cited planning-doc sections, accounted for:

| Planning item | Status | Evidence / owning task |
|---------------|--------|------------------------|
| P1 may add proxy ingress endpoints. | implemented | `cmd/control/main.go:239`, `cmd/control/main.go:668` |
| P0 REST/config/admin APIs remain on port `8080`. | already existed | API mux remains separate from proxy listener in `cmd/control/main.go:661`. |
| HTTP forward proxy listens on port `8081` only when enabled. | implemented | `internal/config/config.go:311`, `internal/config/config.go:426`, `cmd/control/main.go:238` |
| `Proxy-Authorization: Bearer <api_key>` authenticates proxy requests. | implemented | `internal/control/proxy_handler.go:86`, `internal/control/proxy_handler_test.go:50` |
| `Authorization` remains an upstream header, not Straw proxy auth. | implemented | `internal/control/proxy_handler_test.go:50`, `internal/control/proxy_handler_test.go:108` |
| Missing, malformed, revoked, or unauthorized credentials fail before routing. | implemented | `internal/control/proxy_handler.go:43`, `internal/control/proxy_handler_test.go:50`, `internal/control/proxy_handler_test.go:81` |
| Absolute-form HTTP/HTTPS targets map to the decoded request model. | implemented | `internal/control/proxy_handler.go:116`, `internal/control/proxy_handler_test.go:14` |
| Relative targets, userinfo, empty hosts, IPv6 zone IDs, and fragments are rejected. | implemented | `internal/control/proxy_handler.go:126`, `internal/control/proxy_handler_test.go:145`, `internal/control/proxy_handler_test.go:189` |
| Methods are uppercase and known; CONNECT is rejected on this listener. | implemented | `internal/control/proxy_handler.go:117`, `internal/control/request.go:170`; raw CONNECT is owned by `docs/tasks/p1/05-raw-connect-tunnel.md`. |
| Header names, values, counts, and aggregate bytes use REST validation limits. | implemented | `internal/control/proxy_handler.go:217`, `internal/control/request.go:243`, `internal/control/proxy_handler_test.go:198` |
| Remaining headers preserve order and duplicates. | implemented | `cmd/control/main.go:284`, `internal/control/proxy_handler.go:157`, `internal/control/proxy_handler_test.go:216` |
| `Proxy-Authorization`, `X-Straw-*`, hop-by-hop, `Connection`-named, `Host`, `Content-Length`, and `Transfer-Encoding` are stripped. | implemented | `internal/control/proxy_handler.go:276`, `internal/control/proxy_handler.go:281`, `internal/control/proxy_handler_test.go:108` |
| Proxy request mapping sets `routing.ingress_type=http_proxy`. | implemented | `internal/control/proxy_handler.go:151`; route matching and worker capability gating are owned by `docs/tasks/p1/04-routing-ingress-type-and-worker-capability.md`. |
| Proxy responses do not use the REST JSON success envelope. | implemented | `internal/control/proxy_handler.go:331`, `internal/control/proxy_handler_test.go:14`; unbuffered streaming/backpressure was completed by `docs/tasks/p1/03-raw-streaming-response-path.md`. |
| `route_no_match` renders HTTP `421` for decoded proxy modes. | implemented | `internal/control/proxy_handler.go:320`, `internal/control/proxy_handler_test.go:240` |
| Redaction/security rules prevent proxy auth/internal headers from reaching metadata/logs/upstream. | implemented | Stripping happens before `DispatchInput` at `internal/control/proxy_handler.go:136`; metadata redaction already covers proxy auth in `internal/control/request_metadata.go`. |

## Verification

```sh
go test ./internal/control -run 'TestProxy'
go test ./cmd/control -run 'TestBuildProxyHandlerOnlyWhenEnabled'
go test ./internal/config -run 'TestLoadControl'
make check
```

Result: passed.

- Postgres-backed tests: not exercised; diff does not touch Postgres or migrations.
- Live compose verification: skipped because no Docker containers were running and `docker compose ps` returned no services for the root compose stack.

## Reviewer Start Points

- `internal/control/proxy_handler.go`
- `cmd/control/main.go`
- `internal/config/config.go`

## Remaining Work

- Full unbuffered proxy response streaming, backpressure, post-header error handling, and proxy-client cancellation were completed by `docs/tasks/p1/03-raw-streaming-response-path.md`.
- Routing `ingress_type` matching and worker capability gating were completed by `docs/tasks/p1/04-routing-ingress-type-and-worker-capability.md`.
- Raw CONNECT remains owned by `docs/tasks/p1/05-raw-connect-tunnel.md`.

## Blockers

- None.
