# Repository Guidelines

## Project Structure & Module Organization

Straw is a Go module (`github.com/beremaran/straw`) for controlling HTTP requests through egress workers over NATS. Entrypoints are `cmd/control` and `cmd/egress`. Core code lives under `internal/`: `server` for control HTTP handlers and middleware, `egress` for workers and outbound HTTP/TLS transport, `broker` for NATS, `protocol` for message encoding/signing, `validator` for URL checks, and `config` for environment parsing. Tests sit beside code as `*_test.go`; Docker files are in `docker/`, scripts in `scripts/`, and env examples in `config/`.

## Build, Test, and Development Commands

- `make build`: builds `bin/control` and `bin/egress`.
- `make control` / `make egress`: builds one binary.
- `make test`: runs `go test -race ./...`.
- `go test -race -shuffle=on ./...`: runs randomized race-enabled tests.
- `make lint`: runs `golangci-lint run --max-issues-per-linter 0 --max-same-issues 0 ./...`.
- `make format`: runs `gofmt -w ./`.
- `make dev-up`, `make dev-down`, `make dev-shell`: manage the Docker dev environment.
- `make install-tools`: installs `govulncheck` and `golangci-lint`.

For local services, start NATS with `docker compose -f docker/docker-compose.dev.yml up -d nats`.

## Coding Style & Naming Conventions

Use standard Go formatting and short lowercase package names. Exported identifiers use `CamelCase`; unexported identifiers use `camelCase`. Wrap errors with context, name sentinel errors `Err...`, pass `context.Context` into I/O boundaries, and keep comments focused on why code exists. Avoid needless interfaces, named returns, and speculative options.

## Testing Guidelines

Add the smallest unit test that proves changed behavior. Keep tests beside the package they cover with names ending in `_test.go`. Run `make test` for normal verification and add `-shuffle=on` when checking for order-sensitive failures.

## Commit & Pull Request Guidelines

Use Conventional Commits, matching the project history: `feat: add egress transport option`, `fix: reject invalid target URLs`, `docs: update control usage`. Keep commits scoped and imperative. PRs should explain the change, link relevant issues or context, and list verification commands run.

## Strict Rules

- Do not use `git commit --no-verify`; local checks are required.
- Do not add `//nolint:...` or `#nosec` comments; fix the issue instead.
- Do not modify `.golangci.yml`.
- Fix every linter error, including ones that appear pre-existing.
- If running the linter manually, include `--max-issues-per-linter 0 --max-same-issues 0`.

## Security & Configuration Tips

Use `config/.dev.env.example`, `config/.control.env.example`, and `config/.egress.env.example` as local templates. Keep `HMAC_SECRET`, NATS tokens, and private egress settings out of commits. `ALLOW_PRIVATE_IPS=true` is for local development only unless the deployment explicitly needs private targets.
