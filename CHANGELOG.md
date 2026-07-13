# Changelog

All notable changes are documented here. The project follows [Semantic Versioning](https://semver.org/) once releases
are tagged.

## Unreleased

### Added

- Added public-only dependency, secret, conformance, race, and package-graph verification gates; normative Admin,
  Receipt, and compatibility references; and explicit support and governance policies.
- Added maintained curl, CLI, Go, and Python examples; source-surface, documentation ownership/freshness, terminology,
  internal/external link checks; namespaced default/admin/receipt Compose smoke tests; a threat model, launch/release
  gates, operations references, starter alerts, and pinned signed multi-architecture release automation.
- Added live HA, recovery, example, release-image content, diagnostic redaction, exact-toolchain, and dependency-license
  gates so operational and supply-chain claims have executable evidence.
- Replaced the unlicensed `fhttp` runtime path with a request-scoped transport built from licensed uTLS and
  `x/net/http2` primitives. Straw now exposes all 79 exact profile names from the attributed `tls-client` v1.15.1
  catalogue for HTTP/1.1 and HTTP/2, including isolated session caches for PSK profiles; HTTP/3 is intentionally
  excluded, and the entire catalogue plus representative TLS/HTTP/2 dimensions are local-wire tested.
- Added a moderate-or-higher npm audit gate and pinned the documentation site's transitive `uuid` dependency to the
  first patched 11.1.1 release; reviewed install scripts are now explicitly allowed or denied.

- Added an opt-in deployment-scoped Config and Admin REST API plus an API-parity web dashboard.
- Added validated atomic runtime snapshots, JetStream KV persistence, optimistic concurrency, bounded audit history,
  rollback, rollout status, and worker snapshot acknowledgement.
- Added worker inspect, drain, undrain, disable, enable, active-request inspection, and safe cancellation operations.
- Added local and adaptable production runtime-administration Compose profiles with recovery documentation.
- Added an opt-in Redis runtime-state backend for interchangeable Control instances, including fenced worker sessions,
  shared heartbeats/capacity/cooldowns/sticky routing, request ownership and remote cancellation, instance leases,
  configuration invalidation, HA readiness metrics, an HAProxy deployment example, and failure drills.
- Added an opt-in durable receipt-and-check transport with local and S3-compatible object stores, resumable parts,
  idempotent receipt creation, size/SHA-256 verification in Control and Egress, assignment-scoped signed references,
  cancellation and retention cleanup, stored response receipts, lifecycle metrics, Go/Python client support, and
  local/production deployment profiles.

### Changed

- Decomposed Control dispatch, Egress execution, and receipt handling into responsibility-focused files, made receipt
  lifecycle transitions explicit, and separated receipt record/index persistence behind an internal interface.
- Bounded static configuration, stored receipt record, and S3 list decoding to protect parser memory during malformed
  or hostile input.
- Aligned the supported module, CI, examples, and pinned container builder on Go 1.26.5.

- Moved the canonical worker schema and reproducible Go/Python bindings into public, independently tagged protocol
  repositories; the runtime now consumes `github.com/beremaran/straw-protos-go` at an exact version.
- Moved the public Go Control client and common Egress worker machinery to `github.com/beremaran/straw-sdk-go`; the
  canonical HTTP Egress implementation remains in this repository and consumes the exact public tagged SDK.
- Moved the `straw-sdk` Python distribution to `straw-sdk-python`; runtime integration now installs it and its
  `straw-protos` dependency from exact public Git tags.
- Refocused Straw as a single-deployment, self-hosted HTTP/HTTPS egress proxy.
- Made NATS the only required backing service.
- Replaced tenant/API-key provisioning with an optional deployment-wide bearer token.
- Reduced the default local stack to NATS, Control, and one Egress worker.
- Converted production Compose assets into a small, explicit deployment pattern.
- Simplified the CLI and public Go/Python clients around `POST /api/v1/requests`.
- Rebuilt public documentation and repository community guidance for open-source use.
- Aligned the Go module and install examples with `github.com/beremaran/straw-oss`.

### Removed

- Multi-tenancy, RBAC, quotas, rate limiting, and legacy account-scoped administration APIs.
- Postgres, ClickHouse, the previous mandatory object-storage path, and the previous mandatory
  Redis/multi-tenant coordination model.
- Telemetry read APIs, payload capture, MITM, and load-test tooling.

## 0.1.0

Initial pre-release implementation.
