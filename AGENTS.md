# Agent Guide

This is the entrypoint for coding agents working on Straw.

## Project

Straw is a Go distributed HTTP/HTTPS proxy control plane and egress worker. P0 is a vertical slice: one Control service,
one official Go Egress Worker, REST request transport, Core NATS assignment, Postgres config, Redis runtime state,
ClickHouse metadata, and docker-compose for local development.

Start with:

1. `docs/planning/01-purpose.md`
2. `docs/planning/02-phase-boundaries.md`
3. `docs/planning/04-canonical-architecture.md`
4. `docs/planning/05-component-boundaries.md`
5. `docs/planning/31-implementation-order.md`
6. The task file assigned from `docs/tasks/p0.md`

## Workflow

- Pick exactly one task from `docs/tasks/p0.md`.
- If skills are available, use the repo-local skill at `skills/straw-task-runner` for task execution.
- Use sub-agents when available. Prefer them for parallel codebase research, independent implementation slices, test
  triage, and review.
- Keep one owning agent responsible for the selected task, final edits, verification, task status updates, and handoff.
- Do not use sub-agents to start extra P0 tasks, read beyond the assigned task's planning docs, or drift into P1/P2
  work.
- Read only the planning docs named by that task before editing.
- Keep P0 work inside P0. Do not build P1/P2 features unless the task explicitly says so.
- Prefer existing package boundaries and the standard library before new code or dependencies.
- Write the smallest test that fails for real behavior, then the smallest implementation that passes it.
- Update the task file status only when the work is actually complete and verified.
- Leave a handoff note using `docs/agents/templates/handoff.md`.
- We are trying to stop a worrying trend of "looks done, isn't done" in this repository. Task acceptence criterias must
  be validated & verified.
- If a given task is too big of a shirt-size, ask user if it's OK to split into vertical slice, and ask it with the
  proposed split.

Reusable prompt:

```text
Use $straw-task-runner at .agents/skills/straw-task-runner to complete the next unblocked P0 task from docs/tasks/p0.md. Work on one task only, use sub-agents where they help parallelize research/review/independent slices, read the task's required planning docs, run make check, update status only after verification, and leave a handoff note.
```

## Repo Map

- `cmd/control`: Control service entrypoint.
- `cmd/egress`: Egress worker entrypoint.
- `internal/control`: Control implementation packages.
- `internal/egress`: Egress worker implementation packages.
- `internal/config`: Config loading and validation.
- `internal/natsx`: NATS connection and subject helpers.
- `internal/testutil`: Shared test helpers.
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

`make check` runs `gofmt` verification and `go test ./...`.

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
