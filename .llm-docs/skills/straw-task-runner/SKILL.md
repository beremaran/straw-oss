---
name: straw-task-runner
description: Complete one Straw repository task from docs/tasks (p0, p1, or p2). Use when the user asks to complete the next task, work on a specific docs/tasks/{p0,p1,p2}/*.md file, continue implementation from a phase board, or produce a task handoff for this repo.
---

# Straw Task Runner

Use this skill to complete exactly one Straw task safely.

## Sub-agents

You are the owning agent: only you edit files, run verification commands, update statuses, and write the handoff.
Delegate to sub-agents so your context stays on the task instead of filling with exploration:

- **Research fan-out** (read-only): locating code, tracing a flow end to end, summarizing a planning doc section,
  finding every caller of a function. Prefer parallel agents for independent questions.
- **Test triage**: when `make check` fails unexpectedly, hand a sub-agent the failure output and the diff to
  localize the cause while you keep the task state.
- **Independent verification** (workflow step 12): mandatory, and it must be a *fresh* agent — see below.

Sub-agents never edit files, never flip task/board statuses, and never start other tasks.

## Workflow

1. Read `AGENTS.md`, especially the Gap Ownership section.
2. Choose the task:
   - If the user names a task file, use that file.
   - Otherwise use the first `not started` or `in progress` task on the phase board whose prerequisites are
     satisfied.
3. Read the chosen task file completely.
4. Read only the planning docs named in the task file.
5. Build a **coverage table** from the cited planning-doc sections: every in-phase field, endpoint, and behavior
   they define, one row each. You will account for every row at handoff; this is what catches "the planning doc
   listed six fields and the task shipped three" at implementation time instead of at audit time.
6. Check current git status and avoid reverting unrelated user changes.
7. Implement only the selected task.
8. Update tests with the smallest checks that prove the task behavior.
9. Run focused tests first, then `make check`. Two silent-skip holes to close explicitly:
   - **Postgres**: `make check` is green without Postgres coverage when `STRAW_TEST_POSTGRES_DSN` is unset. If the
     diff touches `postgres_*` files or `migrations/`, also run
     `STRAW_TEST_POSTGRES_DSN='postgres://postgres:postgres@localhost:5432/straw_test?sslmode=disable' go test ./...`
     (dedicated `straw_test` database only — see `deploy/docker/README.md`). Record in the handoff whether
     Postgres-backed tests ran.
   - **Live path**: unit-green is not live-green (tasks 39-41 all came from gaps only a real request exposed). If
     the task touches the runtime request path, drive one real request through the compose stack when it is
     available; otherwise record in the handoff that live verification was skipped and why.
10. Run the Completion Audit below yourself.
11. Complete the coverage table: mark each row `implemented` / `already existed` (evidence) / `out of scope`
    (naming the owning task file). An unaccounted row is a stop condition, not a judgment call.
12. **Independent verification — do not grade your own homework.** Spawn a fresh sub-agent given ONLY the task file
    and the diff (not your reasoning or your claims), instructed to follow
    `.llm-docs/skills/verify-straw-task` and return a per-acceptance-criterion verdict table with file:line
    evidence. Every "looks done, isn't done" incident in this repo survived a self-audit; this step exists because
    a verifier without the implementer's assumptions catches what the implementer cannot. Fix anything not
    verified and re-run the verifier. Status does not flip on your own judgment alone.
13. Update statuses only after step 12 passes — and all three locations must agree: the task file `Status:` line,
    the phase board row, and the task's step checkboxes.
14. Write a handoff note using `docs/agents/templates/handoff.md`, including the verifier's per-criterion verdict
    table, the coverage-table outcome, and the Postgres/live-verification statements from step 9.
15. Commit the work (all local checks run; never `--no-verify`). If you cannot commit, the handoff's Blockers
    section and your final message to the user must both state that the work is uncommitted — done-on-the-board
    but uncommitted is invisible to anyone pulling the repo.

## Completion Audit

Before marking any step or task done, verify each claim against the definitions below. Past runs checked off
"wire connection setup" after adding only config validation, and wrote "Remaining Work: None" for code backed
entirely by in-memory fakes. Do not repeat that.

- "Wire", "connect", or "integrate" means the built binary (`cmd/control`, `cmd/egress`) actually constructs and
  uses the real component at runtime. Startup validation, subject helpers, or interface definitions do NOT satisfy
  a wiring step. If the task means library-only work, it says so explicitly.
- A step you did not fully do stays unchecked. Partially done = unchecked, with a note on what remains.
- Every behavior you defer must name the exact task file that owns it (e.g. "deferred to
  `docs/tasks/p0/13-rate-limits-quotas-redis.md`"). If no existing task owns it, create the owning task in the same
  run using `.llm-docs/skills/write-straw-task` (and add it to the phase board), or stop and ask the user. Never
  write "no owning task exists" and move on — the 2026-07-05 audit found five such flags (tasks 20, 22, 24, 30, 32)
  that sat invisible until a full re-audit. Unowned deferrals are how work silently disappears.
- Deferring to the task you are currently completing ("owned by this task if pursued") counts as unowned — the task
  is about to be marked done, so nothing will ever pursue it.
- Out of Scope can still hide a deferral. Before marking done, grep the task file, handoff, and diff for
  `no owning task`, `no owner`, `future work`, and `if needed later`; every hit must be resolved, explicitly
  not-a-gap, or name an owning task file.
- A task must not be marked done on its board while its handoff contains an unowned deferral.
- If your work closes a gap that an earlier task file or handoff documents as open (a "Known limitation" or
  Remaining Work entry), update that earlier note in the same run so it does not go stale.
- That includes soft flags outside Remaining Work: earlier handoffs' Notes/Deviations bullets ("not added",
  "in-process only", "add X when task N needs it"). The 2026-07-06 sweep found three such notes (handoffs 08, 09,
  12) closed by later code but never updated — grep past handoffs for the surface you just built, not only their
  Remaining Work sections.
- Phase-scope discipline cuts both ways: an Out of Scope line like "do not add P1 fields" never excuses skipping
  fields or endpoints the named planning doc marks P0. The coverage table (workflow steps 5/11) is the artifact
  that proves you checked.
- If you introduce an in-memory or fake implementation of something the planning docs require to be backed by a
  real system (NATS, Postgres, Redis, ClickHouse — see `docs/planning/21-state-and-storage.md`), the handoff's
  Remaining Work section must list the real-backend swap and its owning task. "Remaining Work: None" is only valid
  when nothing in the task is faked, stubbed, or deferred.
- Before writing the handoff, grep your diff for `InMemory`, `stub`, `fake`, `synthetic`, and `TODO`, and account
  for every hit in Remaining Work.
- Re-read the task's Acceptance Criteria last, and confirm each one against the code you actually wrote, not the
  code you planned to write — then hand the same question to the independent verifier (workflow step 12).

## Rules

- Work on one task per run.
- Do not build P1/P2 features unless the selected task explicitly requires them.
- Do not add dependencies before proving the standard library or existing dependencies are insufficient.
- Do not modify `.golangci.yml` to silence failures.
- Do not use `--no-verify`.
- Do not add `//nolint` comments.
- Treat linter failures as work to fix, not as pre-existing excuses.
- Sub-agents research and verify; they never edit, never flip statuses, never expand scope.

## Stop Conditions

Stop and ask before editing further if:

- the task conflicts with `docs/planning`;
- the task requires an open decision from `docs/planning/32-open-decisions.md`;
- a new dependency seems necessary;
- verification fails for a reason outside the selected task;
- completing the task requires changing another task's scope;
- you must defer behavior that no existing task file owns and creating the owner is not clearly in-phase;
- a coverage-table row (workflow step 11) cannot be accounted for.

## Good Invocation

```text
Use /straw-task-runner to complete the next unblocked task.
```

For a specific task:

```text
Use /straw-task-runner to complete docs/tasks/p0/03-nats-connection-and-subjects.md.
```
