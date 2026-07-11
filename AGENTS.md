# Agent Guide

This is the entrypoint for coding agents working inside Straw.

## Project

Straw is a Go distributed HTTP/HTTPS proxy control plane and egress worker. It ships a Control service, official Go
Egress Worker, REST and streaming request transports, Core NATS assignment, Postgres configuration, Redis runtime
state, ClickHouse telemetry, SDKs, CLI, and local Compose deployment.

A client calls `POST /api/v1/requests` in `internal/control/proxy_handler.go`. The dispatcher in
`internal/control/dispatcher.go` performs authentication, quota, rate-limit, destination-policy, and routing checks,
then sends an assignment over NATS. `internal/egress/loop.go` and `executor.go` execute the upstream request and
stream the response back. Postgres stores tenant/config/API-key state, Redis stores runtime state, and ClickHouse
stores request metadata and telemetry. `sdk/` is the Go client SDK; `cmd/straw` is the CLI.

Start with the relevant canonical document under `docs/planning/`. The most common entrypoints are:

1. `docs/planning/01-purpose.md`
2. `docs/planning/02-phase-boundaries.md`
3. `docs/planning/04-canonical-architecture.md`
4. `docs/planning/05-component-boundaries.md`
5. `docs/planning/31-implementation-order.md`

`docs/implementation-history.md` retains the useful context and implementation decisions from the completed P0–P2
boards. It is searchable history, not an active task system, and should only be read when current planning/code does
not answer a historical question.

## Working agreement

- Follow the root `AGENTS.md` and the requested `ROADMAP.md` outcome.
- Read only the planning sections relevant to the change before editing.
- Keep work inside the requested outcome. Do not build later-phase behavior implicitly.
- Prefer existing package boundaries and the standard library before new code or dependencies.
- Write the smallest test that fails for real behavior, then the smallest implementation that passes it.
- Run focused tests while iterating and `make check` before completion.
- Update canonical planning/public docs when behavior or a durable decision changes. Do not create per-task specs,
  checklists, boards, or handoffs.
- Put any genuine unfinished product work under the owning outcome's `Remaining work` in the root `ROADMAP.md`.

The triggering agent defines any delegated sub-agent's scope and remains responsible for integration and acceptance.

## Repository map

- `cmd/control`: Control service entrypoint.
- `cmd/egress`: Egress worker entrypoint.
- `cmd/straw`: CLI entrypoint (`internal/cli`).
- `internal/control`: Control implementation packages.
- `internal/egress`: official Egress worker implementation.
- `internal/config`: configuration loading and validation.
- `internal/natsx`: NATS connection and subject helpers.
- `internal/postgresx`, `internal/redisx`, `internal/logging`: infrastructure helpers.
- `internal/testutil`: shared test helpers.
- `sdk`: public Go client and Egress SDKs.
- `python`: public Python client and Egress SDKs.
- `api/proto/straw/v1`: Protobuf contracts.
- `migrations/postgres`: Postgres migrations.
- `deploy/docker`: local Docker deployment.
- `docs/planning`: canonical architecture, protocol, security, and phase decisions.
- `docs/public`: user-facing documentation sources.

The retained non-workflow documentation skill is
`straw/.claude/skills/update-straw-documentation` (and its agent mirrors) for public documentation updates.

## Verification

Run before completion:

```sh
make check
```

This runs formatting checks, `go test ./...`, and golangci-lint with uncapped issue reporting. Useful focused checks:

```sh
go test ./internal/control -run TestName
make postgres-migrations-check
STRAW_TEST_POSTGRES_DSN='postgres://postgres:postgres@localhost:5432/straw_test?sslmode=disable' go test ./...
python3 -m unittest discover python/tests
```

Postgres-backed tests run only when `STRAW_TEST_POSTGRES_DSN` is set. The harness refuses databases whose name does
not end in `_test`; use `straw_test`, never the Compose stack's live `straw` database.

Starting the Compose stack, rebuilding/restarting `control` or `egress`, and driving real requests through it is
encouraged when behavior can be observed live. Never point the test harness at the live database.

## Stop conditions

Stop and ask before continuing if:

- the requested behavior conflicts with canonical `docs/planning` material;
- an unresolved decision in `docs/planning/32-open-decisions.md` blocks the work;
- tests fail for reasons unrelated to the change;
- a new dependency is needed;
- the smallest safe fix materially expands the requested outcome.

## Strict rules

- Never use `git commit --no-verify`.
- Never add `//nolint`, `#nosec`, or similar suppression comments; fix the issue.
- Never modify `.golangci.yml` to evade a finding.
- Fix every linter error. If running golangci-lint directly, include
  `--max-issues-per-linter 0 --max-same-issues 0`.
