# Agent guide

Straw is a Go distributed HTTP/HTTPS egress proxy. One deployment is one trust boundary. NATS is the only required
backing service; do not reintroduce tenants, RBAC, quotas, billing, administration APIs, or mandatory databases.

## Start here

- `README.md` and `ROADMAP.md` define the public product boundary.
- `docs/public/architecture.md` explains the runtime.
- `docs/public/configuration.md` and `docs/public/api/requests.md` define public behavior.
- `CONTRIBUTING.md` defines the contributor workflow.

## Repository map

- `cmd/control`, `internal/control`: Control API and dispatch pipeline
- `cmd/egress`, `internal/egress`: official worker and executor
- `internal/config`, `internal/natsx`: configuration and NATS helpers
- `sdk`, `python`: public Go/Python clients and worker SDKs
- `deploy/local`: supported local Compose stack
- `deploy/production`: adaptable production example
- `docs/public`, `website`: public manual and documentation site

## Working agreement

- Keep changes inside the requested outcome and existing package boundaries.
- Prefer the standard library and existing dependencies. Stop before adding a dependency.
- Add the smallest test that fails for real behavior, then the smallest implementation that passes.
- Update public docs and `CHANGELOG.md` when behavior changes.
- Preserve user changes in a dirty worktree.
- Never use `git commit --no-verify`, lint suppressions, or `.golangci.yml` changes to evade findings.

Python work uses the root uv workspace. Run `uv sync --all-packages --frozen` from the repository root and do not
create nested locks or virtual environments under `python/`.

## Verification

Run before completion:

```sh
make check
```

Also run `make production-deploy-check` for deployment/configuration changes and `make docs-website` for public docs.
Live behavior can be verified with `make dev`; never run destructive commands against infrastructure you do not own.
