# Changelog

All notable changes are documented here. The project follows [Semantic Versioning](https://semver.org/) once releases
are tagged.

## Unreleased

### Added

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

- Moved the canonical worker schema and reproducible Go/Python bindings into private, independently tagged protocol
  repositories; the runtime now consumes `github.com/beremaran/straw-protos-go` at an exact version.
- Moved the public Go Control client and common Egress worker machinery to `github.com/beremaran/straw-sdk-go`; the
  canonical HTTP Egress implementation remains in this repository and consumes the exact tagged SDK.
- Moved the `straw-sdk` Python distribution to `straw-sdk-python`; runtime integration now installs it and its
  `straw-protos` dependency from exact private Git tags.
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
