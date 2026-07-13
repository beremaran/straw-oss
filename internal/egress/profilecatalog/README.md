# tls-client profile catalogue snapshot

This package adapts the HTTP/1.1 and HTTP/2 profile catalogue distributed in
`github.com/bogdanfinn/tls-client@v1.15.1`. The upstream package is licensed
under the BSD-4-Clause text preserved verbatim in `LICENSE.tls-client`.

The generated snapshot deliberately excludes the upstream runtime and has no dependency
on `tls-client` or `fhttp`. The only mechanical adaptations are:

- package name `profiles` to `profilecatalog`;
- `github.com/bogdanfinn/fhttp/http2` types to compatible
  `golang.org/x/net/http2` types; and
- a local `Priority` wrapper because `x/net/http2` exposes `PriorityParam` but
  not fhttp's transport wrapper.

HTTP/3 fields remain inert source data so the pinned constructor signatures
stay reviewable against upstream; Straw does not execute or advertise HTTP/3.

Upstream source SHA-256 values:

| File | SHA-256 |
| --- | --- |
| `contributed_browser_profiles.go` | `937cdaca9ab1875a812f53d8c86da5e596bd9b9acbb51d6cdfbfe9506875c051` |
| `contributed_custom_profiles.go` | `cffe656c2612c408a2cadc935b1bad16a64ad3da100c127136b3ba8b5cde1e3d` |
| `internal_browser_profiles.go` | `61b631961f4affe655d9b0be025b9ab107b247a2bad872000857af8d4d81cccb` |
| `internal_custom_profiles.go` | `f7915a2249f5d282f018bdc5117051f903769931cde9d17adecc9afcb207ce99` |
| `profiles.go` | `ca41162911bf378251d77a97b20a65e462de4ee03d3f3635aaf604752044b151` |
