# Changelog

All notable changes are documented here. The project follows [Semantic Versioning](https://semver.org/) once releases
are tagged.

## Unreleased

### Added

- Added a standard HTTP/HTTPS forward-proxy ingress to Control, including absolute-form HTTP requests,
  policy-checked HTTP/1.1 CONNECT tunnels, proxy authentication, and bounded bidirectional NATS flow control.
- Added the Control API, NATS request transport, official Egress worker, CLI, and maintained Go and Python clients.
- Added deployment-wide authentication, destination policy, worker capacity and health tracking, cancellation,
  retries for replayable requests, phase timings, and Prometheus metrics.
- Added opt-in runtime administration backed by JetStream KV, including validated snapshots, audit history, rollback,
  rollout status, worker lifecycle controls, and an API-parity dashboard.
- Added opt-in highly available Control backed by Redis, including fenced worker sessions, shared routing state,
  request ownership, remote cancellation, readiness metrics, and failure drills.
- Added opt-in receipt transport with local and S3-compatible storage, resumable uploads, SHA-256 verification,
  assignment-scoped references, stored responses, retention cleanup, and lifecycle metrics.
- Added HTTP/1.1 and HTTP/2 fingerprint profiles from the attributed `tls-client` v1.15.1 catalogue, with isolated
  session caches for PSK profiles and local-wire conformance tests.
- Added independently versioned protocol, generated binding, and SDK repositories pinned by immutable tags.
- Added local Compose profiles, adaptable production examples, operational guidance, contract references, maintained
  examples, compatibility checks, and release automation for signed multi-architecture artifacts.
- Added dependency, license, secret, conformance, race, package-graph, documentation, clean-room, image-content,
  diagnostic-redaction, SBOM, provenance, and vulnerability verification gates.

### Security

- Bounded configuration, receipt-record, object-storage listing, request, response, and streaming parsers against
  malformed or hostile input.
- Pinned toolchains, dependencies, container builders, and GitHub Actions; release artifacts include checksums,
  inventories, attestations, signatures, and software bills of materials.
