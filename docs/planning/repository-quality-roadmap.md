# Repository quality audit and roadmap

Status: archived evidence. Owner: maintainer. Superseded by `open-source-readiness-roadmap.md`.

Date: 2026-07-13

This audit evaluates the current `straw-oss` repository against its stated goal: a small, self-hosted HTTP/HTTPS
egress proxy that an unaffiliated user can understand, build, run, inspect, change, and release. It covers tracked
repository structure, package boundaries, implementation concentration, dependencies, scripts, tests, CI, release
engineering, deployment assets, security hygiene, community metadata, and maintainership.

The companion `docs/planning/documentation-audit-roadmap.md` covers the public manual in greater depth. That file was
untracked during this audit and was not modified.

## Executive assessment

Straw has a coherent product boundary, a small top-level runtime, disciplined linting, pinned GitHub Actions, good
unit tests in several leaf packages, and a default Compose stack that matches the advertised NATS/Control/Egress
model. `make check` passes. These are strong foundations.

The repository is not yet ready to earn broad open-source trust. The most serious problem is that the source is
public while required protocol and SDK dependencies are explicitly private. A clean external checkout therefore
cannot reproduce the maintainer's build or CI path. The repository also contains a tracked ad hoc script with a
live-looking bearer token and a query for a removed ClickHouse subsystem. Public links point to retired names and
branches, the roadmap describes nearly everything as delivered without exposing a credible release path, and CI
requires privileged cross-repository credentials even for ordinary pull requests.

Internally, the package list is compact but several packages are compact in name only. `internal/control/dispatcher.go`
is 1,827 lines, `internal/egress/executor.go` is 1,714 lines, and `internal/receipt/service.go` is 1,086 lines. These
files combine state machines, transport, policy, protocol translation, resource management, and error mapping. The
suite's total statement coverage is 44.8%; `internal/control` is 32.0%, `cmd/control` 6.4%, and `cmd/straw` 0.0%.
Passing checks currently say more about formatting and selected behavior than about end-to-end release confidence.

### Current maturity

| Area | Assessment | Main evidence |
| --- | --- | --- |
| Product boundary | Strong | README, roadmap, and architecture consistently define one trust boundary and optional profiles |
| External buildability | Release blocker | Go and Python runtime integration require private exact-tag repositories and CI app credentials |
| Repository hygiene | Unsafe | tracked `test.sh` contains a live-looking token and removed ClickHouse workflow |
| Source organization | Needs decomposition | three core files exceed 1,000 lines and mix multiple runtime responsibilities |
| Dependency direction | Partial | a shell grep protects split-repository pins, but package architecture is not mechanically specified |
| Test strategy | Uneven | leaf-package tests are healthy; Control, command wiring, integration, race, and fuzz coverage are weak or absent |
| CI | Good baseline, brittle boundary | actions are SHA-pinned, but jobs duplicate checks and privileged private access is on the PR path |
| Release engineering | Missing | no container/release workflow, artifact provenance, SBOM, signing, or reproducible release procedure |
| Deployment examples | Useful foundation | local and production Compose profiles exist, but validation is largely textual/render-only |
| Security engineering | Partial | policy and hardened examples exist; secret scanning, dependency review, CodeQL, and threat-model tests do not |
| Docs/community | Partial | core files and templates exist, but several URLs, prerequisites, support paths, and claims are stale |
| Maintainer ergonomics | Fragmented | three byte-identical agent skills, stale historical planning, aliases, and ad hoc scripts add ambiguity |

## Confirmed findings

### P0 — release and trust blockers

#### RQ-001 — Make the public checkout independently buildable

`go.mod` requires `straw-protos-go` and `straw-sdk-go`; `pyproject.toml` installs `straw-sdk-python` from Git; the
changelog calls the split repositories private; and CI creates a GitHub App token with access to five repositories.
This conflicts directly with a complete public release and prevents unaffiliated contributors from reproducing the
normal workflow.

Choose and execute one coherent publication model. The recommended model is to publish the protocol source,
generated bindings, and client/worker SDKs with immutable tags and public read access. If that cannot happen yet,
state that this repository is a source preview rather than an externally buildable release and remove claims to the
contrary. Do not vendor opaque private source as a workaround.

Acceptance:

- a fresh, unauthenticated checkout can run `go mod download`, `uv sync --frozen`, and `make check`;
- fork pull requests do not need repository secrets or a maintainer-owned GitHub App;
- tags, module versions, licenses, provenance, and compatibility are discoverable for every required component;
- an automated clean-room job proves the public-only dependency graph.

#### RQ-002 — Remove and investigate the tracked credential-bearing legacy script

Root `test.sh` contains an `sk_live_...` bearer token and queries ClickHouse at port 8123 even though ClickHouse,
telemetry read APIs, and load-test tooling are declared removed. The file is not executable, documented, or called by
the build. It is both a secret-handling incident and misleading dead code.

Delete the file, revoke or verify revocation of the credential, inspect repository history and provider logs as
appropriate, and use GitHub secret scanning/push protection. Rewrite history only after assessing whether the token
was real and coordinating the impact; ordinary deletion does not remove it from clones.

Acceptance:

- no credential-like literal or removed ClickHouse workflow remains in the tracked tree;
- the incident decision and rotation/revocation evidence are recorded privately;
- automated secret scanning runs on pushes and pull requests, with a documented false-positive process.

#### RQ-003 — Correct public identity and default-branch drift

README and CONTRIBUTING clone `straw-oss` and then `cd straw`; the issue template links to
`github.com/beremaran/straw/security/policy`; the docs license and website edit links use retired `master` URLs; the
local clone still retains a gone `master` branch; and `AGENTS.md` maps removed `sdk` and `python` directories.

Acceptance:

- repository name, clone directory, default branch, security URL, edit URLs, package map, and support links agree;
- a link checker covers Markdown, Docusaurus navigation, and GitHub templates;
- historical plans are explicitly labeled archival and cannot be mistaken for current instructions.

#### RQ-004 — Replace aspirational release status with an actual release gate

`ROADMAP.md` marks three substantial subsystems delivered and says the repository targets a complete first public
release, while `CHANGELOG.md` contains a large unreleased body and there is no release or image-publishing workflow.
The Go requirement also disagrees (`go 1.26.4`, Docker `1.26`, contributor guide `1.24 or later`).

Acceptance:

- select and document the supported Go/toolchain policy in all sources and containers;
- define the pre-1.0 launch criteria, supported artifacts, version matrix, upgrade order, and rollback policy;
- produce a reproducible release workflow for binaries and multi-architecture images with checksums, SBOMs,
  provenance, signatures, release notes, and post-publish smoke tests;
- turn the roadmap into future outcomes and move delivered detail into release notes or architecture decisions.

## P1 — establish clear source and ownership boundaries

#### RQ-010 — Decompose the Control dispatch pipeline by state-machine responsibility

`internal/control/dispatcher.go` owns public dispatch interfaces, routing, assignment, request upload flow control,
response decoding, raw HTTP streaming, receipt integration, protocol conversion, cancellation, timeout mapping, and
many helpers. Splitting by arbitrary line count would make navigation worse; split around explicit state machines.

Target structure within `internal/control`:

- `dispatch.go`: public dispatcher API, orchestration, final outcome;
- `assignment.go`: assignment request/ack and fallback boundary;
- `request_stream.go`: upload state, credit gate, frame publication;
- `response_stream.go`: decoded response state machine and receipt commit;
- `raw_response.go`: `http.ResponseWriter` streaming and trailer rules;
- `protocol_error.go`: protocol-to-pipeline error mapping;
- `protocol_convert.go`: header/frame conversions.

Keep package boundaries stable first. Extract another package only if an interface has more than one real consumer.
Before moving code, add state-transition tables and focused tests for cancellation, disconnects, credit exhaustion,
fallback timing, malformed/out-of-order frames, partial raw responses, and receipt failures.

Acceptance:

- no new behavior or dependency is introduced;
- each file has one explainable reason to change;
- duplicated decoded/raw stream transitions share tested primitives where semantics truly match;
- race tests and focused failure-path tests pass after every mechanical slice.

#### RQ-011 — Separate Egress DNS/policy, transport, pooling, and execution

`internal/egress/executor.go` combines resolver implementation, raw DNS parsing, request execution, retry, tunnel
handling, TLS/HTTP2 configuration, SSRF validation, connection pooling, HTTP validation, error classification, and
wire-frame creation. It also duplicates policy concepts and helpers found in Control (`metadataIPs`, denied prefixes,
suffix matching, HTTP token handling, integer conversion).

Target structure:

- `executor.go`: request lifecycle orchestration;
- `resolver.go`: resolver and CNAME-chain behavior;
- `destination_guard.go`: Egress-side resolved-address enforcement;
- `transport.go`: HTTP/TLS/HTTP2 construction and retry classification;
- `connection_pool.go`: pool key, eviction, stale-IP behavior;
- `request_build.go` and `response_frames.go`: HTTP/wire translation;
- `tunnel.go`: CONNECT path.

Do not prematurely share Control's pre-resolution policy with Egress's post-resolution enforcement. First document
their different security obligations, then extract only invariant pure primitives into a narrowly named internal
package if parity tests demonstrate equivalence.

Acceptance mirrors RQ-010 and adds DNS rebinding, CNAME, redirect, proxy, pooled-IP rotation, HTTP/2 fallback, TLS,
metadata-range, IPv4/IPv6, and tunnel tests.

#### RQ-012 — Split receipt lifecycle from persistence and object composition

`internal/receipt/service.go` combines domain state, response uploads, multipart composition, assignment signing,
cleanup, persistence serialization, idempotency indexing, and key layout. The in-memory orchestration lock also makes
the concurrency and HA guarantees hard to see.

Introduce internal interfaces for record/index persistence and explicit lifecycle transitions before considering a
new package. Separate response upload, multipart completion, assignment authorization, cleanup, and repository/key
logic into focused files. Model receipt states and legal transitions in one place; test every transition,
idempotency race, cleanup race, partial write, corrupt metadata, expiration boundary, and cancellation/commit race.

#### RQ-013 — Clarify the Control composition root

`cmd/control/open_source.go` is a product-history name, not a responsibility. Rename it around server/runtime wiring
and split optional profile assembly (runtime state, runtime admin, receipts) from HTTP server construction. Replace
long constructors with typed dependency structs where that makes invariants visible. Keep `cmd/control` limited to
flags, lifecycle, health, composition, and process exit behavior.

#### RQ-014 — Define and enforce the intended package graph

The current dependency script primarily asserts that old monorepo paths are absent and exact external pins are
present. Keep those release assertions, but add an architecture check that describes allowed imports among `cmd`,
`internal/control`, `internal/egress`, `internal/config`, `internal/natsx`, `internal/receipt`, and
`internal/objectstore`. Use `go list -deps` or a small tested Go command rather than a growing collection of greps.

Also decide whether `internal/testutil` is truly cross-package infrastructure. Keep local test helpers beside their
packages when they serve only one consumer.

## P1 — remove ambiguity and dead structure

#### RQ-020 — Clean tracked and local residue deliberately

Tracked cleanup candidates:

- delete `test.sh` under RQ-002;
- either document and test `scripts/mitm-h2-request.go` or remove it: the changelog says MITM tooling was removed,
  its default CA path does not match the current local layout, and the `scripts` directory becomes a Go package in
  `go test ./...` despite having no tests;
- retire backward-compatible `infra-*` Make aliases after a dated deprecation window if no released automation needs
  them;
- remove generic Docusaurus template images and commands that Straw does not use;
- decide whether completed repository-split plans belong in versioned `docs/decisions`/`docs/archive` or in Git
  history, then consolidate the four overlapping planning files.

The working tree also contains empty, untracked remnants such as `cmd/straw-load`, `internal/loadtest`,
`internal/postgresx`, `internal/redisx`, `migrations/postgres`, and empty docs/example directories. They do not affect
clones, but should be removed locally after confirming no user tooling depends on them. Never add placeholder
`.gitkeep` files merely to preserve an empty taxonomy.

#### RQ-021 — Establish one source for agent/contributor automation

`.agents`, `.claude`, and `.llm-docs` contain byte-identical copies of the same documentation skill. Duplication
guarantees drift. Select one canonical source and generate or link tool-specific projections only if all three
consumers are genuinely supported. Add a parity check if copies are unavoidable. Keep human contributor policy in
`CONTRIBUTING.md`/`AGENTS.md`, not buried in tool-specific instructions.

#### RQ-022 — Normalize names and directory semantics

- choose `egress-worker.md` or `egress_worker.md` consistently with the documentation URL style;
- use names based on responsibility rather than migration history (`open_source.go`, `repository-split-state.md`);
- reserve `scripts/` for maintained developer/release commands with a README or discoverable Make target;
- reserve `docs/planning` for active plans with owner/status/date, and archive or remove completed plans;
- make `examples/` exist only when it contains runnable, CI-covered examples.

## P1 — strengthen verification around real risk

#### RQ-030 — Publish a layered test strategy and close the high-risk gaps

Do not chase a single coverage percentage. Set risk-based expectations and report coverage per package. Current
baseline: total 44.8%, Control 32.0%, Control command 6.4%, CLI command 0.0%; several leaf packages are around 70–80%.

Add layers:

1. pure validation and state-machine unit tests;
2. package integration tests with real NATS/JetStream, Redis, and object storage where relevant;
3. process-level tests for Control/Egress startup, shutdown, health/readiness, signals, and config errors;
4. default and optional-profile Compose smoke tests;
5. cross-version protocol/SDK compatibility tests;
6. release artifact tests from published binaries/images, not source-only builds.

Prioritize negative paths, cancellation, retry/idempotency, concurrency, and failure recovery over happy-path line
coverage. Add a checked-in test matrix mapping product claims to suites.

#### RQ-031 — Add race, fuzz, and robustness gates

Run `go test -race ./...` in CI (or a documented supported subset until platform-specific code is isolated). Add fuzz
targets for request JSON, config/snapshot JSON, URL/host/header validation, NATS envelopes and stream validators,
DNS parsing, object-store XML/metadata, receipt records, and protocol frame sequences. Seed them with conformance
fixtures and past failures. Apply bounded time/memory expectations to parsers and streaming state machines.

#### RQ-032 — Turn conformance fixtures into an executable contract

The four JSON fixtures are currently inventory without an obvious root command or CI assertion proving all runtime
and SDK components consume them. Provide a versioned manifest, schema, expected outcome, producer/consumer matrix,
and one discoverable `make conformance` target. Fail when a fixture is orphaned or protocol version support changes
without compatibility notes.

#### RQ-033 — Test deployment behavior, not only Compose rendering

`production-deploy-check` renders Compose and greps selected strings. Retain the fast structural check, then add
ephemeral smoke tests for startup, auth, readiness, one request, graceful worker scaling, admin persistence, HA
Control loss, Redis degradation/recovery, receipt upload/download/corruption, and teardown. Make destructive targets
explicit and namespace all resources to avoid touching infrastructure outside the test.

#### RQ-034 — Remove CI duplication and make local/remote parity explicit

`ci.yml` runs `make check`; `compatibility.yml` separately repeats a subset on the same PR and push events. Define
which workflow is the required fast gate, which is cross-repository compatibility, and which is scheduled/release
validation. Reuse composite actions or callable workflows for toolchain setup. Pin the uv version consistently.
Cancel superseded runs, set job timeouts, minimize token scope, and ensure fork PRs get useful unprivileged results.

## P1 — build a credible security and supply-chain posture

#### RQ-040 — Add automated security gates proportional to an egress proxy

Add dependency review for pull requests, CodeQL or equivalent Go static analysis, `govulncheck`, secret scanning,
container/image scanning, license inventory, and scheduled dependency updates. Keep `gosec` in lint but do not treat it
as a substitute for these controls. Document triage ownership, severity policy, exception expiry, and disclosure path.

#### RQ-041 — Make privileged CI safe and reviewable

Cross-repository GitHub App tokens should not be minted for untrusted pull-request code. Separate public-source
verification from trusted post-merge/update automation, restrict the installation and repository list, avoid global
credential rewrites where possible, and set explicit workflow/job permissions. Add environment protection for
publishing and prevent tag or dispatch inputs from becoming unchecked branch/ref or command material.

#### RQ-042 — Codify the network security invariants

Create a threat model and executable invariant suite for SSRF defenses, DNS rebinding, redirects, upstream proxies,
CONNECT, header injection, credential redaction, NATS subject authorization, admin separation, Redis degradation,
receipt signatures, object-store boundaries, and request-body limits. Ensure Control's policy projection and Egress's
resolved-address enforcement cannot silently diverge.

#### RQ-043 — Harden build and runtime artifacts

Pin production base images by reviewed digest during release, run builds with reproducible metadata, emit a non-root
OCI image with labels and health semantics, and verify the final image contains only the intended binary and trust
store. Document CA behavior, architectures, graceful-stop signal, read-only filesystem expectations, writable
receipt paths, and resource limits. Generate SBOM, provenance, signatures, and checksums in the release workflow.

## P2 — finish open-source project operations

#### RQ-050 — Establish maintainership, support, and decision records

Add concise governance/support material: maintainer roles, decision process, response expectations, supported
versions, release cadence intent, and where usage questions belong. Add `CODEOWNERS` if it reflects real reviewers.
Create lightweight ADRs for the invariants contributors must preserve: trust boundary, required/optional services,
repository split, protocol compatibility, ordered headers, bounded streaming, runtime snapshots, Redis fencing, and
receipt verification.

#### RQ-051 — Make contribution workflow self-service

Align tool versions; add `make help`; explain focused tests, integration prerequisites, generated/external sources,
dependency update policy, architecture checks, changelog criteria, docs preview, and release-impact review. Consider a
dev-container only if it is maintained and demonstrably reduces setup cost. Do not add badges or scaffolding that has
no operational owner.

#### RQ-052 — Add issue and PR pathways for the real component graph

Update issue component choices to distinguish runtime, protocol, bindings, and external SDK repositories. Add links
that route reports to the correct public project. Expand PR checks for compatibility, security boundary, operational
effect, protocol fixtures, public docs, and release notes without turning the template into a ceremonial checklist.

#### RQ-053 — Maintain a dependency and compatibility policy

Document supported Go/Python/Node/Docker versions, NATS/Redis/S3 expectations, direct dependency purpose, upgrade
cadence, protocol/SDK/runtime compatibility, and security update handling. Automate routine updates with grouped,
reviewable PRs and run cross-version tests before accepting them. Treat TLS fingerprint libraries as a high-risk,
behavior-sensitive dependency with conformance fixtures and explicit upgrade notes.

#### RQ-054 — Replace the generic website shell with a project-owned surface

After correcting links and access blockers, remove unused Docusaurus starter assets, narrow npm scripts to maintained
operations, add project-specific social metadata/favicon/logo treatment, and test accessibility, mobile layout,
search/navigation, and broken anchors. Keep `docs/public` the only content source.

## Recommended delivery plan

### Milestone 0 — Stop the trust leaks (days, before promotion)

Complete RQ-002 and RQ-003 immediately. Decide RQ-001's publication model and stop claiming a complete public release
until its acceptance criteria pass. Freeze feature work while credentials, links, and external buildability are
unresolved. Convert RQ-004 into an explicit launch checklist.

### Milestone 1 — Reproducible public baseline (1–2 weeks)

Publish all required dependencies, add the unauthenticated clean-room build, separate privileged automation, align
toolchains, consolidate CI, add secret/dependency/vulnerability scanning, and create a minimal signed prerelease of
binaries and images. Complete the P0 items from the documentation audit in the same milestone.

### Milestone 2 — Architecture under control (2–4 weeks, incremental PRs)

Complete RQ-010 through RQ-014 as behavior-preserving slices. Start with characterization, race, and failure tests;
then move one state-machine responsibility per PR. In parallel complete RQ-020 through RQ-022 so dead history and
tool-specific duplication no longer obscure the target structure.

### Milestone 3 — Evidence for operational claims (2–4 weeks)

Complete RQ-030 through RQ-034 and RQ-042. Make default, admin, HA, and receipt claims executable in isolated CI.
Turn conformance data into a real cross-repository contract. Publish package-level coverage and reliability trends,
not a vanity global threshold.

### Milestone 4 — Sustainable release and community operation (ongoing)

Complete RQ-040 through RQ-054 plus the contract, operations, contributor, and documentation-quality milestones in
the companion documentation roadmap. Cut a release only when the public-only dependency graph, release artifacts,
upgrade path, and advertised profile smoke tests all pass.

## Task dependency map

| First | Enables |
| --- | --- |
| RQ-002 credential response | safe continued public development |
| RQ-001 publication decision | clean CI, SDK docs, compatibility tests, releases, external contributions |
| RQ-004 version/toolchain policy | Docker builds, contributor setup, support matrix, release workflow |
| RQ-030 characterization tests | safe decomposition in RQ-010 through RQ-013 |
| RQ-014 package graph | preventing boundary regression during decomposition |
| RQ-041 CI trust separation | safe fork contributions and release automation |
| RQ-032 executable conformance | cross-repository compatibility and protocol releases |
| RQ-033 profile smoke tests | credible deployment and release claims |

## Definition of done

The repository-quality program is complete when:

- an unaffiliated user can clone and run every required build/check without private credentials;
- no tracked secret, dead removed-subsystem script, broken repository URL, or misleading current-plan file remains;
- core runtime files are organized around explicit state machines and responsibilities with preserved behavior;
- dependency direction and public contract ownership are mechanically enforced;
- high-risk parsers, protocol flows, concurrency, cancellation, and network-policy boundaries have unit, fuzz, race,
  integration, and failure-path evidence;
- every advertised deployment profile is smoke-tested in isolated infrastructure;
- pull requests from forks receive a useful unprivileged gate and cannot access cross-repository or release secrets;
- releases produce reproducible public binaries and images with compatibility metadata, checksums, SBOM, provenance,
  signatures, upgrade/rollback guidance, and post-publish verification;
- support, governance, security response, version policy, architecture decisions, and contributor workflow are
  explicit enough that routine work does not depend on maintainer memory;
- the public documentation's stronger definition of done is also satisfied.

## Audit evidence and limits

Evidence collected from the current checkout:

- all 188 tracked files and top-level/directory distribution;
- root product, contributor, community, toolchain, dependency, and release files;
- every GitHub workflow and template;
- Go package/import graph and symbol distribution in the largest runtime files;
- local/production Docker and Compose assets and their validation scripts;
- public/manual planning structure and the existing documentation audit;
- `make check` (passed: Go, tagged Python integration, lint, dependency-direction check);
- `go test -coverprofile` (44.8% total statement coverage, with package results cited above);
- ignored/generated and empty-directory inspection;
- stale/legacy terminology, URL, secret-pattern, and removed-subsystem searches.

This was a source and repository audit, not a production penetration test, load test, dependency source-code audit,
or community sentiment study. `make production-deploy-check` and `make docs-website` were not required by this
planning-only change under the repository's verification rules; the final validation should still build the new
Markdown through Docusaurus. Claims about what the community will praise are therefore framed as verifiable project
qualities—trust, reproducibility, clear ownership, operational evidence, and maintainability—not speculative feature
volume.
