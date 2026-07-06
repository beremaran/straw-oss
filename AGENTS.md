# Agent Guide

This is the entrypoint for coding agents working on Straw.

## Project

Straw is a Go distributed HTTP/HTTPS proxy control plane and egress worker. P0 is a vertical slice: one Control service,
one official Go Egress Worker, REST request transport, Core NATS assignment, Postgres config, Redis runtime state,
ClickHouse metadata, and docker-compose for local development.

Request flow: a client calls Control's REST API (`POST /api/v1/requests`, handled in
`internal/control/proxy_handler.go`), the dispatcher (`internal/control/dispatcher.go`) admits (auth,
quota, rate limit, destination policy), routes to a worker assignment, and sends the request over NATS
to an Egress Worker (`internal/egress/loop.go` + `executor.go`), which performs the upstream HTTP call
and replies over NATS. Postgres holds tenant/config/API-key state, Redis holds runtime state (worker
registry, sticky sessions, cache invalidation), ClickHouse holds request metadata/telemetry. `sdk/` is
the minimal Go client SDK; `cmd/straw` is the CLI.

Start with:

1. `docs/planning/01-purpose.md`
2. `docs/planning/02-phase-boundaries.md`
3. `docs/planning/04-canonical-architecture.md`
4. `docs/planning/05-component-boundaries.md`
5. `docs/planning/31-implementation-order.md`
6. The task file assigned from the phase board (`docs/tasks/p0.md`, `p1.md`, or `p2.md`)

## Workflow

- Pick exactly one task from the assigned phase board (`docs/tasks/p0.md`, `p1.md`, or `p2.md`). Default to the
  lowest-numbered phase with unblocked work unless the user names a phase or task.
- If skills are available, use the repo-local skill at `.llm-docs/skills/straw-task-runner` for task execution.
- Use sub-agents when available. Prefer them for parallel codebase research, independent implementation slices, test
  triage, and review.
- Keep one owning agent responsible for the selected task, final edits, verification, task status updates, and handoff.
- Do not use sub-agents to start extra tasks, read beyond the assigned task's planning docs, or drift into another
  phase's work.
- Read only the planning docs named by that task before editing.
- Keep each task's work inside its own phase. Do not build features from a later phase unless the task explicitly
  says so.
- Prefer existing package boundaries and the standard library before new code or dependencies.
- Write the smallest test that fails for real behavior, then the smallest implementation that passes it.
- Update the task file status only when the work is actually complete and verified.
- Leave a handoff note using `docs/agents/templates/handoff.md`.
- We are trying to stop a worrying trend of "looks done, isn't done" in this repository. Task acceptence criterias must
  be validated & verified.
- If a given task is too big of a shirt-size, ask user if it's OK to split into vertical slice, and ask it with the
  proposed split.

## Gap Ownership (no unowned deferrals)

The 2026-07-05 P0 audit found that "flagged but unowned" gaps are how work silently disappears: handoffs for tasks
20, 22, 24, 30, and 32 each honestly wrote "no owning task exists" — and every one of those gaps then sat invisible
until a full audit rediscovered them. The rule that prevents this:

- **A deferral without an owning task file does not exist as far as the boards are concerned.** If you must defer
  behavior and no task owns it, you have two valid moves: create the owning task with
  `.llm-docs/skills/write-straw-task` in the same run, or stop and ask the user. Writing "no owning task" in a
  handoff and moving on is not a valid third option.
- A task must not be marked `done` on its board while its handoff contains an unowned deferral.
- Deferring to the task you are completing ("owned by this task if pursued") is an unowned deferral.
- When a later task closes a gap an earlier task/handoff documented as open, update the earlier doc's note in the
  same run — stale "known limitation" notes cost audit time.
- Scope notes like "do not add P1 fields" never cover fields the planning doc marks P0; when in doubt whether a
  field/endpoint is in-phase, the planning doc wins and ambiguity is a stop condition.

## Repo-Local Skills

- `.llm-docs/skills/straw-task-runner`: complete exactly one task from the boards.
- `.llm-docs/skills/write-straw-task`: author a new task file (any phase) that meets this repo's task standard.
- `.llm-docs/skills/verify-straw-task`: audit a task (or a whole board) for real, wired, verified completeness.
- `.llm-docs/skills/dig-straw-task-handoffs`: sweep completed handoffs for unowned/undone work and task it.
- `.llm-docs/skills/update-straw-documentation`: write public-facing docs under `docs/public/`.

Reusable prompt:

```text
Use $straw-task-runner at .agents/skills/straw-task-runner to complete the next unblocked task from the phase board (docs/tasks/p0.md, p1.md, or p2.md). Work on one task only, use sub-agents where they help parallelize research/review/independent slices, read the task's required planning docs, run make check, update status only after verification, and leave a handoff note.
```

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
- `docs/planning`: Product and architecture source of truth.
- `docs/tasks`: Agent task packs.

## Verification

Run before handoff:

```sh
make check
```

`make check` runs `make fmt-check` (gofmt), `make test` (`go test ./...`), and `make lint`
(`golangci-lint run --max-issues-per-linter 0 --max-same-issues 0` — the required flags are already
baked in). Other useful commands:

```sh
go test ./internal/control -run TestName   # single test
make postgres-migrations-check             # migration file sanity
STRAW_TEST_POSTGRES_DSN='postgres://postgres:postgres@localhost:5432/straw_test?sslmode=disable' go test ./...  # Postgres-backed tests
```

Postgres-backed tests only run when `STRAW_TEST_POSTGRES_DSN` is set, and the harness refuses any
database whose name does not end in `_test` (it truncates tables between tests). Use the dedicated
`straw_test` database, never the compose stack's live `straw` database — see the "Running the
Postgres-backed tests" section of `deploy/docker/README.md`.

Do not be afraid to start the compose stack. Bringing up `deploy/docker`, rebuilding/restarting `control` or
`egress` against it, and driving real requests through it is encouraged whenever a task's behavior can be observed
live — "looks done, isn't done" survives on unit tests alone. The only guarded action is pointing the *test
harness* (`STRAW_TEST_POSTGRES_DSN`) at the live `straw` database; that rule is not a reason to avoid the compose
stack itself (task 42's handoff records exactly that over-application).

## Stop Conditions

Stop and ask before continuing if:

- the assigned task conflicts with `docs/planning`;
- a task requires a P1/P2 decision from `docs/planning/32-open-decisions.md`;
- tests fail for reasons unrelated to your change;
- you need a new dependency;
- the smallest safe fix would cross task boundaries.

## Strict Rules

- Strictly must not use `git commit --no-verify`; local checks are required.
- Strictly must not add `//nolint:...` comments; fix the lint issue instead.
- Strictly forbidden to modify `.golangci.yml`.
- Strictly must fix every linter error; never dismiss failures as pre-existing.
- Strictly forbidden to use `// #nosec` or any `#nosec` comments to disable security linters or checks.
- Don't be a stupid bitch. If you are running the linter for a module or file, make sure you are running it with
  `--max-issues-per-linter 0 --max-same-issues 0` flags to see all issues at once.
