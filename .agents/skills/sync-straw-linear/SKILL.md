---
name: sync-straw-linear
description: Sync Straw repository planning and task docs to Linear for the WiseShopper team and Straw project. Use when asked to publish, mirror, reconcile, backfill, or update local docs/planning and docs/tasks content in Linear issues or documents, especially for keeping Straw phase boards, task files, and planning docs aligned with Linear.
---

# Sync Straw Linear

Sync local Straw docs to Linear without creating duplicates.

Defaults:

- Linear team: `WiseShopper`
- Linear project: `Straw`
- Source repo root: `/Users/beremaran/projects/wiseshopper/straw`
- Planning docs: `docs/planning/*.md`
- Task boards: `docs/tasks/p0.md`, `docs/tasks/p1.md`, `docs/tasks/p2.md`
- Task files: `docs/tasks/p0/*.md`, `docs/tasks/p1/*.md`, `docs/tasks/p2/*.md`

## Workflow

1. Read `AGENTS.md` and the local docs requested by the user. If no subset is named, sync task boards and task
   files first; sync planning docs only when requested or needed for issue context.
2. Use the Linear plugin/app. If Linear tools are not available, ask the user to connect Linear and stop.
3. Verify or locate the Linear project with search terms like `"Straw" "WiseShopper"`. Create the project only if
   the user explicitly asked for bootstrap and search finds no existing project.
4. Build a local inventory:
   - board rows from `docs/tasks/p0.md`, `p1.md`, `p2.md`;
   - each task file title, `Status:`, prerequisites, objective, tests, acceptance criteria, and path;
   - planning docs as whole-document Markdown, preserving headings.
5. Search Linear before every write. Prefer exact source-path matches in descriptions/documents, then exact title
   matches inside project `Straw`.
6. Upsert Linear documents for long-lived docs:
   - one document per planning doc, titled `Straw Planning: <filename stem>`;
   - one document per phase board, titled `Straw Tasks: P0`, `P1`, or `P2`;
   - include `Source: docs/...` near the top.
7. Upsert Linear issues for task files:
   - title format: `Straw <phase>-<number>: <task title>`;
   - team `WiseShopper`, project `Straw`;
   - description includes `Source: docs/tasks/...`, status, objective, prerequisites, tests, acceptance criteria,
     and a short sync note;
   - labels may include `straw`, phase (`p0`, `p1`, `p2`), and local status if those labels already exist. Do not
     create labels unless asked.
8. Preserve Linear-owned fields unless the user asks otherwise. Do not overwrite assignee, estimate, priority, cycle,
   due date, or state from local docs.
9. Map local task status conservatively:
   - `done` may move to a completed Linear state only if the user asked for state sync and the state name is known;
   - `not started` / `in progress` can be written in the description without changing Linear state unless requested.
10. For deletions, archive/close nothing unless explicitly requested. Report local files that no longer have a
    matching source path instead.
11. After writes, return a compact summary: created, updated, unchanged, skipped, and ambiguous matches that need a
    human decision.

## Duplicate Avoidance

Search queries to try before creating:

- `"Source: docs/tasks/p0/12-error-registry-and-mapping.md"`
- `"Straw p0-12"`
- `"12 - Error Registry and Mapping"`
- `"Source: docs/planning/04-canonical-architecture.md"`

If multiple Linear records plausibly match one source file, stop that item and report the candidates. Do not guess.

## Large Bodies

Use Linear documents for large planning docs and phase boards. Keep issue descriptions bounded to the task contract,
not every cited planning paragraph.

If a sync requires sending many large docs or preserving full document bodies beyond what the Linear plugin can
reliably accept, ask the user for one of:

- a Linear API token for a local script using the Linear GraphQL API;
- permission to add a repo-local sync script;
- a smaller batch size or planning-doc subset.

Do not add a token, CLI dependency, or script by default. The plugin tools are enough until a real body-size or
bulk-rate limit fails.
