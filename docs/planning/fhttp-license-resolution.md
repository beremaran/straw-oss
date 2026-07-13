# Fingerprint transport license resolution

Status: resolved. Owner: maintainer. Date: 2026-07-13.

`make license-check` inventories every Go module after `go mod download all` and every npm package installed from the
exact website lock. It exposed that `github.com/bogdanfinn/fhttp@v0.6.8` had no top-level license, copying, or notice
file. Straw and `github.com/bogdanfinn/tls-client` both depended on it, so removing only Straw's direct import would
not have resolved redistribution.

Straw resolved the issue without copying or inferring a license for `fhttp`. The request-scoped transport uses
licensed `github.com/bogdanfinn/utls` and `golang.org/x/net/http2` primitives, and Straw owns the HTTP/1.1 and HTTP/2
request state machines. The runtime dependencies on both `fhttp` and `tls-client` and their unused transitive graph
were removed by `go mod tidy`.

Straw adapts the complete HTTP/1.1 and HTTP/2 profile catalogue published by `tls-client` v1.15.1 under its
BSD-4-Clause file. The upstream text is preserved beside the adapted source and in `THIRD_PARTY_NOTICES.md`, which is
included with binary releases and the Egress image. Source hashes and the narrow mechanical adaptations are recorded
in `internal/egress/profilecatalog/README.md`. HTTP/3 data is not executed or advertised.

The replacement preserves the catalogue's TLS and HTTP/2 profile dimensions, pinned DNS/address enforcement, no
redirects, request-scoped connections, cancellation/deadlines, HTTP/2 flow control/error mapping, streamed responses,
and late trailers. PSK-capable profiles use isolated bounded session caches and omit empty PSK offers. A maximum 1 MiB
inline upload test exercises peer flow-control windows. The local wire observer exercises every advertised profile
and independently verifies representative TLS/HTTP2 dimensions.

Resolution evidence (all green):

- `make license-check` passes with a generated inventory attached to release artifacts;
- `go mod why -m github.com/bogdanfinn/fhttp` and `go mod why -m github.com/bogdanfinn/tls-client` report no dependency;
- `make check race fuzz-smoke conformance` passes;
- the profiled package passes repeated and race-enabled tests, including wire conformance and maximum inline upload;
- `go mod verify`, the reproducible artifact/SBOM gate, and the license inventory cover the resulting graph.
