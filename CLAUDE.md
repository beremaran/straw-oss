# Agent Guide

This is the entrypoint for coding agents working on Straw.

## Project

Straw is a Go distributed HTTP/HTTPS proxy control plane and egress worker. The shipped vertical slices cover one
Control service, one official Go Egress Worker, REST and streaming request transports, Core NATS assignment,
Postgres config, Redis runtime state, ClickHouse metadata, and docker-compose for local development.

Request flow: a client calls Control's REST API (`POST /api/v1/requests`, handled in
`internal/control/proxy_handler.go`), the dispatcher (`internal/control/dispatcher.go`) admits auth, quota, rate
limit, and destination policy, routes to a worker assignment, and sends the request over NATS to an Egress Worker
(`internal/egress/loop.go` + `executor.go`), which performs the upstream HTTP call and replies over NATS. Postgres
holds tenant/config/API-key state, Redis holds runtime state, and ClickHouse holds request metadata/telemetry.
`sdk/` is the Go client SDK; `cmd/straw` is the CLI.

Start with these planning inputs when creating or reviewing Straw specs:

1. `straw/docs/planning/01-purpose.md`
2. `straw/docs/planning/02-phase-boundaries.md`
3. `straw/docs/planning/04-canonical-architecture.md`
4. `straw/docs/planning/05-component-boundaries.md`
5. `straw/docs/planning/31-implementation-order.md`
6. Any task-board or handoff archive cited by the SpecKit feature

## Workflow

- Use the root GitHub SpecKit workflow for new Straw work: `/speckit-specify` -> `/speckit-plan` ->
  `/speckit-tasks` -> `/speckit-implement`.
- Active Straw work lives in root `specs/<feature>/` artifacts. Mention Straw in the feature name or summary when
  the feature is Straw-specific.
- Legacy `straw/docs/tasks/p0.md`, `p1.md`, `p2.md`, and `straw/docs/tasks/p*/` task files are archive/migration
  inputs only. Do not add new active work there unless the user explicitly revives the old board workflow.
- Use the standard SpecKit skills directly. Do not create or use Straw-specific wrapper skills for the normal
  SpecKit workflow.
- Keep one owning agent responsible for the selected SpecKit task or story slice, final edits, verification, task
  checkbox updates, and handoff. Sub-agents research and verify; they never edit or mark tasks complete.
- When spawning Codex sub-agents, set `model` to `gpt-5.6-luna` and `reasoning_effort` to `max` unless the user
  explicitly asks for a different sub-agent model.
- Read only the planning docs named by the selected SpecKit artifacts before editing.
- Keep work inside the feature's declared scope. Do not build later-phase Straw behavior unless the SpecKit feature
  explicitly includes it.
- Prefer existing package boundaries and the standard library before new code or dependencies.
- Write the smallest test that fails for real behavior, then the smallest implementation that passes it.
- Mark SpecKit task checkboxes `[X]` only when the work is complete and verified.
- Leave a handoff note using `straw/docs/agents/templates/handoff.md`.

## Gap Ownership (no unowned deferrals)

Flagged-but-unowned gaps are how work silently disappears. The rules that prevent this:

- **A deferral without an owning SpecKit task does not exist as far as Straw work is concerned.** If you must defer
  behavior and no task owns it, use `/speckit-converge` or append the owning task to the relevant
  `specs/<feature>/tasks.md`, or stop and ask the user.
- A task must not be marked `[X]` while its handoff contains an unowned deferral.
- Deferring to the task you are completing ("owned by this task if pursued") is an unowned deferral.
- When a later task closes a gap an earlier task/handoff documented as open, update the earlier doc's note in the
  same run; stale "known limitation" notes cost audit time.
- Scope notes like "do not add P1 fields" never cover fields the planning doc marks P0; when in doubt whether a
  field/endpoint is in-phase, the planning doc wins and ambiguity is a stop condition.

## Repo-Local Skills

- Use root SpecKit skills for planning, tasking, implementation, convergence, and analysis.
- Keep Straw-specific non-workflow skills only for jobs SpecKit does not replace:
  - `straw/.claude/skills/update-straw-documentation`: write public-facing Straw docs under `straw/docs/public/`.
  - `straw/.agents/skills/sync-straw-linear` / `straw/.llm-docs/skills/sync-straw-linear`: sync legacy planning and
    task archives to Linear when explicitly requested.

## Repo Map

- `cmd/control`: Control service entrypoint.
- `cmd/egress`: Egress worker entrypoint.
- `cmd/straw`: CLI entrypoint (`internal/cli`).
- `internal/control`: Control implementation packages.
- `internal/egress`: Egress worker implementation packages.
- `internal/config`: Config loading and validation.
- `internal/natsx`: NATS connection and subject helpers.
- `internal/postgresx`, `internal/redisx`, `internal/logging`: Postgres/Redis/logging helpers.
- `internal/testutil`: Shared test helpers.
- `sdk`: Public Go client SDK.
- `api/proto/straw/v1`: Protobuf contracts.
- `migrations/postgres`: Postgres migrations.
- `deploy/docker`: Docker-local deployment files.
- `docs/planning`: product and architecture source of truth.
- `docs/tasks`: legacy task archive.

## Verification

Run before handoff:

```sh
make check
```

`make check` runs `make fmt-check` (gofmt), `make test` (`go test ./...`), and `make lint`
(`golangci-lint run --max-issues-per-linter 0 --max-same-issues 0`; the required flags are already baked in).
Other useful commands:

```sh
go test ./internal/control -run TestName
make postgres-migrations-check
STRAW_TEST_POSTGRES_DSN='postgres://postgres:postgres@localhost:5432/straw_test?sslmode=disable' go test ./...
```

Postgres-backed tests only run when `STRAW_TEST_POSTGRES_DSN` is set, and the harness refuses any database whose
name does not end in `_test`. Use the dedicated `straw_test` database, never the compose stack's live `straw`
database.

Do not be afraid to start the compose stack. Bringing up `deploy/docker`, rebuilding/restarting `control` or
`egress` against it, and driving real requests through it is encouraged whenever a task's behavior can be observed
live. The guarded action is pointing the test harness at the live `straw` database.

## Stop Conditions

Stop and ask before continuing if:

- the selected SpecKit task conflicts with `straw/docs/planning`;
- a task requires a P1/P2 decision from `straw/docs/planning/32-open-decisions.md`;
- tests fail for reasons unrelated to your change;
- you need a new dependency;
- the smallest safe fix would cross task boundaries.

## Strict Rules

- Strictly must not use `git commit --no-verify`; local checks are required.
- Strictly must not add `//nolint:...` comments; fix the lint issue instead.
- Strictly forbidden to modify `.golangci.yml`.
- Strictly must fix every linter error; never dismiss failures as pre-existing.
- Strictly forbidden to use `// #nosec` or any `#nosec` comments to disable security linters or checks.
- If you run `golangci-lint` directly, include `--max-issues-per-linter 0 --max-same-issues 0`.
