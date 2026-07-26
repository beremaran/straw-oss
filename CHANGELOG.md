# Changelog

All notable changes are documented here. The project follows [Semantic Versioning](https://semver.org/) once releases
are tagged.

## Unreleased

## 0.1.0 - 2026-07-26

First tagged release. Install Egress before Control, then the CLI and application SDKs, per
[Compatibility and versioning](docs/public/compatibility.md). No migration applies: there is no earlier release to
upgrade from.

### Added

- Completed routing integration: the CLI and public Go/Python SDK tags expose typed routing hints and explicit
  replayability, and REST, absolute-form proxy, and CONNECT ingress share acceptance coverage for routing, pool
  eligibility, sticky behavior, destination policy, assignment fallback boundaries, and complete control-header
  stripping. The final public dependency order is `straw-protos`/bindings `v0.3.0`, SDKs `v0.2.0`, then Straw.
- Completed runtime-admin snapshot preparation: full pre-activation validation now covers routing conditions, sticky
  coherence, executor pools, destination rules, injections, fingerprint profiles, and worker settings; destination
  rules accept raw patterns and persist deterministic normalized fields without retaining stale derived values.
- Restored the backward-compatible `routing` object on `POST /api/v1/requests`, including validated tags, country,
  region, IP type, sticky-session hints, hard worker capability constraints, and routing coverage tests.
- Made executor pools enforceable across registration and routing: disabled pools receive no new assignments, pool
  executor types/tags/capabilities and degraded-worker policy are applied, and invalid or duplicate worker pool claims
  are rejected. Added `egress.capabilities.allowed_pools` to the official worker with default/default compatibility.
- Added a standard HTTP/HTTPS forward-proxy ingress to Control, including absolute-form HTTP requests,
  policy-checked HTTP/1.1 CONNECT tunnels, proxy authentication, and bounded bidirectional NATS flow control.
- Added authenticated `X-Straw-Route-*` proxy headers for bounded tags, country, region, IP type, and sticky-session
  routing hints. Control strips the entire `X-Straw-*` namespace before decoded forwarding or CONNECT establishment,
  and proxy assignment failures can safely fall back before the first client-visible response without replay after the
  raw response or `200 Connection Established` boundary.
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
- Gated container publication on the release verification job: a tag that fails the checks, the clean-room build, the
  race suite, or the license inventory publishes no Control or Egress image.
- Pinned `golang.org/x/text` to v0.39.0 and the documentation site's transitive `minimatch` to a release carrying a
  patched `brace-expansion`, so the release-blocking vulnerability and dependency-audit gates start clean.
