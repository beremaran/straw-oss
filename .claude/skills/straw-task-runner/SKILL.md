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
9. Update task status only after verification.
10. Write a handoff note using `docs/agents/templates/handoff.md`.

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
- completing the task requires changing another task's scope.

## Good Invocation

```text
Use /straw-task-runner to complete the next unblocked P0 task.
```

For a specific task:

```text
Use /straw-task-runner to complete docs/tasks/p0/03-nats-connection-and-subjects.md.
```
