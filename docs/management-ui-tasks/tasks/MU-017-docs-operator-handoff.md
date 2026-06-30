# MU-017: Operator Documentation And Implementation Handoff

Status: done
Phase: 4
Depends on: MU-001 through MU-014
Search tags: docs, runbook, frontend commands, backend gaps, unsupported actions, operator handoff

## Objective

Document how to run, build, test, and operate the Management UI after implementation.

## Scope

- Document local development commands and production build output.
- Document required Management API URL and token behavior.
- Document first-release unsupported actions so operators do not look for missing controls.
- Link the UI spec, Management API docs, OpenAPI reference, and backend task backlog where relevant.
- Note deployment assumptions chosen during implementation.

## Repo Touchpoints

- `docs/management-ui-spec.md`
- `docs/index.md`, only if publishing the UI docs is desired
- `web/management/README.md`
- `docs/management-ui-tasks/*`

## Implementation Tasks

- [x] Add frontend run/build/test instructions.
- [x] Add token storage warning and sign-out behavior notes.
- [x] Add backend gap list matching the System page.
- [x] Add troubleshooting notes for `401`, network/CORS failures, unavailable cache controls, and missing usage summaries.
- [x] Update task tracker statuses for completed implementation work.

## Done Criteria

- [x] A new developer can run and test the UI from documentation.
- [x] Operators can identify unsupported first-release backend actions without guessing.
- [x] Docs do not expose secrets or example real tokens.
- [x] Tracker and task files reflect implementation status at handoff.

