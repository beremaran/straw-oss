# Contributing to Straw

Thanks for helping make Straw easier to run and understand.

## Before you start

- Search existing issues and pull requests.
- Open an issue before a large behavior change, new dependency, or protocol change.
- Keep one pull request focused on one outcome.
- Never include credentials, production traffic, or customer data.

By participating, you agree to follow the [Code of Conduct](CODE_OF_CONDUCT.md). Report security issues through
[SECURITY.md](SECURITY.md), not a public issue.

## Development setup

Required for all changes:

- Go 1.26.5 (the exact version declared by `go.mod`)
- Docker with Compose v2
- `make`
- `golangci-lint`

Python changes also require Python 3.13 and uv. Documentation-site changes require Node.js 20 or later.

```sh
git clone https://github.com/beremaran/straw-oss.git
cd straw-oss
make dev
make check
```

The default stack uses ports 4222, 8222, 8080, and 9090. See `deploy/local/README.md` for overrides.

## Repository and dependency boundaries

Commands contain flags, process lifecycle, health, and composition. Runtime behavior belongs in the corresponding
`internal` package: `internal/control`, `internal/egress`, `internal/config`, `internal/natsx`, `internal/receipt`, or
`internal/objectstore`. Public Go and Python consumers live in the separately tagged SDK repositories. Protocol
source and generated bindings also live in their own tagged repositories; do not copy or hand-edit generated
bindings here.

`make dependency-check` uses `go list` to enforce the direct-import graph and exact external module pins. Stop before
adding a dependency. If a new boundary is genuinely required, explain it in an issue and update the relevant ADR
and compatibility documentation as part of the same reviewed change.

## Making a change

1. Branch from the current default branch.
2. Add the smallest test that demonstrates the behavior.
3. Keep changes within existing package boundaries when possible.
4. Update `docs/public` and `CHANGELOG.md` when public behavior changes.
5. Run the checks below before opening a pull request.

```sh
make check
make production-deploy-check
make docs-website
```

For tagged Python SDK integration:

```sh
uv sync --frozen
uv run --frozen python -m unittest discover integration/python
```

The root lock pins the public Python SDK and binding tags used by the runtime compatibility matrix. Python SDK
development belongs in `straw-sdk-python`.

## Choose verification by risk

Start with the smallest focused test that demonstrates the behavior, then run the ordinary gate. Add broader checks
when the affected claim requires them:

| Change | Focused evidence | Required broader evidence |
| --- | --- | --- |
| Control, Egress, CLI, config, receipt | `go test ./internal/<package> -run TestName` | `make check` |
| Concurrency, cancellation, streams, shared state | focused Go test | `make race` |
| Parsers or state machines | unit/fuzz seed | `make fuzz-smoke` |
| Protocol fixture or binding compatibility | relevant producer/consumer test | `make conformance` |
| Default/admin/receipt deployment | owned disposable profile | `make profile-smoke PROFILE=<profile>` |
| HA or recovery | owned disposable failure/restore drill | `make ha-smoke` or `make state-backup-smoke PROFILE=<profile>` |
| Production configuration/TLS | focused render change | `make production-deploy-check` and `make tls-proxy-check` |
| Public documentation or example | focused live command where applicable | `make docs-website` and `make check` |

Never run destructive profile or recovery commands against shared infrastructure. These maintained targets create
uniquely named disposable Compose projects and remove their resources on exit.

## Change a public contract

A public contract includes static config, runtime snapshots, REST routes and JSON, stable errors, CLI flags/output,
metrics, protobuf/NATS messages, SDK behavior, container behavior, and compatibility guarantees. For any such change:

1. Add positive and negative behavior tests in the owning package or repository.
2. Update the normative page under `docs/public`; include defaults, limits, failure behavior, and compatibility.
3. Update `CHANGELOG.md`. State whether the change is additive, deprecated, breaking, or internal-only.
4. Run `make public-surface-check`. If protocol fixtures change, update the versioned conformance manifest and run
   `make conformance`; orphaned fixtures are rejected.
5. Update exact external tags and the root `uv.lock` only through the owning repository's release order described in
   `docs/public/releases.md`. Run `uv sync --frozen`; do not replace public tags with local URL rewrites.
6. Exercise the representative request, profile, or maintained example against the shipped implementation.

Markdown pages require one H1, valid local links, a page entry in `docs/public/owners.json`, and tested-command
evidence. Use the documentation issue template for gaps. `make docs-check`, `make doc-ownership-check`, and
`make docs-website` are product gates, not release-end cleanup.

## Pull requests

Explain the problem, the chosen boundary, verification performed, and any compatibility or operational effect. Add
screenshots only when they clarify a documentation/UI change. Maintainers may ask to split unrelated work.

Commits should be reviewable and use imperative summaries such as `Simplify worker startup`. Never bypass hooks with
`--no-verify`.

## Design principles

- local development is the shortest supported path;
- one deployment is one trust boundary;
- NATS is the only required backing service;
- production files are adaptable patterns, not a claimed turnkey platform;
- prefer the standard library and existing dependencies;
- keep public behavior documented from installation through operation.
