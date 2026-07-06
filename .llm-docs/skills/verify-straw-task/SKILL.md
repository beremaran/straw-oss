---
name: verify-straw-task
description: Audit one Straw task, a batch, or a whole phase board for real completeness — acceptance criteria verified against code with file:line evidence, wiring confirmed in the built binaries, deferrals checked for ownership. Use when the user asks to verify/audit task completion, asks for a go/no-go on a phase, or doubts a "done" status.
---

# Verify Straw Task

Verify that "done" means done. This repo's failure pattern is "looks done, isn't done": checked-off wiring steps
backed by config validation only, "Remaining Work: None" over in-memory fakes, gaps flagged as "no owning task" and
then forgotten. Audit against those patterns specifically.

## Two invocation modes

- **Audit mode** (user-initiated): one task, a batch, or a board — full procedure below.
- **Independent-verifier mode** (invoked by straw-task-runner step 12): you receive only a task file and a diff.
  Judge the diff against the acceptance criteria with fresh eyes; do not ask the implementer for its reasoning and
  do not trust its claims — the point of your existence is that you don't share its assumptions. Return the
  per-criterion verdict table (criterion | VERIFIED/NOT MET | file:line | proving test) and nothing softer than a
  binary verdict per criterion. You never edit files or statuses in this mode.

## Ground rules

- Evidence or it didn't happen: every verdict cites file:line in *current* code. Never accept the task file's
  checkboxes, the handoff's claims, or a code comment as proof — all three go stale (comments have claimed wiring
  "not yet consumed" that was in fact live).
- Verify against the working tree, and check `git status` first: work can be complete-but-uncommitted (task 41 was),
  which is a finding of its own.
- If a task spec and a planning doc disagree, the planning doc wins (see the task's own phase board's `## Notes`,
  e.g. `docs/tasks/p0.md`, `p1.md`, or `p2.md`).

## Procedure

### 1. Scope and fan-out

For a single task: do it inline. For a batch or a board: use sub-agents (Explore/general-purpose), one per cluster
of related tasks, plus one agent that verifies the phase's scope list in
`docs/planning/02-phase-boundaries.md` bullet-by-bullet against the code. Give each agent the exact task file list,
the verdict format below, and the wiring standard. Run `make check` in parallel with the agents.

### 2. Per-task verification

For each task file:

1. Read its Acceptance Criteria and Steps.
2. For each criterion, find the implementing code and the proving test; record file:line. Judge
   IMPLEMENTED / PARTIAL / MISSING per criterion, then YES / PARTIAL / NO per task.
3. Apply the wiring standard: "wire/connect/integrate" is only met when the built binary (`cmd/control/main.go`,
   `cmd/egress/main.go`) constructs the real component on the runtime path. Grep the binaries for the constructor.
   `InMemory*`/fake/stub reachable from `main.go` fails the criterion regardless of test coverage.
4. Read the handoff in `docs/agents/handoffs/`. For every deferral / Remaining Work / Known limitation entry:
   - Does it name an owning task file? Does that file exist and is it still open?
   - Self-referential deferrals ("owned by this task if pursued" on a done task) are UNOWNED.
   - Is the flag stale — did a later task already fix it? Verify in code; stale flags are findings too.
5. Note criteria satisfied only by tests/fakes with no live-path reachability.

### 3. Full-suite verification

- `make check` must pass.
- Postgres-backed tests are silently skipped without the env var — run them explicitly:
  `STRAW_TEST_POSTGRES_DSN='postgres://postgres:postgres@localhost:5432/straw_test?sslmode=disable' go test ./...`
  (compose stack must be up; the harness refuses non-`_test` database names by design). A green `make check`
  without this line did NOT exercise the Postgres stores — say so rather than claiming full coverage.

### 4. Report

Lead with the verdict (go/no-go or per-task YES/PARTIAL/NO), then:

- a table: task | claim | verified | unmet criteria with evidence;
- a cross-cutting section listing every unowned or stale deferral found;
- housekeeping findings (uncommitted done-work, stale comments/notes, boards out of sync).

Distinguish three severities: broken acceptance criteria (blocks), unowned gaps (must be tasked before sign-off —
per AGENTS.md Gap Ownership, offer to create owners via `.llm-docs/skills/write-straw-task`), and stale
documentation (fix inline, it's cheap).

### 5. Feed the lesson back

If the audit surfaces a failure pattern that `.llm-docs/skills/straw-task-runner` does not already warn about,
append it to that skill's "Completion Audit" rules or this skill's trap list in the same run (both live under
`.llm-docs/skills/`). The 2026-07-05 lessons only got encoded because someone did this — an audit whose findings
don't harden the process will be re-run from scratch next quarter.

## Traps seen in past audits

- Rate limiter/quota existed twice (admin-surface and dispatcher instances); a comment about one was misread as the
  other being unwired. Trace the actual request path (`handler.go` → `dispatcher.go`) before believing any claim.
- Struct fields and DB columns existing does not mean they are populated — check the emitter
  (e.g. `ConfigAuditEvent` had the fields; `audit.go` enqueued them empty).
- "E2E test exists" may mean an in-process fake-transport round-trip, not a compose-stack test. Name which one.
- Out of Scope lines get over-read; check the planning doc's phase table yourself for skipped in-phase surface.
- A deferral that names only a phase ("is a P1 concern", "rollback (P1)") without a task file path is UNOWNED.
  Two 2026-07-06 sweep finds hid this way: credit replenishment (handoff 24 → now `p1/24`) and tenant fields
  (handoff 29 → now `p0/45`, where the "P1" label itself was wrong per `docs/planning/26`). Phase labels in
  handoffs are claims to verify against the planning doc, not ownership.
