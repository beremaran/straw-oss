# 10 - CLI

Status: not started

## Objective

Add a minimal CLI over the Go SDK and P0/P1 config/admin endpoints.

## Required Planning Docs

- `docs/planning/07-public-api-surface.md`
- `docs/planning/26-config-management-api-surface.md`

## Prerequisites

- Task 09 completed.
- P0 task 20 completed.

## Out of Scope

- Do not build an interactive UI.
- Do not add auth modes beyond API keys.
- Do not implement provider marketplace commands.

## Expected Files

- Create: CLI command package or `cmd/straw` according to repo convention.
- Test: CLI command tests.

## Steps

- [ ] Read all required planning docs.
- [ ] Specify the CLI command set before implementation.
- [ ] Implement request submission through the Go SDK.
- [ ] Implement read/write config commands for available config endpoints.
- [ ] Implement worker/admin read and action commands where P0/P1 APIs exist.
- [ ] Render canonical errors without leaking secrets.
- [ ] Add tests for command parsing, API-key loading, request command, config commands, and error output.
- [ ] Run focused CLI tests.
- [ ] Run `make check`.
- [ ] Write a handoff note.

## Tests

- Focused CLI tests.
- `make check`

## Acceptance Criteria

- CLI commands cover the documented minimal API surface.
- The command set is documented in the task handoff.
- Secrets are not printed except one-time key create responses when explicitly requested.

## Handoff Notes

- List every command and environment variable used.

## Stop Conditions

- Stop before adding UI or marketplace workflows.
- Stop if a deferral would have no owning task file.
