Use this structure:

```text
AGENTS.md
docs/
  planning/
    ...
  implementation/
    P0-TASKS.md
    P0-ACCEPTANCE.md
    P0-DECISIONS.md
    P0-NON-GOALS.md
    P0-TEST-MATRIX.md
tasks/
  p0/
    000-repo-scaffold.md
    001-protobuf-contract.md
    002-config-loader.md
    003-postgres-schema.md
    004-nats-core.md
    005-auth-api-keys.md
    006-worker-registration.md
    007-worker-heartbeat-state.md
    008-routing-snapshot.md
    009-assignment-streaming.md
    010-rest-requests-endpoint.md
    011-egress-http-execution.md
    012-destination-policy.md
    013-error-mapping.md
    014-rate-limits-quotas.md
    015-clickhouse-metadata.md
    016-docker-compose-e2e.md
```

Do **not** make a giant “implement P0” task. That is where agents become unreliable. Make each task independently reviewable.

## `AGENTS.md`

This should be short and strict. Something like:

```markdown
# AGENTS.md

## Project

Straw is a distributed HTTP/HTTPS proxy system. P0 is a vertical slice only.

The canonical planning documents are in `docs/planning/`. Do not implement P1/P2 features unless a task explicitly says so.

## Hard P0 exclusions

Do not implement:

- HTTP forward proxy
- raw CONNECT
- MITM
- Provider Adapters
- BodyRef / object storage large-body transport
- external REST response streaming
- payload capture
- redirect following
- HTTP/2
- upstream keep-alive pooling

## Development rules

- Prefer small, reviewable changes.
- Do not change public contracts without updating the matching planning document and tests.
- Do not introduce competing schemas or duplicate protocol definitions.
- Keep Control as the only service that reads Postgres, Redis config state, and ClickHouse.
- Executors must only use resolved per-request instructions from Control.
- Add or update tests with each behavior change.
- Run formatting, linting, unit tests, and relevant integration tests before reporting completion.

## Architecture source of truth

Use these canonical sections:

- API: `docs/planning/07-public-api-surface.md`
- Request lifecycle: `docs/planning/09-canonical-request-lifecycle.md`
- Routing: `docs/planning/10-routing-model.md`
- Worker state: `docs/planning/11-worker-discovery-and-health.md`
- NATS protocol: `docs/planning/12-nats-protocol.md`
- Protobuf: `docs/planning/13-protobuf-contract.md`
- Error registry: `docs/planning/14-canonical-error-registry.md`
- Egress behavior: `docs/planning/16-egress-execution.md`
- Storage: `docs/planning/21-state-and-storage.md`
- ClickHouse: `docs/planning/22-canonical-clickhouse-schema.md`
- Security: `docs/planning/27-security-controls.md`
- Tests: `docs/planning/30-testing-matrix.md`

## Done means

A task is not complete until:

- code is implemented,
- tests are added or updated,
- generated files are refreshed if relevant,
- docs are updated if behavior changed,
- known limitations are listed in the task result.
```

## Task file format

Each task should be highly mechanical. Use this template:

```markdown
# Task: <short name>

## Goal

One or two sentences.

## Source-of-truth docs

- `docs/planning/...`
- `docs/planning/...`

## In scope

- ...
- ...

## Out of scope

- ...
- ...

## Required behavior

- ...
- ...

## Tests required

- ...
- ...

## Acceptance criteria

- [ ] ...
- [ ] ...
- [ ] ...

## Notes for Codex

- Do not implement adjacent features.
- Stop and report if the existing code structure conflicts with the planning docs.
```

## Example: first implementation task

```markdown
# Task 001: Protobuf Contract

## Goal

Create the canonical P0 protobuf contract for Straw NATS messages and public error structures.

## Source-of-truth docs

- `docs/planning/12-nats-protocol.md`
- `docs/planning/13-protobuf-contract.md`
- `docs/planning/14-canonical-error-registry.md`

## In scope

- Add `proto/straw/v1/straw.proto`.
- Define `Envelope`, registration, heartbeat, assignment, stream frames, destination policy, error types, headers, and request/response frame messages.
- Add Buf config.
- Add Go generation setup.
- Add generated Go files if this repo commits generated code.

## Out of scope

- NATS runtime implementation.
- HTTP server implementation.
- Egress execution.
- Postgres schema.

## Required behavior

- proto3 syntax.
- No `required` fields.
- Unknown fields tolerated.
- Unknown enum values rejected by business validation.
- Enum values use prefixed names.
- Zero enum values are unspecified unless explicitly safe.
- Headers are repeated ordered pairs, not maps.

## Tests required

- Buf lint passes.
- Buf breaking check is wired in CI or documented for initial baseline.
- Unit test or validation test rejects unknown/unspecified enum values where required.

## Acceptance criteria

- [ ] `proto/straw/v1/straw.proto` exists.
- [ ] Generated Go package is usable.
- [ ] Error codes match Section 14.
- [ ] StreamFrame payloads match Section 13.
- [ ] DestinationPolicy includes resolution mode and SSRF policy fields.
- [ ] Tests pass.
```

## Recommended Codex workflow

Use a strict loop:

1. Open one task file.
2. Tell Codex: “Implement only this task. Do not start the next task.”
3. Ask it to first inspect relevant docs and summarize the implementation plan.
4. Let it modify files.
5. Ask it to run tests.
6. Review the diff.
7. If acceptable, commit.
8. Start a new Codex task/thread for the next task.

Keep separate Codex threads for separate tasks. Do not let one long thread carry the whole project. Long context tends to blur boundaries.

## Task ordering I would use

Start with foundations before runtime behavior:

```text
000 repo scaffold
001 protobuf contract
002 config loader
003 Postgres schema
004 NATS connection and subject helpers
005 auth/API key model
006 worker registration
007 heartbeat/state machine
008 routing snapshots
009 assignment and stream lifecycle
010 REST /api/v1/requests
011 Egress outbound HTTP
012 destination policy enforcement
013 error mapping
014 rate limits and quotas
015 ClickHouse metadata writes
016 docker-compose E2E
```

Do not start REST request transport before the protobuf/NATS/auth/schema foundations exist. Otherwise Codex will invent placeholders that you later have to unwind.

## Add a “contract drift” checklist

Create `docs/implementation/P0-ACCEPTANCE.md`:

```markdown
# P0 Acceptance Checklist

- [ ] No P1/P2 features implemented.
- [ ] Public REST API matches Section 7.
- [ ] NATS subjects and sequencing match Section 12.
- [ ] Protobuf fields and enums match Section 13.
- [ ] Error codes and HTTP statuses match Section 14.
- [ ] Origin 4xx/5xx responses are not converted into Straw errors.
- [ ] Worker IDs and session IDs never appear in public ErrorResponse.
- [ ] Egress does not query Postgres, Redis, or ClickHouse.
- [ ] Destination deny checks run in Control and Egress.
- [ ] Direct DNS mode dials only validated IPs.
- [ ] Upstream proxy remote resolution is opt-in.
- [ ] Metadata redaction rules are enforced before logs/ClickHouse.
- [ ] Tests cover every P0 row in Section 30.
```

## Add a “known no-go” file

This reduces accidental feature creep:

```markdown
# P0 Non-Goals for Implementation

Do not implement these during P0:

- CONNECT
- MITM
- HTTP forward proxy
- external streaming response endpoint
- BodyRef
- object storage body transport
- payload capture
- Provider Adapters
- HTTP/2
- redirect following
- upstream connection pooling
- SDK beyond minimal generated/prototype client
- UI
- CLI beyond smoke tooling
```

## One more practical recommendation

Before using Codex heavily, create the repo skeleton manually or with one very small Codex task:

```text
cmd/
  control/
  egress/
internal/
  control/
  egress/
  proto/
  natsx/
  config/
  auth/
  routing/
  storage/
  observability/
proto/
  straw/v1/
migrations/
deploy/
  docker-compose.yml
```

Then Codex has somewhere predictable to put code. Without a skeleton, it may invent structure inconsistently across tasks.

Bottom line: yes, create task files. Keep them narrow, reference the planning docs directly, and make every task end in tests plus acceptance criteria. That is the best way to make Codex reliable on this kind of system.

[1]: https://developers.openai.com/codex/guides/agents-md?utm_source=chatgpt.com "Custom instructions with AGENTS.md – Codex"
[2]: https://developers.openai.com/codex/cli?utm_source=chatgpt.com "Codex CLI"
