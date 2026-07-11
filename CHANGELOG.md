# Changelog

All notable changes are documented here. The project follows [Semantic Versioning](https://semver.org/) once releases
are tagged.

## Unreleased

### Changed

- Refocused Straw as a single-deployment, self-hosted HTTP/HTTPS egress proxy.
- Made NATS the only required backing service.
- Replaced tenant/API-key provisioning with an optional deployment-wide bearer token.
- Reduced the default local stack to NATS, Control, and one Egress worker.
- Converted production Compose assets into a small, explicit deployment pattern.
- Simplified the CLI and public Go/Python clients around `POST /api/v1/requests`.
- Rebuilt public documentation and repository community guidance for open-source use.
- Aligned the Go module and install examples with `github.com/beremaran/straw-oss/v2`.

### Removed

- Multi-tenancy, RBAC, quotas, rate limiting, audit/rollback, and administration APIs.
- Postgres, Redis, ClickHouse, object storage, and multi-Control coordination.
- Telemetry read APIs, payload capture, MITM, load-test tooling, and embedded admin UI.

## 0.1.0

Initial pre-release implementation.
