# Repository Split Execution Plan

## Status and authority

This document is the execution specification for splitting Straw before its first public release. Straw is a stealth
project. All repositories and prerelease tags remain private until a later, explicitly approved, coordinated public
launch.

The executing agent is authorized to complete the entire split, including:

- creating private repositories under the `beremaran` GitHub account;
- preserving and pushing filtered Git history;
- changing package and module paths;
- configuring repository settings, CI, rulesets, secrets, and cross-repository automation;
- creating branches, commits, pull requests, private prerelease tags, and GitHub releases;
- moving and deleting code after the cutover gates in this document pass;
- using the existing authenticated `gh` CLI and GitHub APIs;
- preparing, creating, and installing a dedicated GitHub App for cross-repository automation.

The agent must pause only when GitHub requires the owner to perform an interactive browser confirmation, passkey,
password, or two-factor authentication step, or when a required pull request needs owner review. The owner must never
provide passwords, tokens, private keys, recovery codes, or other credentials in the conversation. Secrets must never
be printed, committed, included in logs, or stored in repository files.

This plan does not authorize making any repository public. Public launch requires a separate explicit decision.

## Purpose

Straw should launch with permanent public boundaries rather than publishing monorepo-shaped APIs and separating them
later. The runtime remains cohesive, while the language-neutral protocol, generated bindings, and public SDKs become
independently consumable and releasable products.

The split is a pre-release bootstrap, not a compatibility-preserving migration for external users. Breaking changes
are allowed during private `v0.x` development when coordinated across all affected repositories. The first public
wire-compatibility commitment will be decided only at release readiness; it is deliberately not made by this plan.

## Non-negotiable product boundary

Straw remains a self-hosted Go HTTP/HTTPS egress proxy. One deployment is one trust boundary. NATS is the only
required backing service. The split must not reintroduce tenants, RBAC, quotas, billing, administration products,
mandatory databases, or hosted-service assumptions.

Control, the canonical Egress implementation, NATS transport, configuration, routing, object storage, receipts, CLI,
deployment examples, public manual, and documentation website remain part of `straw-oss`.

## Permanent repository layout

Create and maintain these six repositories under the `beremaran` account:

```text
github.com/beremaran/straw-protos
github.com/beremaran/straw-protos-go
github.com/beremaran/straw-protos-python
github.com/beremaran/straw-sdk-go
github.com/beremaran/straw-sdk-python
github.com/beremaran/straw-oss
```

All six remain private throughout this execution.

The dependency direction is strictly one-way:

```text
straw-oss ───────────→ straw-sdk-go ─────────→ straw-protos-go
    └────────────────────────────────────────→ straw-protos-go

straw-sdk-python ────────────────────────────→ straw-protos-python

straw-protos-go ───── generated from ────────→ straw-protos tag
straw-protos-python ─ generated from ────────→ straw-protos tag
```

Protocol repositories never depend on SDKs or the runtime. SDK repositories never depend on `straw-oss` or its
private packages. No committed build or CI configuration may depend on sibling checkouts.

Do not create an umbrella or seventh repository. `straw-oss` owns temporary integration-workspace scripts and the
cross-repository end-to-end test matrix. An uncommitted local `go.work` may be used for development convenience, but
it must not be required by builds, tests, or CI.

## Permanent package identities

### Go

Use these module and import paths:

```text
github.com/beremaran/straw-protos-go
github.com/beremaran/straw-sdk-go
```

Representative imports are:

```go
strawpb "github.com/beremaran/straw-protos-go/straw/v1"
straw "github.com/beremaran/straw-sdk-go"
"github.com/beremaran/straw-sdk-go/egress"
```

Correct the unreleased runtime module from `github.com/beremaran/straw-oss` to
`github.com/beremaran/straw-oss`. New Go modules begin at `v0.x`. The Protobuf namespace `straw.v1` is independent
from Go module release versions. If a Go module eventually reaches major version 2, Go semantic import versioning
requires a `/v2` module and import path at that time.

### Python

Use these distribution and import names:

```text
Distribution: straw-protos
Import:       straw_protos

Distribution: straw-sdk
Import:       straw
```

Representative imports are:

```python
from straw import Client
from straw_protos.straw.v1 import straw_pb2
```

Do not make two independently installed distributions share the `straw` top-level package. During stealth, install
Python dependencies from exact private Git tags. Do not publish to PyPI or another external package registry. PyPI
Trusted Publishing may be configured later as part of the separately approved public launch.

## Repository responsibilities

### `straw-protos`

Owns only language-neutral protocol assets:

- `.proto` source files;
- Buf module, lint, and breaking-change configuration;
- protocol documentation and compatibility policy;
- language-neutral conformance fixtures and test vectors;
- schema changelog and source tags;
- automation that dispatches tagged source versions to binding repositories.

It must not contain generated Go or Python bindings, NATS clients, routing, HTTP execution, runtime configuration, or
language-specific implementations.

### `straw-protos-go`

Owns:

- generated Go bindings;
- its Go module and generation toolchain;
- Go implementations of protocol-level helpers such as validation, registration-signing payload construction,
  signing, and verification;
- tests against the language-neutral fixtures;
- provenance metadata identifying the exact `straw-protos` tag and commit plus generator versions.

Generated files are never edited manually. Hand-written helpers must be clearly separated from generated output.

### `straw-protos-python`

Owns:

- generated Python bindings and type stubs;
- the `straw-protos` Python distribution using the `straw_protos` import package;
- its generation and packaging toolchain;
- Python implementations equivalent to the Go protocol helpers;
- tests against the same language-neutral fixtures;
- the same source and generator provenance metadata.

Generated files are never edited manually. Hand-written helpers must be clearly separated from generated output.

### `straw-sdk-go`

Owns:

- the public Go Control REST client;
- the common Egress worker SDK machinery and plumbing;
- public worker registration, session, heartbeat, assignment, stream, credit, cancellation, and body-reference
  abstractions;
- SDK unit and conformance tests;
- focused examples that demonstrate the public Go SDK;
- focused API, compatibility, and contributor documentation.

It depends on `straw-protos-go` and must not import `straw-oss/internal/...`.

### `straw-sdk-python`

Owns:

- the public Python Control REST client;
- the common Python Egress worker SDK machinery and plumbing;
- the `straw-sdk` distribution using the `straw` import package;
- SDK unit and conformance tests;
- focused examples that demonstrate the public Python SDK;
- focused API, compatibility, and contributor documentation.

It depends on `straw-protos-python` and must not duplicate generated bindings.

### `straw-oss`

Owns:

- Control and the canonical Egress product implementation;
- the CLI and all private runtime packages;
- NATS, configuration, routing, object storage, receipts, logging, and operational integrations;
- local and production deployment assets;
- the complete user-facing manual and Docusaurus website;
- runnable whole-system and deployment examples;
- the cross-repository integration and compatibility matrix.

The canonical Egress implementation remains in `straw-oss`. It consumes the common/base machinery from
`straw-sdk-go/egress`, while its official HTTP executor, TLS profiles, configuration integration, logging, metrics,
and operational behavior remain in `straw-oss`.

Delete `examples/egress-static`; do not migrate or replace it. It is not a meaningful example now that the canonical
Egress implementation exercises the public SDK plumbing.

Keep `docs/public` and `website` in `straw-oss`. Extracted repositories carry only focused README, API,
compatibility, security, and contributor information. The complete product documentation remains authoritative in
`straw-oss`.

## Protocol cleanup before extraction

Nothing has been released, so clean the protocol before its first tag instead of permanently retaining obsolete
concepts for hypothetical compatibility.

Inventory every message, field, enum, subject, helper, and protocol operation. In particular, review and remove or
redesign legacy tenant, RBAC, permission, quota, rate-limit, hosted-service, and other concepts outside Straw's
one-deployment/one-trust-boundary model. Do not remove active deployment-scoped authentication, pools, routing,
capacity, policy, receipt, or runtime concepts merely because similarly named legacy concepts exist.

The cleanup and extraction must be separate reviewable commits. Before moving files, classify every current file
under `api/proto/straw/v1`, including:

```text
straw.proto
straw.pb.go
validate.go
registration_sign.go
contract_test.go
registration_sign_test.go
README.md
```

Generated bindings move to binding repositories. Normative language-neutral rules and fixtures move to
`straw-protos`. Go implementations and their language tests move to `straw-protos-go`; equivalent Python
implementations and tests move to `straw-protos-python`.

The private `straw.v1` contract may continue to evolve during stealth. Private prerelease tags are immutable and
reproducible but do not promise compatibility with older `v0.x` tags. At public release readiness, explicitly decide
and document:

- the first supported wire contract;
- backward- and forward-compatibility rules;
- unknown-field and unknown-enum behavior;
- permitted compatible additions;
- reservation rules for removed fields and enum values;
- when a new namespace such as `straw.v2` is required;
- the supported SDK/runtime version matrix.

Do not describe `straw.v1` as literally immutable. Once publicly supported, it should be compatibility-governed:
compatible additions remain possible, while breaking wire changes require an explicitly versioned contract.

## Conformance and compatibility ownership

Build a trusted conformance baseline before the first consumer cutover. `straw-protos` owns language-neutral fixtures
covering at least:

- registration signing and verification;
- envelope encoding and parsing;
- protocol version negotiation;
- assignment acknowledgement;
- stream frame ordering and offsets;
- upload and download credit/backpressure;
- cancellation and cancelled responses;
- error frames and mappings;
- body references, size, checksum, and expiry behavior;
- unknown fields and enum values where applicable.

Both binding repositories run the relevant fixtures. Both SDKs own their unit and worker conformance tests.
`straw-oss` owns the end-to-end matrix across Control, the canonical Go Egress, tagged Go SDK workers, and tagged
Python SDK workers.

During stealth, CI tests only the currently approved tagged versions. Older `v0.x` compatibility is best-effort and
not guaranteed. Once public releases exist, expand the matrix to cover old Control/new worker, new Control/old
worker, current Go SDK/current Control, and current Python SDK/current Control according to the published support
policy.

## Git history extraction and security

Preserve original history using filtered extraction from `straw-oss` rather than clean initial commits. Keep relevant
authorship and commit messages without copying unrelated runtime history.

Before pushing each extracted repository:

1. inspect the complete filtered file list and commit history;
2. scan the complete filtered history for credentials, tokens, private endpoints, personal data, and secrets;
3. confirm unrelated files and generated caches are absent;
4. confirm large or binary artifacts are intentional;
5. verify license and attribution continuity;
6. stop and remediate before pushing if any sensitive or unrelated history is found.

The scan covers full retained history, not only the final tree, because the repositories will later become public.
Never rewrite or destructively alter the existing `straw-oss` working tree to perform extraction. Use separate
temporary clones or worktrees and preserve all user changes.

During extraction, freeze non-split changes to `api/proto`, `sdk`, and `python`. Runtime work outside these boundaries
may continue only when it does not change protocol or SDK surfaces.

## Versioning and dependency policy

Use ordinary private `v0.x` tags during stealth to rehearse the real release process. Do not use floating branches,
untagged revisions, local path dependencies, or committed `replace` directives for cross-repository consumption.

Generated bindings use exactly the source protocol version:

```text
straw-protos        v0.3.0
straw-protos-go     v0.3.0
straw-protos-python v0.3.0
```

SDKs and `straw-oss` version independently.

All cross-repository dependencies are pinned to exact private tags during stealth:

- Go modules require concrete tagged versions and commit `go.sum`;
- Python projects use exact private Git tags and commit their root `uv.lock`;
- each extracted Python repository is its own uv workspace and may have its own root lockfile;
- CI must prove each repository builds from a clean dependency cache without sibling checkouts.

The root uv workspace rule in the current monorepo applies only until Python is extracted. Do not create a nested
lockfile under the current `python/` directory before extraction.

## Generation and release automation

Each binding repository owns its generator configuration, pinned tools, packaging, and release workflow. A
`straw-protos` tag triggers generation but does not generate or commit language outputs itself.

Required flow:

1. Create and approve a private `straw-protos` tag.
2. `straw-protos` dispatches workflows in both binding repositories through the dedicated GitHub App.
3. Each binding repository checks out the exact source tag and resolves its commit SHA.
4. Each generates with pinned tool and plugin versions.
5. Each verifies reproducibility and runs its fixture and package tests.
6. Each opens a generated pull request in its own repository containing source and generator provenance.
7. The owner reviews and merges each pull request; no formal approving-review requirement is configured for the solo
   owner.
8. After merge, automation validates provenance, creates the matching tag, and creates a private GitHub release.
9. During stealth, no Python artifact is published externally; consumers install from the private Git tag.
10. New binding tags automatically open dependency-update pull requests in both SDKs and in `straw-oss` as
    applicable.
11. Downstream pull requests require CI and owner review/merge; automation never silently updates consumers.

If one binding generation or release fails, do not roll back, move, or recreate an existing tag. Fix the failed
repository, rerun idempotently from the same source tag, and complete the missing matching release. Automation must
detect an existing matching release and refuse to publish conflicting provenance.

Each generated release records at least:

- exact `straw-protos` tag and commit SHA;
- generator and plugin names and versions;
- generation configuration revision;
- generated repository commit SHA;
- reproducibility/test result;
- an explicit generated-code notice.

## GitHub authentication and repository controls

Use the existing authenticated `gh` CLI for owner-authorized repository operations. Do not request or expose the
owner's existing token.

Create a private, dedicated GitHub App installed only on these six repositories for cross-repository workflows. Give
it only the permissions required for repository contents, pull requests, workflow dispatch/status, and metadata.
Avoid webhook permissions unless the implemented design actually uses webhooks. Store the App ID, installation
information, and private key only in appropriate GitHub Actions secrets or environments; never store them in source,
local tracked files, command output, or conversation text.

GitHub App registration or installation may require a one-time browser confirmation. The agent may prepare and
drive the supported manifest/settings flow, but must pause for the owner to complete any password, passkey, or 2FA
challenge personally. Afterward, continue through `gh` and GitHub APIs where supported.

Use this baseline in all repositories:

- default branch `main`;
- changes through pull requests;
- required CI checks before merge;
- no force pushes or deletion of `main`;
- squash merge by default;
- signed commits or release tags where practical;
- no required approving-review count for the solo owner;
- generated repositories reject manual changes to generated files;
- least-privilege Actions and explicit workflow permissions;
- pinned third-party Actions according to the project's supply-chain policy.

Apply equivalent rulesets or branch protections where the account and repository plan support them. If GitHub does
not support a desired control for these private personal repositories, record the limitation and enforce the closest
equivalent in CI rather than silently omitting it.

Every repository uses the MIT license and scope-adapted versions of Straw's security, code-of-conduct, and
contribution policies.

Repository-specific bugs and changes use that repository's Issues. Cross-cutting product and integration work is
tracked in `straw-oss`. Enable GitHub Private Vulnerability Reporting where supported in every repository, and make
each `SECURITY.md` explain how to report an issue whose affected repository is uncertain.

## Migration and cutover sequence

Execute in this order. Keep changes reviewable and do not combine protocol redesign with mechanical extraction.

1. Audit the dirty worktree and preserve all user changes.
2. Record the pre-split verification baseline with `make check`, `make production-deploy-check`, and
   `make docs-website` as applicable.
3. Apply the temporary protocol/SDK/Python extraction freeze.
4. Correct the unreleased `straw-oss` Go module path by removing `/v2`; update imports and verify.
5. Inventory and clean the private protocol to match Straw's product boundary.
6. Create language-neutral fixtures and establish passing Go/Python conformance tests locally.
7. Create the five new private GitHub repositories and configure their baseline settings.
8. Prepare or install the dedicated GitHub App and configure secrets without exposing them.
9. Extract `straw-protos` with filtered history, scan its full history, complete its standalone scaffolding and CI,
   push it, and create its first private tag.
10. Extract and complete `straw-protos-go`; generate from the tagged protocol, verify provenance and conformance,
    merge through a PR, and create the matching private tag automatically.
11. Extract and complete `straw-protos-python` in the same way and create its matching private tag automatically.
12. Update `straw-oss` to consume the tagged external Go bindings and pass all runtime/conformance tests before
    deleting its local generated bindings or protocol helper implementations.
13. Extract `straw-sdk-go` with filtered history, remove all private-runtime dependencies, consume the tagged Go
    bindings, establish standalone CI, push, review, and tag it.
14. Refactor the canonical Egress implementation to consume SDK base machinery while retaining canonical product
    behavior in `straw-oss`.
15. Delete `examples/egress-static`.
16. Update `straw-oss` to consume the tagged Go SDK and pass the complete test suite before deleting its local SDK
    copy.
17. Extract `straw-sdk-python` with filtered history, convert it into an independent uv project, consume the exact
    private binding tag, establish standalone CI, push, review, and tag it.
18. Update integration tests and documentation in `straw-oss`, then remove the old local Python SDK copy only after
    tagged-package integration passes.
19. Add automated downstream dependency-update PRs and the full current-version cross-repository matrix.
20. Run clean-cache, no-sibling-checkout verification for every repository.
21. Run final `straw-oss` verification, including `make check`, `make production-deploy-check`, and
    `make docs-website`.
22. Audit repository settings, histories, tags, releases, provenance, documentation links, and dependency direction.
23. Remove the extraction freeze and publish a private completion report. Keep every repository private.

## Mandatory cutover gate

Do not remove a source package or directory from `straw-oss` until its extracted replacement has:

1. passed standalone CI with no sibling checkout or local replacement;
2. passed full filtered-history and secret scanning;
3. received an immutable private tag;
4. been consumed successfully by `straw-oss` through that exact tag;
5. passed the relevant conformance and cross-repository integration suite;
6. passed the complete applicable `straw-oss` verification commands;
7. had its documentation and dependency links updated.

Keep extraction and deletion commits separate where doing so improves rollback and review. Never leave
`straw-oss` depending on an untagged branch or local checkout.

## Completion criteria

The split is complete only when:

- all six repositories exist under `beremaran` and remain private;
- filtered history is preserved and full-history scans pass;
- permanent Go and Python package identities are in use;
- `straw-oss` no longer uses the unreleased `/v2` module path;
- the cleaned protocol is canonical in `straw-protos`;
- generated bindings are reproducible from exact protocol tags with recorded provenance;
- matching binding tags exist and downstream update PR automation works;
- both SDKs build and test independently against exact tagged bindings;
- the canonical Egress remains in `straw-oss` and exercises `straw-sdk-go` base machinery;
- `examples/egress-static` is removed;
- `straw-oss` owns and passes the tagged cross-repository compatibility matrix;
- no repository relies on a sibling checkout, floating dependency, local replacement, or externally published Python
  package;
- repository policies, CI, security reporting, licenses, and documentation are configured consistently;
- all required verification commands pass;
- no repository has been made public.

## Deferred public-launch decisions

Do not decide these prematurely during the split:

- the first public version number for each independently versioned product;
- the exact point at which `straw.v1` gains a public compatibility guarantee;
- the long-term supported-version matrix;
- PyPI publication and Trusted Publishing configuration;
- public Go module proxy availability;
- the coordinated date and release announcement.

At launch readiness, verify all six repositories and make them public as one coordinated operation. Do not expose a
partial graph with private dependency links or incomplete documentation.

## Ready-to-use goal for a new session

Use the following goal verbatim or by referencing this document:

> Execute the complete private pre-release repository split described in
> `docs/planning/repository-split.md`. Treat that document as the authoritative specification and authorization
> boundary. Create and configure all five new private repositories under `beremaran`, preserve and scan filtered
> history, clean and extract the protocol, establish reproducible tagged binding generation, extract both SDKs,
> update `straw-oss` to consume exact private tags, configure the dedicated least-privilege GitHub App and
> cross-repository automation, run every cutover gate and verification command, and continue until all completion
> criteria pass. Preserve existing user changes. Keep every repository private. Never expose credentials. Pause only
> for an unavoidable GitHub owner authentication confirmation or for owner review of a pull request that the plan
> requires before merge.

