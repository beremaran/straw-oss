# Documentation audit and roadmap

Status: archived evidence. Owner: maintainer. Superseded by `open-source-readiness-roadmap.md`.

Date: 2026-07-13

This audit covers the documentation a user, operator, integrator, contributor, or maintainer can discover in
`straw-oss`: the root project files, `docs/public`, deployment READMEs, the Docusaurus site, GitHub community files,
and developer-facing package and workflow guidance. It compares those materials with the shipped command, config,
HTTP handler, runtime-policy, metrics, deployment, test, and build surfaces in this repository.

The target is not “more pages.” The target is a trustworthy manual in which a new user can succeed quickly, an
operator can make safe production decisions, an integrator can implement against complete contracts, and a
contributor can change or release the project without relying on maintainer memory.

## Executive assessment

Straw has a good documentation skeleton and unusually clear product boundaries for a pre-1.0 project. The manual
already spans quickstart, architecture, configuration, API use, SDKs, operations, security, troubleshooting, and the
three optional deployment profiles. The prose is direct, the trust-boundary language is consistent, and production
assets are correctly described as adaptable examples.

It is not yet comprehensive or reliably self-service. The highest-impact problem is trust: several first-run facts
are stale or wrong, while major public contracts are summarized rather than specified. The manual is strongest as a
guided tour and weakest as a reference. Advanced features appear “delivered” in the roadmap without the endpoint,
schema, failure, compatibility, and operational detail that lets an external user adopt them safely.

Current maturity by audience:

| Area | Assessment | Main reason |
| --- | --- | --- |
| First-run experience | Needs correction | clone directory and toolchain requirements disagree with the repository |
| Concepts and architecture | Good foundation | clear components and trust boundary, but shallow protocol and failure diagrams |
| Request API | Partial reference | request/success shapes exist; complete errors, limits, headers, and semantics do not |
| Admin and receipt APIs | Workflow-only | endpoints are listed or demonstrated, not specified as contracts |
| Configuration | Major gap | examples replace a complete field/default/constraint reference |
| SDK and CLI usage | Partial and externally blocked | basic calls exist; installability, compatibility, advanced use, and CLI behavior are incomplete |
| Deployment and security | Good foundation | lacks platform recipes, hardening verification, capacity planning, and complete threat model |
| Operations | Thin | metric names exist; alerts, dashboards, SLOs, backup/restore, upgrades, and incident runbooks do not |
| Troubleshooting | Thin | seven symptom entries cannot cover the shipped profiles and failure modes |
| Contributor experience | Partial | basic loop exists; architecture decisions, testing map, protocol work, docs process, and releases are underspecified |
| Community/project governance | Partial | conduct, security, issue and PR templates exist; support, governance, compatibility, and release policies are unclear |
| Documentation quality system | Missing | no doc ownership, freshness checks, executable examples, API/schema generation, or systematic link/spell/style checks |

## P0: repair correctness and access blockers

These should be fixed before promoting the project to external users.

### DOC-001 — Make every quickstart runnable from a clean machine

Evidence:

- `README.md` and `CONTRIBUTING.md` clone `straw-oss.git` and then run `cd straw`; Git creates `straw-oss` by default.
- The root module declares `go 1.26.4`, while `CONTRIBUTING.md` says Go 1.24 or later.
- The quickstart does not state expected startup time, expected output, verification response, or teardown/reset steps.

Work:

- Correct the directory and toolchain requirements everywhere.
- Choose and document one supported Go installation path and explain automatic toolchain behavior if relied upon.
- Add a copy/paste first request with an abbreviated expected response and `make dev-status`, `make dev-logs`,
  `make dev-down`, and `make dev-reset` lifecycle commands.
- Test the README and public quickstart independently on a clean supported environment.

Acceptance:

- A new contributor can clone, start, call, inspect, and stop Straw using only the page.
- Every command is exercised in CI or a repeatable smoke-test script.

### DOC-002 — Remove broken or misleading website navigation

Evidence:

- `website/docusaurus.config.js` sends edit links to `tree/master`; the active branch is `main`.
- Footer navigation omits the advanced feature pages and contributor/release guidance.
- The landing page still uses generic Docusaurus assets and has no version/status, install, or support path.

Work:

- Point edit links to `main` using the correct GitHub edit URL form.
- Redesign navigation around Learn, Use, Operate, Reference, and Contribute journeys.
- Replace template imagery and social metadata; add project status, version expectations, and support links.
- Add previous/next paths and meaningful cross-links from every task page.

Acceptance:

- All internal, edit, footer, and external links pass automated checks.
- A landing-page visitor can identify what Straw is, whether it fits, how to try it, and where to get help in one view.

### DOC-003 — Resolve public/private dependency and installation contradictions

Evidence:

- The public SDK page tells Python users to install an exact private Git SSH dependency.
- The custom-worker page links to a Go SDK while describing the Python SDK as private.
- CI and planning material confirm several protocol, binding, and SDK repositories are private.
- The roadmap describes the clients as part of a complete public release, but an external user cannot follow all
  documented installation paths.

Work:

- Decide which SDKs, bindings, protocol contracts, images, and tags are public at launch.
- Until publication, label unavailable paths clearly and provide a REST/curl path that is fully usable.
- Publish a compatibility table across Straw, protocol bindings, Go SDK, Python SDK, and worker SDK versions.
- Document artifact locations, checksums/provenance, supported platforms, and upgrade order.

Acceptance:

- Every installation command works for an unaffiliated user with no private credentials.
- No public page promises an artifact or integration surface that cannot be obtained.

## P1: specify the public contracts

### DOC-010 — Build a complete static configuration reference

The current page shows representative JSON and prose for selected defaults. It does not enumerate every field in
`internal/config/config.go`, including all Egress capabilities, HTTP/2 and connection-pool constraints, object-store
options, NATS tuning, server listeners, cross-field validation, zero/default behavior, environment dependencies, and
restart implications.

Work and acceptance:

- Create Control and Egress reference tables with path, type, required/optional, default, allowed values/range,
  secret sensitivity, reload/restart behavior, and examples.
- Document validation and cross-field rules, especially NATS payload/frame sizing, runtime-state TTLs, ports,
  receipt limits, S3 encryption, unique worker IDs, and mutually exclusive NATS authentication modes.
- Provide minimal, production-base, admin, HA, and receipt examples that validate against the shipped parser.
- Establish a schema or test that fails when a public config field is added without reference coverage.

### DOC-011 — Specify the runtime policy snapshot

`internal/config/snapshot.go` exposes routing rules, pools, destination rules, injection policies, fingerprint
profiles, and worker settings through the Admin API. The public admin guide demonstrates replacing one timeout but
does not define these objects, their match order, normalization, allowed values, interactions, or examples.

Work and acceptance:

- Document every snapshot object and validation rule from `snapshot_validate.go` and related routing/policy code.
- Explain priority and first-match behavior, pool eligibility, sticky fallback, deny/allow precedence, DNS/IP
  implications, header-operation order, fingerprint support, and lifecycle overrides.
- Include safe recipes: deny private networks, route by geography/tag, drain a worker, inject a header, and enable a
  fingerprint profile.
- Supply a full valid snapshot and negative examples whose failures are asserted in tests.

### DOC-012 — Turn the Request API page into a complete contract

The page lists request fields and a sample success, but omits several normative details: media types and request size,
unknown/duplicate JSON behavior, exact method/header/URL limits, default timeout, redirect behavior, compression,
DNS rebinding and destination-policy timing, replay semantics, cancellation/client disconnect behavior, timing-field
definitions, and compatibility guarantees.

Work and acceptance:

- Document HTTP method/path, authentication, content type, request limits, field constraints/defaults, response
  headers, success semantics, redirect and decoding behavior, cancellation, and retry rules.
- Publish every stable error from `internal/control/errors.go` with category, HTTP status, retryability, likely causes,
  and caller action. The current “common codes” list covers fewer than half of the registry and includes names that
  do not match the registry (`connect_timeout` and `response_header_timeout`).
- Define `request_id` and timing fields precisely and add representative examples for success, destination 4xx/5xx,
  invalid input, capacity, timeout, body limit, fingerprint, and policy denial.
- Add contract tests or generated fixtures that detect drift between the registry/types and the reference.

### DOC-013 — Add full Admin API reference

The runtime-administration page is an operator tutorial. It needs a separate reference for all nine handlers in
`internal/control/admin_handler.go`.

For each endpoint document authentication, headers (`If-Match`, `ETag`, `X-Straw-Actor`), parameters, request and
response schema, status codes, errors, concurrency behavior, idempotency, audit effects, and examples. Define list
ordering and bounds, cancellation ownership in HA, rollout states, custom-worker acknowledgement behavior, and the
dashboard's security boundary. Validate examples against handler tests.

### DOC-014 — Add full Receipt API reference

The receipt guide provides a good lifecycle tutorial but not a stable contract for the seven handlers in
`internal/control/receipt_handler.go`.

Specify create, status, cancel, part upload, complete, content download, and assignment-object endpoints; all request
and response fields; authorization; content headers; status/error codes; idempotency conflicts; part numbering and
size limits; state transitions; retention; signed URL query parameters; and HA/shared-storage requirements. Add a
state diagram and tested multipart/resume, cancellation, corruption, expiry, and response-download examples.

### DOC-015 — Define compatibility and versioning promises

Create a public compatibility policy covering REST endpoints, JSON additions/removals, config versioning, error codes,
runtime snapshots, NATS/protobuf worker protocol, SDK semver, container tags, mixed Control/Egress versions, and the
pre-1.0 exception policy. Link it from README, changelog, API references, SDK pages, contributor guidance, and release
checklists. Include a supported-version matrix and explicit deprecation process.

## P1: make operation safe and repeatable

### DOC-020 — Expand deployment guides into selectable patterns

- Add a decision guide for default, runtime-admin, receipt, and HA profiles, including required services and state.
- Document container images/tags, architectures, ports, volumes, filesystem permissions, capabilities, resource
  sizing, ulimits, DNS, proxies, IPv6, certificates, and graceful shutdown.
- Add at least one fully worked but still adaptable reverse-proxy/TLS example and one orchestrator example selected
  from demonstrated community demand; do not imply turnkey production support.
- Document upgrade and rollback procedures for each profile, including compatible order and state backups.
- Explain scale calculations for Control, Egress concurrency, NATS payload/bandwidth, Redis, and receipt storage.

### DOC-021 — Create an operations handbook

- Give every metric its type, labels, unit, meaning, profile, and interpretation; fix punctuation and naming drift in
  the existing metric list.
- Provide starter Prometheus alert rules and a dashboard with explicit “example” status.
- Define service-level indicators and suggested objectives without pretending they fit every deployment.
- Add runbooks for no workers, saturation, NATS loss, Redis loss, JetStream loss, object-store failure, high latency,
  checksum rejection, stalled rollout, disk pressure, credential rotation, and partial upgrade.
- Specify JSON log fields, levels, redaction rules, request correlation, and example queries.
- Add tested backup and restore drills for JetStream runtime config, Redis expectations, and receipt objects/records.

### DOC-022 — Complete the security model

- Add a threat model and data-flow diagram covering SSRF, DNS changes, redirects, header injection, credential
  exposure, NATS subjects, admin token compromise, signed receipt URLs, stored bodies, logs, and custom workers.
- Document the exact built-in destination policy and its limitations; distinguish application policy from network
  egress enforcement.
- Provide hardening and verification checklists for each profile, token/key rotation, TLS placement, secret delivery,
  least-privilege S3 policy, NATS permissions, Redis TLS/authentication, and log sanitation.
- State data-at-rest/in-transit behavior, retention and deletion limitations, and security update/support policy.

### DOC-023 — Make troubleshooting diagnostic, not symptom-only

Use a consistent format: symptom, fast check, likely causes, confirmation commands, fix, and escalation data. Cover
startup/config parsing, health versus readiness, auth/admin auth, worker registration and protocol mismatch, routing,
capacity, DNS/TLS/redirects, HTTP/2/fingerprint fallback, NATS payloads, Redis/HA, JetStream/admin rollout, receipts/S3,
timeouts, shutdown, upgrades, SDK/CLI errors, and platform-specific Docker issues. Add a safe diagnostic bundle recipe
that redacts tokens, URLs, headers, and body data.

## P2: improve each user journey

### DOC-030 — Add task-oriented tutorials and examples

The checked-in `examples` directory is empty. Add small, maintained examples for curl, CLI, Go, and Python; ordered
and duplicate headers; inline request bodies; error handling and safe retry; request and response receipts; auth;
routing/policy; worker scaling; and observability. Each example must state prerequisites, expected output, cleanup,
and the versions it was tested with. Prefer executable examples over duplicated prose.

### DOC-031 — Expand CLI documentation

Document installation/distribution, shell completion if supported, exit codes, stdout versus stderr, JSON stability,
environment precedence, file-size behavior, signals/timeouts, receipt workflows, examples, and common errors. Ensure
`straw help`, flag errors, and the page are generated from or tested against the same source.

### DOC-032 — Expand client SDK documentation

For each public SDK document installation, supported runtime versions, client lifecycle, timeout/context behavior,
authentication, headers/bodies, receipts, API errors, retries, thread/concurrency safety, resource cleanup, logging,
testing, and version compatibility. Link to generated package/API docs and runnable repositories. Separate the stable
client surface from the advanced worker SDK and state their different compatibility promises.

### DOC-033 — Document custom worker development

The current custom-worker section is one paragraph. Add prerequisites, supported SDK versions, registration and
capability lifecycle, assignment admission, heartbeat/readiness, streaming and flow control, cancellation, deadline
handling, error mapping, runtime snapshot acknowledgements, receipt references, graceful shutdown, conformance
fixtures, security obligations, and a minimal tested worker. Clearly identify which protocol details remain unstable.

### DOC-034 — Improve conceptual learning material

Add concise diagrams for request lifecycle, deployment topologies, configuration/runtime-policy ownership, and receipt
states. Explain Control versus Egress responsibilities, Core NATS versus JetStream, Redis's optional role, delivery
and retry semantics, ordering/backpressure, worker selection, TLS fingerprints, and why one deployment is one trust
boundary. Link concepts directly to configuration and operational consequences.

## P2: make contribution and maintenance self-service

### DOC-040 — Build a contributor handbook

- Correct prerequisites and document setup for private/public dependency access as appropriate.
- Add repository/package map, dependency-direction rules, how to run focused tests, test types, race/coverage policy,
  integration prerequisites, fixtures, local CI parity, lint/format behavior, and common failures.
- Document how to change API/config/protocol surfaces, add metrics/errors, write migrations, update generated bindings,
  and decide when docs/changelog/compatibility notes are required.
- Add documentation authoring instructions: site preview, front matter, style, diagrams, link rules, runnable examples,
  and review checklist.
- Record the support policy for Go, Python, Node, Docker/Compose, OS, and architectures.

### DOC-041 — Document architecture decisions and invariants

Create lightweight ADRs for the durable choices future contributors must not accidentally reverse: one deployment
as one trust boundary, NATS as the sole required service, optional JetStream/Redis/object storage, ordered headers,
bounded streaming, runtime snapshot ownership, Redis fencing, receipt verification, and repository/SDK split. Keep
historical planning documents clearly marked as internal history rather than current contributor guidance.

### DOC-042 — Make releases reproducible

Replace the aspirational six-line release checklist with the actual release graph: prerequisites, version selection,
cross-repository order, generated bindings, compatibility CI, container build/publish, SBOM/provenance/signing policy,
release notes, migration notes, docs version publication, smoke tests, rollback, and post-release verification. State
which steps are automated today and who can perform them.

### DOC-043 — Fill community governance and support gaps

Add a support policy or `SUPPORT.md`, governance/maintainer expectations, issue/PR response expectations, decision and
appeal path, release cadence expectations, and a clear place for questions. Add documentation-specific issue
templates and “good first issue” guidance. Consider `CODEOWNERS`, funding only if applicable, and a public project
status/maintenance statement. Do not invent a community channel that is not actively maintained.

## P2: install a documentation quality system

### DOC-050 — Automate drift detection

- Run the Docusaurus build, internal/external link checks, spelling/terminology checks, Markdown lint, and code-block
  validation in CI.
- Generate or assert coverage for config fields, HTTP routes, error registry entries, CLI flags, metrics, and JSON
  examples from source-of-truth tests or schemas.
- Smoke-test the quickstart and profile-specific examples on a schedule and before release.
- Fail changes that add a public endpoint, config field, metric, or stable error without corresponding reference
  updates or an explicit internal-only annotation.

### DOC-051 — Establish documentation ownership and review

Define page owners, required reviewers for API/security/operations content, a style and terminology guide, review
dates for volatile pages, and a quarterly freshness sweep. Add a pull-request checklist for audience, prerequisites,
tested commands, defaults/limits/failures, security boundary, cross-links, and changelog impact. Track docs work as
ordinary product work with acceptance criteria, not as a release-end cleanup.

### DOC-052 — Add docs analytics and feedback carefully

Provide “report a docs issue” links with page context. If privacy-compatible analytics are adopted, document them and
measure failed searches, high-exit setup pages, and common issue themes. Use community evidence to choose new platform
recipes; do not add a large matrix of unmaintained deployment guides preemptively.

## Recommended delivery sequence

### Milestone 0 — Trustworthy first contact

Complete DOC-001 through DOC-003. Publish only after clean-machine quickstart and unaffiliated-user artifact access
are verified. This is the smallest credible launch gate.

### Milestone 1 — Contract-complete core

Complete DOC-010 through DOC-015. Treat the request API, config, runtime snapshot, errors, admin API, receipt API, and
compatibility policy as one versioned reference set. Add drift tests in parallel rather than after the pages are
written.

### Milestone 2 — Safe self-hosting

Complete DOC-020 through DOC-023, beginning with profile selection, metrics/reference tables, backup/restore, and
incident runbooks. Validate every destructive or failure-drill command against owned local Compose resources only.

### Milestone 3 — Adoption paths

Complete DOC-030 through DOC-034 once SDK/artifact publication decisions are settled. Use user feedback to prioritize
languages and deployment environments; keep the REST path fully independent of SDK availability.

### Milestone 4 — Sustainable community maintenance

Complete DOC-040 through DOC-052. Make release, contribution, and documentation checks part of CI and normal review.
At this point, comprehensiveness becomes maintainable rather than a one-time documentation push.

## Definition of done for the documentation program

The program is complete when:

- a clean-machine user can complete the default quickstart and every advertised optional-profile tutorial;
- an unaffiliated user can obtain every advertised artifact without private credentials;
- every public config field, HTTP route, error code, CLI flag, and metric has a source-linked normative reference;
- request, admin, and receipt examples are contract-tested against the shipped implementation;
- operators have profile selection, hardening, scaling, upgrade/rollback, backup/restore, alerting, and incident guides;
- contributors can set up, test, change a public contract, update docs, and follow a real release procedure without
  undocumented maintainer knowledge;
- internal/edit/external links and the site build pass in CI, and volatile examples receive scheduled smoke tests;
- compatibility, security, support, governance, and deprecation policies are explicit and discoverable;
- each major page has an owner, review date, feedback path, and evidence that its commands were tested.

## Audit method and source map

The audit inspected:

- product boundary and community entry points: `README.md`, `ROADMAP.md`, `CONTRIBUTING.md`, `CHANGELOG.md`,
  `SECURITY.md`, `CODE_OF_CONDUCT.md`, and `.github` templates/workflows;
- public manual and site navigation: every file under `docs/public`, `website/sidebars.js`,
  `website/docusaurus.config.js`, and `website/README.md`;
- public runtime surfaces: `cmd/control`, `cmd/egress`, `cmd/straw`, `internal/control`, `internal/config`,
  `internal/egress`, `internal/receipt`, `internal/objectstore`, and `internal/cli`;
- deployment and verification surfaces: `deploy/local`, `deploy/production`, `Makefile`, `conformance`, `integration`,
  `scripts`, `go.mod`, `pyproject.toml`, and `uv.lock`.

This is a repository audit, not community research: there is not yet enough public issue/discussion evidence in the
tree to claim which additional platforms the community values most. Platform-specific expansion should therefore
follow real adoption feedback, while the contract, security, operations, and contributor gaps above are directly
demonstrated by shipped code and current documentation.
