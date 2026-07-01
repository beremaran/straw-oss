# Repository Guidelines

## Project Structure

Straw is a Go module (`github.com/beremaran/straw`). Entrypoints live in `cmd/relay` and `cmd/endpoint`. Relay HTTP code is under `internal/server`; endpoint HTTP/TLS transport code is under `internal/endpoint`; shared packages live in `pkg/` (`broker`, `endpoint`, `protocol`, `validator`).

## Build, Test, and Development Commands

- `make build`: builds `bin/relay` and `bin/endpoint`.
- `make test`: runs `go test -race ./...`.
- `go test -race -shuffle=on ./...`: mirrors CI-style randomized tests.
- `make lint`: runs `golangci-lint run --max-issues-per-linter 0 --max-same-issues 0 ./...`.
- `make format`: runs `gofmt -w ./`.
- `make install-tools`: installs `govulncheck` and `golangci-lint`.

Use `config/.relay.env.example` and `config/.endpoint.env.example` as starting points for local service configuration.

## Coding Style

Use standard Go formatting: tabs via `gofmt`, short lowercase package names, `CamelCase` for exported identifiers, and `camelCase` for unexported identifiers. Keep comments focused on why code exists. Prefer wrapping errors, naming sentinel errors with `Err...`, passing contexts into I/O calls, and avoiding needless interfaces or named returns.

## Strict Rules

- Strictly must not use `git commit --no-verify`; local checks are required.
- Strictly must not add `//nolint:...` comments; fix the lint issue instead.
- Strictly forbidden to modify `.golangci.yml`.
- Strictly must fix every linter error; never dismiss failures as pre-existing.
- Strictly forbidden to use `// #nosec` or any `#nosec` comments to disable security linters or checks.
- If running the linter for a module or file, include `--max-issues-per-linter 0 --max-same-issues 0`.

## Testing Guidelines

Place unit tests beside code as `*_test.go`. Add the smallest test that proves changed behavior.

## Commit & Pull Request Guidelines

Prefer Conventional Commits as documented in `CONTRIBUTING.md`, e.g. `feat: add endpoint timeout` or `fix: reject invalid target URLs`. Keep messages imperative and scoped to one change.
