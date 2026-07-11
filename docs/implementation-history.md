# Straw implementation history

Straw's P0, P1, and P2 implementation boards are complete. This document retains the useful intent, context,
scope, and implementation decisions from the former per-task records without keeping an active task-board
workflow. Canonical current behavior remains in `docs/planning/`, public docs, code, and tests.

## Completed scope

### P0 Task Board

| # | Status | Task |
|---|--------|------|
| 01 | done | [Repository scaffold, config loader, schema validation, generated protobuf](#p0-01) |
| 02 | done | [Canonical protobuf and Buf](#p0-02) |
| 03 | done | [NATS connection and subjects](#p0-03) |
| 04 | done | [Postgres schema](#p0-04) |
| 05 | done | [Config cache and invalidation](#p0-05) |
| 06 | done | [Control REST request endpoint](#p0-06) |
| 07 | done | [Auth, RBAC, and API keys](#p0-07) |
| 08 | done | [Worker registration, heartbeat, and state](#p0-08) |
| 09 | done | [Routing evaluation](#p0-09) |
| 10 | done | [Assignment and stream lifecycle](#p0-10) |
| 11 | done | [Egress outbound execution](#p0-11) |
| 12 | done | [Error registry and mapping](#p0-12) |
| 13 | done | [Rate limits, quotas, and Redis](#p0-13) |
| 14 | done | [ClickHouse metadata write path](#p0-14) |
| 16 | done | [NATS client foundation](#p0-16) |
| 17 | done | [Worker registration and heartbeat over NATS](#p0-17) |
| 18 | done | [Postgres foundation and identity stores](#p0-18) |
| 19 | done | [Postgres config stores and snapshot assembly](#p0-19) |
| 20 | done | [Config admin APIs for routing, deny, injection, fingerprint](#p0-20) |
| 21 | done | [Redis wiring and config invalidation](#p0-21) |
| 22 | done | [Control destination policy resolution](#p0-22) |
| 23 | done | [Egress assignment execution loop](#p0-23) |
| 24 | done | [Control request dispatch pipeline](#p0-24) |
| 25 | done | [P0 test matrix and compose](#p0-25) |
| 26 | done | [Egress destination policy precedence and suffix enforcement](#p0-26) |
| 27 | done | [Admin request cancellation pipeline](#p0-27) |
| 28 | done | [Control observability metrics](#p0-28) |
| 29 | done | [Tenant lifecycle API and status enforcement](#p0-29) |
| 30 | done | [Executor pool config API](#p0-30) |
| 31 | done | [Config change history API](#p0-31) |
| 32 | done | [Request-outcome and worker/audit telemetry](#p0-32) |
| 33 | done | [Ingress header CR/LF validation](#p0-33) |
| 34 | done | [Lint/vet and test-harness hardening](#p0-34) |
| 35 | done | [Worker registration replay protection and persistent identity key](#p0-35) |
| 36 | done | [Canonical config base path normalization](#p0-36) |
| 37 | done | [Structured JSON logging](#p0-37) |
| 38 | done | [Egress worker health endpoints](#p0-38) |
| 39 | done | [Test suite live-database guard](#p0-39) |
| 40 | done | [Egress registration retry and recovery](#p0-40) |
| 41 | done | [Request phase timing accuracy](#p0-41) |
| 42 | done | [Executor pool capability fields](#p0-42) |
| 43 | done | [Deny rule taxonomy alignment](#p0-43) |
| 44 | done | [Config audit event enrichment](#p0-44) |
| 45 | done | [Redis-backed worker runtime state](#p0-45) |
| 46 | done | [Tenant P0 schema fields](#p0-46) |
| 47 | done | [Live compose verification of tasks 42-45 surfaces](#p0-46) |
| 48 | done | [Deny host_suffix leading-dot normalization fix](#p0-48) |
| 49 | done | [Egress worker capability declaration from config](#p0-49) |
| 50 | done | [Config audit event field_path population](#p0-50) |
| Shared file | Tasks that modify it |
|-------------|----------------------|
| `cmd/control/main.go` (route/startup wiring) | 27, 28, 29, 30, 31, 32, 35, 36, 37 |
| `cmd/egress/main.go` | 35, 37, 38 |
| `internal/control/dispatcher.go` | 27, 28, 32 |
| `internal/control/admin_handlers.go` | 29, 30 |
| `internal/control/config_admin_handlers.go` | 30, 31 |
| `internal/control/request_metadata.go` | 28, 32 |
| `internal/config/config.go` | 35, 38 |

#### Retained board notes

- Planning docs were audited and reconciled on 2026-07-03 (see the "Audit Reconciliation" section of
  `docs/planning/a-reconciliation-notes.md`). Task specs 07-15 were updated to match; if a task spec and a planning
  doc disagree, the planning doc wins.
- Integration audit 2026-07-03: tasks 03-13 built correct, tested domain logic with zero live-backend
  integration. Corrected handoffs and tasks 16-24 record the gaps.
- Task 47 (user-approved 2026-07-06) adopts the "live compose verification: skipped" notes carried by the
  handoffs of tasks 42-45; each of those handoffs now names it as owner.

### P1 Task Board

| # | Status | Task |
|---|--------|------|
| 01 | done | [HTTP proxy ingress spec](#p1-01) |
| 02 | done | [HTTP proxy ingress](#p1-02) |
| 03 | done | [Raw streaming response path](#p1-03) |
| 04 | done | [Routing ingress type and worker capability](#p1-04) |
| 05 | done | [Raw CONNECT tunnel](#p1-05) |
| 06 | done | [REST streaming endpoint](#p1-06) |
| 07 | done | [Config rollback API](#p1-07) |
| 08 | done | [Multi-tenant worker credentials](#p1-08) |
| 09 | done | [Go SDK](#p1-09) |
| 10 | done | [CLI](#p1-10) |
| 11 | done | [Telemetry schema and query limits spec](#p1-11) |
| 12 | done | [Telemetry read APIs](#p1-12) |
| 13 | done | [Observability dashboards](#p1-13) |
| 14 | done | [Minimal admin UI](#p1-14) |
| 15 | done | [Egress metrics exposure](#p1-15) |
| 16 | done | [Upstream connection pooling](#p1-16) |
| 17 | done | [Worker loss and NATS outage hardening](#p1-17) |
| 18 | done | [Load and backpressure testing](#p1-18) |
| 19 | done | [Production deployment templates](#p1-19) |
| 20 | done | [Control log events ClickHouse ingestion](#p1-20) |
| 21 | done | [IDNA hostname support](#p1-21) |
| 22 | done | [Egress credential config schema reconciliation](#p1-22) |
| 23 | done | [Multi-Control durable cancellation state](#p1-23) |
| 24 | done | [Streaming credit replenishment](#p1-24) |
| 25 | done | [CNAME chain inspection for deny rules](#p1-25) |
| 26 | done | [Upstream connection pooling implementation](#p1-26) |
| 27 | done | [Egress log events NATS transport](#p1-27) |
| 28 | done | [SDK and CLI REST streaming client surface](#p1-28) |
| 29 | done | [Python Client SDK](#p1-29) |
| 30 | done | [Grafana dashboard mount path test consistency](#p1-30) |

#### Retained board notes

- Tasks 01, 11, and 16 are spec-first because the planning docs explicitly leave important behavior undefined.
- Tasks 06 and 15 were unblocked by the 2026-07-06 decisions recorded in `docs/planning/32-open-decisions.md`.
- Task 21 was unblocked on 2026-07-06 by explicit approval to add `golang.org/x/net/idna`.
- Tenant-authored fingerprint profiles and redirect following are deliberately not tasked here pending owner decisions.
- Tasks 22 and 23 adopt two previously unowned deferrals surfaced by the 2026-07-05 P0 audit. Task 22 owns the
  egress credential config flat-vs-nested doc/code mismatch flagged in
  `the retired implementation handoff for 35-worker-registration-replay-and-identity-key.md`. Task 23 owned the multi-Control durable
  cancellation gap flagged in `the retired implementation handoff for 27-admin-request-cancellation.md`; the multi-Control decision
  was resolved on 2026-07-07 and task 23 is done.
- Task 24 was added by the 2026-07-06 handoff sweep to own the e2c credit-governed streaming gap flagged in
  `the retired implementation handoff for 24-control-request-dispatch-pipeline.md` ("Real streaming credit replenishment is a P1
  concern", previously unowned). Tasks 03 and 05 depend on it.
- Task 25 was added by the 2026-07-06 handoff sweep to own the intermediate-hop `denied_cname_suffixes` gap
  flagged (unowned) in `the retired implementation handoff for 26-egress-destination-policy-precedence.md` ("CNAME Chain Depth");
  it completed with a stdlib-only raw-chain resolver, so no external DNS dependency gate remains.
- Task 26 was added by P1 task 16 to own the optional upstream connection-pooling implementation after the spec and
  test rows were written. Task 16 remains spec-only.
- Task 27 was added by P1 task 20's 2026-07-06 scope split to own Egress structured-log transport over NATS to
  Control-backed ClickHouse `log_events`. Task 20 owns Control's local `slog` tee only; ClickHouse is the only
  canonical log sink.
- Task 28 was added by P1 task 06 to own SDK/CLI client support for the now-implemented
  `/api/v1/requests:stream` endpoint. P1 task 09 and task 10 skipped that client surface while task 06 was incomplete.
- Task 29 was added by explicit 2026-07-08 request to own a Python client SDK. P1 task 09 deliberately excluded
  non-Go SDKs, so this is the P1 owner for Python request and REST-stream client support.
- Task 30 was added by the 2026-07-08 handoff sweep to own the red `make check` flagged in
  `the retired implementation handoff for p1-29-python-client-sdk.md`: commit `aba1602a` moved the Grafana dashboard provider path
  consistently in `straw.yml`/`docker-compose.yml` but left `deploy/observability/dashboard_test.go` on the old
  path. p1-29 had assigned it to the `done` task p1/13; a completed task cannot own a later regression, so this is
  the real owner.

### P2 Task Board

| # | Status | Task |
|---|--------|------|
| 01 | done | [MITM leaf certificate design](#p2-01) |
| 02 | done | [MITM ingress](#p2-02) |
| 03 | done | [MITM CA management](#p2-03) |
| 19 | done | [MITM leaf bundle KMS provider](#p2-19) |
| 04 | done | [MITM authenticated CONNECT bootstrap](#p2-04) |
| 20 | done | [MITM leaf certificate cache](#p2-20) |
| 05 | done | [BodyRef transport selection](#p2-05) |
| 06 | done | [Object storage foundation](#p2-06) |
| 07 | done | [BodyRef request body flow](#p2-07) |
| 08 | done | [BodyRef response body flow](#p2-08) |
| 09 | done | [Payload capture policy](#p2-09) |
| 10 | done | [Payload capture engine](#p2-10) |
| 11 | done | [Payload capture storage](#p2-11) |
| 12 | done | [Egress SDK protocol foundation](#p2-12) |
| 13 | done | [Example custom Egress implementation](#p2-13) |
| 14 | done | [HTTP/2 semantics spec](#p2-14) |
| 15 | done | [Outbound HTTP/2](#p2-15) |
| 16 | done | [MITM HTTP/2 ALPN and Basic H2 Ingress](#p2-16) |
| 17 | done | [Quota reconciliation](#p2-17) |
| 18 | done | [MITM CA configure and rotate API](#p2-18) |
| 21 | done | [Object storage lifecycle retention](#p2-21) |
| 22 | done | [Egress SDK official worker session runtime rebase](#p2-22) |
| 23 | done | [Executor-delegated resolution enum rename](#p2-23) |
| 24 | done | [Egress SDK conformance and live verification](#p2-24) |
| 25 | done | [Ingress HTTP/2 stream identity and cancellation](#p2-25) |
| 29 | done | [Ingress HTTP/2 headers and trailers](#p2-29) |
| 30 | done | [Ingress HTTP/2 upload flow control and live proof](#p2-30) |
| 26 | done | [Egress SDK decoded stream runtime rebase](#p2-26) |
| 27 | done | [Egress SDK raw tunnel runtime rebase](#p2-27) |
| 31 | done | [Egress SDK BodyRef runtime rebase](#p2-31) |
| 28 | done | [Egress SDK runtime test migration and wiring proof](#p2-28) |
| 32 | superseded | [Python Egress SDK](#p2-32) |
| 32a | done | [Python Egress SDK protocol foundation](#p2-32a) |
| 32b | done | [Python Egress SDK assignment runtime](#p2-32b) |

#### Retained board notes

- Tasks 01, 05, 12, and 17 were formerly gated by named entries in `docs/planning/32-open-decisions.md`; all four
  decisions were resolved on 2026-07-07 and those decision gates are no longer blockers.
- The Provider Adapter concept was dropped 2026-07-07 (superseded `P2 Provider Adapter Baseline` entry in
  `docs/planning/32-open-decisions.md`); the oversized Egress SDK task was split on 2026-07-07 after user approval.
  Task 12 owns the public SDK protocol foundation; task 22 owns the session-level official-worker rebase; tasks 26,
  27, 31, and 28 own the remaining decoded stream, raw tunnel, BodyRef, and test-migration slices split from the
  original oversized task 22 (the raw-tunnel/BodyRef follow-on was split again on 2026-07-07 into task 27 raw tunnel
  and task 31 BodyRef because they are independent surfaces with independent tests); task 23 owns the Provider Adapter
  to executor-delegated enum/doc rename; task 24 owns SDK conformance plus live compose verification; and task 13 owns
  the example custom implementation after task 24.
- Tasks 15 and 16 consume task 14's completed HTTP/2 semantics spec. Task 16 completed the policy-gated MITM ALPN and
  basic h2 MITM request slice. The remaining ingress HTTP/2 stream semantics, split out after independent verification
  rejected the original combined task as too large, were themselves split again on 2026-07-07 into three slices: task
  25 (stream identity and cancellation), task 29 (headers and trailers), and task 30 (upload flow control and live
  compose proof).
- `docs/planning/30-testing-matrix.md` requires MITM, BodyRef, payload-capture, Egress SDK, and HTTP/2 test rows
  before those features ship; each task must add the rows it needs.
- Task 18 was added by task 03's completion audit to own the mutable tenant_admin MITM CA configure/rotate API named
  in `docs/planning/17-mitm-design-p2.md` and `docs/planning/07-public-api-surface.md`. Task 03 implemented static
  operator CA config plus public CA download only.
- The original task 04 was split on 2026-07-07 because it mixed two independent implementation surfaces. Task 04 now
  owns authenticated CONNECT bootstrap and tenant-aware leaf lookup/generation. Task 20 owns encrypted cache storage,
  Redis locks, local singleflight, TTLs, and flood controls.
- Task 19 is intentionally listed before tasks 04 and 20 because task 20's cache/storage work depends on the
  KMS-compatible leaf-bundle provider/config.
- Task 21 was added by task 07's completion audit to own the Section 18 step-9 lifecycle backstop (bucket-level
  retention rule that expires objects orphaned by a Control crash). Task 07 implemented explicit per-object
  DELETE/abort cleanup; the bucket lifecycle rule was deferred by task 06 and is shared by tasks 07/08/11.
- Task 32 was added by explicit 2026-07-08 request to own a Python Egress SDK. P1 task 29 owns the Python client SDK
  only and explicitly excludes Egress SDK/custom-worker behavior. Task 32 was split on 2026-07-08 (user-approved)
  into task 32a (protocol foundation: generated Python protobuf bindings, subject construction, Envelope/registration
  signing, minimal Core NATS wire client) and task 32b (assignment runtime, streaming, conformance tests, usage
  docs), because the combined scope was sized close to the entire Go Egress SDK (`sdk/egress/`, ~3500 lines) and
  needed an explicit decision on adding `protobuf` as a new Python dependency before any code could be written. Task
  32 is marked `superseded`; 32a and 32b are the owning tasks.

## Task decisions and implementation notes

<a id="p0-01"></a>

### 01 - Repository Scaffold, Config Loader, Schema Validation, Generated Protobuf

**Status:** done

#### Objective

Create the first buildable Go scaffold for P0, including static config loading, config validation, and generated protobuf plumbing.

#### Out of Scope

- Do not implement Control REST transport.
- Do not implement NATS assignment.
- Do not implement Postgres, Redis, ClickHouse, or worker state.
- Do not add P1/P2 proxy, CONNECT, MITM, BodyRef, or payload capture behavior.

#### Handoff Notes

- State the config file format and why it was chosen.
- List any config keys intentionally deferred.

<a id="p0-02"></a>

### 02 - Canonical Protobuf and Buf

**Status:** done

#### Objective

Define the canonical `straw.v1` protobuf contract and Buf checks for P0 transport.

#### Out of Scope

- Do not implement request execution.
- Do not add P2 BodyRef behavior beyond contract definitions required by the plan.
- Do not add provider adapter-specific messages.

#### Handoff Notes

- State where generated Go files live.
- State how to regenerate protobuf files.

<a id="p0-03"></a>

### 03 - NATS Connection and Subjects

**Status:** done

#### Objective

Add the P0 NATS connection layer, startup max-payload validation, and exact-session subject helpers.

#### Out of Scope

- Do not implement worker registration state machine.
- Do not dispatch real outbound requests.
- Do not use queue groups for executor dispatch.
- Do not introduce durable queues or replay behavior.

#### Handoff Notes

- List every subject format implemented.
- Note any subject intentionally deferred to a later task.

<a id="p0-04"></a>

### 04 - Postgres Schema

**Status:** done

#### Objective

Create the P0 Postgres schema for tenants, keys, workers, pools, routes, deny rules, injection policies, rate limits, quotas, config versions, and audit source records.

#### Out of Scope

- Do not implement config APIs.
- Do not implement Redis cache invalidation.
- Do not store API key secrets in plaintext.
- Do not add billing-grade quota reconciliation.

#### Handoff Notes

- Include the exact migration command used.
- Note any schema choices that must be mirrored by application code.

<a id="p0-05"></a>

### 05 - Config Cache and Invalidation

**Status:** done

#### Objective

Implement Control's config snapshot cache with Postgres versioning and Redis invalidation.

#### Out of Scope

- Do not implement the full config management REST API.
- Do not let Egress query Postgres or Redis.
- Do not cache data in Redis without TTL.

#### Handoff Notes

- Document cache lifetime and invalidation behavior.
- Note Redis outage behavior.

<a id="p0-06"></a>

### 06 - Control REST Request Endpoint

**Status:** done

#### Objective

Implement Control's minimal synchronous REST `/api/v1/requests` endpoint for P0.

#### Out of Scope

- Do not implement REST response streaming.
- Do not implement HTTP forward proxy, CONNECT, MITM, redirects, or HTTP/2.
- Do not implement SDK, CLI, or UI.

#### Handoff Notes

- Include sample valid and invalid request payloads.
- Note any execution path still stubbed by later tasks.

<a id="p0-07"></a>

### 07 - Auth, RBAC, and API Keys

**Status:** done

#### Objective

Implement API-key authentication, tenant resolution, RBAC, API key lifecycle, and revocation cache invalidation.

#### Out of Scope

- Do not implement OAuth or user sessions.
- Do not let tenant keys create tenants.
- Do not log API key secrets.
- Do not implement billing or marketplace workflows.
- Do not allow multi-tenant worker credentials: P0 creation forces `tenant_scope` to the caller's tenant and rejects
  `allowed_pools` entries referencing any other tenant (multi-tenant credentials are a P1 platform-scoped operation).

#### Handoff Notes

- Document bootstrap behavior.
- List roles and allowed P0 actions.

<a id="p0-08"></a>

### 08 - Worker Registration, Heartbeat, and State

**Status:** done

#### Objective

Implement worker registration, heartbeat processing, worker state, duplicate-session handling, admin disable/drain, and cooldown.

#### Out of Scope

- Do not implement route selection.
- Do not implement outbound HTTP execution.
- Do not store worker runtime state durably in Postgres except configured worker records.

#### Handoff Notes

- Document state names and timeout constants.
- Note how test time is controlled.

<a id="p0-09"></a>

### 09 - Routing Evaluation

**Status:** done

#### Objective

Implement tenant-isolated routing snapshot evaluation and worker eligibility for P0.

#### Out of Scope

- Do not implement provider adapters.
- Do not implement automatic retries or replay workflows.
- Do not implement P1/P2 degraded policy beyond the P0 rules.

#### Handoff Notes

- Document tie-breaking behavior.
- Note any routing inputs intentionally ignored for P0.

<a id="p0-10"></a>

### 10 - Assignment and Stream Lifecycle

**Status:** done

#### Objective

Implement Control-to-Egress assignment and stream frame lifecycle with sequence, offset, credit, timeout, terminal, cancellation, and fallback rules.

#### Out of Scope

- Do not implement Egress outbound HTTP internals beyond a fake executor needed for tests.
- Do not add silent request replay.
- Do not add durable queue behavior.

#### Handoff Notes

- Document lifecycle states and timeout constants.
- Note how fake NATS/executor behavior is tested.

<a id="p0-11"></a>

### 11 - Egress Outbound Execution

**Status:** done

#### Objective

Implement the official Go Egress outbound HTTP execution path with P0 transport defaults, deadlines, and resolved-IP DestinationPolicy enforcement.

#### Out of Scope

- Do not let Egress query Postgres, Redis, or ClickHouse.
- Do not enable outbound HTTP/2 or upstream keep-alives except behind an explicit tested P0 feature flag.
- Do not implement redirects, CONNECT, MITM, payload capture, or provider adapters.

#### Handoff Notes

- Document transport defaults.
- State exactly which error facts and canonical codes Egress emits (must stay within the Section 13 executor-emittable set).

<a id="p0-12"></a>

### 12 - Error Registry and Mapping

**Status:** done

#### Objective

Implement the canonical ErrorResponse registry and HTTP/retry/category mapping.

#### Out of Scope

- Do not change upstream 4xx/5xx passthrough envelope semantics.
- Do not add undocumented error codes.
- Do not expose secret values in error details.

#### Handoff Notes

- List any error code names added or renamed.
- Note where new code should add future errors.

<a id="p0-13"></a>

### 13 - Rate Limits, Quotas, and Redis

**Status:** done

#### Objective

Implement Redis-backed P0 rate limits, quota hot counters, worker state storage, sticky sessions, and explicit Redis failure policies.

#### Out of Scope

- Do not claim billing-grade quota accuracy.
- Do not store durable config in Redis.
- Do not create Redis keys without TTL.

#### Handoff Notes

- Document Redis key prefixes and TTLs.
- State fail-open or fail-closed behavior per feature.

<a id="p0-14"></a>

### 14 - ClickHouse Metadata Write Path

**Status:** done

#### Objective

Implement asynchronous ClickHouse request metadata writes with redaction, sanitization, bounded queueing, and outage behavior.

#### Out of Scope

- Do not fail request transport because ClickHouse writes fail.
- Do not implement telemetry read APIs or dashboards.
- Do not implement payload capture.

#### Handoff Notes

- Document queue size and drop policy.
- List fields intentionally omitted from metadata.

<a id="p0-16"></a>

### 16 - NATS Client Foundation

**Status:** done

#### Objective

Add the real NATS client and connection lifecycle, and wire a live NATS connection into both `cmd/control` and
`cmd/egress` startup. This task authorizes adding the `github.com/nats-io/nats.go` dependency.

#### Out of Scope

- Do not implement any subscription or business message flow (registration, heartbeat, assignment, or stream
  frames) — that is tasks 17, 23, and 24.
- Do not add JetStream, durable streams, or queue-group dispatch for executor subjects.
- Do not implement worker or Control business logic beyond holding a connected client.

#### Handoff Notes

- Document the reconnect/backoff parameters used.
- Note that subscription/publish wiring for registration, heartbeat, assignment, and stream frames is deferred to
  tasks 17, 23, and 24.

<a id="p0-17"></a>

### 17 - Worker Registration and Heartbeat over NATS

**Status:** done

#### Objective

Turn worker discovery into a live wire protocol. Egress gets a real run loop: publish registration using the
existing builders in `internal/egress/registration.go`, then periodic heartbeats. Control subscribes on the
registration/heartbeat subjects (helpers in `internal/natsx/natsx.go`) using the `control` queue group and feeds
`WorkerRegistry.Register`/`Heartbeat`. Duplicate-session replacement and heartbeat-timeout state transitions must
work end to end over the wire, not just in-process against fakes.

#### Out of Scope

- Do not implement assignment consumption or stream execution (task 23).
- Do not move worker runtime state to Redis (documented deferral in task 13's handoff; single-Control P0
  limitation).
- Do not implement admin disable/drain HTTP endpoints (already implemented by task 08; reuse as-is).

#### Handoff Notes

- Document the run-loop signal handling and heartbeat cadence used.
- Note that assignment consumption (task 23) and Redis-backed worker runtime state remain deferred, and to which
  task file.

<a id="p0-18"></a>

### 18 - Postgres Foundation and Identity Stores

**Status:** done

#### Objective

Add the real Postgres connection foundation and Postgres-backed identity stores, then wire Control to use them instead
of in-memory identity state. This task authorizes adding the `github.com/jackc/pgx/v5` dependency.

#### Out of Scope

- Do not implement config-resource stores or tenant snapshot assembly (task 19).
- Do not implement config/admin HTTP APIs beyond identity endpoints already owned by task 07 and backed here.
- Do not implement ClickHouse metadata writes (task 14).
- Do not remove in-memory stores used by unit tests.

#### Handoff Notes

- Document the migration application path and any bootstrap seed behavior.
- Note that config-resource Postgres stores and snapshot assembly are deferred to `docs/implementation-history.md#p0-19-postgres-config-stores-and-snapshot.md`.

<a id="p0-19"></a>

### 19 - Postgres Config Stores and Snapshot Assembly

**Status:** done

#### Objective

Implement Postgres-backed durable config stores and tenant snapshot assembly so Control can load immutable request-time
snapshots from Postgres instead of in-memory config fakes.

#### Out of Scope

- Do not implement config-management HTTP handlers (task 20).
- Do not implement Redis pub/sub invalidation or periodic version polling (task 21).
- Do not wire request dispatch to consume the full snapshot (task 24).
- Do not add P1 rollback.

#### Handoff Notes

- Document the snapshot fields added and the config-version transaction boundary.
- Note that HTTP APIs are deferred to `docs/implementation-history.md#p0-20-config-admin-apis.md` and Redis invalidation is deferred to
  `docs/implementation-history.md#p0-21-redis-wiring-and-config-invalidation.md`.

<a id="p0-20"></a>

### 20 - Config Admin APIs for Routing, Deny, Injection, Fingerprint

**Status:** done

#### Objective

Implement the missing P0 config-management HTTP surface for routing rules, deny rules, injection policies, and
read-only fingerprint profiles, backed by the Postgres stores from task 19.

#### Out of Scope

- Do not implement P1 rollback.
- Do not implement tenant-authored fingerprint profiles.
- Do not implement P2 payload-capture policy APIs.
- Do not implement request dispatch (task 24).

#### Handoff Notes

- Document every endpoint added and the roles allowed to call it.
- Note that Redis-backed invalidation implementation is deferred to `docs/implementation-history.md#p0-21-redis-wiring-and-config-invalidation.md`.

<a id="p0-21"></a>

### 21 - Redis Wiring and Config Invalidation

**Status:** done

#### Objective

Wire the existing Redis-backed runtime components into Control, implement Redis-backed config invalidation, and add the
durable fallback checks required when pub/sub messages are missed.

#### Out of Scope

- Do not move `WorkerRegistry` runtime state to Redis in P0; the single-Control limitation is documented by the task 13
  handoff.
- Do not call admission checks from the request path (task 24).
- Do not use Redis as a durable config source.

#### Handoff Notes

- Document Redis key prefixes, TTLs, pub/sub channel shape, polling cadence, and fail policies.
- Note that request-path admission and sticky consumption are deferred to `docs/implementation-history.md#p0-24-control-request-dispatch-pipeline.md`.

<a id="p0-22"></a>

### 22 - Control Destination Policy Resolution

**Status:** done

#### Objective

Add the Control-side per-request policy resolver that validates target destinations, deny rules, header injection, and
fingerprint profile support before task 24 dispatches a request to Egress.

#### Out of Scope

- Do not wire the resolver into the REST request handler (task 24).
- Do not implement MITM or proxy ingress.
- Do not implement redirect following.
- Do not move Egress resolved-IP validation into Control; Egress still performs final DNS/IP policy validation.

#### Handoff Notes

- Document the resolver API and which snapshot fields it consumes.
- Note that request-handler wiring is deferred to `docs/implementation-history.md#p0-24-control-request-dispatch-pipeline.md`.

<a id="p0-23"></a>

### 23 - Egress Assignment Execution Loop

**Status:** done

#### Objective

Turn `cmd/egress` into a live worker that consumes exact-session assignments over NATS, executes outbound requests with
the existing executor, and streams terminal protocol frames back to Control.

#### Out of Scope

- Do not implement BodyRef transport.
- Do not implement provider adapters.
- Do not implement HTTP/2, MITM, or proxy ingress.
- Do not implement Control request dispatch (task 24).

#### Handoff Notes

- Document assignment subscription ordering, timeout behavior, and shutdown behavior.
- Note that Control-side dispatch and response buffering are deferred to `docs/implementation-history.md#p0-24-control-request-dispatch-pipeline.md`.

<a id="p0-24"></a>

### 24 - Control Request Dispatch Pipeline

**Status:** done

#### Objective

Replace the synthetic success stub in the REST request handler with the real P0 Control dispatch pipeline from
admission through routing, assignment, streaming, response buffering, and canonical error handling.

#### Out of Scope

- Do not implement REST streaming (`/api/v1/requests:stream`).
- Do not implement HTTP proxy, CONNECT, MITM, BodyRef, payload capture, provider adapters, or HTTP/2.
- Do not add SDK/client retry behavior.

#### Handoff Notes

- Document the pipeline order, fallback boundaries, and any remaining P1/P2 exclusions.
- List the focused end-to-end command used for verification.

<a id="p0-25"></a>

### 25 - P0 Test Matrix and Compose

**Status:** done

#### Objective

Close the P0 test matrix and provide a local docker-compose environment for the full vertical slice.

#### Out of Scope

- Do not add Kubernetes, Swarm, or production deployment templates.
- Do not add P1/P2 proxy, CONNECT, MITM, BodyRef, provider adapter, telemetry UI, or payload capture tests.

#### Handoff Notes

- Include compose commands and expected ports.
- Include any test rows still blocked and why.

<a id="p0-26"></a>

### 26 - Egress Destination Policy Precedence and Suffix Enforcement

**Status:** done

#### Objective

Fix `internal/egress/executor.go`'s destination-policy validation so it matches the semantics Control's
destination-policy resolver (task 22) actually promises, and wire the two `DestinationPolicy` bundle fields Egress
currently never reads.

This task exists because of two gaps identified while implementing task 22
(`the retired implementation handoff for 22-control-destination-policy-resolution.md`):

1. **AllowedCidrs is not a true override.** `internal/egress/executor.go`'s `validateCIDRPolicy`/`deniedByDefault`
   (originally task 11) treats `DestinationPolicy.allowed_cidrs` as a restrict-to-allowlist gate: if non-empty, an
   address must be inside it, but the address is *still* separately checked against the private/loopback/link-local/
   metadata booleans and the static default-deny prefix list regardless. Control's resolver
   (`internal/control/destination_policy.go`, `evaluateLiteralIPDeny`) treats an explicit tenant allow-type deny rule
   as a genuine override, per `docs/planning/27-security-controls.md` ("Private/link-local/metadata IP blocks are
   denied by default unless a tenant admin explicitly allows them for a tenant or deployment"). Net effect today: a
   tenant that allow-lists a private/loopback/metadata IP passes Control's pre-dispatch check but is still rejected by
   Egress after DNS resolution — the "allow override" feature does not actually work end-to-end.
2. **denied_host_suffixes and denied_cname_suffixes are never read.** Control's resolver populates both fields on the
   `DestinationPolicy` bundle it sends (host-type and cname-type tenant deny rules), but
   `internal/egress/executor.go` has no code path that consults either field. cname-type deny rules can only be
   meaningfully enforced by Egress, since Control performs no DNS resolution — so as it stands, cname-type deny rules
   have no enforcement anywhere in the system.

#### Out of Scope

- Do not change Control's resolver semantics (`internal/control/destination_policy.go`) — it already implements the
  target behavior; this task brings Egress into agreement with it.
- Do not implement redirect following or MITM.
- Do not add a per-tenant `allow_private_ranges`/`allow_loopback`/`allow_link_local`/`allow_multicast`/
  `allow_metadata_ips` config surface — those booleans remain deployment-level and P0 has no config wiring for them;
  this task only fixes how `allowed_cidrs` and the suffix fields interact with the existing checks.

#### Handoff Notes

- Document the exact precedence order implemented (allowed_cidrs override -> denied_cidrs -> metadata -> private/
  loopback/link-local/multicast -> default-deny prefixes -> host/cname suffixes).
- Note the CNAME chain depth actually inspected (single-hop via `LookupCNAME` unless a deeper approach was
  implemented) so a future task can extend it if needed.

<a id="p0-27"></a>

### 27 - Admin Request Cancellation Pipeline

**Status:** done

#### Objective

Make `POST /api/v1/admin/requests/{request_id}/cancel` a working P0 endpoint that cancels an in-flight request end to
end, backed by an in-flight request registry and a real cancel dispatch to the executor.

#### Context (gap being closed)

The 2026-07-04 review found that only the authorization predicate `AuthorizeAdminCancel`
(`internal/control/lifecycle.go`) exists and is unit-tested. There is no HTTP route, no registry mapping
`request_id` to a running request, and no path that signals cancellation to a running dispatch. `docs/planning/26`
lists `POST /requests/{request_id}/cancel` as a P0 runtime admin endpoint and task 10 lists "admin cancel" as a
required cancellation source, so the feature is specified but unreachable. Client-disconnect and deadline
cancellation are already implemented in `dispatcher.go` `readResponse`; this task adds the admin-initiated path only.

#### Out of Scope

- Do not implement client-facing cancel APIs beyond the admin endpoint.
- Do not add streaming REST, proxy, CONNECT, or P1/P2 cancellation semantics.
- Do not add durable/persisted cancellation state; the registry is in-process (single-Control P0, consistent with the
  task 13 worker-runtime-state deferral).

#### Handoff Notes

- Document the registry lifetime, the cancel-to-`CancelFrame` path, and how test time/NATS is controlled.
- Note that the registry is intentionally in-process for single-Control P0.

<a id="p0-28"></a>

### 28 - Control Observability Metrics

**Status:** done

#### Objective

Expose the P0 Prometheus metrics surface on Control's metrics port (`/metrics`) with the metric set defined in
`docs/planning/23-observability.md`.

#### Context (gap being closed)

The 2026-07-04 review found the metrics port serves only `/healthz` and `/readyz`; `/metrics` returns 404 and none
of the `docs/planning/23` P0 metrics are implemented. No task owned this surface. Note the P0/P1 split: **Control
`/metrics` is P0** per `docs/planning/23`; the *optional direct worker Prometheus scrape* in
`docs/planning/31` P1 item 5 is a separate, deferred concern and is out of scope here.

#### Out of Scope

- Do not implement direct worker Prometheus scraping (owned by `docs/implementation-history.md#p1-15-egress-metrics-exposure.md`).
- Do not implement telemetry read APIs, dashboards, or exemplars beyond what the stdlib client offers.
- Do not add high-cardinality labels (no raw URLs; see `docs/planning/23`).

#### Handoff Notes

- Document each metric, its labels, and where it is incremented/observed.
- Note the P0-vs-P1 boundary (Control `/metrics` P0; worker scrape P1).

<a id="p0-29"></a>

### 29 - Tenant Lifecycle API and Status Enforcement

**Status:** done

#### Objective

Implement the remaining P0 tenant endpoints (list, get, update, soft-delete), persist and load tenant `name` and
`rate_limit_ceiling`, and enforce tenant status during authentication so a suspended or soft-deleted tenant's keys
fail with `tenant_not_found`.

#### Context (gap being closed)

The 2026-07-04 review found only `POST /tenants` is implemented; `GET /tenants`, `GET /tenants/{id}`,
`PUT /tenants/{id}`, and `DELETE /tenants/{id}` from the `docs/planning/26` P0 table are missing. Two consequences:
(1) the `Authenticator` never checks tenant status, so `tenant_not_found` (a `docs/planning/14` P0 code for a
missing/deleted tenant) is unreachable and a disabled tenant's keys keep executing; (2) the `tenants` table carries
only `id/status/timestamps`, so `Tenant.RateLimitCeiling` is never persisted or loaded — the rate-limit ceiling
enforcement wired in tasks 13/26 is inert in the real binary. Task 18 explicitly deferred these columns to a later
task; this is that task.

Vocabulary reconciliation: migration `0001_init.sql` gave `tenants` a status CHECK of
`('active', 'disabled', 'deleted')` and a `soft_deleted_at` column, but `docs/planning/26` defines the tenant status
enum as `active | suspended | deleted` and the shared soft-delete contract sets `deleted_at` (which migration 0002
already uses for config resources). The planning doc wins: this task's migration renames the column and replaces the
CHECK, migrating any `'disabled'` rows to `'suspended'`.

#### Out of Scope

- `default_timeout_ms`, `max_timeout_ms`, `metadata_query_storage`, and `metadata_path_storage` are P0 per
  `docs/planning/26` "P0 Config Resource Schemas"; they are owned by
  `docs/implementation-history.md#p0-46-tenant-p0-schema-fields.md`.
- Do not implement config rollback (P1).
- Do not move worker runtime state or add multi-Control coordination.

#### Handoff Notes

- Document the new columns and which application code reads them.
- Note the exact status values that gate authentication.

<a id="p0-30"></a>

### 30 - Executor Pool Config API

**Status:** done

#### Objective

Expose the P0 `/executor-pools` config-management CRUD surface over the existing Postgres pool store, and source pool
policy into routing instead of the current `nil` placeholder.

#### Context (gap being closed)

The 2026-07-04 review found `docs/planning/26`'s P0 `/executor-pools` endpoints (POST/GET/PUT/DELETE) are not wired,
even though the Postgres store methods (`PostgresConfigStore.UpsertExecutorPool`, `DeleteExecutorPool`) and the
snapshot read (`readExecutorPools`) already exist from task 19. Separately, `dispatcher.go` constructs
`NewStaticPoolPolicyProvider(nil)`, so degraded-pool policy has no configuration source. This task adds the HTTP
surface and a list/get store method, and feeds pool policy from the snapshot.

#### Out of Scope

- Do not add P1 pool-policy fields not in the `docs/planning/26` P0 pool schema.
- Do not implement rollback (P1) or tenant-authored fingerprint profiles.

#### Handoff Notes

- Document each endpoint and its roles.
- Note how the pool-policy provider is now populated and any degraded-policy default.

<a id="p0-31"></a>

### 31 - Config Change History API

**Status:** done

#### Objective

Implement the P0 `GET /changes` config audit-history endpoint with pagination over `config_audit_source`.

#### Context (gap being closed)

The 2026-07-04 review found `docs/planning/26`'s P0 `GET /changes` endpoint is not wired. The backing table
(`config_audit_source`) and an unpaginated `postgresAuditStore.ListTenant` already exist from task 18; this task adds
pagination and the read-only HTTP surface. Audit rows are already redacted at write time (task 19), so this is a
read-only exposure task.

#### Out of Scope

- Do not implement the ClickHouse `config_audit_events` mirror (that is task 32).
- Do not implement config rollback (P1).
- Do not add write or mutation behavior to this endpoint.

#### Handoff Notes

- Document the pagination behavior and the fields returned.
- Note that the ClickHouse audit mirror is owned by task 32.

<a id="p0-32"></a>

### 32 - Request-Outcome Telemetry and Worker/Audit ClickHouse Writes

**Status:** done

#### Objective

Record the real request outcome (status, error, timings, sizes) into ClickHouse `request_events` after dispatch, and
add the `worker_events` and `config_audit_events` write paths defined in the canonical schema.

#### Context (gap being closed)

The 2026-07-04 review found `request_events` rows are enqueued once, pre-dispatch (`handler.go`), with hardcoded
outcome fields (`upstream_status = 200`, `error_code = ""`, all timings and `response_size_bytes = 0`), and are never
updated with the real result — so every persisted telemetry row is a placeholder. The `worker_events`,
`config_audit_events`, and `log_events` tables in `deploy/docker/clickhouse-schema.sql` have no writer. Task 14's
explicit criteria (redaction, sanitization, bounded queue, outage tolerance) are met and must be preserved; this task
adds outcome accuracy and the two missing write paths.

#### Out of Scope

- Do not implement telemetry read APIs or dashboards (P1).
- Do not implement payload capture (P2).
- Do not build the `log_events` ingestion pipeline; Control-local ingestion is owned by
  `docs/implementation-history.md#p1-20-log-events-ingestion.md`, and Egress-to-Control log transport is owned by
  `docs/implementation-history.md#p1-27-egress-log-events-nats-transport.md`.

#### Handoff Notes

- Document the outcome-capture point in the pipeline and the fields written per table.
- List fields intentionally omitted and the `log_events` deferral's owning tasks
  (`docs/implementation-history.md#p1-20-log-events-ingestion.md` and `docs/implementation-history.md#p1-27-egress-log-events-nats-transport.md`).

<a id="p0-33"></a>

### 33 - Ingress Header Value CR/LF Validation

**Status:** done

#### Objective

Reject client-supplied header values whose base64-decoded bytes contain CR/LF (or other control characters) at REST
ingress with `invalid_request`, instead of letting them pass validation and surface downstream as
`executor_internal_error`.

#### Context (gap being closed)

The 2026-07-04 review found `validateHeaders` in `internal/control/request.go` checks the raw base64 string for CR/LF
(which base64 can never contain) and never scans the decoded bytes. A header whose decoded value contains CR/LF
therefore passes ingress and is only caught at the egress boundary (`safeOutboundHeader`), returning
`executor_internal_error` (HTTP 502) instead of a clean `invalid_request` (HTTP 400) at ingress. This is not an
injection hole — egress and Go's `net/http` both block the bytes before the wire — but the rejection is at the wrong
layer with a misleading code, and the existing test `TestValidateRequestCRInHeaderValue` documents that the decoded
value is not checked.

#### Out of Scope

- Do not change injection-policy header validation (owned by tasks 20/22).
- Do not change the egress-side `safeOutboundHeader` defense (defense in depth stays).

#### Handoff Notes

- Note the exact control characters rejected and where.

<a id="p0-34"></a>

### 34 - Lint/Vet Hardening, Test-Harness Bootstrap, and Repo Hygiene

**Status:** done

#### Objective

Close the engineering-hygiene gaps found by the 2026-07-04 review: enable `go vet` in `make check`, fix the
`copylocks` violation it surfaces, make the Postgres-backed tests self-bootstrap their schema, and stop leaving build
artifacts in the tree.

#### Out of Scope

- Do not add new product features or endpoints.
- Do not broaden the linter set beyond `govet` and the fixes required to pass it.

#### Handoff Notes

- Note the `copylocks` fix and any signature change to `Capabilities`.
- Note the test-harness bootstrap change and the exact fresh-DB verification command.

<a id="p0-35"></a>

### 35 - Worker Registration Replay Protection and Persistent Identity Key

**Status:** done

#### Objective

Complete the `docs/planning/27` "Worker Credential Signing" control: bind a nonce and issued-at timestamp into the
signed registration token with Redis-backed replay protection (fail-closed by default), and give the egress worker a
configured persistent Ed25519 private key so a live worker can register against a pre-seeded credential.

#### Context (gap being closed)

The 2026-07-04 review follow-up found the Ed25519 signature path itself is implemented and enforced:
`RegistrationSigningPayload`/`SignRegistration`/`VerifyRegistrationSignature`
(`api/proto/straw/v1/registration_sign.go`) sign a domain-separated payload binding
`worker_id`/`credential_id`/`executor_type`/protocol version, and `WorkerRegistry.Register` verifies it against the
credential's stored public key alongside status/scope/capability checks. Two `docs/planning/27` requirements are
missing:

1. The signed payload carries no nonce or issued-at, so a captured `RegisterRequest` authorizes replays forever. The
   comment in `registration_sign.go` documents this gap. `docs/planning/27` requires nonces stored in Redis with TTL
   scoped by `credential_id`, registration failing closed when Redis is unavailable (fail-open only by explicit
   deployment opt-in), and configurable clock-skew tolerance defaulting to 60 seconds. The nonce travels inside the
   signed token, so Core NATS request/reply needs no extra channel.
2. `cmd/egress/main.go` generates a throwaway keypair each boot and no config field can load a persistent key, so a
   live worker's signature can never match a seeded credential's `public_key_ed25519_base64`.
   `deploy/docker/README.md` documents that compose registration "will not succeed out of the box" for exactly this
   reason.

`docs/planning/11` lists the signed token and the validation set Control already performs; `docs/planning/27` adds the
nonce/replay requirements on top (union, not conflict). Existing scope/capability/protocol validation must not change.

#### Out of Scope

- Do not implement key rotation or a key-delivery API; the key is static config in P0.
- Do not implement multi-tenant worker credentials (P1 task 08).
- Do not change the existing scope/capability/protocol registration validation.

#### Handoff Notes

- Document the final signed-payload field order, the nonce TTL/skew defaults, and the fail-policy flag.
- Document the dev key/credential seeding used by compose and why it is dev-only.

<a id="p0-36"></a>

### 36 - Canonical Config Base Path Normalization

**Status:** done

#### Objective

Serve every durable config endpoint under the canonical `/api/v1/config` base path from `docs/planning/26`, removing
the bare root-path registrations.

#### Context (gap being closed)

The 2026-07-04 review follow-up found `serveAdminRoutes` (`cmd/control/main.go`) registers the identity and
limits endpoints at bare root paths — `POST /tenants`, `POST|GET /platform-api-keys`,
`POST /platform-api-keys/{id}/revoke`, `POST|GET /api-keys`, `POST /api-keys/{id}/revoke`,
`POST|GET /worker-credentials`, `POST /worker-credentials/{id}/revoke`, `GET /quotas`, `PUT /tenants/{id}/quotas`,
`GET|PUT /rate-limits` — while routing-rules, deny-rules, injection-policies, and fingerprint-profiles are correctly
registered under `/api/v1/config`. `docs/planning/26` sets `/api/v1/config` as the canonical durable config base path
and `docs/planning/a-reconciliation-notes.md` records `/api/v1` as the single public API base path. No external
clients exist yet, so the bare paths move outright; no aliases or redirects. Runtime admin endpoints already live under
`/api/v1/admin` and are not touched.

Tasks 29, 30, and 31 add further config routes and register them only under the canonical path; land this task first
or expect a small rebase (shared `cmd/control/main.go`).

#### Out of Scope

- Do not add, remove, or change endpoint behavior, RBAC, or payloads; this is a path move only.
- Do not add aliases, redirects, or deprecation handling for the old bare paths.
- Do not touch the `/api/v1/admin` runtime endpoints or `POST /api/v1/requests`.

#### Handoff Notes

- List the moved routes and confirm no behavior or RBAC change rode along.

<a id="p0-37"></a>

### 37 - Structured JSON Logging

**Status:** done

#### Objective

Emit structured JSON logs from both services per `docs/planning/23`: `service`, `timestamp`, `level` on every line,
plus `request_id`, `tenant_id`, and `error_code` where available; `worker_id` only in internal logs. Use the stdlib
`log/slog` JSON handler.

#### Context (gap being closed)

The 2026-07-04 review follow-up found no structured logging anywhere: `log/slog` is unused and all logging goes
through plain `log.Printf` in `cmd/control/main.go`, `cmd/egress/main.go`, and
`internal/control/invalidation_redis.go`. `docs/planning/23` requires structured JSON logs from all services. The
call-site count is small, so this is a mechanical conversion plus a `service` attribute per binary. Shipping logs to
the ClickHouse `log_events` table is a separate, deferred concern: Control-local ingestion is owned by
`docs/implementation-history.md#p1-20-log-events-ingestion.md`, and Egress-to-Control log transport is owned by
`docs/implementation-history.md#p1-27-egress-log-events-nats-transport.md`.

#### Out of Scope

- Do not build the ClickHouse `log_events` ingestion pipeline (Control side owned by
  `docs/implementation-history.md#p1-20-log-events-ingestion.md`; Egress transport owned by
  `docs/implementation-history.md#p1-27-egress-log-events-nats-transport.md`).
- Do not add new log lines beyond converting existing ones and wiring the handler; per-request debug logging is not a
  P0 requirement.
- Do not add a logging dependency; stdlib `log/slog` only.

#### Handoff Notes

- Document the handler setup, the `service` values, and which call sites carry contextual attributes.
- See `the retired implementation handoff for 37-structured-json-logging.md`.

<a id="p0-38"></a>

### 38 - Egress Worker Health Endpoints

**Status:** done

#### Objective

Expose local `/healthz` and `/readyz` on the egress worker per `docs/planning/23` ("P0 should prefer direct local
`/healthz` and `/readyz`" for egress), with readiness reflecting registration success and draining state.

#### Context (gap being closed)

The 2026-07-04 review follow-up found the egress binary has no HTTP listener at all, so the `docs/planning/23`
P0-preferred worker health surface does not exist and the compose stack cannot healthcheck the worker container.
Control already has the pattern in `cmd/control/health.go`. Direct worker Prometheus `/metrics` scraping stays P1
(`docs/implementation-history.md#p1-15-egress-metrics-exposure.md`) and is not part of this task.

#### Out of Scope

- Do not expose `/metrics` on the worker (P1 task 15).
- Do not add remote health reporting beyond the existing heartbeat.

#### Handoff Notes

- Document the readiness state transitions and the chosen port.

<a id="p0-39"></a>

### 39 - Test Suite Live-Database Guard

**Status:** done

#### Objective

Stop the Postgres-backed test harness from destroying a live deployment's data when
`STRAW_TEST_POSTGRES_DSN` points at a database that is in real use (e.g. the docker-compose `straw` database).

#### Context (gap being closed)

The 2026-07-05 live end-to-end verification hit this twice in one session:

1. Running `go test ./...` with `STRAW_TEST_POSTGRES_DSN` pointed at the compose stack's `straw` database
   truncated `tenants`, `worker_credentials`, and `api_keys` mid-session, wiping the running stack's seeded
   state (the harness truncates identity tables between tests — see `internal/control/postgres_store_test.go`).
2. The reverse direction: test fixtures leaked into the shared database. A test-seeded platform `system_admin`
   key (prefix `boot`, fixed ID `00000000-0000-0000-0000-000000000030`) survived the run, and because
   `BootstrapFromEnv` is a no-op when any active platform system_admin exists, it silently blocked the
   `STRAW_BOOTSTRAP_SYSTEM_ADMIN_KEY` bootstrap on every subsequent Control startup until manually deleted.

Both failure modes are silent. The docs currently even suggest the compose DSN as the test DSN.

#### Out of Scope

- Do not introduce testcontainers or any new dependency; the guard is plain SQL + convention.
- Do not change what the harness truncates or how migrations apply.

#### Handoff Notes

- Record the chosen guard mechanism and the test-database naming convention.

<a id="p0-40"></a>

### 40 - Egress Registration Retry and Recovery

**Status:** done

#### Objective

Make the egress worker survive Control being temporarily unavailable: retry registration with backoff instead of
exiting fatally, and recover when its session dies on the Control side (e.g. a Control restart wiping the in-memory
worker registry).

#### Context (gap being closed)

Observed live on 2026-07-05 in the compose stack: restarting `control` and `egress` together stranded the worker.
`egress.Run` (`internal/egress/runtime.go`) calls `Register` exactly once and returns its error, so `cmd/egress`
exits with `egress run loop: request straw.v1.control.register: nats: no responders available for request` whenever
Control's NATS subscriptions are not up yet. Recovery is delegated entirely to the container restart policy's
backoff, leaving multi-minute windows where `GET /api/v1/admin/workers` is `[]` and every dispatch fails
`route_unavailable`; outside docker there is no recovery at all. Compose's `depends_on: service_started` masks this
on first boot only by racing in Control's favor.

The second half of the same gap: Control's worker registry is in-memory, so a Control restart forgets the session
while the worker keeps heartbeating it. Control ignores stale-session heartbeats (`TestHeartbeatStaleSessionIgnored`),
the worker never learns, and it stays invisible until manually restarted.

#### Out of Scope

- Control-side outage hardening, cooldowns, and in-flight loss semantics (owned by
  `docs/implementation-history.md#p1-17-worker-loss-and-nats-outage-hardening.md`).
- Redis-backed worker session/heartbeat/load and cooldown runtime state is owned by
  `docs/implementation-history.md#p0-45-redis-backed-worker-runtime-state.md`.

#### Handoff Notes

- Record the backoff parameters and the stale-session decision.

<a id="p0-41"></a>

### 41 - Request Phase Timing Accuracy

**Status:** done

#### Objective

Make the per-phase timings in `request_events` (`routing_ms`, `assignment_ms`, `egress_ms`, `total_ms`) reflect the
real elapsed phases of a successful dispatch, so ClickHouse telemetry is usable for the latency analysis
`docs/planning/23` expects.

#### Context (gap being closed)

Observed on the live compose stack on 2026-07-05: two successful REST -> NATS -> egress -> `https://example.com`
round-trips recorded

| request | routing_ms | assignment_ms | egress_ms | total_ms |
|---------|-----------|---------------|-----------|----------|
| req_1783218379439088304 | 0 | 3 | 0 | 257 |
| req_1783218544603559033 | 0 | 1 | 0 | 179 |

`egress_ms = 0` on a ~200ms upstream fetch is wrong: nearly all of `total_ms` is the egress phase.
`routing_ms = 0` may be legitimately sub-millisecond — verify rather than assume.

Code pointers from the initial look (unverified beyond this): the egress worker does emit `OutboundStartFrame`
(`internal/egress/executor.go:742`), Control stamps `egressStarted` on that frame and computes
`result.egressMs` on the `End` frame (`internal/control/dispatcher.go`, `acceptResponseProgress` /
`acceptResponseTerminal`), and `egressMillis` returns 0 when the start is zero. Something in that chain loses the
start time or the subtraction on the live path — the in-process tests did not catch it, so the first step is a
failing test that reproduces the zero.

#### Out of Scope

- No new timing fields or schema changes.
- No latency dashboards or read APIs (P1 tasks 12/13).

#### Handoff Notes

- Name the root cause and why the in-process tests missed it.

<a id="p0-42"></a>

### 42 - Executor Pool Capability Fields

**Status:** done

#### Objective

Add the `docs/planning/26` P0 executor-pool capability fields — `allowed_ip_types`, `allowed_countries`,
`allowed_regions` — to the pool schema, config API, snapshot, and routing eligibility, so pools can constrain which
workers serve them the way the planning doc specifies.

#### Context (gap being closed)

The 2026-07-05 P0 verification audit found these three fields absent from the entire codebase (zero hits across Go,
SQL, and proto) even though the `docs/planning/26` P0 Executor Pool schema names them. Task 30 built the
`/executor-pools` CRUD surface but its Out of Scope note ("do not add P1 pool-policy fields") was misread as covering
these P0 fields; its handoff flagged the gap with "no task file currently owns closing this gap." This task is that
owner. Workers already report `country`, `region`, and `ip_type` capabilities (worker credential
`allowed_capabilities`, registration), so the enforcement side has data to match against.

#### Out of Scope

- Do not add fields beyond the three named ones (`tags`, `allow_degraded_workers`, `executor_type`, `enabled` already
  exist).
- Do not build geo/IP-type detection on the worker; use the capabilities workers already report at registration.
- Do not change the P1 rollback or fingerprint surfaces.

#### Handoff Notes

- Document the migration and the eligibility-matching semantics (empty = unrestricted).
- If any capability value taxonomy is ambiguous in `docs/planning/26`, record the interpretation chosen.

<a id="p0-43"></a>

### 43 - Deny Rule Taxonomy Alignment

**Status:** done

#### Objective

Align the `deny_rules` schema and config API with the `docs/planning/26` P0 Deny Rule schema: rule `type` in
`cidr | host | host_suffix | cname_suffix | metadata_ip | private_range`, `action` in `deny | allow_override`, and a
`reason` field — replacing the narrower `host|cidr|cname|ip` + `deny|allow` taxonomy shipped by task 04/20.

#### Context (gap being closed)

The 2026-07-05 P0 verification audit confirmed `migrations/postgres/0001_init.sql` still constrains
`action IN ('deny','allow')` with the narrow `kind` set, and the comment at
`internal/control/config_admin_handlers.go:486` acknowledges the planning taxonomy as unimplemented with no owning
task. This task is that owner. Note the enforcement side already covers most of the intent —
`internal/control/destination_policy.go` compiles rules into `DestinationPolicyResult` and
`internal/egress/executor.go` enforces host-suffix, CNAME-suffix, and resolved-IP/private-range checks — so this task
is primarily about the config schema/API expressiveness and the compile step, not new egress enforcement.

#### Out of Scope

- Do not change the egress-side precedence order established by task 26.
- Do not add redirect or MITM-related rules (P1/P2).

#### Handoff Notes

- Document the old->new value mapping and the `allow_override` precedence decision.

<a id="p0-44"></a>

### 44 - Config Audit Event Enrichment

**Status:** done

#### Objective

Populate `config_version`, `field_path`, `old_value_json`, and `new_value_json` on `config_audit_events` rows, with
secret redaction, so the ClickHouse audit trail matches the canonical schema instead of shipping those columns empty.

#### Context (gap being closed)

The 2026-07-05 P0 verification audit confirmed `internal/control/audit.go` (`auditStoreWithEvents.Record`) enqueues
`ConfigAuditEvent` rows with only tenant/actor/resource/action populated — the struct and the ClickHouse schema
(`deploy/docker/clickhouse-schema.sql`, `docs/planning/22-canonical-clickhouse-schema.md`) already carry
`field_path`/`old_value_json`/`new_value_json`, but the upstream `AuditRecord` never supplies them. Task 32's handoff
flagged this with "no existing task file owns this specific enrichment." This task is that owner. It also unblocks
P1 rollback (`docs/implementation-history.md#p1-07-config-rollback-api.md`), which restores values from audit source records.

#### Out of Scope

- Do not build the P1 rollback API; only produce the records it will need.
- Do not add read/query APIs for the enriched fields (P1 telemetry read APIs own that).

#### Handoff Notes

- State the field-level vs whole-resource JSON decision and list the redacted field classes.

<a id="p0-45"></a>

### 45 - Redis-Backed Worker Runtime State

**Status:** done

#### Objective

Move worker session, heartbeat/load, and cooldown runtime state from Control's process memory into the existing Redis
runtime-state tier, so Control restart or replica boundaries do not erase the worker availability state that
`docs/planning/21` assigns to Redis.

#### Context (gap being closed)

The handoff sweep found a still-unowned P0-spec gap. `docs/planning/21-state-and-storage.md:62-76` says Redis stores
ephemeral runtime state with TTLs, including worker session/heartbeat/load state and cooldown state. Current code
still keeps that state only in `WorkerRegistry`'s in-process map: `internal/control/worker_registry.go:204-207`
documents it as the P0 in-process store and says Redis-backed TTL state is future work. Task 21 explicitly left
`WorkerRegistry` runtime state single-Control/in-memory (`the retired implementation handoff for 21-redis-wiring-and-config-invalidation.md:89-90`),
and task 40's Out of Scope recorded persistent worker state as having no owning task
(`docs/implementation-history.md#p0-40-egress-registration-retry.md:37-41`). This task is that owner.

#### Out of Scope

- Do not make worker runtime state durable; Redis remains ephemeral and every key needs a TTL or documented lifecycle.
- Do not change global or tenant worker admin disable persistence; those already belong in Postgres.
- Do not build multi-Control durable request cancellation; `docs/implementation-history.md#p1-23-multi-control-durable-cancellation-state.md`
  owns that separate in-flight request gap.
- Do not add persistent request queues or automatic replay workflows.

#### Handoff Notes

- Record the Redis key prefixes, TTLs, and Redis-outage fallback behavior.
- Confirm whether the old in-memory implementation remains only as a test fallback or is deleted.
- Confirm the task 21 and task 40 stale notes now point at this task.

<a id="p0-46"></a>

### 46 - Live Compose Verification of Tasks 42-45 Surfaces

**Status:** done

#### Objective

Drive the surfaces built by tasks 42 (executor pool capability fields), 43 (deny rule taxonomy), 44 (config audit
event enrichment), and 45 (Redis-backed worker runtime state) end-to-end through the live docker-compose stack,
observing real effects in Postgres, Redis, ClickHouse, and assignment behavior — closing the "live compose
verification: skipped" note each of those four handoffs carried.

#### Context (gap being closed)

The 2026-07-06 handoff sweep found four consecutive done tasks whose handoffs skipped live verification and said
so honestly:

- `the retired implementation handoff for 42-executor-pool-capability-fields.md` — "a live pool-restriction CRUD + assignment check
  has not been driven end-to-end ... it should be done as a deliberate, user-approved step".
- `the retired implementation handoff for 43-deny-rule-taxonomy-alignment.md` — "Live compose verification: skipped".
- `the retired implementation handoff for 44-config-audit-event-enrichment.md` — "Live compose verification: skipped".
- `the retired implementation handoff for 45-redis-backed-worker-runtime-state.md` — "Live compose verification: skipped because
  the full ... compose stack was not running".

The user approved this task on 2026-07-06. All four surfaces have unit and Postgres-integration coverage; what is
unverified is the built binaries exercising them against the real compose backends. This is a verification-only
task: it changes no implementation code. If a live run exposes a real defect, fixing it is a stop condition (ask,
or task it), not silent scope growth.

#### Out of Scope

- No implementation-code changes; docs and (if needed) compose/README fixes only.
- No load or performance claims (owned by `docs/implementation-history.md#p1-18-load-and-backpressure-testing.md`).
- No new automated test harness; this is a documented manual/live verification run. Automating it, if wanted,
  needs its own task.

#### Handoff Notes

- Record the compose stack revision (git SHA), every command run, and each observation with enough detail to
  replay.
- Record any defect found live and the stop-condition outcome (user decision or new owning task).

<a id="p0-46"></a>

### 46 - Tenant P0 Schema Fields

**Status:** done

#### Objective

Add the four canonical P0 tenant fields — `default_timeout_ms`, `max_timeout_ms`, `metadata_query_storage`,
`metadata_path_storage` — to the tenant schema, store, and admin API, and enforce them: request timeout defaulting
and clamping uses the tenant's values, and ClickHouse metadata sanitization of `target_url` follows the tenant's
query/path storage policy instead of the current hard-coded behavior.

#### Context (gap being closed)

`docs/planning/26-config-management-api-surface.md` lists all four fields in the Tenant schema under "P0 Config
Resource Schemas" (lines ~124-141: "minimal canonical P0 shapes ... must not remove these fields"). Task
`docs/implementation-history.md#p0-29-tenant-lifecycle-and-status-enforcement.md` skipped them via an Out of Scope line calling them
"P1 tenant metadata-storage-policy fields" — the exact "assumed P1, not checked" failure the Gap Ownership rules in
`AGENTS.md` name. The 2026-07-06 handoff sweep (this task's provenance) verified the gap in current code:

- `internal/control/tenant_store.go:27-42` — `Tenant` struct has no timeout or metadata-storage fields.
- `migrations/postgres/0003_tenant_fields.sql` — adds name/ceiling/config_version only.
- `internal/control/request_metadata.go:244-258` — `sanitizeTargetURL` unconditionally drops the query and stores
  the full path; no tenant policy, no `hash` option (planning/21 says default is `hash` for path).
- Timeout defaulting/clamping is static-config only (`internal/config/config.go:80`,
  `internal/control/dispatcher.go:76,691`); no per-tenant override exists.

Flagged (as "P1 fields ... not added") in
`the retired implementation handoff for 29-tenant-lifecycle-and-status-enforcement.md`.

#### Out of Scope

- Do not build telemetry read APIs over the stored metadata (P1 tasks 11/12 own that).
- Do not add payload/header capture (P2).
- Do not change the static `control.request.*` config keys; tenant values layer on top of them (tenant
  `max_timeout_ms` may not exceed the static ceiling).

#### Handoff Notes

- Record the hash function chosen for `hash` mode and where the tenant snapshot is read on the metadata path.
- Record how tenant timeouts compose with the static `control.request.*` ceilings.

<a id="p0-48"></a>

### 48 - Deny `host_suffix` Leading-Dot Normalization Fix

**Status:** done

#### Objective

A `host_suffix` deny rule created with a leading-dot value (e.g. `.evil.example`) enforces exactly like the same
rule without the leading dot: it denies the suffix host and all its subdomains, both in Control's pre-dispatch
check and in Egress's post-resolution check. After this task, no accepted `host_suffix` (or `host`) deny rule can
be silently non-enforcing because of a leading dot in its value.

#### Context (gap being closed)

The 2026-07-07 live compose verification (`docs/implementation-history.md#p0-46-live-compose-verification.md`, surface 43) found that a
`host_suffix` deny rule created via the admin API with a leading-dot value is accepted with HTTP 200 but **never
matches** — a request to a subdomain of that suffix is not denied and instead falls through to Egress (where it
failed DNS in the test). A tenant admin who writes `.evil.example` believes they denied that suffix; they did not.

Current-code evidence:

- `internal/control/config_admin_handlers.go:695-696` — `normalizeHostname` does
  `strings.TrimSuffix(strings.ToLower(strings.TrimSpace(host)), ".")`: it trims a *trailing* dot only, so a
  leading-dot value like `.evil.example` is stored verbatim in `deny_rules.normalized_host`.
- `internal/control/destination_policy.go:354-364` — `hostMatchesSuffix(host, suffix)` returns
  `host == suffix || strings.HasSuffix(host, "."+suffix)`. With `suffix = ".evil.example"` the second arm searches
  for `"..evil.example"` (double dot), which no real host ends with, so the rule never matches.
- Live evidence (2026-07-07): `{"type":"host_suffix","value":".blocked.example","action":"deny"}` → 200, then
  `GET https://sub.blocked.example/` was **not** denied; the identical rule with value `blocked2.example` (no
  leading dot) → `GET https://sub.blocked2.example/` returned `destination_denied`.
- `internal/egress/executor.go` `hostMatchesSuffix` mirrors the Control matcher (per the comment at
  `internal/control/destination_policy.go:351-353`), so the leading-dot value is non-enforcing on the Egress side
  too — the rule is fully inert, not merely a Control pre-check miss.

The gap exists because task 43 (deny-rule taxonomy alignment) formalized `host_suffix` and its normalization but
neither stripped a leading dot on write nor rejected it, and both matchers assume the stored suffix has no leading
dot.

#### Out of Scope

- Do not change `cidr` / `metadata_ip` / `private_range` CIDR normalization, or `cname_suffix` semantics; this task
  is scoped to hostname (`host`, `host_suffix`) leading-dot handling only.
- Do not change the wire/DB column shape of `deny_rules`; this is a value-normalization fix, not a schema change.
- Do not weaken the existing trailing-dot trimming or lowercase/trim behavior.

#### Handoff Notes

- Record the decision (strip vs reject) and the rationale.
- Record whether the live surface-43 re-check was run and its result.
- If Egress's `hostMatchesSuffix` needed any change to stay in parity, note it (this crosses into `cmd/egress`
  runtime behavior and must be verified, not assumed).

<a id="p0-49"></a>

### 49 - Egress Worker Capability Declaration from Static Config

**Status:** done

#### Objective

The official Go Egress Worker reads its declared capabilities (`countries`, `regions`, `ip_types`, and — where not
already sourced elsewhere — `tags`) from static config and sends them in its NATS `RegisterRequest`, so that a
worker can legitimately claim a non-empty capability set. After this task, an executor pool's
`allowed_ip_types` / `allowed_countries` / `allowed_regions` restriction (task 42) can actually exclude a live
worker whose declared capabilities are not a subset of the restriction — the exclusion branch becomes
observable end-to-end, not unit-test-only.

#### Context (gap being closed)

The 2026-07-07 live compose verification (`docs/implementation-history.md#p0-46-live-compose-verification.md`, surface 42) could
verify pool-restriction *persistence* and the *matching* (positive) routing path live, but could **not** drive the
*non-matching* (exclusion) path, because the shipped Egress worker always registers with empty
countries/regions/ip_types. An empty claimed set is a subset of every restriction (`subset([], anything) == true`,
`internal/control/worker_registry.go:1096`), so the official worker is never excluded by any pool restriction.

Current-code evidence:

- `cmd/egress/main.go:214-218` — `caps := egress.Capabilities{SoftwareVersion: "dev", MaxConcurrency: ...,
  AllowedPools: pools}`: `Countries`, `Regions`, `IPTypes` are never set.
- `internal/config/config.go` — `EgressConfig` declares `AllowedPools` (`allowed_pools`) but has no
  countries/regions/ip_types/tags capability fields to load.
- `internal/egress/registration.go:35-37` and `:71-73` — the `Capabilities` struct and the `RegisterRequest`
  builder already carry and send `Countries` / `Regions` / `IpTypes`; only the config→caps population is missing.
- `docs/planning/24-static-configuration.md:82-86` — `egress.capabilities.countries`, `egress.capabilities.regions`,
  `egress.capabilities.ip_types` (and `.tags`, `.supported_ingress_modes`) are canonical static config keys with
  `[]` defaults.

Registration already *rejects* claims outside the credential's `allowed_capabilities`
(`internal/control/worker_registry.go:608-616`), so a worker that declares capabilities must have a credential
scoped to permit them — the enforcement side is built; only the worker's declaration side is missing.

#### Out of Scope

- Do not change the pool-side restriction fields, routing `poolAllows`, or the credential capability-scope
  rejection — those are built (task 42) and correct.
- Do not add per-request or dynamic capability negotiation; capabilities are static config declared at registration.
- Do not change `max_concurrency`/`supported_ingress_modes` sourcing if they are already wired elsewhere; add only
  the missing declaration fields, and state in the handoff which were already present.

#### Handoff Notes

- Record the config key shape chosen and its relationship to `docs/implementation-history.md#p1-22-...` (flat vs nested).
- Record which capability fields were already wired (`max_concurrency`, `supported_ingress_modes`) vs added here.
- Record the live exclusion-path result if run; if compose was unavailable, say so explicitly and keep this task as
  the named owner in the surface-42 notes.

<a id="p0-50"></a>

### 50 - Config Audit Event `field_path` Population

**Status:** done

#### Objective

Config audit events written to Postgres `config_audit_source` and mirrored to ClickHouse `config_audit_events`
carry a meaningful `field_path` (or a deliberate, documented sentinel) rather than an always-empty string, so the
`field_path` column and the `/changes` audit history satisfy the `docs/planning/26` "Config Audit Change" shape.
After this task, an auditor can tell from an audit row which field(s) changed, not only the whole-object before/after
JSON.

#### Context (gap being closed)

The 2026-07-07 live compose verification (`docs/implementation-history.md#p0-46-live-compose-verification.md`, surface 44) confirmed
that audit rows now carry `config_version`, `old_value_json`, and `new_value_json` (task 44) with secrets redacted —
but `field_path` is **empty on every row**, for both `create` and `update` actions.

Current-code evidence:

- `internal/control/audit.go:127-159` — `recordAudit` accepts a `fieldPath string` parameter and threads it into
  `AuditRecord.FieldPath`, but every call site passes `""`:
  `grep -n 'recordAudit(' internal/control/*.go` shows ~15 calls in `admin_handlers.go` and
  `config_admin_handlers.go`, all with an empty `fieldPath` argument (e.g. `admin_handlers.go:220`, `:341`, `:415`).
- `docs/planning/26-config-management-api-surface.md:404` — the canonical "Config Audit Change" object carries
  `"field_path": "match_conditions.target_host"` alongside per-field `old_value_json`/`new_value_json`, i.e. the
  spec models a field-scoped change, not only a whole-object diff.
- `docs/planning/22-canonical-clickhouse-schema.md:90` — `config_audit_events.field_path String` is a P0 column.

Task 44 (P0) closed the `config_version`/`old_value_json`/`new_value_json` half of the 2026-07-05 audit gap using a
whole-object diff and left `field_path` empty; that remnant of the original gap has had no owning task until now.

#### Out of Scope

- Do not change secret redaction, the `SkipPostgres` double-write prevention, or the `config_version`/old/new value
  wiring — those are built (task 44) and verified live.
- Do not build the P1 rollback flow (`docs/planning/26` "P1 Config Resource Schemas"); this task is audit-read
  fidelity only.
- Do not remove the whole-object `old_value_json`/`new_value_json`; `field_path` complements them, it does not
  replace them.

#### Handoff Notes

- Record the `field_path` convention and why (per-field vs whole-object sentinel), and whether the planning-doc
  per-field model is fully met or approximated.
- Record whether the live surface-44 re-check was run and its result.

<a id="p1-01"></a>

### 01 - HTTP Proxy Ingress Spec

**Status:** done

#### Objective

Specify the P1 HTTP forward proxy contract before implementation: proxy authentication, raw-socket error rendering,
request parsing, response streaming, and backpressure.

#### Out of Scope

- Do not implement the proxy listener.
- Do not implement CONNECT.
- Do not add SDK, CLI, or UI work.

#### Handoff Notes

- Link the new planning appendix.
- List any new open decisions.

<a id="p1-02"></a>

### 02 - HTTP Proxy Ingress

**Status:** done

#### Objective

Implement the HTTP forward proxy listener on port 8081 and translate accepted proxy requests into the same internal
request pipeline used by REST.

#### Out of Scope

- Do not implement CONNECT.
- Do not implement REST streaming.
- Do not implement MITM.

#### Handoff Notes

- Document listener config and supported request forms.

<a id="p1-03"></a>

### 03 - Raw Streaming Response Path

**Status:** done

#### Objective

Add the Control-to-client raw response streaming path needed by proxy modes and shared by the future REST streaming
endpoint.

#### Out of Scope

- Do not implement `/api/v1/requests:stream`.
- Do not implement CONNECT tunneling.
- Do not change the P0 REST JSON envelope.

#### Handoff Notes

- Document how trailers and post-header errors are represented.

<a id="p1-04"></a>

### 04 - Routing Ingress Type and Worker Capability

**Status:** done

#### Objective

Thread `ingress_type` through routing and worker capability checks so REST, HTTP proxy, CONNECT, and MITM requests can
be routed only to compatible workers.

#### Out of Scope

- Do not implement proxy, CONNECT, or MITM listeners.
- Do not add new ingress modes beyond the documented enum.

#### Handoff Notes

- Document default ingress-mode behavior for old workers/config.

<a id="p1-05"></a>

### 05 - Raw CONNECT Tunnel

**Status:** done

#### Objective

Implement P1 raw CONNECT tunnel ingress on port 8082 using existing NATS stream credit semantics and destination policy
validation.

#### Out of Scope

- Do not allow CONNECT on the REST endpoint.
- Do not add SOCKS5, WebSockets, generic TCP, UDP, or QUIC.
- Do not implement MITM.

#### Handoff Notes

- Document tunnel timeout and accounting choices.

<a id="p1-06"></a>

### 06 - REST Streaming Endpoint

**Status:** done

#### Objective

Implement `POST /api/v1/requests:stream` using the resolved P1 binary frame format.

#### Out of Scope

- Do not change `POST /api/v1/requests`.
- Do not implement BodyRef.
- Do not implement proxy ingress.

#### Handoff Notes

- Link the resolved decision and list the implemented frame types.

<a id="p1-07"></a>

### 07 - Config Rollback API

**Status:** done

#### Objective

Implement `POST /api/v1/config/rollback` so tenant admins can create a new config version from audit-source history
without restoring redacted secrets.

#### Out of Scope

- Do not restore fields redacted as secrets.
- Do not reuse an old config version number.
- Do not add generic point-in-time database restore.

#### Handoff Notes

- Document which resources are rollback-safe.

<a id="p1-08"></a>

### 08 - Multi-Tenant Worker Credentials

**Status:** done

#### Objective

Add platform-scoped creation and validation of worker credentials that can serve multiple tenants.

#### Out of Scope

- Do not let tenant-scoped keys create multi-tenant credentials.
- Do not add marketplace or provider billing behavior.
- Do not weaken worker registration signature validation.

#### Handoff Notes

- Document the stored scope shape and migration behavior.

<a id="p1-09"></a>

### 09 - Go SDK

**Status:** done

#### Objective

Create the minimal Go SDK for Straw request transport and public error handling.

#### Out of Scope

- Do not add non-Go SDKs.
- Do not add retry orchestration beyond documented replayable defaults.
- Do not wrap P2 features.

#### Handoff Notes

- Document package path, public types, and supported endpoints.

<a id="p1-10"></a>

### 10 - CLI

**Status:** done

#### Objective

Add a minimal CLI over the Go SDK and P0/P1 config/admin endpoints.

#### Out of Scope

- Do not build an interactive UI.
- Do not add auth modes beyond API keys.
- Do not implement provider marketplace commands.

#### Handoff Notes

- List every command and environment variable used.

<a id="p1-11"></a>

### 11 - Telemetry Schema and Query Limits Spec

**Status:** done

#### Objective

Specify tenant-facing telemetry schemas, filters, pagination, and ClickHouse query limits before implementing telemetry
read APIs.

#### Out of Scope

- Do not implement telemetry endpoints.
- Do not expose worker IDs, session IDs, or selected executor values to tenant-facing APIs.

#### Handoff Notes

- Link the spec and note any unresolved query tradeoffs.

<a id="p1-12"></a>

### 12 - Telemetry Read APIs

**Status:** done

#### Objective

Implement tenant-safe telemetry read APIs over ClickHouse metadata.

#### Out of Scope

- Do not add payload capture reads.
- Do not expose internal worker/session topology to tenant-scoped keys.
- Do not implement dashboards.

#### Handoff Notes

- Document supported filters and limits.

<a id="p1-13"></a>

### 13 - Observability Dashboards

**Status:** done

#### Objective

Add operational dashboards for the documented metrics, health, outage, and SLO signals.

#### Out of Scope

- Do not implement telemetry API schemas.
- Do not expose tenant-private data in shared dashboards.

#### Handoff Notes

- List dashboard files and required data sources.

<a id="p1-14"></a>

### 14 - Minimal Admin UI

**Status:** done

#### Objective

Build a minimal read-mostly admin and observability UI over existing admin, config, and telemetry APIs.

#### Out of Scope

- Do not replace the API as source of truth.
- Do not add user/password auth.
- Do not implement marketplace or billing UI.

#### Handoff Notes

- List views and API endpoints consumed.

<a id="p1-15"></a>

### 15 - Egress Metrics Exposure

**Status:** done

#### Objective

Implement Control-aggregated Egress metrics behind an explicit enablement flag.

#### Out of Scope

- Do not expose a worker-local `/metrics` endpoint or map a worker metrics port.
- Do not add unrelated dashboard work.

#### Handoff Notes

- Link the resolved decision and list metrics added.

<a id="p1-16"></a>

### 16 - Upstream Connection Pooling

**Status:** done

#### Objective

Specify optional upstream connection pooling and the feature flag/testing bar required before any implementation.

#### Out of Scope

- Do not implement pooling in this spec task.
- Do not enable outbound HTTP/2.
- Do not weaken the resolver/validator/dialer invariant.

#### Handoff Notes

- Link the spec and list implementation tasks it enables.

<a id="p1-17"></a>

### 17 - Worker Loss and NATS Outage Hardening

**Status:** done

#### Objective

Harden worker-loss and NATS-outage behavior beyond the P0 baseline, including the explicit decision on pre-connect
fallback after `RequestStart`.

#### Out of Scope

- Do not add automatic client retry workflows.
- Do not change replay rules without tests.
- Do not add persistent request queues.

#### Handoff Notes

- Document the pre-connect fallback decision.

<a id="p1-18"></a>

### 18 - Load and Backpressure Testing

**Status:** done

#### Objective

Add load and backpressure tests that validate routing SLOs, active request limits, worker capacity behavior, and NATS
credit flow under pressure, including asserting that request-metadata rows actually land in a live ClickHouse table
under load (the flag `the retired implementation handoff for 25-p0-test-matrix-and-compose.md` left unowned — this task is the owner).

#### Out of Scope

- Do not claim production capacity benchmarks from laptop-only runs.
- Do not add billing-grade quota reconciliation.

#### Handoff Notes

- Record hardware/runtime assumptions for any local load numbers.

<a id="p1-19"></a>

### 19 - Production Deployment Templates

**Status:** done

#### Objective

Add production deployment templates and operator docs for the P0/P1 service set.

#### Out of Scope

- Do not add regional NATS topology unless it has a written owner decision.
- Do not map unused ingress ports.
- Do not implement managed disaster recovery.

#### Handoff Notes

- List templates, required secrets, and unsupported deployment assumptions.

<a id="p1-20"></a>

### 20 - Control Log Events ClickHouse Ingestion

**Status:** done

#### Objective

Ship Control's structured service logs to the ClickHouse `log_events` table defined in `docs/planning/22`, using the
same async, bounded, non-blocking write discipline as `request_events`.

#### Out of Scope

- Do not modify `cmd/egress/main.go` or add Egress ClickHouse config; Egress log transport is owned by
  `docs/implementation-history.md#p1-27-egress-log-events-nats-transport.md`.
- Do not build log search/read APIs (P1 tasks 11-12 own telemetry reads).
- Do not capture request/response payloads.
- Do not add Loki or any non-ClickHouse canonical log sink.

#### Handoff Notes

- Document the queue bounds, drop policy, and the fields mapped into `extra`.
- State that Egress log ingestion remains owned by `docs/implementation-history.md#p1-27-egress-log-events-nats-transport.md`.

<a id="p1-21"></a>

### 21 - IDNA Hostname Support

**Status:** done

#### Objective

Accept internationalized (non-ASCII) target hostnames by converting them to punycode/A-labels before destination
policy evaluation and dispatch, replacing the current fail-closed rejection.

#### Context (gap being closed)

Task p0/22 (Control destination policy resolution) deliberately rejects non-ASCII hostnames fail-closed and its
handoff flagged IDNA as unowned. That is safe but wrong for legitimate international domains. This task is the owner.
Security constraint: policy evaluation (deny rules, host/CNAME suffix matching) must run on the normalized A-label
form so that Unicode look-alikes cannot bypass suffix or host rules.

#### Out of Scope

- Do not hand-roll IDNA/punycode conversion; adding `golang.org/x/net/idna` is explicitly approved for this task.
- No display-form (U-label) round-tripping in telemetry beyond recording the normalized form.

<a id="p1-22"></a>

### 22 - Egress Credential Config Schema Reconciliation (flat vs nested)

**Status:** done

#### Objective

The egress credential configuration has exactly one canonical shape across code, deploy fixtures, and the planning
doc. After this task, `docs/planning/24-static-configuration.md` no longer describes two conflicting shapes for the
worker identity/credential keys, the config loader accepts only the canonical shape, and a config test proves the
canonical form round-trips and the non-canonical `egress.credential.*` nested form does not silently "work."

#### Context (gap being closed)

The P0 audit and the task-35 handoff (`the retired implementation handoff for 35-worker-registration-replay-and-identity-key.md`,
"Remaining Work") flagged a config-schema/doc mismatch and explicitly recorded it as a previously unowned gap. This
task is the owner.

Current-code evidence:

- `internal/config/config.go:138-148` — `EgressConfig` declares `WorkerID` (`worker_id`), `CredentialID`
  (`credential_id`), and `PrivateKeyEd25519Env` (`private_key_ed25519_env`) as **flat top-level** JSON keys under
  `egress`.
- `deploy/docker/egress.json:4-6` — the working deploy fixture uses the **flat** keys (`worker_id`,
  `credential_id`, `private_key_ed25519_env`), matching the code and loading successfully in compose.
- `docs/planning/24-static-configuration.md` is internally inconsistent:
  - the key table lists both the **nested** `egress.credential.credential_id_env` /
    `egress.credential.private_key_env` (lines ~77-78) **and** the flat `egress.private_key_ed25519_env` (line ~79);
  - the Egress Config Example shows the flat keys (lines ~219-221) **and** a separate nested `credential:` block
    (lines ~231-233);
  - the note at lines ~109-112 already concedes the flat shape is what is implemented and states the reconciliation
    was previously unowned.

Decision (reconciliation direction — do not re-litigate): **the flat shape is canonical; correct the planning doc to
the flat shape.** Rationale: task 35 deliberately chose flat keys "for consistency with the fields it sits next to,"
the running deploy fixture and the validated loader are already flat, and the planning doc itself documents the flat
shape as the implemented one. The generic "planning doc wins" rule does not force the nested shape here because the
planning doc is self-contradictory and already records the flat shape as intended — the reason-otherwise condition in
the AGENTS.md rule is met. If, while executing, you find a concrete correctness reason the nested shape must win
instead, that is a stop-and-ask condition, not a silent reversal.

#### Out of Scope

- Do not change any credential *values*, secret-handling behavior, or the env-var indirection convention (secrets stay
  in env vars, never inline) — only the config *key shape* and its documentation.
- Do not touch the `egress.capabilities.*`, `egress.outbound_*`, `egress.dns.*`, or NATS config keys; this task is
  scoped to the three identity/credential keys (`worker_id`, `credential_id`, `private_key_ed25519_env`).
  (Cross-note: P0 task 49, `docs/implementation-history.md#p0-49-egress-capability-declaration-from-config.md`, added the nested
  `egress.capabilities.*` object on 2026-07-07 matching `docs/planning/24`; that nested block is intentionally the
  planning-doc shape and is not part of this task's flat-key reconciliation.)
- Do not add a backward-compat shim that accepts both shapes; a single canonical shape is the whole point.

#### Handoff Notes

- Record that the flat shape was chosen as canonical and why (task-35 precedent + running fixture + validated loader),
  so a future reader does not re-open the direction question.
- Confirm the task-35 handoff's previously-unowned flag for this gap is now closed by this task.

<a id="p1-23"></a>

### 23 - Multi-Control Durable Cancellation State

**Status:** done

#### Objective

Admin request cancellation works across more than one Control replica. When an admin cancel targets a `request_id`
whose dispatch is in-flight on a **different** Control instance than the one that received the cancel call, the
owning Control still tears the request down (publishes the request's `CancelFrame` and returns the cancelled
terminal outcome), instead of the cancel being lost because the in-flight entry lives only in the receiving
instance's process memory.

#### Context (gap being closed)

The P0 audit and the task-27 handoff (`the retired implementation handoff for 27-admin-request-cancellation.md`, "Remaining Work")
flagged that P0 cancellation is single-Control only and recorded it as having **no owning task** ("if needed later
they'd be a new P1/P2 task"). This task is the owner.

Current-code evidence (single-Control, in-process only):

- `internal/control/inflight.go` — `InFlightRegistry` maps `request_id -> {tenant_id, cancel func()}` in a plain
  in-memory map guarded by a mutex; the stored `cancel func()` is a live `context.CancelFunc` that cannot cross a
  process boundary.
- `internal/control/dispatcher.go` — `Dispatch` registers `(request_id, tenant_id, cancel)` with `d.opts.InFlight`
  and `defer`-deregisters on every return; the registry is populated only by the instance running the dispatch.
- `internal/control/request_admin_handlers.go` — `CancelRequest` resolves the target via
  `InFlightRegistry.Cancel`, which returns `ErrRequestNotFound` when the `request_id` is not in *this* process's
  map — exactly the miss that happens when a sibling Control owns the request.
- `cmd/control/main.go` (`buildControlMux`) constructs **one** `control.NewInFlightRegistry()` per Control process
  and wires the same instance into both the dispatcher and the admin handler — confirming the state is per-process.

Owning task for the single-Control implementation this extends: `docs/implementation-history.md#p0-27-admin-request-cancellation.md`.

#### Out of Scope

- No persistent request queue and no automatic retry/replay workflow (both are Future Work in
  `docs/planning/02-phase-boundaries.md`, lines 95-96).
- No change to the authorization model — `AuthorizeAdminCancel` and the role checks in `CancelRequest` stay exactly
  as P0 task 27 left them; only the *reach* of a cancel across instances changes.
- Do not introduce a new datastore; reuse the existing Redis runtime-state tier already wired into Control.
- Do not change the single-Control fast path's behavior or its tests; the local in-process cancel must remain the
  path taken when the receiving instance owns the request.

#### Handoff Notes

- Record the chosen cross-instance signal mechanism and why it stays within the existing Redis runtime-state tier.
- Record that the local in-process fast path is preserved and how the test proves it is taken for locally-owned
  requests.
- Confirm the task-27 "no owning task" deferral is now closed.

<a id="p1-24"></a>

### 24 - Streaming Credit Replenishment

**Status:** done

#### Objective

Make the e2c response path actually credit-governed: the egress executor streams upstream response bodies as
multiple `DataFrame`s bounded by granted download credit (instead of one buffered frame), and Control replenishes
download credit with `CreditFrame`s on the c2e subject as it consumes buffered bytes, per the credit rules in
`docs/planning/12-nats-protocol.md`.

#### Context (gap being closed)

`the retired implementation handoff for 24-control-request-dispatch-pipeline.md` flagged that the original P0 slice did not send
c2e `CreditFrame` replenishment and did not credit-govern response streaming. Verified before this task
(2026-07-06 sweep):

- `internal/egress/loop.go:363-367` — received `CreditFrame`s were deliberately ignored in the P0 slice.
- `internal/control/dispatcher.go` — grants only the initial download credit via `AssignRequest`
  (`InitialDownloadCreditBytes`, line ~681); no replenishment `CreditFrame` is ever published.

P1 task 03 (raw streaming response path) owns Control-side client-facing backpressure but its Expected Files are
Control-only; P1 task 05 (raw CONNECT tunnel) *assumes* "the existing c2e/e2c credit protocol" works. Neither owns
the egress-side chunked send or Control's replenishment — this task does, and both depend on it.

#### Out of Scope

- Do not build the client-facing raw response writer (P1 task 03).
- Do not implement CONNECT tunneling (P1 task 05).
- Do not change upload-direction (c2e request body) credit behavior beyond what symmetry requires.
- Do not change the P0 REST JSON envelope or its inline body limit.

#### Handoff Notes

- Record the replenishment policy chosen (when/how much credit is granted back) and cite the planning/12 rule.

<a id="p1-25"></a>

### 25 - CNAME Chain Inspection for Deny Rules

**Status:** completed

#### Objective

Enforce `denied_cname_suffixes` against every hop of the target's CNAME chain, not just the final canonical name:
a deny rule whose suffix matches an intermediate (non-final) CNAME hop rejects the request with
`destination_denied`, closing the bypass where a denied intermediary hides behind a clean final target.

#### Context (gap being closed)

`the retired implementation handoff for 26-egress-destination-policy-precedence.md` ("CNAME Chain Depth", lines 57-63) flagged —
with no owning task — that only the final canonical name is inspected. Verified in current code (2026-07-06
sweep):

- `internal/egress/executor.go:52-58` — the `Resolver` interface documents that Go's `net.Resolver.LookupCNAME`
  follows the whole chain internally but returns only the final name, so "only the final canonical name is
  available for denied_cname_suffixes enforcement".
- `internal/egress/executor.go:694-705` — enforcement matches suffixes against that single returned name.

`docs/planning/27-security-controls.md:49` lists "CNAME chains" in the deny-rule normalization requirements, the
same list whose IDNA row is owned by `docs/implementation-history.md#p1-21-idna-hostname-support.md`; this task is the sibling owner
for the CNAME-chain row. The p0/26 task text explicitly permitted "document why deeper chain inspection is out of
reach with the stdlib resolver" — this task lifts that ceiling. It completed with standard-library DNS message
parsing, so no external resolver dependency gate remains.

#### Out of Scope

- No change to `denied_host_suffixes`, CIDR, or IP-literal enforcement semantics.
- No Control-side DNS resolution — CNAME enforcement stays in Egress, per p0/26.
- No resolver caching layer; per-request lookup semantics stay as they are.

#### Handoff Notes

- Record why stdlib sufficed and the chain normalization applied per hop.

<a id="p1-26"></a>

### 26 - Upstream Connection Pooling Implementation

**Status:** done

#### Objective

Implement the optional, default-off Egress upstream connection pool specified in
`docs/planning/b-upstream-connection-pooling.md`, preserving the direct-local SSRF invariant and P0 transport defaults
when disabled.

#### Context (gap being closed)

P1 task 16 specified upstream connection pooling but deliberately did not change transport code. The current Egress
executor still hard-codes P0 defaults:

- `internal/egress/executor.go:118` sets `DisableKeepAlives: true`.
- `internal/egress/executor.go:119` sets `ForceAttemptHTTP2: false`.
- `internal/egress/executor_test.go:489` asserts keep-alives stay disabled.

The new spec makes pooling allowed only behind `egress.upstream_connection_pool.enabled` and only after tests prove the
pool-key and SSRF boundaries.

#### Out of Scope

- Do not enable outbound HTTP/2.
- Do not add proxy-mode connection pooling; the P1 pooling spec is direct-local only.
- Do not change routing, fallback, or retry behavior.
- Do not weaken the resolver/validator/dialer invariant.

#### Handoff Notes

- Record the concrete pool-key implementation and the tests proving each Section 30 row.
- State whether live compose verification was run; if skipped, explain why.

<a id="p1-27"></a>

### 27 - Egress Log Events NATS Transport

**Status:** done

#### Objective

Ship Egress worker structured logs to the canonical ClickHouse `log_events` table through Control: Egress emits
bounded, non-blocking log telemetry over Core NATS, and Control receives it and enqueues rows through the existing
Control-owned `log_events` writer.

#### Context (gap being closed)

P1 task 20 was split on 2026-07-06 after review found its original `cmd/egress/main.go` ClickHouse-wiring step
conflicted with `docs/planning/04-canonical-architecture.md`: Control owns observability aggregation, and executors are
not allowed to query ClickHouse. Current code proves the Egress half is not implemented:

- `cmd/egress/main.go:69` sets a local stdout JSON logger with `service=egress`; it has no log transport writer.
- `api/proto/straw/v1/straw.proto:116-124` defines NATS envelope payloads for registration, heartbeat, assignment,
  and request streams only; there is no log-event payload.
- `docs/planning/12-nats-protocol.md:52-58` defines no canonical log telemetry subject.

Task 20 owns Control's local `slog` tee into ClickHouse. This task owns the Egress-to-Control transport for the same
canonical sink. ClickHouse remains the only canonical log sink; do not add Loki.

#### Out of Scope

- Do not add ClickHouse config, credentials, or direct ClickHouse writes to Egress.
- Do not add Loki, OpenTelemetry, Promtail, or any other canonical log sink.
- Do not build log search/read APIs.
- Do not capture request or response payloads.
- Do not introduce JetStream, durable log queues, or replay semantics.

#### Handoff Notes

- Document the NATS subject, protobuf message, queue bounds, drop policy, redaction behavior, and any live verification
  performed.

<a id="p1-28"></a>

### 28 - SDK and CLI REST Streaming Client Surface

**Status:** done

#### Objective

Expose the P1 `/api/v1/requests:stream` binary framing endpoint through the Go SDK and the CLI so clients can submit a
request once, receive upstream metadata before body bytes, consume body chunks without buffering the full response, and
observe trailers, terminal timing, and post-metadata error frames.

#### Context (gap being closed)

P1 task 09 deliberately skipped SDK stream support while task 06 was incomplete. Task 06 now owns the server endpoint,
but the current SDK and CLI still expose only the non-streaming JSON request path:

- `sdk/client.go:14` defines only `requestsPath = "/api/v1/requests"`.
- `sdk/client.go:52` exposes only `Client.Do`, which reads the full JSON response body before returning.
- `sdk/doc.go:3-4` lists only `POST /api/v1/requests` as a supported SDK endpoint.
- `internal/cli/cli.go:117-150` implements the `request` command through `sdk.Client.Do`, so the CLI also buffers the
  non-streaming JSON envelope and has no stream mode.
- `the retired implementation handoff for p1-09-go-sdk.md:42` recorded `/api/v1/requests:stream` as out of scope because task 06 was not
  complete; this task is the owner after task 06.

#### Out of Scope

- Do not change Control's stream endpoint framing.
- Do not add BodyRef, MITM, proxy-ingress, retry orchestration, or non-Go SDKs.
- Do not buffer the entire streamed response in the SDK or CLI before exposing body bytes.

#### Handoff Notes

- Record the SDK API shape, the CLI invocation, stream frame parsing behavior, and how post-metadata errors are
  represented to callers.

<a id="p1-29"></a>

### 29 - Python Client SDK

**Status:** done

#### Objective

Add a minimal Python client SDK for Straw's public request transport so Python callers can submit blocking JSON
requests, consume REST streaming frames incrementally, parse canonical public errors, and use the same replayable
defaults as the Go SDK without depending on Straw internals.

#### Context (gap being closed)

This task was requested directly on 2026-07-08 to add a Python client SDK. Current code proves only the Go client SDK
exists:

- `sdk/doc.go:1-12` documents `sdk` as Straw's minimal Go SDK and lists only Go public types.
- `sdk/client.go:1-2` declares the existing SDK package as a Go client for Straw's public API.
- `sdk/stream.go:14-17` defines the Go client's `/api/v1/requests:stream` path and binary content type.
- `sdk/types.go:5-139` defines the Go request, response, error, stream-frame, and replayable-default types.
- `docs/implementation-history.md#p1-09-go-sdk.md:21` explicitly put non-Go SDKs out of scope for the original SDK task.
- `find . -maxdepth 3 \( -name 'pyproject.toml' -o -name 'setup.py' -o -name '*.py' \)` currently finds no Python
  client package; only `deploy/docker/kms-mock.py` exists.

#### Out of Scope

- Do not add retry orchestration beyond documented `replayable` defaults.
- Do not add Egress SDK, custom worker, BodyRef, MITM, payload-capture, or telemetry API clients.
- Do not add a CLI surface for Python.
- Do not publish to PyPI or add release automation.

#### Handoff Notes

- Record the Python package path, public API shape, error representation, streaming iterator behavior, packaging choice,
  and exact Python test command used.

<a id="p1-30"></a>

### 30 - Grafana Dashboard Mount Path Test Consistency

**Status:** done

#### Objective

Restore a green `make check` by reconciling `deploy/observability/dashboard_test.go` with the Grafana dashboard
provisioning paths currently shipped in `docker-compose.yml` and the provider config, so
`TestGrafanaProvisioningMatchesComposeMounts` asserts the real, self-consistent runtime paths instead of stale
pre-`aba1602a` strings.

#### Context (gap being closed)

`make test` (and therefore `make check`) is **red on `master` right now** — verified 2026-07-08:

```
--- FAIL: TestGrafanaProvisioningMatchesComposeMounts (0.00s)
    dashboard_test.go:101: observability deployment config missing "/etc/grafana/provisioning/dashboards/straw"
```

Root cause: commit `aba1602a` ("chore: stuff", 2026-07-08 01:47) moved the Grafana dashboard file-provider path
**consistently** in two files but never updated the test that pins them:

- `deploy/observability/grafana/provisioning/dashboards/straw.yml:11` — `path: /etc/grafana/dashboards/straw`
- `docker-compose.yml:192` — `./deploy/observability/grafana/dashboards:/etc/grafana/dashboards/straw:ro`

The provider config and the compose mount **agree with each other** (dashboards land at
`/etc/grafana/dashboards/straw`, and that is where the provider looks), so the running stack is internally
consistent. The outlier is the test:

- `deploy/observability/dashboard_test.go:86` still expects `/etc/grafana/provisioning/dashboards/straw`.
- `deploy/observability/dashboard_test.go:90` still expects the old mount
  `./deploy/observability/grafana/dashboards:/etc/grafana/provisioning/dashboards/straw:ro`.

This gap was flagged in `the retired implementation handoff for p1-29-python-client-sdk.md` (Verification section) but assigned to
`docs/implementation-history.md#p1-13-observability-dashboards.md`, which is `done`. A completed task cannot own a regression
introduced by a later, unrelated commit, so per AGENTS.md Gap Ownership this task is the real owner. p1-29's
suggested "one-line revert" is the **wrong** direction: reverting only the two config paths would re-break
provider/compose consistency to satisfy a stale test. The correct fix updates the test to the shipped paths (unless
the planning doc requires the dashboards to live under `/etc/grafana/provisioning/dashboards/straw`, in which case
revert all three consistently — see Stop Conditions).

#### Out of Scope

- No change to dashboard JSON content, datasource, Prometheus, or blackbox config.
- No new dashboards or provisioning providers.
- No change to the `grafana/provisioning/datasources` or `grafana/provisioning/dashboards` (provider config dir)
  mounts, which are unaffected by `aba1602a`.

#### Handoff Notes

- Record which direction was chosen (update test vs. revert config) and the planning-doc basis for it.

<a id="p2-01"></a>

### 01 - MITM Leaf Certificate Design

**Status:** done

#### Objective

Specify MITM leaf-certificate storage, encryption, cache miss coalescing, and unique-SNI flood controls before MITM
implementation starts.

#### Out of Scope

- Do not implement MITM ingress.
- Do not generate production CA material.
- Do not create certificate cache code.

#### Handoff Notes

- Link the design and the resolved open decision.

<a id="p2-02"></a>

### 02 - MITM Ingress

**Status:** done

#### Objective

Implement decoded HTTPS MITM ingress on port 8083 using server-side TLS and the same internal request model as REST and
HTTP proxy.

#### Out of Scope

- Do not implement certificate cache storage (task 20).
- Do not claim client JA3/JA4 spoofing.
- Do not implement HTTP/2 ALPN before task 16.

#### Handoff Notes

- Document TLS stack choice and limitations.

<a id="p2-03"></a>

### 03 - MITM CA Management

**Status:** done

#### Objective

Add operator-provided MITM CA configuration, optional local dev CA helpers, and the authenticated CA download endpoint.

#### Out of Scope

- Do not generate production CA keys inside Straw.
- Do not implement leaf certificate cache (task 20).
- Do not let non-admin users rotate/configure the CA.

#### Handoff Notes

- Document config keys and dev-helper limitations.

<a id="p2-04"></a>

### 04 - MITM Authenticated CONNECT Bootstrap

**Status:** done

#### Objective

Replace the current direct TLS MITM listener with an explicit-proxy CONNECT bootstrap so Control authenticates the
tenant before starting the inner TLS handshake and leaf certificate lookup/generation. Keep decoded MITM request
dispatch working, but do not add encrypted cache storage, Redis locking, or flood controls in this task.

#### Context (gap being closed)

The original P2 task 04 mixed two separate implementation surfaces: reshaping MITM ingress so tenant identity is known
before leaf generation, and adding the encrypted leaf cache. That made agents stop at the task's tenant-before-handshake
constraint instead of implementing a vertical slice.

Current Control still starts MITM as a direct TLS listener: `cmd/control/main.go:320` builds an `http.Server`,
`cmd/control/main.go:326` calls `ListenAndServeTLS`, and `cmd/control/main.go:345` installs `tls.Config.GetCertificate`
with only `hello.ServerName` available. Current MITM request authentication happens inside the HTTPS handler at
`internal/control/mitm_handler.go:19`, after the TLS certificate has already been selected. The existing raw CONNECT
path already authenticates before hijack at `internal/control/connect_handler.go:41`, validates the CONNECT authority at
`internal/control/connect_handler.go:48`, hijacks at `internal/control/connect_handler.go:62`, and has a reusable
`200 Connection Established` helper at `internal/control/connect_handler.go:148`.

Appendix C now requires tenant identity before cache keys, KMS AAD, or per-tenant flood limits are evaluated. This task
owns the runtime prerequisite: authenticate CONNECT first, then run the inner server-side TLS handshake with a
tenant-aware leaf-generation hook. The encrypted cache itself is owned by
`docs/implementation-history.md#p2-20-mitm-leaf-cert-cache.md`.

#### Out of Scope

- Do not implement encrypted leaf cache storage, Redis keys/locks, local singleflight, or flood controls; task 20 owns
  those.
- Do not change the private-key storage policy chosen in task 01.
- Do not implement MITM HTTP/2 ALPN; task 16 owns that.
- Do not implement tenant-admin CA configure/rotate APIs; task 18 owns those.

#### Handoff Notes

- Document the runtime flow from CONNECT auth to inner TLS to decoded MITM dispatch.
- Document the leaf lookup hook signature and exactly which tenant/SNI/authority values task 20 should use.
- Document whether the old direct TLS path was removed or made fail closed.

<a id="p2-05"></a>

### 05 - BodyRef Transport Selection

**Status:** done

#### Objective

Enable BodyRef transport selection using the response-body mode resolved on 2026-07-07.

#### Out of Scope

- Do not implement object storage client internals (task 06).
- Do not ship both response-body modes.
- Do not enable BodyRef in P0.

#### Handoff Notes

- Link the resolved decision and list config keys.

<a id="p2-06"></a>

### 06 - Object Storage Foundation

**Status:** done

#### Objective

Add the object storage foundation for BodyRef and payload-capture body references.

#### Out of Scope

- Do not implement request or response BodyRef flows.
- Do not implement payload capture.
- Do not allow executors to list buckets.

#### Handoff Notes

- Document provider assumptions, key prefix, retention, and credential scope.

<a id="p2-07"></a>

### 07 - BodyRef Request Body Flow

**Status:** done

#### Objective

Implement the S3 BodyRef request-body upload flow from Control to Egress.

#### Out of Scope

- Do not implement response-body BodyRef.
- Do not implement payload capture.
- Do not implement DirectStreamRef unless task 05 selected and specified it.

#### Handoff Notes

- Document object cleanup and checksum behavior.

<a id="p2-08"></a>

### 08 - BodyRef Response Body Flow

**Status:** done

#### Objective

Implement the single response-body BodyRef mode resolved on 2026-07-07.

#### Out of Scope

- Do not implement both response-body modes.
- Do not implement payload capture storage.
- Do not bypass response-size and outage tests.

#### Handoff Notes

- Link the resolved decision and list any unsupported mode.

<a id="p2-09"></a>

### 09 - Payload Capture Policy

**Status:** done

#### Objective

Add the tenant payload-capture policy schema and config API.

#### Out of Scope

- Do not implement the capture engine.
- Do not add live traffic mutation.
- Do not enable capture by default.

#### Handoff Notes

- Document the schema created from the previously deferred Section 26 row.

<a id="p2-10"></a>

### 10 - Payload Capture Engine

**Status:** done

#### Objective

Implement the non-mutating payload capture tee with storage-only redaction and bounded capture decisions.

#### Out of Scope

- Do not mutate forwarded request or response bytes.
- Do not add body regex/JSONPath redaction.
- Do not decompress bodies unless a later task explicitly owns it.

#### Handoff Notes

- Compression handling checks the "Content-Encoding" headers. If compression is detected and not allowed by CaptureOptions, the bodies are dropped.
- Body regex and JSONPath redaction are out of scope for Phase 2 baseline.

<a id="p2-11"></a>

### 11 - Payload Capture Storage

**Status:** done

#### Objective

Persist payload capture metadata in ClickHouse and store large captured bodies by object reference.

#### Out of Scope

- Do not change capture policy.
- Do not store unlimited bodies in ClickHouse.
- Do not create tenant-facing read APIs unless already owned by telemetry tasks.

#### Handoff Notes

- Document retention and object-reference cleanup behavior.

<a id="p2-12"></a>

### 12 - Egress SDK Protocol Foundation

**Status:** done

#### Objective

Create the public `sdk/egress` package foundation for custom Egress implementations: public protocol types,
registration/heartbeat helpers, assignment admission, stream subject/envelope helpers, and the pluggable `Executor`
interface derived from the official worker's existing execution seam. This task deliberately stops before rebasing the
official worker so the public API boundary can be verified first.

#### Context (gap being closed)

The 2026-07-07 decision `P2 Provider Adapter Baseline` (superseded entry in `docs/planning/32-open-decisions.md`)
dropped the Provider Adapter concept: provider integrations become custom Egress implementations built on an Egress
SDK. Current code has no public Egress SDK: `sdk/` contains only the client SDK, while worker protocol machinery is
inside unimportable `internal/egress` and `internal/natsx`. Evidence: `internal/egress/registration.go` defines
`Identity`, `Capabilities`, `BuildRegisterRequest`, `Register`, and `Heartbeat`; `internal/egress/assignment.go`
defines `Capacity` and `EvaluateAssignment`; `internal/egress/loop.go` defines the live `Worker`; `internal/natsx`
owns subject and envelope helpers. The official executor already exposes the intended seam:
`Execute(ctx, start, body, attempt, send)` in `internal/egress/executor.go`; the full extraction was too large for one
safe task, so follow-on tasks 22-24 own the rebase, enum rename, and conformance/live verification.

#### Out of Scope

- Do not rebase `cmd/egress` or `internal/egress` onto `sdk/egress`; task 22 owns that.
- Do not rename `DESTINATION_RESOLUTION_PROVIDER_ADAPTER`; task 23 owns the protobuf/doc cleanup.
- Do not add the SDK conformance/live compose verification; task 24 owns that after the rebase.
- Do not add the example custom implementation; task 13 owns that after task 24.
- No marketplace discovery, provider billing, provider account provisioning, or new execution behavior.

#### Handoff Notes

- Record the public `Executor` interface shape and the SDK helpers added.
- Record any `internal/egress` behavior intentionally left untouched for task 22.
- State that task 13 remains blocked until task 24 is complete.

<a id="p2-13"></a>

### 13 - Example Custom Egress Implementation

**Status:** done

#### Objective

Ship one example custom Egress implementation built purely on `sdk/egress` — a static-response executor that answers
every assignment with a fixed page — proving that a third party can implement an Egress node (including
provider-forwarding ones) with only the public SDK, and documenting the operator obligations that come with custom
execution.

#### Context (gap being closed)

The 2026-07-07 decision `P2 Provider Adapter Baseline` (superseded entry in `docs/planning/32-open-decisions.md`)
replaced the named static provider adapter with "one example custom Egress implementation" — vendor-neutral, since
Straw no longer names providers. `docs/planning/02-phase-boundaries.md` (P2 list) requires it. Nothing outside
`cmd/egress` implements the SDK today, so the SDK's public seam is unproven until this exists. This task depends on
task 24's conformance/live verification (`docs/implementation-history.md#p2-24-egress-sdk-conformance-and-live-verification.md`), which
proves task 12's `sdk/egress` package and task 22's official-worker rebase.

#### Out of Scope

- No named provider integration (Bright Data, Scrape.do, etc.) and no provider credentials handling.
- No marketplace, billing, or account provisioning.
- No changes to `sdk/egress` beyond what a real third-party consumer could make (i.e., none; API gaps found here
  require a new owning task or a stop).

#### Handoff Notes

- Record any friction a third-party implementer would hit with the SDK API (missing hooks, unclear types); if real,
  create an owning task for it.

<a id="p2-14"></a>

### 14 - HTTP/2 Semantics Spec

**Status:** done

#### Objective

Specify HTTP/2 semantics before any outbound or ingress HTTP/2 implementation begins.

#### Out of Scope

- Do not implement HTTP/2.
- Do not enable upstream connection reuse.
- Do not change P0 HTTP/1.1 behavior.

#### Handoff Notes

- Link the spec and name tasks 15/16 as consumers.

<a id="p2-15"></a>

### 15 - Outbound HTTP/2

**Status:** done

#### Objective

Implement outbound HTTP/2 behind an explicit tested feature flag after task 14 defines the semantics.

#### Out of Scope

- Do not enable HTTP/2 by default.
- Do not implement ingress HTTP/2 or MITM ALPN.
- Do not change HTTP/1.1 defaults.

#### Handoff Notes

- Document flag/config and downgrade behavior.

<a id="p2-16"></a>

### 16 - MITM HTTP/2 ALPN and Basic H2 Ingress

**Status:** done

#### Objective

Implement the MITM HTTP/2 ALPN gate specified by task 14: Control offers `h2` on the authenticated MITM inner TLS
handshake only when `control.http2.enabled` is true and tenant routing policy permits MITM, while preserving HTTP/1.1
compatibility and proving basic HTTP/2 MITM requests route through the normal decoded MITM handler.

Full ingress HTTP/2 cancellation, NATS-credit flow-control, trailer, and connection-level fanout semantics were split
to `docs/implementation-history.md#p2-25-ingress-http2-stream-semantics.md` after independent verification found this original task too
large for one honest vertical slice.

#### Out of Scope

- Do not implement outbound HTTP/2.
- Do not implement full ingress HTTP/2 stream cancellation, NATS-credit flow-control, trailer forwarding, or
  connection-level error fanout; these task 14 semantics are split across
  `docs/implementation-history.md#p2-25-ingress-http2-stream-semantics.md` (identity/cancellation/fanout),
  `docs/implementation-history.md#p2-29-ingress-http2-headers-and-trailers.md` (headers/trailers), and
  `docs/implementation-history.md#p2-30-ingress-http2-upload-flow-control-and-live-proof.md` (flow control/live proof).
- Do not change HTTP/1.1 ingress behavior.

#### Handoff Notes

- Document supported ingress modes and ALPN behavior.
- Include the independent verifier verdict that accepted the ALPN slice and rejected full stream semantics, with tasks
  25, 29, and 30 named as owners for the remaining scope.

<a id="p2-17"></a>

### 17 - Quota Reconciliation

**Status:** done

#### Objective

Implement billing-grade quota reconciliation, as resolved by the P2 quota accuracy decision on 2026-07-07.

#### Out of Scope

- Do not claim billing-grade accuracy without the required tests.
- Do not change P0 admission-control quota semantics without migration notes.

#### Handoff Notes

- Link the resolved decision and document accuracy limits.

<a id="p2-18"></a>

### 18 - MITM CA Configure and Rotate API

**Status:** done

#### Objective

Add the tenant_admin-only MITM CA configure/rotate API promised by the P2 MITM planning docs, without generating
production CA private keys inside Straw or exposing CA secrets.

#### Context (gap being closed)

Task 03 implemented operator-provided static CA config plus authenticated public CA download, but did not add a mutable
configure/rotate endpoint. The planning docs still require tenant admin rights for CA configure/rotate:
`docs/planning/17-mitm-design-p2.md` states that tenant admins configure and rotate the CA, and
`docs/planning/07-public-api-surface.md` states that tenant admin rights are required to rotate or configure the CA.
Current code only registers `GET /api/v1/mitm/ca.pem` (`cmd/control/main.go:1020`) and the handler only returns the
configured public cert (`internal/control/mitm_ca_handler.go:41`).

#### Out of Scope

- Do not generate production CA private keys inside Straw.
- Do not return CA private key material from any API.
- Do not change the task 01 private-key storage policy.

#### Handoff Notes

- Document the API path, request/response schema, secret redaction behavior, and any cache invalidation/versioning.

<a id="p2-19"></a>

### 19 - MITM Leaf Bundle KMS Provider

**Status:** done

#### Objective

Add the minimal KMS-compatible encryption provider boundary and runtime config that task 20 can use before any
generated MITM leaf bundle containing private-key material is written outside Control memory.

#### Context (gap being closed)

The original task 04 preflight found that it could not implement encrypted shared leaf-bundle storage without first
adding a KMS provider/config owner. The cache work is now task 20; it must store generated leaf certificate bundles
according to task 01 without changing the
resolved private-key policy. Appendix C requires generated bundles to include the public certificate chain and private
key, and requires stored bundles to be encrypted through a KMS-compatible mechanism before they leave Control memory.
Current Control config only exposes MITM enablement, port, CA files, and cert validity (`internal/config/config.go:97`),
with only a cert-validity env override (`internal/config/config.go:422`). The current MITM TLS path calls
`generateMITMLeaf` directly from `GetCertificate` (`cmd/control/main.go:332`), and `generateMITMLeaf` returns a
`tls.Certificate` containing the private key without any encrypted bundle/envelope mechanism
(`cmd/control/main.go:438`). This task owns the missing provider/config prerequisite so task 20 can stay focused on
cache, locks, TTL, and flood controls.

#### Out of Scope

- Do not implement the MITM leaf certificate cache, Redis keys/locks, local singleflight, or flood controls; task 20
  owns those.
- Do not add a cloud-provider SDK or new dependency.
- Do not add a plaintext, static deployment-key, or "dev convenience" provider that production code can use for stored
  leaf private keys.
- Do not change the private-key storage policy chosen in task 01.

#### Handoff Notes

- Document the config key names, env vars, envelope fields, AAD fields, fake-provider test behavior, and the exact
  constructor task 20 should call.

<a id="p2-20"></a>

### 20 - MITM Leaf Certificate Cache

**Status:** done

#### Objective

Implement MITM leaf certificate generation, encrypted Redis cache storage, TTL capping, miss coalescing, and generation
flood controls behind the tenant-aware certificate hook created by task 04.

#### Context (gap being closed)

The original P2 task 04 was too large: it had to both reshape MITM runtime around authenticated CONNECT and implement
the encrypted shared cache. The runtime prerequisite is now owned by
`docs/implementation-history.md#p2-04-mitm-authenticated-connect-bootstrap.md`; this task owns only the cache/storage/coalescing/flood-control
slice.

Current leaf generation is uncached and in-memory only: `cmd/control/main.go:420` generates a new RSA key,
`cmd/control/main.go:451` signs a new leaf, and `cmd/control/main.go:456` returns a `tls.Certificate` containing the
private key. Task 19 added the provider boundary and AAD type in `internal/control/mitm_leaf_bundle_crypto.go`, but no
production code writes encrypted MITM leaf bundles to Redis, coalesces same-SNI misses, or bounds unique-SNI generation.
Section 21 explicitly lists P2 MITM cert cache/locks as Redis runtime state.

#### Out of Scope

- Do not implement authenticated CONNECT bootstrap; task 04 owns it.
- Do not change the private-key storage policy chosen in task 01.
- Do not add disk or object-storage leaf caches unless a required planning doc has made that storage mandatory and
  configured; Redis is the shared cache for this task.
- Do not implement tenant-admin CA configure/rotate APIs; task 18 owns those.
- Do not implement MITM HTTP/2 ALPN; task 16 owns that.

#### Handoff Notes

- Document Redis key prefixes, value shape, TTLs, lock TTLs, concurrency limits, and unique-SNI limit settings.
- Document the AAD fields and CA identity/version source used for encrypted bundles.
- Document Redis outage/fail policy and lock-loss behavior.
- Document Postgres-backed and live verification status.

<a id="p2-21"></a>

### 21 - Object Storage Lifecycle Retention

**Status:** done

#### Objective

Enforce the Section 18 retention/lifecycle backstop that expires orphaned body objects, so objects left behind by a
Control crash (upload succeeded, explicit abort/DELETE never ran) do not accumulate indefinitely in the body bucket.

#### Out of Scope

- Do not change the request/response BodyRef upload or download flows.
- Do not change the explicit per-object DELETE/abort already implemented in tasks 07/08.

#### Handoff Notes

- Document where the lifecycle rule lives and how an operator overrides retention.

<a id="p2-22"></a>

### 22 - Egress SDK Official Worker Session Runtime Rebase

**Status:** done

#### Objective

Move the official worker's session-level runtime onto `sdk/egress`: registration request/reply, bounded registration
retry, heartbeat request/reply, session loss re-registration, ready/draining state, and `cmd/egress` construction must
run through the public SDK. The exact-session assignment loop remains temporarily delegated to `internal/egress` and is
owned by follow-on tasks 26, 27, 31, and 28.

#### Context (gap being closed)

Task 12 creates the public SDK foundation but intentionally stops before moving the live worker loop. Current code still
wires the official worker directly through `internal/egress.Run(ctx, natsConn, id, caps, executor, heartbeatInterval,
ready)` in `cmd/egress/main.go`; `internal/egress/runtime.go` owns registration, heartbeat, retry, session loss, and
ready/draining behavior; and `internal/egress.NewWorker` still owns the assignment stream loop. The original task 22
mixed session runtime, decoded stream runtime, raw tunnel/BodyRef runtime, and test migration in one slice; the user
approved splitting it on 2026-07-07. This task owns the first slice only.

#### Out of Scope

- Do not move decoded request stream framing, cancellation, or response credit; task 26 owns that.
- Do not move raw tunnel handling (task 27 owns it) or BodyRef handling (task 31 owns it).
- Do not finish migrating all runtime tests or add final conformance/live verification; tasks 28 and 24 own that.
- Do not rename `DESTINATION_RESOLUTION_PROVIDER_ADAPTER`; task 23 owns that.
- Do not add the standalone custom Egress example; task 13 owns that.
- Do not add new execution behavior; the official outbound HTTP engine remains in `internal/egress`.
- Do not change NATS wire messages, subject shapes, stream sequencing, or error semantics.

#### Handoff Notes

- Record exactly what moved to `sdk/egress` and what stayed in `internal/egress`.
- Record the `cmd/egress` wiring evidence.
- State that tasks 26, 27, 31, and 28 still own decoded stream runtime, raw tunnel runtime, BodyRef runtime, and full
  runtime test migration, and that task 24 still owns independent SDK conformance plus live compose verification.

<a id="p2-23"></a>

### 23 - Executor-Delegated Resolution Enum Rename

**Status:** done

#### Objective

Rename the provider-adapter destination-resolution enum value to executor-delegated terminology while preserving wire
number 3, reserving the old name, regenerating protobuf code, and updating validation/docs so non-generated code no
longer uses Provider Adapter naming.

#### Context (gap being closed)

The 2026-07-07 `P2 Provider Adapter Baseline` decision superseded the Provider Adapter concept, but the protobuf and
docs still expose `DESTINATION_RESOLUTION_PROVIDER_ADAPTER`. Evidence: `api/proto/straw/v1/straw.proto` defines
`DESTINATION_RESOLUTION_PROVIDER_ADAPTER = 3`; `api/proto/straw/v1/validate.go` accepts the generated constant; and
`docs/planning/27-security-controls.md` says the rename is owned by the P2 Egress SDK task. The original task 12 mixed
this compatibility-sensitive rename with the SDK extraction; this task owns it separately.

#### Out of Scope

- Do not change wire number 3 or the `DestinationPolicy.resolution_mode` field number.
- Do not change destination-policy behavior.
- Do not implement custom Egress examples or SDK runtime behavior.

#### Handoff Notes

- Record the new enum value name.
- Record Buf lint and breaking-check results.
- Record the grep used to prove stale Provider Adapter naming is gone from non-generated code except historical notes
  or the reserved proto name.

<a id="p2-24"></a>

### 24 - Egress SDK Conformance and Live Verification

**Status:** done

#### Objective

Prove the rebased Egress SDK works without implementer assumptions: add an SDK-only stub-executor conformance test and
drive one real request through the compose stack against the `cmd/egress` binary that now runs on `sdk/egress`.

#### Context (gap being closed)

Tasks 12 and 22 create and wire the SDK, but the P2 decision requires acceptance tests for "SDK-built worker protocol
conformance" and "official worker on the SDK passing the existing E2E flow" (`docs/planning/32-open-decisions.md`).
The original task 12 included both implementation and live verification, which made it too large to verify honestly.
This task owns the independent conformance/live proof after the rebase lands.

#### Out of Scope

- Do not add the example custom implementation; task 13 owns that after this proof passes.
- Do not change the SDK API except for defects that block the conformance test; if an API change is needed, keep it
  minimal and record it.
- Do not add new execution behavior to the official worker.

#### Handoff Notes

- Include the conformance verdict table: registration, heartbeat, assignment, response stream, executor error.
- Include live compose commands and result.
- State whether task 13 can proceed purely against `sdk/egress`.

<a id="p2-25"></a>

### 25 - Ingress HTTP/2 Stream Identity and Cancellation

**Status:** done

#### Objective

Complete the stream-identity and cancellation slice of the task 14 ingress HTTP/2 semantics that were split out of
task 16: each ingress HTTP/2 stream maps to exactly one Straw `request_id`, a client stream reset/disconnect maps to a
NATS `CancelFrame` for that request only, and a client HTTP/2 connection-level failure fans out cancellation to every
active in-flight stream on that connection.

#### Context (gap being closed)

Task 16 originally mixed MITM ALPN enablement with the full HTTP/2 ingress semantics from
`docs/planning/c-http2-semantics.md`. Two independent verifiers rejected marking task 16 done because the implemented
diff only proves policy-gated MITM ALPN plus basic h2 MITM requests. The original combined task 25 then bundled
identity, cancellation, flow-control, pseudo-headers, trailers, connection fanout, and live proof into one slice — too
large for one honest run. That combined task was split on 2026-07-07 into three: this task (identity + cancellation),
task 29 (headers + trailers), and task 30 (upload flow control + live proof).

Current code relies on Go's `net/http` h2 server setup in `internal/control/mitm_connect_handler.go`, while request
dispatch flows through the decoded MITM handler (`internal/control/mitm_handler.go`) and the dispatcher
(`internal/control/dispatcher.go`) without ingress-h2-specific per-stream request-id, cancellation, or
connection-fanout tests.

#### Out of Scope

- Do not implement pseudo-header normalization, unsafe colon-header handling, or trailer forwarding; task 29 owns those.
- Do not implement NATS-credit upload flow control or the live compose HTTP/2 proof; task 30 owns those.
- Do not change outbound HTTP/2 or egress downgrade behavior; task 15 owns that.
- Do not change MITM CA, leaf generation, or leaf cache behavior.
- Do not add a new tenant HTTP/2 policy surface unless the required planning docs are updated first; task 16 uses the
  existing MITM routing-policy gate.

#### Handoff Notes

- Include a per-row coverage table for the identity/cancellation/fanout rows of `docs/planning/c-http2-semantics.md`.
- State that task 29 owns pseudo-headers/trailers and task 30 owns flow control plus the live compose proof.

<a id="p2-26"></a>

### 26 - Egress SDK Decoded Stream Runtime Rebase

**Status:** done

#### Objective

Move decoded HTTP assignment handling from `internal/egress` into `sdk/egress`: exact-session assignment subscription,
subscriber flush before `AssignAck`, `RequestStart` and inline body reads, stream sequencing, cancellation, response
credit, executor error frames, and e2c publish behavior must run through the public SDK.

#### Context (gap being closed)

The original task 22 was split on 2026-07-07 because it mixed the whole worker runtime in one oversized slice. After
task 22, `sdk/egress` owns registration and heartbeat, but `internal/egress/loop.go` still owns
`NewWorker`, `Serve`, `handleAssign`, `prepareRequestStream`, decoded `runRequest`, `waitForResult`, response-credit
gating, and e2c publishing. This task owns that decoded stream protocol move. Raw tunnel hooks are separate in task 27
and BodyRef request-body hooks in task 31.

#### Out of Scope

- Do not move raw CONNECT tunnel handling; task 27 owns it.
- Do not move BodyRef request-body download hooks; task 31 owns it.
- Do not add final SDK conformance/live verification; task 24 owns that after tasks 22, 23, 26, 27, 28, and 31.
- Do not change NATS wire messages, subject shapes, stream sequencing, or error semantics.

#### Handoff Notes

- Record what decoded runtime moved to `sdk/egress` and any temporary compatibility wrappers left behind.
- State that task 27 still owns raw tunnel runtime movement and task 31 owns BodyRef request-body runtime movement.

<a id="p2-27"></a>

### 27 - Egress SDK Raw Tunnel Runtime Rebase

**Status:** done

#### Objective

Move the raw CONNECT tunnel request-stream runtime from `internal/egress` into `sdk/egress`: raw tunnel upload and
download streaming, upload/download credit for tunnel frames, and tunnel cancellation must run through the public SDK.
The official worker's actual dialer stays in `internal/egress` behind a minimal SDK dial/open interface.

#### Context (gap being closed)

The original task 22 was split on 2026-07-07 because it mixed the whole worker runtime in one oversized slice. That
split originally left raw tunnel and BodyRef runtime together in one follow-on task, which was itself too large: raw
CONNECT tunnel streaming and request-body BodyRef download/verification are independent surfaces with independent tests.
This task owns the raw tunnel move only; task 31 owns the BodyRef request-body runtime move.

Current code keeps raw tunnel behavior in `internal/egress/loop.go`: `runRawTunnel` (`loop.go:378`), the
`rawTunnelStream` type and its `run`/`handleFrame`/`handleData`/`publish` methods (`loop.go:405-496`), plus upload
credit and cancellation. Custom SDK workers cannot implement the raw tunnel protocol until these hooks are public and
internal-free.

#### Out of Scope

- Do not move BodyRef request-body download/verification hooks; task 31 owns those.
- Do not move decoded stream runtime; task 26 owns it.
- Do not add final SDK conformance/live verification; task 24 owns that after tasks 22, 23, 26, 27, 28, and 31.
- Do not change NATS wire messages, subject shapes, stream sequencing, or error semantics.

#### Handoff Notes

- Record the SDK dial/open interface and what official-worker dial behavior remains internal.
- State that task 31 still owns BodyRef request-body runtime movement.

<a id="p2-28"></a>

### 28 - Egress SDK Runtime Test Migration and Wiring Proof

**Status:** done

#### Objective

Finish the official-worker rebase proof by moving or deleting stale `internal/egress` protocol-runtime tests, leaving
`internal/egress` focused on outbound execution, and adding a command-level wiring proof that `cmd/egress` constructs
the SDK runtime with the official executor.

#### Context (gap being closed)

Tasks 22, 26, 27, and 31 move runtime behavior in slices. The original task 22 also required moving/adapting runtime tests
and proving `cmd/egress` wiring, but doing that before the runtime move is complete leaves duplicate tests and
ambiguous ownership. Current tests such as `internal/egress/loop_test.go`, `internal/egress/runtime_test.go`, and
`internal/egress/assignment_test.go` cover protocol machinery that belongs under `sdk/egress` once tasks 22, 26, 27,
and 31 are complete.

#### Out of Scope

- Do not add live compose verification; task 24 owns it.
- Do not rename Provider Adapter terminology; task 23 owns it.
- Do not add a custom Egress example; task 13 owns it after task 24.

#### Handoff Notes

- Record the test files moved, narrowed, or deleted.
- State that task 24 owns live compose verification and SDK-only conformance.

<a id="p2-29"></a>

### 29 - Ingress HTTP/2 Headers and Trailers

**Status:** done

#### Objective

Complete the header and trailer slice of the task 14 ingress HTTP/2 semantics: pseudo-headers are normalized as
specified by task 14, unsafe custom colon-prefixed headers are rejected or stripped, and HTTP/2 trailers are forwarded
or recorded following the task 14 / NATS `TrailersFrame` ordering contract.

#### Context (gap being closed)

The original combined task 25 bundled the full ingress HTTP/2 semantics into one slice that two verifiers rejected as
too large. It was split on 2026-07-07 into three: task 25 (stream identity + cancellation), this task (headers +
trailers), and task 30 (upload flow control + live proof). This task owns only the header/trailer semantics.

Current code terminates ingress h2 via Go's `net/http` h2 server in `internal/control/mitm_connect_handler.go` and
dispatches through `internal/control/mitm_handler.go`, without ingress-h2-specific pseudo-header normalization,
colon-header rejection, or trailer-ordering tests.

#### Out of Scope

- Do not implement or change stream identity, cancellation, or connection fanout; task 25 owns those.
- Do not implement NATS-credit upload flow control or the live compose HTTP/2 proof; task 30 owns those.
- Do not change outbound HTTP/2 or egress downgrade behavior; task 15 owns that.
- Do not add a new tenant HTTP/2 policy surface unless the required planning docs are updated first.

#### Handoff Notes

- Include a per-row coverage table for the pseudo-header and trailer rows of `docs/planning/c-http2-semantics.md`.
- State that task 30 owns flow control plus the live compose proof.

<a id="p2-30"></a>

### 30 - Ingress HTTP/2 Upload Flow Control and Live Proof

**Status:** done

#### Objective

Complete the final slice of the task 14 ingress HTTP/2 semantics: ingress upload flow control so exhausted NATS upload
credit applies HTTP/2 backpressure without unbounded buffering, and one live HTTP/2 MITM request driven through the
compose stack via the normal Control -> NATS -> Egress stream protocol, proving end-to-end protocol translation.

#### Context (gap being closed)

The original combined task 25 bundled the full ingress HTTP/2 semantics into one slice that two verifiers rejected as
too large. It was split on 2026-07-07 into three: task 25 (stream identity + cancellation), task 29 (headers +
trailers), and this task (upload flow control + live proof). This is the last slice; it also carries the live
protocol-translation proof for all three because that proof exercises identity, cancellation, and headers together.

Current code terminates ingress h2 via Go's `net/http` h2 server in `internal/control/mitm_connect_handler.go` and
dispatches through `internal/control/mitm_handler.go` and `internal/control/dispatcher.go`, without ingress-h2 upload
credit backpressure tests or a recorded live HTTP/2 MITM request through the normal stream path.

#### Out of Scope

- Do not re-implement stream identity, cancellation, or header/trailer semantics; tasks 25 and 29 own those.
- Do not change outbound HTTP/2 or egress downgrade behavior; task 15 owns that.
- Do not change MITM CA, leaf generation, or leaf cache behavior.
- Do not change the NATS protocol contract to make flow control work; that is a stop condition.

#### Handoff Notes

- Include the flow-control and normal-stream-translation coverage rows for `docs/planning/c-http2-semantics.md`.
- Record live compose commands and the request result.
- State whether any remaining HTTP/2 behavior is out of phase, and name the owning task if so.

<a id="p2-31"></a>

### 31 - Egress SDK BodyRef Request-Body Runtime Rebase

**Status:** done

#### Objective

Move the request-body `BodyRefFrame` runtime from `internal/egress` into `sdk/egress`: BodyRef scope validation and the
download/verification hooks (checksum, expiry, unavailable-object mapping) must run through the public SDK. The official
worker's BodyRef HTTP client stays in `internal/egress` behind a minimal SDK body-ref interface.

#### Context (gap being closed)

The original task 22 was split on 2026-07-07. Its raw-tunnel/BodyRef follow-on was itself too large, so on 2026-07-07
it was split again: task 27 owns the raw CONNECT tunnel move, and this task owns the BodyRef request-body move. The two
are independent surfaces with independent tests.

Current code splits BodyRef handling between `internal/egress/loop.go` — `acceptBodyRef` (`loop.go:674`) and its
object-key scope check invoking `downloadBodyRef` (`loop.go:685`) — and `internal/egress/executor.go` —
`downloadBodyRef` (`executor.go:455`) with checksum/expiry verification. Custom SDK workers cannot implement the
request-body protocol until these hooks are public and internal-free.

#### Out of Scope

- Do not move raw CONNECT tunnel runtime; task 27 owns it.
- Do not move decoded stream runtime; task 26 owns it.
- Do not change object-storage server behavior or lifecycle rules; tasks 06-08 and 21 own those.
- Do not add final SDK conformance/live verification; task 24 owns that after tasks 22, 23, 26, 27, 28, and 31.
- Do not change NATS wire messages, subject shapes, stream sequencing, or error semantics.

#### Handoff Notes

- Record the SDK body-ref interface and what official-worker BodyRef behavior remains internal.
- State whether task 28 can remove any temporary compatibility wrappers.

<a id="p2-32"></a>

### 32 - Python Egress SDK

**Status:** superseded

#### Objective

Add a minimal Python Egress SDK that lets a custom Python worker register with Control, heartbeat, accept one decoded
HTTP assignment, read request frames, and stream response or error frames back over the documented Core NATS protocol
without importing Straw Go internals.

#### Context (gap being closed)

This task was requested directly on 2026-07-08 to add a Python Egress SDK. Current code proves the Egress SDK exists
only in Go:

- `docs/planning/02-phase-boundaries.md:75-84` places the public Egress SDK and custom Egress implementations in P2.
- `docs/planning/05-component-boundaries.md:35-48` describes the Egress SDK as a public Go SDK with custom
  implementations behind a pluggable execution seam; no non-Go Egress SDK is specified.
- `sdk/egress/types.go:38-47` defines the Go `Executor` and tenant-aware execution seam.
- `sdk/egress/runtime.go:26-45` defines the Go NATS connection surface and registration entrypoint.
- `sdk/egress/assignment.go:30-63` defines the Go assignment worker runtime.
- `docs/implementation-history.md#p1-29-python-client-sdk.md:40-43` explicitly excludes Egress SDK/custom-worker behavior from the Python
  client SDK task.
- `find . -maxdepth 4 \( -path './.git' -o -path './tmp' \) -prune -o \( -name 'pyproject.toml' -o -name 'setup.py' -o -name '*.py' \) -print`
  currently finds no Python SDK package; only `deploy/docker/kms-mock.py` exists.

#### Out of Scope

- Do not add the Python client/request SDK; `docs/implementation-history.md#p1-29-python-client-sdk.md` owns client-side HTTP APIs.
- Do not add raw CONNECT, BodyRef, MITM, payload capture, HTTP/2-specific behavior, provider integrations, billing, or
  marketplace features.
- Do not change Control, `cmd/egress`, or the protobuf wire contract to fit Python.
- Do not publish to PyPI or add release automation.

#### Handoff Notes

- Superseded 2026-07-08: this task was split, with user approval, into
  `docs/implementation-history.md#p2-32a-python-egress-sdk-protocol-foundation.md` (protobuf generation, subjects, signing, minimal NATS
  wire client) and `docs/implementation-history.md#p2-32b-python-egress-sdk-assignment-runtime.md` (assignment runtime, streaming,
  conformance tests, usage docs), because the combined scope was sized close to the entire Go Egress SDK
  (`sdk/egress/`, ~3500 lines) and required a protobuf/NATS-dependency decision (resolved: add `protobuf` as a new
  Python dependency, generated via `grpcio-tools`' bundled `protoc`) that needed explicit user sign-off before any
  code was written. 32a and 32b are the owning tasks for all remaining work originally scoped here; this task file is
  not picked up as open work.

<a id="p2-32a"></a>

### 32a - Python Egress SDK Protocol Foundation

**Status:** done

#### Objective

Give the Python Egress SDK a wire-compatible protocol layer: generated Python protobuf bindings for
`api/proto/straw/v1/straw.proto`, subject construction and safe-token validation for every canonical NATS subject,
`Envelope` construction/validation by payload type, registration-request signing, heartbeat envelope construction, and
the smallest Core NATS wire client the SDK needs — all without touching the protobuf/NATS wire contract or Go/Control
runtime code. No assignment runtime is built in this task; it produces the foundation task 32b's runtime is built on.

#### Context (gap being closed)

Task 32 ("Python Egress SDK") was split on 2026-07-08 after user approval because it was sized close to the entire Go
Egress SDK (`sdk/egress/`, ~3500 lines across `types.go`, `runtime.go`, `assignment.go`, `stream.go`, `bodyref.go`, plus
conformance tests) and this environment has neither `protoc`/`buf` nor an approved Python NATS client dependency. The
user approved adding `protobuf` as a new Python dependency (generated via `grpcio-tools`' bundled `protoc`, mirroring
how Go generates `.pb.go` from the same source `.proto` — `grpcio-tools` itself is a codegen-time tool, not necessarily
a shipped runtime dependency) instead of hand-rolling protobuf wire encoding.

Current code proves no Python protocol layer exists:

- `python/pyproject.toml:6` declares `dependencies = []`; `python/straw/client.py` (P1 task 29) is a stdlib-only REST
  client and explicitly excludes Egress SDK/custom-worker behavior
  (`docs/implementation-history.md#p1-29-python-client-sdk.md` Out of Scope: "Do not add Egress SDK, custom worker, BodyRef, MITM,
  payload-capture, or telemetry API clients").
- `find . -maxdepth 4 -name '*.proto' -o -name 'straw_pb2.py'` finds `api/proto/straw/v1/straw.proto` but no generated
  Python bindings anywhere in the repo.
- `sdk/egress/types.go:38-47` (public `Executor`/tenant-aware seam) and `api/proto/straw/v1/registration_sign.go`
  (registration signing) define the Go-side contract this task must be wire-compatible with; no Python equivalent
  exists.
- `buf.gen.yaml` only generates Go (`local: protoc-gen-go`, `out: api/proto`); no Python plugin is configured.

#### Out of Scope

- Do not implement the assignment worker runtime, streaming loop, or executor callable invocation — task 32b owns
  that.
- Do not add the Python client/request SDK; `docs/implementation-history.md#p1-29-python-client-sdk.md` owns client-side HTTP APIs.
- Do not add raw CONNECT, BodyRef, MITM, payload capture, HTTP/2-specific behavior, provider integrations, billing, or
  marketplace features.
- Do not change Control, `cmd/egress`, the protobuf schema, or any wire contract to fit Python.
- Do not publish to PyPI or add release automation.
- Do not add `grpcio` (the RPC runtime) as a shipped dependency; only `protobuf` (the generated-message runtime) and,
  if needed for codegen, `grpcio-tools` as a dev-only tool are in scope.

#### Handoff Notes

- Protobuf generation command (reproducible; verified byte-identical on re-run):
  `python -m grpc_tools.protoc -I api/proto --python_out=python/straw/proto --pyi_out=python/straw/proto api/proto/straw/v1/straw.proto`
  (via `grpcio-tools`, a dev-time-only codegen tool — not a shipped runtime dependency).
- NATS wire client: hand-rolled (`python/straw/egress/natsclient.py`), not a dependency. No NATS Python client was
  approved and the zero-dependency baseline in `python/pyproject.toml` made hand-rolling the smallest usable option;
  implements CONNECT handshake, PUB, SUB, UNSUB, flush (PING/PONG), and MSG parsing over one blocking TCP socket.
- Registration signing: pure-Python Ed25519 (`python/straw/egress/_ed25519.py`, RFC 8032 reference algorithm) because
  no crypto dependency (e.g. `cryptography`) was approved either. Cross-validated during development against the
  `cryptography` package's OpenSSL-backed Ed25519 (byte-identical output on 20+ random vectors), and the fixed
  regression fixture in `python/tests/test_egress_ed25519.py` was itself generated via `cryptography` so the test
  checks against an independent implementation, not only self-consistency.
- `registration_signing_payload()` in `protocol.py` reproduces `api/proto/straw/v1/registration_sign.go`'s
  `RegistrationSigningPayload` byte-for-byte (domain prefix, `worker_id\n`, `credential_id\n`, `executor_type\n`,
  `major.minor\n`, `len(nonce):nonce\n`, `issued_at_unix_ms`).
- No incompatibility found between the Go SDK contract and this Python implementation.
- The assignment runtime (registration/heartbeat loop, stream frame handling, executor invocation) is deferred to
  task 32b (`docs/implementation-history.md#p2-32b-python-egress-sdk-assignment-runtime.md`), which is already the owning task — not an
  unowned deferral.

<a id="p2-32b"></a>

### 32b - Python Egress SDK Assignment Runtime

**Status:** done

#### Objective

Give the Python Egress SDK a working assignment runtime built on task 32a's protocol layer: a registration/heartbeat
loop, correct subscription-before-ack assignment ordering, decoded HTTP request-frame reading, an executor callable
seam, sequenced streamed response/error publishing without buffering the full response, and credit-based backpressure
— proven by conformance tests against a fake/local NATS wire harness. Ship usage docs for the completed Python Egress
SDK (protocol + runtime) and retire task 32 as superseded.

#### Context (gap being closed)

Task 32 ("Python Egress SDK") was split on 2026-07-08 after user approval into 32a (protocol foundation: generated
protobuf bindings, subjects, signing, minimal NATS wire client) and 32b (this task: the assignment runtime built on
that foundation), because the combined scope was sized close to the entire Go Egress SDK (`sdk/egress/`, ~3500 lines).
This task depends on 32a's protocol layer existing; before 32a lands, no Python code can construct a valid `Envelope`,
subject, or signed registration request to build a runtime on top of.

The Go reference contract this task must be behaviorally compatible with:

- `sdk/egress/runtime.go:26-45` — NATS connection surface and registration entrypoint.
- `sdk/egress/assignment.go:30-63` (and the full file, ~816 lines) — assignment worker runtime: subscription
  ordering, frame reading, executor invocation, response streaming.
- `sdk/egress/stream.go` — stream frame sequencing helpers.
- `sdk/egress/assignment_test.go` and `sdk/egress/conformance_wire_test.go` — the behavioral proofs (subscription-
  before-ack ordering, no full-response buffering, executor-error-to-ErrorFrame mapping) this task's Python tests
  must reproduce in Python.

#### Out of Scope

- Do not add the Python client/request SDK; `docs/implementation-history.md#p1-29-python-client-sdk.md` owns client-side HTTP APIs.
- Do not add raw CONNECT, BodyRef, MITM, payload capture, HTTP/2-specific behavior, provider integrations, billing, or
  marketplace features.
- Do not change Control, `cmd/egress`, or the protobuf/NATS wire contract to fit Python.
- Do not publish to PyPI or add release automation.
- Do not modify task 32a's protocol layer beyond what is strictly needed to fix a genuine defect found while building
  the runtime on top of it (record any such fix in the handoff).

#### Handoff Notes

- Executor callable shape: `Callable[[DecodedRequest], DecodedResponse]`. `DecodedRequest` carries
  `method`/`url`/`headers`/`body`/`attempt`; `DecodedResponse` carries `status`/`headers` plus `body: Iterable[bytes]`
  — a generator streams without buffering the full response since the runtime pulls and publishes one chunk at a
  time (`python/straw/egress/runtime.py:433-442`). Raising from the callable or while iterating `body` is caught and
  mapped to an `ErrorFrame` (`ERROR_CODE_EXECUTOR_INTERNAL_ERROR`) instead of an `EndFrame`.
- Credit/backpressure: `_CreditGate` (`runtime.py:483-520`) tracks the current e2c byte grant; when exhausted it
  blocks reading the request's `c2e` subscription for a `CreditFrame` (or a `CancelFrame`, which aborts), applying
  the same `_StreamValidator` used for the request body so stream_seq/attempt/offset rules are shared across both
  reads on `c2e`.
- Test command: `python3 -m unittest discover python/tests` (also `python3 -m unittest tests.test_egress_runtime -v`
  from `python/` for just the new suite). Ran 3x in a row with no flakiness after fixing two test-harness races (see
  Remaining Work notes below — both in test code, not runtime.py).
- Go/Python incompatibility found: none in the wire contract. The intentional behavioral narrowing vs.
  `sdk/egress/assignment.go` is scope, not a defect: decoded HTTP only (no raw tunnel/BodyRef), and one assignment
  served at a time per session instead of the Go SDK's per-session concurrency — documented as a `ponytail:` comment
  at the top of `runtime.py` and in `python/README.md`'s Egress SDK section, with the upgrade path named (a
  per-request worker pool, or use the Go SDK for concurrent/other-mode workers).
- Task 32's Status/Handoff Notes were already `superseded` naming 32a/32b before this run (set during 32a); no change
  needed this run — confirmed unchanged in the diff.

## Retained testing-matrix audit

## P0 Testing Matrix Audit

This maps every row of `docs/planning/30-testing-matrix.md` to the tests that cover it, per task
`docs/implementation-history.md#p0-25-p0-test-matrix-and-compose.md` acceptance criterion 1. Run all with `go test ./...`.
Rows that are tool-gated or not applicable to the P0 functional suite are marked and justified.

| Matrix area | Representative tests | Notes |
|-------------|----------------------|-------|
| Protobuf | `TestStreamFrameBodyRefCompiles`, `TestAssignRequestCreditFieldsExist`, `TestValidateRejectsUnknownEnums` (api/proto/straw/v1) | `buf lint` / `buf breaking` are tool-gated: run via the `buf` CLI against `buf.yaml`/`buf.gen.yaml`; not part of `go test`. |
| NATS subjects | `TestSubjects`, `TestValidateSubjectToken`, `TestValidateMaxPayload`, `TestValidateServers`, `TestConnectAndVerifyMaxPayload` (natsx) | Exact assignment subject + safe token + max-payload validation. No pool queue-group dispatch: assignment subject is per worker/session. |
| Registration | `TestRegisterValid`, `TestRegisterRejections`, `TestRegisterInvalidCredentialKey`, `TestRegisterCapabilityOutOfScope`, `TestRegisterRevokedCredential`, `TestRegisterDuplicateSessionReplacement` | Invalid signature = `TestRegisterInvalidCredentialKey`. |
| Heartbeat | `TestHeartbeatHealthStates`, `TestHeartbeatUnavailableThenDead`, `TestHeartbeatStaleSessionIgnored` | 15s unavailable / 30s dead thresholds. |
| Worker state | `TestGlobalDisablePrecedence`, `TestTenantDisableIsolation`, `TestDrainingExclusion`, `TestCooldownExcludesThenRecovers`, `TestCooldownWindowExpiry`, `TestRegisterDuplicateSessionReplacement` | |
| Routing | `TestRoutingPriorityOrder`, `TestRoutingHardClientHints`, `TestRoutingTenantIsolation`, `TestRoutingDegradedPoolPolicy`, `TestRoutingNoMatch`, `TestRoutingStickySuccess`, `TestRoutingStickyFailure`, `TestRoutingStickyFallback` | |
| Assignment | `TestAssignmentLifecycle`, `TestWorkerRejectsAssignmentAtCapacity`, `TestDrainingExclusion`, `TestDispatcherAssignmentTimeout`, `TestAssignmentPreStartFailuresAllowFallback` | Ack timeout = `TestDispatcherAssignmentTimeout`; no-duplicate-retry = fallback boundary tests. |
| Streaming | `TestStreamValidatorRules`, `TestStreamValidatorCreditOffsetAndIdle`, `TestDispatcherStreamProtocolError`, `TestWorkerCreditExhaustionAbortsWithoutPublishing` | Sequence gap = `TestDispatcherStreamProtocolError`. |
| Terminal | `TestAssignmentLifecycle`, `TestAssignmentFallbackBoundaryAndAdminCancel`, `TestHeartbeatUnavailableThenDead` | Worker-death path: registry marks `RuntimeDead` (`worker_registry.go`), surfaced as a terminal dispatch error; exercised via the dead-state and assignment-timeout tests. |
| Cancellation | `TestDispatcherCancellation`, `TestDispatcherAdminCancelEndToEnd`, `TestDispatcherAdminCancelForeignTenantRejected`, `TestCancelRequestSystemAdminCancelsAnyRequest`, `TestCancelRequestTenantAdminAndOperatorCancelOwnTenant`, `TestCancelRequestForeignTenantInsufficientPermissionsNoDisclosure`, `TestCancelRequestUnknownRequestTenantScopeSameAsForeign`, `TestWorkerCancelFrameDuringExecutionProducesCancelledFrame`, `TestExecutorEnforcesTotalDeadline` | Client disconnect = `TestDispatcherCancellation`; admin cancel end-to-end (registry -> ctx cancel -> `cancelled` outcome + `CancelFrame`) = `TestDispatcherAdminCancelEndToEnd`; foreign-tenant/unknown-request-id rejection via the live `POST /api/v1/admin/requests/{request_id}/cancel` handler = the `TestCancelRequest*` rows in `internal/control/request_admin_handlers_test.go`, not only the `AuthorizeAdminCancel` predicate. |
| Fallback | `TestAssignmentPreStartFailuresAllowFallback`, `TestAssignmentFallbackBoundaryAndAdminCancel` | No fallback after `RequestStart` unless replayable. |
| Error mapping | `TestErrorRegistryComplete`, `TestErrorRegistryCoversEveryProtoCode`, `TestOriginStatusPassthroughIsNotErrorResponse`, `TestValidateExecutorErrorMapsOutOfSetCodes`, `TestExecutorEmittableSetMatchesContract` | Out-of-set ErrorFrame -> `executor_internal_error` = `TestValidateExecutorErrorMapsOutOfSetCodes`. |
| REST schema | `TestHandlerValidRequest`, `TestValidateRequestEmptyHostRejected`, `TestHandlerHostHeaderRejected`, `TestHandlerDuplicateHeaders`, `TestHandlerBodyLimitExceeded`, `TestHandlerCONNECTRejected` | |
| REST outcome | `TestOriginStatusPassthroughIsNotErrorResponse`, `TestHandlerSuccessEnvelopeStructure`, `TestDispatcherControlNATSEgressRoundTrip` | Upstream status returned in envelope proven end-to-end by the round-trip test (upstream 418 -> API 200). |
| Body limits | `TestHandlerBodyLimitExceeded` (request), `TestDispatcherResponseBodyTooLarge` (response, with direction) | |
| P0 exclusions | `TestValidateRequestCaptureHintOtherThanNone`, `TestHandlerCaptureHintRejected`, `TestHandlerUnknownFieldsRejected`, `TestValidateRequestBodyRefRejected`, `TestStreamValidatorRejectsP0BodyRef` | |
| Rate limits | `TestRateLimiterDimensionsAreIndependent`, `TestRateLimiterDeniesOverLimitWithRetryAfter`, `TestRateLimiterRedisFailurePolicy`, `TestRateLimiterMemoryGuardrailFallback`, `TestRateLimitCeilingRejectsExceedingValues` | |
| Quotas | `TestQuotaAdmissionRequestCount`, `TestQuotaAdmissionBandwidthAccounting`, `TestQuotaAdmissionRedisFailurePolicy`, `TestQuotaAdmissionNotBillingGrade`, `TestQuotaKeysHaveTTL` | |
| Deny rules | `TestResolveDestinationPolicy_HostDenyNormalization`, `TestResolveDestinationPolicy_CIDRAllowOverridesPrivateDefault`, `TestResolveDestinationPolicy_PrivateRangeDefaultDenied`, `TestResolveDestinationPolicy_MetadataIPDefaultDenied`, `TestExecutorBlocksDNSRebindingByDialingValidatedIP`, `TestResolveDestinationPolicy_NonASCIIHostRejected` | Redirect-target deny is a P1 future test (no redirect following in P0: `TestExecutorDoesNotFollowRedirects`). |
| Egress policy | `TestResolveDestinationPolicy_*` (RequestStart carries DestinationPolicy), `TestExecutorRejectsResolvedDeniedIPAndRedactsDetails` | Egress enforces resolved-IP deny without querying Control DBs. |
| HTTP behavior | `TestP0TransportDefaults`, `TestExecutorDoesNotFollowRedirects`, `TestHandlerCONNECTRejected` | CONNECT / HTTP-2 / keep-alive disabled in P0. |
| Redaction | `TestHandlerURLUserInfoRejected`, `TestSanitizeTargetURLDropsQuery`, `TestRedactSensitiveHeaderValue`, `TestBuildRequestEventRecordsActorAndSanitizedTarget` | |
| Config API | `TestConfigCacheSaveVersionConflict`, `TestRateLimitConfigVersionConflict`, `TestRoutingRuleCRUDAndRBAC`, `TestPostgresSaveTenantSnapshotOptimisticVersioning` | Config/admin path separation = route table in `TestRoutingRuleCRUDAndRBAC`. |
| Invalidation | `TestRedisInvalidationPublishSubscribe`, `TestConfigCacheMissedPubSubRecovery`, `TestConfigCachePollAllTenantsRecoversMissedInvalidation`, `TestConfigCacheAPIKeyRevocationInvalidation`, `TestRevokeTenantAPIKeyInvalidatesConfigCache` | |
| ClickHouse | `TestRequestMetadataWriterFlushSuccess`, `TestRequestMetadataWriterOutageKeepsQueuedEvents`, `TestRequestMetadataWriterDropsOldestWhenFull`, `TestSanitizeTargetURLDropsQuery` | Async write, outage, bounded-queue drop, sanitized target_url. Binary wiring: `wireClickHouse` in `cmd/control/main.go`. |
| Load | — | **Not applicable to the P0 functional suite.** p50/p99 routing latency and load benchmarks require a load harness; deferred to P1 observability/load work, no automated `go test` row in P0. |
| Auth | `TestPlatformAPIKeyLifecycle`, `TestPlatformKeyCannotExecuteDataPlaneRequest`, `TestTenantKeyCannotCreateTenants`, `TestAuthenticateRejectsRevokedKey`, `TestQuotaWriteRequiresPlatformKey`, `TestWorkerCredentialCreateRejectsForeignTenantScope` | |
| Audit | `TestActorAuditSourceRecorded`, `TestPostgresAuditStoreActorRecords`, `TestPostgresConfigStoreRedactsInjectionPolicyAudit`, `TestHashAPIKeySecretNeverEqualsPlaintext` | |
| Identifiers | `TestPostgresTenantStoreDuplicateRejected`, `TestWorkerCredentialCreateForcesCallerTenantScope`, `TestRegisterCapabilityOutOfScope` | Multi-tenant pool scope validated via credential tenant-scope enforcement. |
| HTTP validation | `TestValidateRequestInvalidHeaderName`, `TestValidateRequestCRInHeaderValue`, `TestHandlerURLFragmentRejected`, `TestResolveDestinationPolicy_NonASCIIHostRejected` | |
| Injection auth | `TestInjectionPolicySafetyRules`, `TestResolveDestinationPolicy_InjectionDeniedHeaderRejected` | Operator cannot set Authorization/Cookie injection; tenant_admin audited sensitive policy. |
| Worker admin | `TestTenantWorkerActionAffectsOnlyThatTenant`, `TestGlobalWorkerActionRequiresSystemAdmin`, `TestTenantWorkerListOmitsOtherTenants`, `TestListWorkersPlatformSeesSessionTenantDoesNot` | Foreign-tenant request cancel reject = `TestCancelRequestForeignTenantInsufficientPermissionsNoDisclosure` and `TestDispatcherAdminCancelForeignTenantRejected` against the live `POST /api/v1/admin/requests/{request_id}/cancel` endpoint, not only the `AuthorizeAdminCancel` predicate. |
| NATS ordering | `TestNATSOrderingRequiresFlushedStreamSubscription` | |
| SSRF | `TestExecutorBlocksDNSRebindingByDialingValidatedIP`, `TestExecutorDeniesPrivateAndMetadataIPsByDefault` | |
| Timeout | `TestExecutorEnforcesTotalDeadline`, `TestAckDeadlineUsesEarlierClock` | Total deadline wins over phase timeout = `TestAckDeadlineUsesEarlierClock`. |

### Outage rows (docs/planning/29)

| Outage | Test |
|--------|------|
| Redis unavailable (fail policy) | `TestRateLimiterRedisFailurePolicy`, `TestQuotaAdmissionRedisFailurePolicy`, `TestRedisStickyStoreDegradesOnRedisFailure`, `TestOpenRedisUnreachableStillReturnsClient` (Control still starts) |
| NATS unavailable (`transport_unavailable`) | `TestDispatcherNATSUnavailable` |
| ClickHouse unavailable (bounded buffer, drop oldest) | `TestRequestMetadataWriterOutageKeepsQueuedEvents`, `TestRequestMetadataWriterDropsOldestWhenFull` |
| Postgres unavailable (cached snapshots) | `TestConfigCacheSnapshotHit` (serves from cache without a store round-trip) |

### Full vertical slice

`TestDispatcherControlNATSEgressRoundTrip` drives a validated REST request through the Control dispatcher, over
NATS (assignment + request/response streams), into a live egress worker that executes against a real upstream, and
back — asserting the upstream status and body are returned in the response envelope. Liveness/readiness probes are
covered by `TestHealthzAlwaysOK` and `TestReadyzReflectsReadiness` (cmd/control).

### Not claimed

No P1/P2 rows (proxy, CONNECT tunnelling, MITM, BodyRef, payload capture, provider adapter, telemetry read APIs,
connection pooling, HTTP/2) are implemented or claimed as tested.
