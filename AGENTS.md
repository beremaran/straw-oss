# Repository Guidelines

## Project Structure & Module Organization

Straw is a Go 1.25 module (`github.com/beremaran/straw`). Entrypoints live in `cmd/relay` and `cmd/endpoint`. Private application code is under `internal/`, grouped by domain, service, infra, server, config, and observability concerns. Public SDK-style packages live in `pkg/` (`broker`, `endpoint`, `protocol`, `validator`). API contracts are in `api/openapi.yaml`; MkDocs content is in `docs/`; scripts are in `scripts/`; broader suites are in `test/` (`security`, `integration`, `contract`, `load`).

## Build, Test, and Development Commands

- `make build`: builds `bin/relay` and `bin/endpoint`.
- `make test`: runs `go test -race ./...`.
- `go test -race -shuffle=on ./...`: mirrors CI test behavior.
- `make lint`: runs `golangci-lint run ./...`.
- `make format`: runs `gofmt -w ./`.
- `make docs`: lints/builds OpenAPI docs and MkDocs output.
- `make docs-serve`: serves local documentation.
- `make install-tools`: installs `govulncheck` and `golangci-lint`.

Use `.relay.env.example` and `.endpoint.env.example` as starting points for local service configuration.

## Coding Style & Naming Conventions

Use standard Go formatting: tabs via `gofmt`, short lowercase package names, `CamelCase` for exported identifiers, and `camelCase` for unexported identifiers. Keep comments focused on why code exists. The configured linter set is strict; prefer wrapping errors, naming sentinel errors with `Err...`, passing contexts into I/O calls, and avoiding needless interfaces or named returns.

## Strict Rules

- Strictly must not use `git commit --no-verify`; local checks are required.
- Strictly must not add `//nolint:...` comments; fix the lint issue instead.
- Strictly forbidden to modify `.golangci.yml`.
- Strictly must fix every linter error; never dismiss failures as pre-existing.

## Testing Guidelines

Place unit tests beside code as `*_test.go`. Put cross-package behavior in `test/security`, `test/integration`, or `test/contract` only when it truly spans components. Integration tests use external services/testcontainers, so Docker may be required. No coverage threshold is configured; add the smallest test that proves the changed behavior.

## Commit & Pull Request Guidelines

Prefer Conventional Commits as documented in `CONTRIBUTING.md`, e.g. `feat(admin): add audit export` or `fix: reject replayed signatures`. Keep messages imperative and scoped to one change.

Pull requests target `main`, include a concise problem/solution description, link relevant issues or docs, identify the change type, and list verification commands run. Update `docs/` or `api/openapi.yaml` when behavior or contracts change, and ensure build, test, and lint checks pass before review.
