---
name: straw-task-runner
description: Complete one Straw repository task from docs/tasks, especially P0 task packs. Use when the user asks to complete the next task, work on a specific docs/tasks/p0/*.md file, continue implementation from the task board, or produce a task handoff for this repo.
---

# Straw Task Runner

Use this skill to complete exactly one Straw task safely.

## Workflow

1. Read `AGENTS.md`.
2. Choose the task:
   - If the user names a task file, use that file.
   - Otherwise use the first `not started` or `in progress` task in `docs/tasks/p0.md` whose prerequisites are satisfied.
3. Read the chosen task file completely.
4. Read only the planning docs named in the task file.
5. Check current git status and avoid reverting unrelated user changes.
6. Implement only the selected task.
7. Update tests with the smallest checks that prove the task behavior.
8. Run focused tests first, then `make check`.
9. Run the completion audit below.
10. Update task status only after verification and the audit.
11. Write a handoff note using `docs/agents/templates/handoff.md`.

## Completion Audit

Before marking any step or task done, verify each claim against the definitions below. Past runs checked off
"wire connection setup" after adding only config validation, and wrote "Remaining Work: None" for code backed
entirely by in-memory fakes. Do not repeat that.

- "Wire", "connect", or "integrate" means the built binary (`cmd/control`, `cmd/egress`) actually constructs and
  uses the real component at runtime. Startup validation, subject helpers, or interface definitions do NOT satisfy
  a wiring step. If the task means library-only work, it says so explicitly.
- A step you did not fully do stays unchecked. Partially done = unchecked, with a note on what remains.
- Every behavior you defer must name the exact task file that owns it (e.g. "deferred to
  `docs/tasks/p0/13-rate-limits-quotas-redis.md`"). If no existing task owns it, STOP: report the gap to the user
  instead of writing "deferred to later tasks". Unowned deferrals are how work silently disappears.
- If you introduce an in-memory or fake implementation of something the planning docs require to be backed by a
  real system (NATS, Postgres, Redis, ClickHouse — see `docs/planning/21-state-and-storage.md`), the handoff's
  Remaining Work section must list the real-backend swap and its owning task. "Remaining Work: None" is only valid
  when nothing in the task is faked, stubbed, or deferred.
- Before writing the handoff, grep your diff for `InMemory`, `stub`, `fake`, `synthetic`, and `TODO`, and account
  for every hit in Remaining Work.
- Re-read the task's Acceptance Criteria last, and confirm each one against the code you actually wrote, not the
  code you planned to write.

## Rules

- Work on one task per run.
- Do not build P1/P2 features unless the selected task explicitly requires them.
- Do not add dependencies before proving the standard library or existing dependencies are insufficient.
- Do not modify `.golangci.yml` to silence failures.
- Do not use `--no-verify`.
- Do not add `//nolint` comments.
- Treat linter failures as work to fix, not as pre-existing excuses.

## Stop Conditions

Stop and ask before editing further if:

- the task conflicts with `docs/planning`;
- the task requires an open decision from `docs/planning/32-open-decisions.md`;
- a new dependency seems necessary;
- verification fails for a reason outside the selected task;
- completing the task requires changing another task's scope;
- you must defer behavior that no existing task file owns.

## Good Invocation

```text
Use /straw-task-runner to complete the next unblocked P0 task.
```

For a specific task:

```text
Use /straw-task-runner to complete docs/tasks/p0/03-nats-connection-and-subjects.md.
```
