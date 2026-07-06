---
name: dig-straw-task-handoffs
description: Sweep completed task handoffs in docs/agents/handoffs/ for deferred, partial, or unowned work; verify each flag against current code; create owning tasks (via write-straw-task) for anything real and unowned. Use when the user asks to find leftover/hidden work, after a batch of tasks completes, or periodically as debt hygiene.
---

# Dig Straw Task Handoffs

Handoffs are where undone work hides. Past handoffs honestly wrote "no owning task exists for this" — and because
nothing swept them, those gaps stayed invisible until a full P0 audit rediscovered them (tasks 20, 22, 24, 30, 32).
This skill is that sweep.

## Procedure

### 1. Harvest

Read every file in `docs/agents/handoffs/` (fan out sub-agents over chunks of ~10 if there are many). From each,
extract every entry under or resembling: `Remaining Work`, `Blockers`, `Known limitation`, `deferred`, `flagged`,
`gap`, `not implemented`, `no owning task`, `P1-leaning`, `if pursued`. Also harvest "Known limitation" sections in
the task files themselves (`docs/tasks/*/**.md`) — task 25 carried one that went stale there, not in a handoff.

Record per item: source file, quoted claim, claimed owner (task path or "none").

### 2. Classify — against current code, not against the handoff's era

Handoffs describe the code as it was. For every harvested item, verify in the working tree before classifying:

- **RESOLVED**: a later task already closed it (grep/read the code to confirm). Finding: the flagging doc is stale —
  update its note in place to say which task closed it (see task 25's rewritten "Known limitation" for the pattern).
- **OWNED**: names an owning task file that exists and is still open on its board. Confirm the owner's text actually
  covers the deferred behavior — a topical-overlap guess is not ownership. No action if genuinely covered.
- **UNOWNED**: no owner, owner file missing, owner already `done` without having done it, or self-referential
  ("owned by this task if pursued" on a completed task). Action required.
- **NOT-A-GAP**: explicitly out of phase per `docs/planning/02-phase-boundaries.md` and correctly parked (e.g. a P2
  feature). No action beyond noting it.

### 3. Task the unowned

For every UNOWNED item, invoke `.llm-docs/skills/write-straw-task` to create the owning task:

- Phase = whatever phase the planning docs assign the surface to (a P0-spec gap gets a P0 task even after P0
  shipped — see p0/42-44).
- The new task's Context section must cite the flagging handoff and the current-code evidence you gathered in
  step 2.
- Update the flagging handoff/task doc: replace "no owning task" with the new task's path.
- Add the board row.

If an item is too ambiguous to task (needs a product decision), don't invent scope — list it in the report as
needs-decision with the specific question.

### 4. Report

- Table: item | source | classification | action taken (task created / doc updated / none).
- Totals: how many flags swept, how many were stale, how many new tasks created.
- The needs-decision list, if any.
- End state must satisfy AGENTS.md Gap Ownership: zero UNOWNED items remain — each is either tasked, updated as
  resolved, or escalated to the user as needs-decision.

### 5. Feed the lesson back

If the sweep reveals a *category* of leak the process doesn't guard against (not just individual items), append the
pattern to `.llm-docs/skills/straw-task-runner`'s Completion Audit or `.llm-docs/skills/verify-straw-task`'s trap
list in the same run, so the next runner is warned instead of the next sweep rediscovering it.

## Notes

- Read-mostly skill: it edits only docs (handoffs, task files, boards) and never implementation code. Fixing a gap
  belongs to the task it creates, executed later by straw-task-runner.
- Deduplicate before tasking: several handoffs may flag the same gap (log_events was flagged by 32 and 37 and is
  owned once, by `docs/tasks/p1/20-log-events-ingestion.md`). One gap, one owner.
