# Open-source readiness roadmap

Status: implementation complete; launch gates pending. Owner: maintainer.

Date: 2026-07-13

This roadmap unifies the repository-quality and documentation audits into one delivery plan for making Straw a
credible, self-service open-source project. The source audits remain the evidence record:

- `docs/planning/repository-quality-roadmap.md` covers implementation, repository, CI, security, release, and
  maintainership quality;
- `docs/planning/documentation-audit-roadmap.md` covers the public manual, contracts, operations, user journeys, and
  documentation quality system.

The roadmap does not expand the product boundary. Straw remains a small, self-hosted HTTP/HTTPS egress proxy; one
deployment is one trust boundary; NATS is the only required backing service. JetStream, Redis, and object storage
remain optional profile dependencies. Hosted tenants, RBAC, quotas, billing, administration platforms, and mandatory
databases remain out of scope.

## Outcome and current position

At the audit baseline, Straw had a coherent product boundary, a compact top-level runtime, pinned CI actions, useful
leaf-package tests, a working default Compose topology, and a broad documentation skeleton. The main obstacle was
trust rather than feature volume: an unaffiliated user could not reproduce the normal build because required protocol
and SDK repositories were private; a tracked legacy script contained a credential-like value; public identity and
toolchain instructions drifted; release automation was absent; core runtime responsibilities were concentrated in a
few very large files; and major public contracts and operational procedures were described but not specified or
tested. The evidence ledger records the implemented state and intentionally deferred publication gates.

The program therefore proceeds in dependency order: stop trust leaks, establish a reproducible public baseline,
stabilize code and contract boundaries, prove operational claims, then make release and maintenance routine.

| Area | Baseline assessment | Program outcome |
| --- | --- | --- |
| Public access | Release blocker | Clean unauthenticated checkout, dependencies, artifacts, and fork CI |
| Repository hygiene | Unsafe | No credential residue, stale identity, dead workflows, or ambiguous plans |
| Runtime design | Concentrated | Explicit state machines, focused files, and enforced package direction |
| Public contracts | Partial | Complete, versioned config/API/CLI/metric references with drift checks |
| Verification | Uneven | Risk-based unit, race, fuzz, integration, profile, and artifact tests |
| Operations and security | Partial | Threat model, hardening, observability, recovery, and executable runbooks |
| Releases | Missing | Reproducible signed binaries/images with compatibility and rollback guidance |
| Contribution and governance | Fragmented | Self-service workflow, ownership, support, decisions, and feedback loops |

## Priority rules

- **P0** blocks public promotion or safe contribution and is completed before feature work resumes.
- **P1** is required for a credible pre-1.0 release and may proceed in parallel only where dependencies allow.
- **P2** makes adoption and maintenance sustainable after the release baseline is trustworthy.
- Every behavior change updates tests, public documentation, compatibility notes where relevant, and `CHANGELOG.md`.
- Refactors begin with characterization tests and preserve behavior; package extraction requires a demonstrated
  boundary, not a file-size target.

## P0 — stop trust leaks and establish a public baseline

### OR-001 — Make the public checkout and installation paths independently usable

Choose and execute one publication model for protocol source, generated bindings, Go/Python clients, and worker SDKs.
The preferred model is immutable public tags with discoverable licenses, provenance, and compatibility. Until every
artifact is public, label the repository as a source preview and keep a fully usable REST/curl path.

Acceptance:

- a fresh unauthenticated checkout runs `go mod download`, `uv sync --frozen`, and `make check`;
- every documented installation command works without private credentials;
- fork pull requests receive useful unprivileged checks without maintainer-owned GitHub App credentials;
- a clean-room job proves the public-only dependency graph;
- a compatibility table covers Straw, protocol bindings, Go SDK, Python SDK, worker SDK, and artifact tags.

Sources: RQ-001, DOC-003.

### OR-002 — Remove and investigate credential-bearing legacy residue

Delete the tracked root `test.sh`, verify or revoke its `sk_live_...` credential, and investigate repository history
and provider logs as appropriate. Record incident evidence privately. Do not rewrite shared history without a
coordinated decision. Enable secret scanning and push protection with an owned false-positive process.

Acceptance:

- no credential-like literal or removed ClickHouse workflow remains in the tracked tree;
- revocation/rotation and history-handling decisions are recorded privately;
- automated secret scanning protects pushes and pull requests.

Source: RQ-002.

### OR-003 — Correct identity, quickstarts, navigation, and toolchain drift

Align repository name, clone directory, default branch, security URL, edit links, package map, support links, and the
supported Go/toolchain policy. Make the default quickstart a complete clone/start/call/inspect/stop/reset journey with
expected output. Replace misleading website navigation and generic landing-page assets with clear Learn, Use,
Operate, Reference, and Contribute paths.

Acceptance:

- an unaffiliated user completes the quickstart from a clean supported machine using one page;
- commands are exercised by a repeatable smoke test and links pass automated checks;
- the landing page states project status, version expectations, installation path, and support route;
- historical plans are clearly archival and cannot be mistaken for current instructions.

Sources: RQ-003, RQ-004 (toolchain), DOC-001, DOC-002.

### OR-004 — Replace aspirational status with an enforceable release gate

Turn `ROADMAP.md` into future outcomes and move delivered implementation detail into release notes or decision
records. Define launch criteria, supported artifacts and platforms, version matrix, cross-repository release order,
upgrade/rollback policy, and prerelease exceptions.

Acceptance:

- one launch checklist links each advertised claim to documentation and an executable test;
- binaries and multi-architecture images are reproducibly built and published with checksums, SBOMs, provenance,
  signatures, release notes, and post-publish smoke tests;
- the documented release graph states which steps are automated and who owns the rest.

Sources: RQ-004, RQ-043, DOC-015, DOC-042.

## P1 — stabilize implementation and contract boundaries

### OR-010 — Decompose Control around explicit state machines

Split `internal/control/dispatcher.go` by orchestration, assignment, request upload, decoded response, raw response,
protocol error mapping, and protocol conversion. Keep the package boundary stable until an extracted interface has
more than one real consumer. Characterize cancellation, disconnects, credit exhaustion, fallback timing,
out-of-order frames, partial responses, and receipt failures before moving code.

Acceptance:

- each file has one explainable reason to change and no behavior or dependency is added;
- decoded/raw transitions share only tested primitives whose semantics match;
- focused failure-path and race tests pass after each mechanical slice.

Source: RQ-010.

### OR-011 — Separate Egress resolution, policy, transport, pooling, and execution

Split `internal/egress/executor.go` into lifecycle orchestration, resolver/CNAME handling, resolved-address enforcement,
HTTP/TLS/HTTP2 transport and retry classification, connection pooling, request/response translation, and CONNECT.
Document why Control pre-resolution policy and Egress post-resolution enforcement differ before sharing pure
invariants.

Acceptance:

- behavior is preserved under DNS rebinding, CNAME, redirect, proxy, pooled-IP rotation, HTTP/2 fallback, TLS,
  metadata-range, IPv4/IPv6, and tunnel tests;
- any shared policy primitive has parity tests and a narrowly defined responsibility.

Sources: RQ-011, RQ-042.

### OR-012 — Make receipt lifecycle, persistence, and composition explicit

Define receipt states and legal transitions in one place. Introduce internal record/index persistence interfaces, then
separate upload, multipart completion, assignment authorization, cleanup, serialization, and key layout. Make
concurrency and HA guarantees visible rather than implicit in an in-memory orchestration lock.

Acceptance:

- every state transition has a focused test;
- idempotency and cleanup races, partial writes, corrupt metadata, expiration boundaries, and cancellation/commit
  races have deterministic coverage;
- persistence and object composition can change without changing lifecycle policy.

Source: RQ-012.

### OR-013 — Clarify composition and enforce the intended package graph

Rename `cmd/control/open_source.go` around its wiring responsibility, separate optional profile assembly from HTTP
server construction, and keep commands limited to flags, lifecycle, health, composition, and exit behavior. Replace
the grep-only architecture guard with a tested `go list`-based allowed-import graph while retaining exact external-pin
assertions.

Acceptance:

- composition invariants are visible through typed dependency structures where useful;
- CI rejects forbidden dependencies among commands, Control, Egress, config, NATS, receipt, and object storage;
- test helpers remain local unless multiple packages genuinely consume them.

Sources: RQ-013, RQ-014.

### OR-014 — Publish a versioned normative contract set

Create one linked reference set for:

- all static Control/Egress config fields, defaults, constraints, secrets, cross-field rules, and restart behavior;
- runtime snapshot objects, validation, ordering, policy precedence, lifecycle controls, and safe recipes;
- Request API authentication, limits, headers, fields, timing, redirects, cancellation, retries, and every stable error;
- all Admin API and Receipt API endpoints, schemas, concurrency/idempotency rules, authorization, state, and errors;
- CLI flags, exit codes, output stability, environment precedence, files, signals, timeouts, and receipt workflows;
- every metric's type, labels, unit, profile, and interpretation;
- compatibility and deprecation rules across REST, JSON, config, snapshots, protobuf/NATS, SDKs, and containers.

Acceptance:

- every public config field, route, error, CLI flag, and metric maps to a normative reference;
- representative positive and negative examples execute against the shipped implementation;
- source-backed coverage checks fail when a public surface changes without reference and compatibility updates.

Sources: DOC-010 through DOC-015, DOC-031, DOC-050.

### OR-015 — Remove ambiguity and dead structure

Decide the fate of `scripts/mitm-h2-request.go`, deprecated Make aliases, generic site assets, empty examples, stale
split plans, and local empty remnants. Normalize responsibility-based file names and documentation URL conventions.
Keep one canonical agent/contributor automation source, generating projections only for real consumers with a parity
check.

Acceptance:

- `scripts/`, `examples/`, and `docs/planning` have documented, consistently enforced purposes;
- current plans carry status/date/ownership and historical plans live in an explicit archive or Git history;
- no duplicated agent skill can silently drift.

Sources: RQ-020 through RQ-022.

## P1 — prove reliability, deployment, and security claims

### OR-020 — Install a risk-based layered verification strategy

Publish a test matrix connecting product claims to pure/state-machine tests, package integrations with real optional
services, process lifecycle tests, Compose profile smoke tests, cross-version compatibility, and published artifact
tests. Report package coverage as a diagnostic rather than enforcing one vanity percentage. Prioritize negative paths,
concurrency, cancellation, recovery, retry, and idempotency.

Acceptance:

- Control, command wiring, CLI, and other high-risk low-coverage areas have explicit target scenarios;
- `go test -race` runs over the supported scope;
- bounded fuzz targets cover JSON, URLs/headers, NATS envelopes, DNS, object metadata, receipt records, and frame
  sequences;
- failures preserve useful artifacts and are reproducible locally.

Sources: RQ-030, RQ-031.

### OR-021 — Make conformance and CI trust boundaries executable

Give conformance fixtures a versioned manifest, schema, expected outcomes, producer/consumer matrix, and a root
`make conformance` target. Separate fast public-source checks, trusted cross-repository compatibility, scheduled
robustness, and protected release jobs. Reuse setup, pin tools consistently, cancel superseded runs, set timeouts, and
minimize token permissions.

Acceptance:

- orphaned fixtures and unsupported protocol changes fail with actionable compatibility guidance;
- untrusted pull-request code cannot mint cross-repository or publishing credentials;
- local commands and required remote checks have an explicit parity map.

Sources: RQ-032, RQ-034, RQ-041.

### OR-022 — Test deployed behavior, not only configuration rendering

Retain fast Compose render checks and add isolated ephemeral smoke tests for default, admin, HA, and receipt profiles:
startup, authentication, readiness, one request, worker scaling and graceful shutdown, snapshot persistence, Control
loss, Redis degradation/recovery, receipt upload/download/corruption, and teardown. Namespace resources and make every
destructive target explicit.

Acceptance:

- each advertised profile has a repeatable owned-infrastructure test;
- deployment docs include selectable patterns, sizing inputs, ports/volumes/permissions, TLS/proxy/IPv6 behavior,
  graceful shutdown, and profile-specific upgrade/rollback order;
- at least one reverse-proxy/TLS example is validated; orchestrator examples follow demonstrated user demand.

Sources: RQ-033, DOC-020.

### OR-023 — Codify the security and supply-chain model

Create one threat model and executable invariant suite covering SSRF, DNS changes, redirects, proxies, CONNECT,
header injection, credential redaction, NATS subjects, admin separation, Redis degradation, receipt signatures,
stored objects, custom workers, and request limits. Add dependency review, CodeQL or equivalent, `govulncheck`, image
scanning, license inventory, scheduled updates, and owned triage/exception policies.

Acceptance:

- profile-specific hardening and verification checklists cover secrets, TLS, NATS/Redis/S3 permissions, retention,
  logging, and network enforcement;
- Control policy projection and Egress resolved-address enforcement cannot silently diverge;
- release images are minimal, non-root, digest-pinned, labelled, health-aware, and verified for intended contents.

Sources: RQ-040, RQ-042, RQ-043, DOC-022.

### OR-024 — Create an executable operations handbook

Document observability, suggested SLIs/SLOs, starter alerts/dashboard, structured log fields and redaction, backup and
restore, upgrades, and incident diagnosis. Use a consistent troubleshooting format: symptom, fast check, likely
causes, confirmation, fix, and safe escalation bundle.

Acceptance:

- runbooks cover worker loss/saturation, NATS, Redis, JetStream, object storage, latency, checksum failures, rollout,
  disk, credentials, partial upgrades, protocol mismatch, DNS/TLS/redirects, HTTP/2/fingerprints, shutdown, and SDK/CLI;
- backup/restore and failure-drill commands are tested only against owned isolated resources;
- diagnostic bundles redact tokens, URLs, headers, and body data.

Sources: DOC-021, DOC-023.

## P2 — make adoption and maintenance self-service

### OR-030 — Build maintained learning paths and examples

Add concise lifecycle/topology/ownership/state diagrams and executable examples for curl, CLI, Go, and Python; ordered
and duplicate headers; bodies; errors and safe retry; receipts; authentication; routing/policy; scaling; and
observability. Expand SDK and custom-worker guidance only for artifacts users can obtain.

Acceptance:

- each example states prerequisites, tested versions, expected output, and cleanup;
- SDK guidance covers lifecycle, timeout/context, errors/retries, concurrency, cleanup, logging, and compatibility;
- custom-worker guidance covers registration, capabilities, flow control, cancellation, deadlines, snapshots,
  receipts, shutdown, conformance, security duties, and unstable protocol areas.

Sources: DOC-030, DOC-032 through DOC-034.

### OR-031 — Make contribution, decisions, and releases reproducible

Create a contributor handbook covering setup, package graph, focused/integration/race testing, fixtures, local CI,
generated/external sources, public-surface changes, documentation authoring, changelog/compatibility criteria, and
supported tooling. Record lightweight ADRs for durable product and protocol invariants. Provide a discoverable
`make help` and an exact release procedure.

Acceptance:

- a new contributor can change a public contract and satisfy all required checks without maintainer memory;
- ADRs preserve the trust boundary, required/optional services, repository split, ordered headers, bounded streams,
  runtime snapshots, Redis fencing, and receipt verification;
- release instructions match the automated release gate in OR-004.

Sources: RQ-050, RQ-051, RQ-053, DOC-040 through DOC-042.

### OR-032 — Establish governance, ownership, and project pathways

Publish maintainer roles, decision/appeal process, response expectations, supported versions, release cadence intent,
security handling, and a maintained place for usage questions. Route issue/PR templates across runtime, protocol,
bindings, and SDK repositories. Add `CODEOWNERS` only where it reflects real reviewers.

Acceptance:

- support and maintenance status are discoverable from the README and website;
- issue and PR templates capture compatibility, security, operations, fixtures, docs, and release impact without
  becoming ceremonial checklists;
- documentation has page ownership, specialist review rules, freshness dates, and a quarterly review process.

Sources: RQ-050, RQ-052, DOC-043, DOC-051.

### OR-033 — Make documentation quality continuous

Run the site build, internal/external link checks, spelling/terminology, Markdown lint, and code-block validation in
CI. Smoke-test volatile quickstarts and profiles on a schedule and before release. Add contextual issue links; adopt
privacy-compatible analytics only with a documented purpose and use actual feedback to prioritize platform recipes.

Acceptance:

- documentation checks are ordinary required product checks rather than release-end cleanup;
- every major page has an owner, review date, feedback path, and tested-command evidence;
- failed searches, setup exits, and issue themes can inform work without creating an unmaintained platform matrix.

Sources: RQ-054, DOC-050 through DOC-052.

## Delivery sequence and dependencies

### Milestone 0 — Trustworthy first contact (days; blocks promotion)

Complete OR-002 and OR-003. Decide OR-001's publication model and correct claims immediately. Define OR-004's launch
gate. Freeze new feature work until credential response, public access, identity, and quickstart correctness are
resolved.

### Milestone 1 — Reproducible public baseline (1–2 weeks)

Complete OR-001 and the first usable OR-004 release path. Separate privileged CI, align toolchains, add clean-room
verification and baseline security gates, and publish a minimal signed prerelease whose artifacts pass their own
smoke tests.

### Milestone 2 — Controlled architecture and contracts (2–4 weeks; incremental PRs)

Begin OR-010 through OR-013 with characterization/race tests, moving one responsibility per PR. Complete OR-014 and
OR-015 alongside them so the public contract and repository structure converge rather than drift independently.

### Milestone 3 — Evidence for self-hosting claims (2–4 weeks)

Complete OR-020 through OR-024. Make all four deployment profiles, protocol conformance, network invariants,
observability, recovery, and operator procedures executable in isolated CI.

### Milestone 4 — Sustainable adoption and maintenance (ongoing)

Complete OR-030 through OR-033. Promote the prerelease only after the release gate, public-only graph, compatibility
matrix, upgrade path, documentation checks, and advertised profile tests all pass.

| Prerequisite | Enables |
| --- | --- |
| OR-002 credential response | safe continued public development |
| OR-001 publication model | public CI, SDK docs, compatibility tests, releases, external contributions |
| OR-003 toolchain and identity alignment | builds, contributor setup, support matrix, links, website trust |
| OR-004 release/version policy | artifact builds, compatibility promises, upgrades and rollback |
| OR-020 characterization tests | safe decomposition in OR-010 through OR-012 |
| OR-013 package graph | prevention of boundary regression during decomposition |
| OR-021 CI trust separation and conformance | safe fork contributions and cross-repository releases |
| OR-022 profile smoke tests | credible deployment, operations, and release claims |
| OR-014 normative contracts | stable tutorials, SDK docs, support, and drift detection |

## Program definition of done

The unified program is complete when:

- an unaffiliated user can obtain every advertised dependency and artifact, then clone, build, check, start, call,
  inspect, and stop Straw without private credentials or undocumented knowledge;
- no tracked secret, removed-subsystem script, broken identity, misleading plan, or duplicated automation source remains;
- core runtime code is organized around explicit state machines and responsibilities, with dependency direction
  mechanically enforced;
- every public config field, route, error, CLI flag, metric, protocol surface, and compatibility rule has a tested
  normative reference;
- high-risk policy, parser, streaming, concurrency, cancellation, retry, idempotency, and recovery behavior has unit,
  race, fuzz, integration, conformance, and failure-path evidence proportional to risk;
- every advertised deployment profile and published artifact is smoke-tested in isolated infrastructure;
- fork CI is useful and unprivileged, while compatibility and publishing credentials are protected;
- releases produce reproducible public binaries and images with checksums, SBOMs, provenance, signatures, upgrade and
  rollback guidance, release notes, and post-publish verification;
- operators have profile selection, hardening, sizing, observability, backup/restore, upgrade, rollback, and incident
  guidance validated against owned infrastructure;
- contributors can make and release routine changes using explicit ownership, governance, support, decision, testing,
  documentation, and compatibility processes;
- documentation build, link, terminology, example, drift, ownership, freshness, and feedback checks run continuously.

## Traceability

The source IDs remain stable audit references. This mapping demonstrates that consolidation did not discard a finding.

| Unified item | Repository audit | Documentation audit |
| --- | --- | --- |
| OR-001 | RQ-001 | DOC-003 |
| OR-002 | RQ-002 | — |
| OR-003 | RQ-003, RQ-004 | DOC-001, DOC-002 |
| OR-004 | RQ-004, RQ-043 | DOC-015, DOC-042 |
| OR-010 | RQ-010 | — |
| OR-011 | RQ-011, RQ-042 | — |
| OR-012 | RQ-012 | — |
| OR-013 | RQ-013, RQ-014 | — |
| OR-014 | — | DOC-010, DOC-011, DOC-012, DOC-013, DOC-014, DOC-015, DOC-031, DOC-050 |
| OR-015 | RQ-020, RQ-021, RQ-022 | — |
| OR-020 | RQ-030, RQ-031 | — |
| OR-021 | RQ-032, RQ-034, RQ-041 | — |
| OR-022 | RQ-033 | DOC-020 |
| OR-023 | RQ-040, RQ-042, RQ-043 | DOC-022 |
| OR-024 | — | DOC-021, DOC-023 |
| OR-030 | — | DOC-030, DOC-032, DOC-033, DOC-034 |
| OR-031 | RQ-050, RQ-051, RQ-053 | DOC-040, DOC-041, DOC-042 |
| OR-032 | RQ-050, RQ-052 | DOC-043, DOC-051 |
| OR-033 | RQ-054 | DOC-050, DOC-051, DOC-052 |

The audit methods, source inventories, measured coverage baseline, and limitations remain in the two source audits and
are intentionally not duplicated here.
