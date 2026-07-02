# Agent Workflow

Use this flow for every task.

1. Select one open task from `docs/tasks/p0.md`.
2. Read the task file and its named planning docs.
3. Confirm prerequisites are already complete.
4. Add or update the smallest relevant tests.
5. Implement only the behavior in the task.
6. Run `make check`.
7. Record what changed and what remains in a handoff note.

Do not work multiple P0 tasks in one pass unless the task explicitly says it is a cleanup or integration task.

## Status Values

- `not started`: no implementation committed.
- `in progress`: active local work exists.
- `blocked`: cannot continue without a decision or prerequisite.
- `review`: implementation and checks are ready for review.
- `done`: reviewed and accepted.
