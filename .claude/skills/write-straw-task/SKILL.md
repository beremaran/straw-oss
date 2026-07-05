---
name: write-straw-task
description: Author a new Straw task file for any phase (docs/tasks/p0|p1|p2) that meets this repo's task standard, and register it on the phase board. Use when a gap needs an owning task, when planning work is being decomposed into tasks, or when another skill (straw-task-runner, dig-straw-task-handoffs, verify-straw-task) needs to create an owner for a deferral.
---

# Write Straw Task

Produce one task file that a fresh agent can execute with no other context than the repo. A task file is a
contract: if an executing agent can satisfy the Acceptance Criteria while the intended behavior still does not
exist, the task file failed. Write against that failure mode.

## Before writing

1. Read `AGENTS.md`, especially the Gap Ownership section.
2. Read 2-3 recent task files in the target phase directory as format exemplars
   (`docs/tasks/p0/30-executor-pool-config-api.md` and `docs/tasks/p0/44-config-audit-event-enrichment.md` are good
   models). Match their structure exactly.
3. Read the planning doc sections the task will cite. Never cite a planning doc you have not opened — tasks citing
   unread docs are how scope gets misread (task 30 skipped P0 pool fields because "P1 fields" was assumed, not
   checked).
4. Verify the gap is real in *current* code before tasking it: grep/read the code and record file:line evidence in
   the Context section. Do not write a task for something a later change already fixed.
5. Check no existing open task already owns this work (grep `docs/tasks/` for the key terms). Extending an existing
   open task beats creating a duplicate.
6. Delegate to read-only sub-agents where it keeps you on track: gathering the current-code evidence when the gap
   spans several files, or sweeping `docs/tasks/` for an existing owner. You write the task file yourself —
   sub-agents research, they don't author.

## Phase placement

- If a planning doc marks the surface P0/P1/P2, the task goes in that phase's directory — even if the phase is
  "done"; gap-closure tasks land in their spec's phase (see p0 tasks 42-44).
- If phase membership is ambiguous in the planning docs, stop and ask; do not guess.
- Number = next unused integer in that phase directory. Filename: `NN-kebab-case-slug.md`.

## Required structure (every section, in this order)

```markdown
# NN - Title

Status: not started

## Objective        — one paragraph; the observable behavior that will exist when done.
## Context (gap being closed)  — provenance: which audit/handoff/review found this, with file:line evidence in
                       current code; why the gap exists (what earlier task misread or deferred). This is what stops
                       the next agent from re-litigating scope.
## Required Planning Docs      — exact paths, with section/line hints (e.g. "P0 Deny Rule schema, lines ~315-328").
                       Only docs the executor must read.
## Prerequisites    — task numbers that must be done first, and why in parentheses.
## Out of Scope     — explicit non-goals. NEVER write a scope exclusion that could be read as covering something the
                       planning doc marks in-phase; name the excluded items precisely.
## Expected Files   — Add/Modify/Test lists with real paths. Check the paths exist (or say "Add:"). This is the
                       executor's map; wrong paths send it exploring.
## Steps            — ordered checkboxes, each independently checkable. First step is always "Read all required
                       planning docs." Include "Run focused tests, then `make check`." and "Write a handoff note."
                       A wiring step must say which binary (`cmd/control`, `cmd/egress`) constructs the component.
## Tests            — the commands (`go test ./internal/control`, `make check`).
## Acceptance Criteria         — falsifiable statements an auditor can verify against code, each ideally naming the
                       proof (a test, a grep that must come back empty, a behavior). Bad: "pool fields supported."
                       Good: "a worker outside allowed_countries is not assignable for that pool, proven by a
                       routing test." If the task closes a flagged gap, include a criterion that the flag/comment
                       is removed or updated.
## Handoff Notes    — what decisions/interpretations the handoff must record.
## Stop Conditions  — task-specific stops, always ending with:
                       "Stop if a deferral would have no owning task file."
```

## Sizing

One task = one vertical slice completable in one focused run: schema + store + handler + wiring + tests for ONE
behavior. If the Steps list needs more than ~10 checkboxes or touches both binaries for unrelated reasons, split it
and cross-link the halves as prerequisites. When splitting is debatable, propose the split to the user instead of
shipping a too-big task.

## Register the task

- Add a row to the phase board (`docs/tasks/p0.md` / `p1.md` / `p2.md`) with status `not started` and a relative
  link.
- If the task was created to adopt a flagged gap, add or update the board's Notes section naming the audit/handoff
  that surfaced it and stating the gap is now owned. If the flagging handoff or task doc says "no owning task",
  update that sentence to name this task.

## Self-check before finishing

- Could an agent check every Acceptance Criterion mechanically? (If a criterion needs interpretation, rewrite it.)
- Does Context prove the gap exists in current code, with evidence?
- Does every cited planning doc path exist, and did you read the cited section?
- Is the board row added and any stale "unowned" flag updated?
