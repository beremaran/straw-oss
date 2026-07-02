# Straw Proxy Plan

## 1. Purpose

Straw is a distributed HTTP/HTTPS proxy system for high-scale web scraping. It centralizes proxy usage behind a single
entrypoint while letting operators combine their own egress endpoints with third-party proxy providers.

Straw solves the operational problem of setting up endpoints, choosing where requests should egress, and managing
routing rules across many proxy sources. It provides browser fingerprint simulation, configurable routing, vendor
aggregation, and both plain CONNECT tunneling and MITM-based HTTPS handling.

MITM interception is part of the core purpose, but not for surveillance or general man-in-the-middle use. Straw uses it
to decode HTTPS requests into a simpler internal request/response message flow between Control and Egress, improving
reliability while still allowing raw CONNECT tunnels when needed.

Straw is not an anonymity tool and not a browser automation platform. Its job is to pass requests through the configured
route to the desired egress endpoint.
## 2. Goals

Concrete capabilities the system must provide:

- Provide four client entrypoints through Control:
    - REST API for explicit request/response transport operations.
    - HTTP forward proxy for plain HTTP requests.
    - CONNECT tunnel proxy for raw HTTPS TCP tunnels.
    - MITM HTTPS proxy for decoded request/response handling.
- Route traffic through operator-owned Egress workers and operator-configured upstream proxies or vendors.
- Select routes by tags, country, region, IP type, sticky session affinity, fallback rules, and upstream proxy or worker
  availability.
- Support browser fingerprint simulation for outbound Egress requests, selectable by profile.
- Support configured HTTP header/cookie injection based on tags and target domain.
- Authenticate clients with API keys, authenticate workers, restrict management actions to admins, and preserve tenant
  isolation.
- Preserve HTTP behavior needed for scraping workloads, including streaming uploads/downloads, client-disconnect
  cancellation, stable error codes, and clear timeout mapping.
- Allow optional payload capture globally or by tags when explicitly enabled by the operator.
- Store durable configuration and state in Postgres, and use Redis only for ephemeral state such as sessions, rate
  limits, queues, or short-lived routing data.
- Expose structured logs, metrics, tracing, propagated request IDs, health checks, and readiness checks.
- Include Phase 1 client SDKs, CLI, and UI for operating and using Straw.
- Ship one official Egress implementation written in Go.
- Run locally with docker-compose and support production operation with horizontally scaled Control instances, regional
  Egress pools, graceful worker draining, and graceful shutdown.
- Enforce abuse and overload controls, including rate limits, quotas, destination deny rules, and payload/header
  redaction.
- Use stable versioned Control/Egress contracts with protobuf messages, typed errors, request correlation,
  transport-level retry semantics, timeouts, and backward-compatible rolling deploys, so custom Egress implementations
  can be built from day one.
- Avoid fixed architectural limits on request concurrency or request rate; practical limits should come from configured
  capacity, worker pools, queues, quotas, and infrastructure.
- Support unlimited request and response body sizes by design, with configurable deployment limits.
- Make request timeouts configurable, with expected defaults in the 30-60 second range.
- Keep Control-side routing and coordination overhead under 500 ms, excluding actual outbound request execution time on
  Egress workers.
- Include runnable checks for routing, config, parsing, protobuf compatibility, NATS request/reply, worker registration,
  REST/proxy/CONNECT/MITM flows, worker loss, NATS outage, timeout paths, backpressure, and load behavior.

## 3. Non-Goals

Things intentionally outside scope so future decisions do not drift.

- Straw is not a scraping orchestrator. It does not provide crawler scheduling, browser orchestration, CAPTCHA solving,
  content extraction/parsing, scraping retry policies, batch execution, persistent request queues, replay workflows,
  or "run this later" job APIs. Requests come in, are transported, and finish.
- Phase 1 is limited to HTTP and HTTPS transport. WebSockets, SOCKS5, generic TCP, UDP/QUIC, and non-web protocols are
  future work.
- Straw does not guarantee anonymity, identity hiding, attribution protection, residential IP procurement, or any
  privacy-network behavior.
- Phase 1 is not a managed SaaS product. Billing, payments, self-serve signup, customer dashboards, public marketplaces,
  and hosted multi-customer business workflows are reserved for later monetization work.
- Phase 1 does not include marketplace/vendor integrations, automatic third-party proxy account provisioning, provider
  billing reconciliation, or marketplace routing economics. It may still chain to operator-configured upstream proxies
  or vendors.
- Straw does not provide compliance or legal enforcement features such as jurisdiction policy engines, consent
  management, legal review workflows, or automated `robots.txt` enforcement. Operators are responsible for lawful use.
- Straw does not make content-aware scraping decisions. It does not perform semantic page understanding, response
  classification, automatic login/session workflows, or smart behavior based on response bodies. Content is processed
  only as needed to stream, buffer, and transport it.
- Traffic payload capture and storage are not the default behavior. Operators may explicitly enable payload capture
  globally or by tags for debugging or auditing, subject to their own policy and responsibility.
- Browser fingerprint simulation is best effort. Straw does not guarantee undetectability, CAPTCHA avoidance, WAF
  bypass, anti-bot bypass, or successful access to any target site.
- Straw is not intended to run as an unauthenticated public open proxy. Clients, workers, and management actions must be
  authenticated.
- Straw does not provide general traffic tampering, script injection, credential harvesting, surveillance, or
  content-filtering features. The only planned mutation is configured HTTP header/cookie injection based on tags and
  target domain.
- Straw exposes health and readiness endpoints, but does not provide built-in global failover, managed disaster
  recovery, zero-downtime guarantees, or exactly-once request execution. Availability beyond documented deployment
  patterns is the operator's responsibility.
- Phase 1 includes client SDKs, CLI, and UI, but only one official Egress implementation is planned: the Go worker.
  Other platforms and languages can implement the Control/Egress protocol, but are not promised as first-party workers.
- Phase 1 does not include a plugin system, embedded scripting, worker marketplace, or runtime module loader inside the
  official Egress worker.

## 4. System Overview

Straw is control-centered. SDKs, CLI, UI, REST clients, HTTP proxy clients, CONNECT clients, and MITM proxy clients all
enter through Control. Control is the only public-facing runtime service: it owns client ingress, admin/config APIs,
authentication, authorization, tenant isolation, routing policy, and coordination.

Control is horizontally scalable and mostly stateless. Durable tenant configuration lives in Postgres, while operational
telemetry lives in ClickHouse. Ephemeral state such as sticky sessions, rate limits, queues, worker/adapter
availability,
and load snapshots lives in Redis. Only Control connects to Postgres, ClickHouse, and Redis; Egress workers and Provider
Adapters communicate through NATS, plus configured large-body transport when needed.

```mermaid
flowchart LR
  subgraph Clients["Clients"]
    SDK["SDKs"]
    CLI["CLI"]
    UI["UI"]
    REST["REST clients"]
    Proxy["HTTP / CONNECT / MITM proxy clients"]
  end

  Control["Control\npublic ingress, auth, routing, config"]
  Postgres["Postgres\ndurable tenant/config state"]
  ClickHouse["ClickHouse\noperational data and analytics"]
  Redis["Redis\nephemeral sessions, limits, availability, load"]
  NATS["NATS\nrequest/reply, registration, heartbeat"]

  subgraph Executors["Internal executors"]
    Egress["Egress Workers\nlong-lived registered workers"]
    Adapter["Provider Adapters\noptional direct vendor/upstream execution"]
  end

  Body["Large-body transport\nobject storage or direct stream"]
  Targets["Target sites"]
  Vendors["Vendor / upstream proxy endpoints"]

  Clients --> Control
  Control <--> Postgres
  Control <--> ClickHouse
  Control <--> Redis
  Control <--> NATS
  NATS <--> Egress
  NATS <--> Adapter

  Control -. configurable large bodies .- Body
  Egress -. configurable large bodies .- Body
  Adapter -. configurable large bodies .- Body

  Egress --> Targets
  Adapter --> Vendors
  Adapter --> Targets

```

The control/config plane runs through Control. Admins and automation use the UI, CLI, SDKs, or REST APIs to manage
tenants, API keys, routing rules, upstream/vendor configuration, fingerprint profiles, injection policies, worker
credentials, quotas, and payload capture policy. Control persists durable configuration in Postgres, operational data in
ClickHouse, and uses Redis for shared ephemeral coordination.

The request/data plane also starts at Control. REST, plain HTTP proxy, and MITM HTTPS requests are decoded into a shared
internal HTTP request/response model. MITM is the default HTTPS proxy behavior. Raw CONNECT remains an explicit separate
tunnel path because it forwards bytes rather than decoded HTTP messages. MITM clients must either trust Straw's CA or
explicitly disable certificate verification.

Routing uses one unified route model. A route can target an Egress worker pool or a Provider Adapter pool. Egress
workers are long-lived registered workers that execute traffic from operator-controlled network locations. Provider
Adapters are first-class but optional runtime components used when a route should hit a vendor or upstream provider
directly instead of wasting bandwidth through an Egress worker. Control chooses the adapter pool/class; the adapter
chooses the exact vendor endpoint, account, or upstream target using its provider-specific logic.

Control dispatches work over NATS using versioned protobuf contracts, request/reply correlation, typed errors, timeouts,
registration, heartbeat, and load reporting. Egress workers and Provider Adapters are worker-like NATS participants:
they register capabilities, report health/load, receive assigned work, and return responses or typed failures.
Downstream executors should verify signed or otherwise authorized dispatch messages when that can be done with minimal
overhead; otherwise they trust Control as the authorization boundary for Phase 1.

NATS is the default body path. For bodies that exceed configured message limits or deployment policy, Straw supports
configurable large-body handling through object storage references or a direct streaming channel. Egress workers and
Provider Adapters may connect to that configured body transport, but they must not connect to Postgres, ClickHouse, or
Redis.

Outbound request mutation is split by responsibility. Control resolves tenant policy, routing metadata, fingerprint
profile selection, and header/cookie injection rules. The executing component, either Egress or Provider Adapter,
applies transport-level browser fingerprint behavior and final outbound header/cookie changes because it owns the actual
outbound connection.

## 5. Components

Responsibilities of each major service.

- Control Server: the single public-facing deployable service. It owns REST, admin/config APIs, HTTP forward proxy,
  CONNECT tunnel proxy, MITM proxy, client authentication, admin authorization, tenant isolation, routing decisions,
  request cancellation, and NATS dispatch to executors. Control is the only component that reads and writes Postgres,
  ClickHouse, and Redis.
- Egress Worker: the official Go executor for operator-owned egress locations. It registers with Control over NATS,
  reports capabilities, health, and load, executes assigned outbound HTTP/HTTPS requests, applies browser fingerprint
  behavior and final header/cookie injection, supports proxy chaining when configured, and streams request/response
  bodies through NATS or the configured large-body transport.
- Provider Adapter: a Phase 1 executor for routes that should go directly through an upstream proxy or vendor instead of
  an Egress worker. It participates in NATS like a worker, reports capabilities, health, and load, receives assigned
  work from Control, and owns provider-specific endpoint, account, and upstream selection.
- NATS: internal message transport between Control, Egress Workers, and Provider Adapters. It owns request/reply
  correlation, registration, heartbeats, queue groups, transport-level timeouts, typed failures, backpressure signaling,
  and queue-related behavior. NATS is also the default body path when bodies fit configured message limits.
- Large-Body Transport: configured data path for request or response bodies that should not travel inside NATS messages.
  Phase 1 supports S3-compatible object storage references and direct streaming channels. Control, Egress Workers, and
  Provider Adapters may use it; it does not own routing, auth, or durable metadata.
- Redis: shared ephemeral runtime state. It owns sticky session affinity, rate limiting counters, worker and adapter
  availability snapshots, backpressure/load state, and in-flight request state with TTLs. It does not own durable
  configuration or queue semantics.
- Postgres: durable system and tenant state. It owns tenants, users, API keys, worker credentials, routing rules,
  upstream/vendor configuration, fingerprint profiles, header/cookie injection policies, and quotas.
- ClickHouse: high-volume operational data. It owns request metadata, audit logs, timing metrics, and payload capture
  metadata.

Observability is not a separate deployable component. Each service emits its own structured logs, metrics, traces,
propagated request IDs, health checks, and readiness checks.

## 6. Client Interfaces

How clients talk to Control.

Control exposes one public interface family with four transport entrypoints:
versioned REST APIs, plain HTTP forward proxying, default MITM HTTPS proxying,
and explicit raw CONNECT tunneling. All interfaces authenticate to the same
tenant/client identity model. REST clients use `Authorization: Bearer ...`;
proxy clients use `Proxy-Authorization`. Admin and configuration operations
require admin-scoped credentials.

The REST API is versioned under `/v1` and is split by purpose:

- Request Transport API: synchronous request/response execution. A submitted
  request blocks or streams until the upstream response, timeout, cancellation,
  or typed failure returns. Phase 1 does not expose async job IDs, polling,
  replay, or persistent request queues.
- Admin and Config APIs: tenant, API key, route, upstream/vendor, fingerprint,
  injection policy, worker credential, quota, and payload-capture management.
  These APIs are separate from request transport paths, such as `/v1/admin/*`
  or `/v1/config/*`, and require admin authorization.

REST request metadata is supplied as structured JSON fields. This includes
routing tags, country, region, IP type, sticky session ID, fingerprint profile,
timeouts, payload capture hints, and any other per-request routing controls.
REST supports arbitrary request and response bodies: small bodies may be inline,
while large uploads and downloads must stream or use configured body references
so REST is not limited to JSON payload sizes.

The HTTP proxy interface follows standard forward proxy semantics for plain
HTTP. Clients send absolute-form requests such as
`GET http://example.com/path HTTP/1.1`, Control authenticates the client,
extracts routing metadata from `X-Straw-*` headers, dispatches the decoded
request, and streams the upstream response back without a Straw response
envelope.

HTTPS proxying defaults to MITM mode. Clients use the MITM proxy listener and
either trust the Straw CA or explicitly disable certificate verification in the
client. Control terminates the client TLS connection, reconstructs the HTTPS
request into the same internal request/response model used by REST and plain
HTTP proxying, applies decoded-request features such as routing metadata and
configured header/cookie injection, and streams the decoded upstream response
back to the client.

Raw CONNECT is an explicit separate listener or mode for clients that need an
opaque tunnel. After Control authenticates the client, reads any routing
metadata present on the CONNECT request, selects a route, and the executor
opens the upstream tunnel, Control returns `200 Connection Established`. The
tunnel is then byte-forwarded without HTTP decoding. Raw CONNECT does not
support decoded-response inspection,
header/cookie injection, payload capture beyond tunnel metadata, or browser
fingerprint behavior that requires access to HTTP messages.

Decoded transport modes return raw upstream responses, not JSON envelopes. When
Control, an executor, or another Straw component rejects or fails a decoded
REST, plain HTTP proxy, or MITM request before an upstream response is visible
to the client, it returns a nonstandard HTTP status code with a JSON envelope
containing the typed error code, retryability, message, and request ID. Raw
CONNECT failures before `200 Connection Established` use CONNECT-compatible
failure behavior; failures after `200` close the tunnel and are recorded in
observability data.

Official SDKs, the CLI, and the UI are thin clients over the REST request,
admin, and config APIs. They do not define separate wire protocols or behavior.

## 7. Request Lifecycle

End-to-end flow from client request to egress response.

Control creates a request ID as soon as a request reaches any ingress path.
That ID is returned on Straw-caused errors and propagated through logs,
metrics, traces, NATS messages, executor work, and persisted request metadata.
Unauthenticated or malformed ingress attempts also get request IDs and are
logged as security/audit events, even when no tenant-bound request record can
be created.

Before any body bytes are forwarded, Control parses ingress-specific request metadata, authenticates the client, checks
quotas and rate limits, applies destination deny rules, computes the request deadline, resolves routing policy, and
selects an executor. Control persists audit/debug request metadata in ClickHouse on a best-effort basis before dispatch;
persistence failures do not fail the request, but they are logged and counted. Persisted metadata includes request ID,
tenant/client identity, ingress type, target host/path, route hints, selected route and executor, timing, byte counts,
final status/result, typed error code where applicable, and payload-capture policy decision. Payloads are not persisted
unless capture is explicitly enabled.

REST, plain HTTP proxy, and MITM HTTPS ingress normalize into one decoded
internal request model. REST metadata comes from structured fields, plain HTTP
proxy metadata comes from `X-Straw-*` headers, and MITM metadata comes from the
proxy request before Control terminates client TLS and reconstructs the HTTPS
request. MITM certificate generation and cache behavior are part of the MITM
design section; the lifecycle only depends on MITM producing the same decoded
request shape. Straw routing/control headers are stripped from outbound
requests unless an explicit injection policy re-adds a value.

Decoded requests are asynchronous inside Control even though REST and proxy
clients observe synchronous request/response calls. Control creates an
in-flight request, sends one normalized assignment to the selected executor
with method, URL, headers, routing metadata, fingerprint/injection policy
resolution, deadline, and body stream or body reference, then waits for the
executor to accept or reject the assignment. If the executor rejects before any
body bytes are forwarded and fallback policy allows another eligible route,
Control may try another route. Once body streaming or a client-visible response
has started, executor loss or rejection fails the active request.

Request bodies stream only after route selection and executor assignment
acceptance. Control reads from the client according to downstream backpressure
from NATS, the executor, or the configured large-body transport, using bounded
buffers only. Normal chunks travel through NATS when they fit configured
message limits; bodies that exceed message limits or deployment policy use the
configured large-body transport, such as object storage references or a direct
streaming channel. Payload capture, when enabled, tees the stream and follows
capture truncation or size policy; capture does not force buffering or mutate
the body.

The executor applies final outbound header/cookie injection and transport-level
fingerprint behavior immediately before making the upstream request. Straw does
not follow upstream redirects by default; redirect responses are streamed back
to the client as normal upstream responses. If operator policy allows it, a
request header or REST flag may opt into following redirects because redirect
following can change routing cost and upstream request count.

Executor responses are streamed back as `response_start` with status and
headers, followed by body chunks, optional trailers, and either `response_end`
or a typed error. Control starts the client response as soon as upstream
status/headers arrive and streams body bytes untouched. Trailers are preserved
when the ingress path, internal transport, and executor support them; otherwise
the limitation is documented by that path. Payload capture remains a tee and
does not inspect or transform normal response bodies.

For successful upstream responses, REST transport, plain HTTP proxy, and MITM
proxy return the upstream status, headers, body, and trailers directly rather
than wrapping them in a Straw envelope. Straw-caused Control, executor, routing,
timeout, validation, or transport errors use nonstandard HTTP status codes and
a JSON error envelope for all decoded flows, with exact status values and error
codes defined in the error handling section. If such an error happens after
client-visible response headers or body bytes have already been sent, Control
cannot switch formats; it terminates the stream and records the typed failure
in logs, traces, metrics, and final metadata.

Raw CONNECT is a separate byte-tunnel lifecycle. Control authenticates the
client, reads routing metadata from the CONNECT request, checks quotas and deny
rules, creates and propagates the request ID, persists metadata best-effort,
selects an executor, and asks the executor to open the upstream tunnel. Control
returns `200 Connection Established` only after the executor has opened that
tunnel. After `200`, bytes tunnel through Control and Straw's internal
transport; the client is never handed off directly to an executor. Before
`200`, failures can return a CONNECT-style status and Straw error headers or
envelope as defined by error handling. After `200`, failures close the tunnel
and are recorded in logs, traces, metrics, and final metadata.

Client disconnects and explicit cancellations trigger best-effort cancellation
from Control to the executor, after which Control drops any later response data.
Final metadata is still updated with disconnect/cancellation result, timing,
and known byte counts. If an executor dies mid-request or mid-tunnel, Control
uses the routing fallback safety rules to decide whether best-effort reroute is
allowed; otherwise the active request fails with a typed worker-loss error.
Control may briefly queue a request only within configured backpressure limits
when all matching executors are busy; otherwise it fails before request body
streaming starts. During Control draining or graceful shutdown, Control stops
accepting new requests and lets in-flight lifecycles finish until their
deadlines.

## 8. Routing Model

How Control chooses an executor.

Routing chooses a generic executor: either an Egress worker pool or a Provider
Adapter pool. Clients do not force route IDs. They provide route hints such as
tags, target country, target region, IP type, sticky session ID, ingress type,
and target host. Admin-configured rules decide the selected route.

Rules are tenant-scoped, explicitly prioritized, enabled or disabled, and
evaluated in priority order. Each rule has match conditions and targets one
named executor pool. Lower-priority rules model fallback; rules do not contain
nested pool lists or weighted traffic splitting in Phase 1. Missing request
hints mean no preference, but any hint the client does provide is a hard
constraint. Fallback may relax admin preferences, but it must not relax client
hints.

Rule matching supports:

- Tags: all requested tags must be present.
- Country: strict ISO-3166 alpha-2 country codes.
- Region: optional worker or provider-advertised values scoped to country.
- IP Type: `datacenter`, `residential`, `mobile`, `isp`, or `unknown`.
- Ingress Type: optional match for REST, plain HTTP proxy, MITM, or raw CONNECT.
- Target Host: exact host and suffix domain matches only.

Egress workers and Provider Adapters advertise capabilities through the same
model: executor type, pool membership, tags, countries and regions, IP types,
supported ingress modes, stable egress identities when available, load, health,
and draining state. Provider-specific endpoint or account selection stays
inside the Provider Adapter.

Control evaluates routing from a cached tenant route snapshot, not by querying
Postgres on every request. Admin config writes update Postgres, bump the
routing config version, and publish a Redis invalidation so Control instances
reload the affected tenant snapshot. A short TTL refresh is the fallback if an
invalidation is missed. A request uses the snapshot captured at request start
for all retry and fallback attempts.

After a rule selects a pool, Control chooses the least-loaded healthy eligible
executor in that pool, using round-robin as a tie breaker. Draining executors
are excluded from new assignments but may finish active requests until they
complete or hit their deadline.

Sticky sessions pin to a stable egress identity, such as an upstream endpoint or
IP, rather than a process instance when that identity exists. If no stable
identity is available, Control may pin to the executor instance. Sticky affinity
is stored in Redis with a rule-level TTL and tenant default, refreshed on use.
If the pinned egress identity is unavailable, the request fails by default. A
rule or request may explicitly allow sticky fallback, in which case Control can
choose another eligible executor and update the affinity.

Fallback first tries another eligible executor in the same selected rule, then
lower-priority fallback rules. Fallback is allowed only before request body
streaming starts, or when the request body is replayable and the method is safe
or idempotent. Once body streaming has started for a non-replayable request,
the active request fails instead of being retried through another route.

When no rule matches, Control returns typed `route_no_match`. Decoded modes use
HTTP `421 Misdirected Request`; raw CONNECT uses CONNECT-compatible failure
before tunnel establishment. There is no implicit default route: admins must
create a catch-all rule if they want one.

When a rule matches but all eligible executors are unhealthy, draining,
overloaded, unavailable, or exhausted after fallback, Control returns typed
`route_unavailable`. Decoded modes use HTTP `503 Service Unavailable`; raw
CONNECT uses CONNECT-compatible failure before tunnel establishment.

## 9. Worker Discovery and Health

How workers join, stay alive, and leave.

Egress Workers and Provider Adapters use the same discovery and health protocol
over NATS. The protocol identifies the executor type as `egress_worker` or
`provider_adapter`; exact NATS subject names are defined in the NATS protocol
section. Workers do not call Control directly for discovery or health.

Each worker has a globally unique, operator-assigned `worker_id` configured on
the worker side. If the worker presents valid worker credentials, Control
accepts first registration automatically and records or updates the durable
worker metadata. Credentials define tenant and pool scope: a credential may be
single-tenant or multi-tenant, but the worker cannot register capabilities
outside that scope. Pools must already exist in Control configuration; workers
cannot create pools implicitly during registration.

Registration happens on startup and whenever static capabilities change.
Control rejects missing required fields, out-of-scope capabilities, or
incompatible protocol/software versions with typed registration errors. A valid
registration returns an acknowledgement and a runtime `session_id`; until then,
the worker stays connected but does not join assignment queue groups. Required
registration data includes:

- `worker_id`, `executor_type`, credential scope, and protocol/software version.
- Pool names, tags, countries, regions, IP types, and supported ingress modes.
- Stable egress identity when known, such as an upstream endpoint or source IP.
- Max concurrency and initial draining state.

Static capabilities are trusted in Phase 1 after credential-scope validation.
Control does not actively verify advertised country, region, or IP type before
routing. It relies on health, load, assignment outcomes, logs, and operator
configuration; active egress verification is future work.

Heartbeats update dynamic runtime state only. They include `worker_id`,
`session_id`, worker-reported health, a short reason string, active request
count, max concurrency, available capacity, queue depth if any, and draining
state. Heartbeats may include a worker timestamp for diagnostics, but liveness
uses Control receive time so clock drift cannot mark workers alive or dead.
Workers send heartbeats every `5s` by default.

Worker-reported health is one of `ready`, `degraded`, or `unhealthy`.
`ready` workers are eligible when capacity is available. `degraded` workers can
remain eligible if they still report available capacity. `unhealthy` workers
are not eligible for new assignments. Zero available capacity also excludes a
worker from new assignments, but it does not make the worker unhealthy or dead.
Workers should not maintain a meaningful local assignment queue; Control gates
assignment on reported capacity, and overloaded workers reject with a typed
capacity error.

Control computes final routing availability from worker-reported health,
heartbeat freshness, admin disable state, draining state, capacity, compatible
version, duplicate-session status, and recent assignment failures. The default
freshness thresholds are:

- `15s` without heartbeat: unavailable for new assignments.
- `30s` without heartbeat: dead; live routing/session state is removed.

Durable worker records and audit history remain when a worker is dead. The same
`worker_id` can become routable again by registering a new session. If an
unavailable worker resumes fresh heartbeats before being marked dead, Control
automatically makes it eligible again once it reports `ready` or eligible
`degraded` health with available capacity and is not disabled, draining, or in
cooldown.

Duplicate active sessions for one `worker_id` are not allowed. A newer valid
registration receives a new `session_id` and replaces the old session after a
`10s` grace window. The old session receives no new assignments during grace.
After grace, remaining active requests either finish if already completed or
follow the worker-loss fallback behavior. Heartbeats from stale `session_id`
values are ignored.

Draining is runtime state. A worker can start draining by reporting
`draining=true`, and an admin can also mark a worker draining through Control.
In both cases, Control stops assigning new work while allowing in-flight work
to finish until each request deadline. Draining clears on new registration
unless the worker reports `draining=true` again. A separate admin disable flag
is durable, survives worker restart, and excludes the worker from routing while
still allowing it to register and report health for observability.

Graceful shutdown sends a best-effort final stopping or draining update so
Control can mark the worker unavailable immediately. Crashes are detected by
heartbeat timeout. When a worker is unavailable, dead, replaced, or lost during
an assignment, Control attempts best-effort reroute only where the routing
fallback rules allow it: before request body streaming starts, or when the body
is replayable and the method is safe or idempotent. Otherwise the request fails
with a typed worker-lost or route-unavailable error.

Control applies a short cooldown for repeated assignment/start failures. The
default trigger is `3` failures within `60s`; the worker is excluded from new
assignments for `30s` or until a healthy heartbeat reports available capacity,
whichever comes first.

Postgres stores durable worker identity, credential, and admin disable metadata. Redis stores ephemeral current
registration, `session_id`, heartbeat, load, availability, draining, cooldown, and routing snapshots. ClickHouse stores
worker audit metadata. Registration and admin state changes are durable audit events written to ClickHouse;
high-frequency
heartbeat and health transitions stay in Redis, logs, metrics, and traces. Local worker liveness/readiness HTTP
endpoints
may exist for deployment probes, but Control discovery and routing health depend on NATS registration and heartbeats.

## 10. NATS Protocol

Messaging shape between Control, Egress Workers, and Provider Adapters.

Phase 1 uses Core NATS only. NATS is an internal service transport, not a
durable job queue, replay log, tenant authorization system, or hidden backlog.
All NATS payloads are protobuf binary messages wrapped in a small common
envelope containing `request_id`, `tenant_id`, `trace_id`, `deadline`,
`protocol_version`, and `attempt`. Message-specific fields stay in the concrete
protobuf body. JSON is used only at public REST/admin boundaries.

Subjects use a stable major-version prefix. Minor protobuf schema evolution
uses backward-compatible fields rather than new subject names. Participants
reject incompatible major protocol versions during registration or assignment
and tolerate unknown backward-compatible protobuf fields. Field-level schema
rules are defined in the protobuf contracts section.

Phase 1 uses this subject table:

- `straw.v1.control.register`: worker or adapter registration request/reply.
- `straw.v1.control.heartbeat`: worker or adapter heartbeat request/reply.
- `straw.v1.executor.<worker_id>.<session_id>.assign`: assignment request/reply
  for one registered executor session.
- `straw.v1.req.<request_id>.<worker_id>.<session_id>.c2e`: request-scoped
  Control-to-executor frames.
- `straw.v1.req.<request_id>.<worker_id>.<session_id>.e2c`: request-scoped
  executor-to-Control frames.

`request_id`, `worker_id`, and `session_id` values used in subjects must be
dot-free safe tokens. Registration rejects invalid `worker_id` values instead
of escaping subject tokens dynamically. Tenant, pool, route, capability, and
executor-type data stay in protobuf fields, not subject names. Provider
Adapters use the same executor protocol and subjects as Egress Workers, with
`executor_type` distinguishing behavior.

Registration is request/reply on `straw.v1.control.register`, handled by any
Control instance in the `control` queue group. A valid registration returns
typed status and a runtime `session_id`. A worker or adapter subscribes to its
assignment subject only after registration returns `ok`, and it re-registers
after any NATS reconnect before accepting new assignments. Draining, stopping,
stale, or replaced sessions unsubscribe from assignment immediately but may
remain connected for heartbeats and active request-scoped streams.

Heartbeats are request/reply on `straw.v1.control.heartbeat`, also handled by
the `control` queue group. The heartbeat ack can return `ok`,
`stale_session`, `re_register`, `disabled`, or `drain` so basic runtime control
does not need a separate channel.

Control owns routing and executor choice. It sends each assignment to the exact
executor session subject, not to a pool queue group, because Control already
tracks sticky sessions, capacity, health, draining, cooldowns, and fallback.
Control sends one active assignment attempt at a time; Phase 1 does not use
parallel speculative dispatch. NATS queue groups are used for shared Control
service handling, not executor load balancing.

Assignment is a two-step flow. Control sends an assignment request and waits
for an immediate accept/reject reply. The default assignment ack timeout is
`2s`, configurable, and capped by the request deadline. If assignment publish
or request/reply fails, Control marks that executor attempt failed and lets the
routing fallback rules decide whether another executor can be tried safely.
There are no generic NATS transport retries because duplicate assignment is
worse than a clean failed attempt.

After assignment accept, request and response data move over deterministic
request-scoped subjects derived from `request_id`, `worker_id`, and
`session_id`. Decoded HTTP bodies and raw CONNECT tunnel bytes use the same
frame protocol, with an explicit mode distinguishing decoded HTTP from opaque
tunnel behavior. Frames include monotonically increasing sequence numbers.
Receivers fail the active request on gaps, duplicates, or out-of-order frames.
Ordering is guaranteed only within a single request stream; there is no
cross-request ordering contract.

Request-scoped protobuf message types include stream frames, credit updates,
cancel, error, end, and cancelled. Credit messages travel on the opposite
direction subject: `c2e` carries request body or tunnel upload frames plus
credit for executor-to-Control response/tunnel bytes, while `e2c` carries
response or tunnel download frames plus credit for Control-to-executor upload
bytes. Senders stop when credit is exhausted, keeping buffers bounded instead
of relying on NATS buffering for backpressure.

Every accepted assignment ends with exactly one terminal message: `end`,
`error`, or `cancelled`. After a terminal message or deadline, Control and the
executor close request-scoped subscriptions and ignore late frames. Late frames
do not change the client-visible result; they are counted as protocol errors
and can contribute to worker cooldown if repeated.

Control sends a best-effort request-scoped `cancel` message when the client
disconnects, an admin action or shutdown abandons the request, a timeout fires,
or fallback makes an accepted attempt obsolete. Deadlines remain the hard stop:
executors must stop work when the assignment deadline expires even if a cancel
message is missed.

Timeout handling has three layers: assignment ack timeout, per-frame idle
timeout, and total request deadline. The assignment ack timeout decides whether
the selected executor accepted work. The idle timeout detects stream silence
after accept. The request deadline is the absolute cap for all NATS and
executor work for that request.

NATS is the default body path only while messages stay under deployment limits.
The default maximum application payload per protobuf message is `1 MiB`,
configurable. Larger request or response bodies use the configured large-body
transport instead of tuning Straw around oversized broker messages.

NATS authentication is service-level in Phase 1. Control, workers, and adapters
receive credentials scoped to their allowed subject prefixes. Tenant
authorization remains in Control and worker credential scope validation during
registration. NATS publish/request/reply failures are exposed externally as
typed `transport_unavailable`; executor reject, timeout, loss, capacity, and
upstream failures keep their more specific typed error codes.

## 11. Protobuf Contracts

Schemas shared by Control and Egress.

Phase 1 uses one shared protobuf package for Control, Egress Workers, and
Provider Adapters:

- Protobuf package: `straw.v1`.
- Go package name: `strawpb`.
- Source file: `proto/straw/v1/straw.proto`.
- Tooling files: `buf.yaml` and `buf.gen.yaml`.
- Optional helper script: `scripts/proto-generate`.

All NATS payloads are one binary protobuf `Envelope`. The envelope contains the
common transport fields and a `oneof payload` so every participant has one
decode path:

- `request_id`, `tenant_id`, `trace_id`.
- `deadline_unix_ms` for the absolute request deadline.
- `protocol_major` and `protocol_minor`.
- `attempt`, starting at `1` and incrementing for fallback attempts.
- `oneof payload`: `RegisterRequest`, `RegisterAck`, `HeartbeatRequest`,
  `HeartbeatAck`, `AssignRequest`, `AssignAck`, and `StreamFrame`.

IDs are plain strings, not binary UUIDs. This keeps REST, logs, NATS subjects,
and generated clients simple across languages. Validation rules are documented
in the schema comments: IDs must be non-empty where business logic requires
them; IDs used in NATS subjects, such as `request_id`, `worker_id`, and
`session_id`, must be dot-free safe tokens. `tenant_id` stays in protobuf
fields and is never included in NATS subjects. Workers include tenant IDs for
correlation only; authorization still comes from validated worker credentials
and registration scope.

Time values use simple integers instead of protobuf timestamp wrapper types:

- Absolute times use `int64 *_unix_ms`.
- Durations and timeouts use `uint32 *_ms`.

The schema uses proto3 and no `required` fields. Receivers validate mandatory
business fields after decoding and return typed `validation_error` failures for
missing, zero, oversized, or inconsistent values. Unknown fields are ignored for
rolling deploy compatibility. Unknown enum values and unsupported `oneof`
payloads are rejected as unsupported or incompatible protocol input.

Fixed-value fields are protobuf enums with explicit `*_UNSPECIFIED = 0`
values. Enums include `IpType`, `IngressType`, `ExecutorType`, worker health,
assignment mode, payload capture decision, registration status, heartbeat ack
status, assignment ack status, fingerprint preset, and shared error codes.
Custom implementations must treat unknown enum values as unsupported rather
than silently defaulting.

HTTP headers use ordered repeated pairs, not maps:

- `Header { string name; bytes value; }`.
- Ordering and duplicates are preserved, including repeated `Set-Cookie`
  headers.

Routing metadata mirrors the routing model. Request metadata carries tags,
country, region, IP type, sticky session ID, ingress type, and target host.
Control-selected fields, such as route ID, pool ID, executor ID, and stable
egress identity, are optional presence-sensitive scalar fields.

`RegisterRequest` advertises static identity and capabilities:

- `worker_id`, `executor_type`, credential identifier, and opaque
  `signed_token`.
- Supported protocol major/minor and software version.
- Pool names, tags, countries, regions, IP types, supported ingress modes, and
  stable egress identity when known.
- Max concurrency and initial draining state.

`RegisterAck` uses a status enum with `OK`, `REJECTED_AUTH`,
`REJECTED_SCOPE`, `REJECTED_VERSION`, and `REJECTED_VALIDATION`. It includes
`session_id` only when status is `OK`; rejected responses include a shared
`Error` and operator-facing message.

`HeartbeatRequest` carries dynamic runtime state only:

- `worker_id`, `session_id`, health, reason, active request count, max
  concurrency, available capacity, optional queue depth, draining state, and
  optional `worker_unix_ms` for diagnostics.

`HeartbeatAck` uses `OK`, `STALE_SESSION`, `RE_REGISTER`, `DISABLED`, and
`DRAIN`.

Assignment reserves capacity before request details are streamed. `AssignRequest`
does not carry the full HTTP request. It carries only reservation data:

- Mode: decoded HTTP or raw tunnel.
- Deadline.
- Expected upload size when known.
- Selected route, pool, executor, and stable egress metadata.

`AssignAck` uses enum values for `ACCEPTED`, `REJECTED_CAPACITY`,
`REJECTED_DRAINING`, `REJECTED_UNSUPPORTED`, and `REJECTED_ERROR`. Rejections
include messages; `REJECTED_ERROR` includes the shared `Error`.

After an accepted assignment, request-scoped traffic uses one `StreamFrame`
message with a `oneof` payload:

- `request_start`
- `response_start`
- `data`
- `credit`
- `cancel`
- `error`
- `trailers`
- `end`
- `cancelled`

Frame direction is implied by the NATS subject: `c2e` carries client-to-executor
upload or tunnel bytes, and `e2c` carries executor-to-Control response or
tunnel bytes. Sequence numbers apply only to data-bearing `DataFrame` messages.
Receivers fail the request on data sequence gaps, duplicates, or out-of-order
frames. Control frames such as credit, cancel, error, trailers, end, and
cancelled are not part of the data sequence.

`CreditFrame` uses byte credit only. Combined with the per-frame data limit,
this bounds memory without frame-count bookkeeping.

Request and response bodies use the same body model:

- Normal bodies stream as repeated `DataFrame { uint64 sequence; bytes data; }`
  messages.
- Large bodies use `BodyRef`.
- `BodyRef` supports S3-compatible object storage references and generic direct
  streaming references from day one.
- Direct stream references are transport-neutral:
  `DirectStreamRef { string stream_id; map<string,string> params; }`.

`RequestStart` carries the executor-ready request:

- Mode: decoded HTTP or raw tunnel.
- HTTP method and full absolute URL as one string.
- Stripped outbound headers only; Straw routing/control headers are never sent
  to executors.
- Routing metadata and selected route/executor metadata.
- Computed deadline and `replayable_body`.
- Payload capture decision and size limits.
- Executor-ready resolved fingerprint and header/cookie injection instructions.

Executors never query config and never interpret raw admin policy objects.
Control sends the resolved instructions needed for this request plus
policy/version IDs for audit correlation. Header and cookie injection
instructions are ordered operations, such as add, set, or remove header and
add or set cookie, so execution order is deterministic.

Fingerprint instructions are a fixed protobuf message, not an opaque JSON blob.
The official worker uses `tls-client`; the protobuf enum mirrors the
`tls-client` preset set in the current Straw contract. When `tls-client` adds
presets, Straw adds enum values in a contract update. Executors reject unknown
or unsupported presets with `unsupported_fingerprint`.

Payload capture is not a boolean. It is an enum decision with size limits:
`NONE`, `METADATA_ONLY`, `HEADERS`, `BODY_TRUNCATED`, and `BODY_FULL`.

`ResponseStart` is upstream-only. It carries the upstream status, ordered
upstream headers, and optional timing fields such as DNS, connect, TLS, and
TTFB durations when available. Straw-generated JSON error envelopes are not
represented as `ResponseStart`; they are Control's client-facing behavior only
when a decoded request fails before upstream response headers are visible.

Trailers are represented by a separate `TrailersFrame` before `EndFrame`.
`EndFrame` marks successful completion and carries final byte counts and timing
summary when known. `CancelFrame` includes a shared error code and reason so
client disconnects, deadlines, admin cancels, and superseded fallback attempts
are distinguishable. `CancelledFrame` acknowledges cancellation and carries
final byte counts and timing when known.

`Error` is shared across registration, assignment, and streams. It includes:

- Shared typed error code.
- Retryability.
- Operator-facing message.
- HTTP status for decoded client-facing failures.
- Details map.
- Optional upstream status.

The shared error code enum covers auth, validation, routing, transport
unavailable, timeout, capacity, worker loss, unsupported fingerprint, upstream
failure, protocol error, cancellation, and unknown internal error. An `Error`
frame after `ResponseStart` means the stream failed after upstream headers or
body began; Control closes the client stream and records the typed failure
instead of trying to switch to a JSON error envelope.

Validation and protocol failures close the active stream. Peers reject invalid
frames, sequence gaps, duplicate or out-of-order data frames, oversized fields,
unsupported enum values, unsupported payload types, and missing mandatory
business fields.

Default limits are explicit in schema comments and config defaults:

- `DataFrame.data`: `1 MiB`.
- Total headers per message: `128 KiB`.
- Metadata/details map entries: `64`.
- Error message length: `4 KiB`.

Optional presence-sensitive scalar values use proto3 `optional`, including
`upstream_status`, `queue_depth`, `worker_unix_ms`, expected upload size, and
selected route/executor fields.

Compatibility is enforced with Buf from day one:

- `buf lint` runs in CI.
- `buf breaking` compares against the last released protobuf contract.
- Removed fields reserve both field numbers and names.
- Changes must not alter field meaning, field type, enum numeric values,
  requiredness expectations, or `oneof` membership incompatibly.
- Minor evolution uses additive optional fields and new enum values.
- Major protocol breaks require a new major protocol version and NATS subject
  prefix.

Generated code targets Go, TypeScript, JavaScript, Python, C#, Java, Kotlin,
and Rust. Generated output is not committed. The repo keeps generation config
and scripts for every target, and CI verifies generation succeeds. Releases
publish language-native generated packages: Go module, npm package, PyPI
package, NuGet package, Maven/Gradle artifacts for Java and Kotlin, and Cargo
crate. The protobuf contract version is canonical; generated packages use that
same version and do not have independent language-specific patch versions.

Phase 1 relies on NATS subject credentials for dispatch authorization and does
not include dispatch signature fields. Deterministic protobuf serialization for
signing or hashing is a Phase 2 concern only; Phase 1 does not depend on
byte-stable serialization.

## 12. HTTP Semantics

* **Methods:** All standard HTTP methods are supported. Control parses ingress requests and normalizes the method into
  the `RequestStart` protobuf message. Egress workers execute the method exactly as provided without restriction.


* **Headers:** Control evaluates ingress headers and strictly strips internal routing hints (e.g., `X-Straw-*`) and
  standard proxy-revealing headers. All remaining client headers are encoded into the
  `Header { string name; bytes value; }` protobuf array, preserving exact ordering and duplicate keys for the Egress
  worker. Final outbound headers are applied at Egress based on the resolved fingerprint and injection policy.


* **Cookies:** Straw is strictly a transport layer. Cookies pass through opaquely within the standard header arrays.
  Neither Control, Redis, nor Egress maintains cookie jars, session stores, or header rewrite rules outside of explicit
  injection policies.


* **Redirects:** Egress workers do not follow upstream redirects by default. `3xx` responses stream back through NATS to
  the client via standard `ResponseStart` and `DataFrame` messages. If operator configuration permits, a request flag
  can instruct the Egress worker to handle redirect following internally, which alters upstream request counts and
  routing costs.


* **Compression:** Upstream `Content-Encoding` (e.g., gzip, brotli) is preserved. Egress workers do not decode or
  recompress upstream responses. Encoded bytes stream directly to the client as raw `DataFrame` payloads. Payload
  capture, if enabled, stores the raw compressed bytes without inspection. Decoding and recompression features are
  deferred to Phase 2.


* **Trailers:** HTTP trailers are fully supported. Egress workers parse upstream trailers and dispatch them using the
  explicit `TrailersFrame` over the request-scoped NATS stream before the `EndFrame`. Control streams these trailers to
  the client when the specific ingress protocol supports them.


* **Connection Reuse:** Phase 1 implements 1:1 request execution. Egress workers open a fresh transport connection for
  every assigned request. Upstream connection pooling and keep-alive state are explicitly deferred to Phase 2.
* **WebSockets:** Phase 1 explicitly rejects all `Upgrade: websocket` requests. Control intercepts these at the ingress
  layer and returns an immediate typed error without attempting route selection or Egress assignment.


* **HTTP/2:** HTTP/2 is supported at both ingress (Client to Control) and egress (Egress to Upstream) boundaries.
  Control multiplexes concurrent inbound streams natively, while Egress delegates to `tls-client` to negotiate HTTP/2
  ALPN matching the requested fingerprint preset.


* **TLS Behavior:** Egress outbound TLS execution is owned by `tls-client`, which dynamically handles SNI and ALPN to
  match the fingerprint profile provided by Control. Upstream certificate verification is strict by default, but clients
  can supply a flag to explicitly disable validation. Client certificates (mTLS) are supported; Control passes the
  necessary certificate references to Egress, which binds them into the execution context.

## 13. MITM Design

Straw uses MITM interception to decode incoming HTTPS client traffic into the shared internal request/response model
used by REST and plain HTTP flows. This ensures features like header injection, routing metadata extraction, and payload
capture work seamlessly across secure tunnels, without Control making outbound upstream connections itself.

**CA and Client Trust**
Straw does not automatically generate root or intermediate Certificate Authorities (CAs) at runtime. Operators must
supply pre-generated CA certificates and private keys via configuration. To facilitate deployment, the repository
provides offline convenience scripts for generating these assets.

For clients to route traffic through the MITM proxy without TLS errors, they must trust this configured CA. Control
exposes a dedicated, unauthenticated HTTP endpoint to distribute the public CA certificate, allowing automated
provisioning for scrapers and local environments. Alternatively, clients can explicitly disable certificate verification
on their end.

**Inbound TLS Termination**
Inbound client TLS connections terminate entirely at the Control service. To maximize compatibility with diverse
scraping clients and to bypass restrictive ingress TLS fingerprinting (such as JA3/JA4 mismatches), Control utilizes the
`tls-client` stack for inbound termination rather than the standard Go `crypto/tls` library. This ensures the proxy
ingress cleanly mimics flexible browser and client behaviors.

**Dynamic Certificate Generation and Caching**
When a client initiates an HTTPS proxy connection (e.g., via `CONNECT` prior to MITM interception or direct SNI
routing), Control dynamically generates a leaf certificate for the requested target domain, signed by the configured
Straw CA.

To prevent generating a new certificate for every handshake, Control utilizes a multi-tiered storage and caching
strategy:

* **Durable Storage:** Generated per-host certificates are written to disk by default. For horizontally scaled or
  stateless Control deployments, this storage backend is configurable to use S3-compatible object storage.
* **Ephemeral Caching:** Control caches active certificates in Redis. Redis manages the TTL for this cache, ensuring
  fast-path TLS handshakes for high-frequency target domains without incurring disk or network storage latency on every
  request.

**Upstream TLS Delegation**
Control handles strictly inbound TLS. It never initiates the upstream TLS handshake to the target site. Once Control
terminates the client connection and decodes the HTTP request, the request is dispatched over NATS to the selected
Egress worker. The Egress worker exclusively handles the outbound TLS connection, applying the resolved browser
fingerprint profile, SNI, and ALPN negotiations directly against the target infrastructure.

**Security and Isolation Boundaries**
Because MITM terminates the secure tunnel, Control processes raw, unencrypted HTTP plaintext in memory. In Phase 1,
Straw does not enforce advanced multitenant cryptographic isolation or encrypted-in-memory buffers for this in-flight
data.

Straw is architected for high-scale web scraping, not as a zero-trust privacy network. Decrypted data is treated as
transient operational plaintext, streaming through bounded buffers before being transmitted over NATS or the large-body
transport. Operators are responsible for environment-level security and access controls if their scraping workloads
handle sensitive or regulated data.

## 14. Egress Execution

How workers perform outbound requests.

**`tls-client` Integration and CGO Boundary**
Egress workers utilize `bogdanfinn/tls-client` to handle TLS negotiation and mimic precise browser fingerprint behavior.
Because `tls-client` relies on CGO and FFI to bridge Go and the underlying TLS library, blocking overhead is a severe
risk to high-concurrency request dispatch. To prevent stalling the primary NATS message loop, Egress workers isolate TLS
execution. The worker manages a dedicated pool of goroutines or persistent OS threads (via `runtime.LockOSThread`). It
maintains long-lived, per-thread `tls-client` instances to amortize initialization costs, passing connection dialing and
handshake parameters across the CGO boundary asynchronously via non-blocking channels.

**Browser Fingerprints**
Control strictly dictates fingerprint behavior via the `straw.v1` protobuf schema. The Egress worker implements a 1:1
mapping between these protobuf enums and `tls-client` preset profiles (e.g., specific Chrome, Firefox, or Safari
versions). The worker does not infer or guess profiles; it strictly applies the exact profile resolved by Control. If
Control dispatches an unknown or unsupported enum, the worker immediately rejects the assignment with the typed
`unsupported_fingerprint` error.

**Proxy Chaining**
When routes require hitting a third-party residential or datacenter proxy network before reaching the target, the
upstream proxy configuration—including URLs and authentication credentials—lives locally within the Egress worker's
configuration. Control does not manage upstream proxy credentials. The Egress worker handles proxy authentication
dynamically before tunneling the payload. Upstream connection failures, auth rejections, or timeouts at the proxy layer
are natively trapped by the worker and translated into client-facing typed errors (e.g., `upstream_failure`), while
verbose internal logs capture the raw proxy rejection for operator debugging.

**DNS Resolution**
By default, Egress workers rely on the host OS local resolver to prevent DNS leakage and leverage existing
infrastructure caching. Operators can explicitly override this by configuring custom remote DNS servers directly on the
worker. Additionally, Egress workers can opt-in to use a caching DNS resolver maintained by Control, ensuring uniform,
high-speed resolution across geographically distributed worker pools.

**Source IP Selection**
For hosts with multiple network interfaces or IP blocks, Egress relies on standard OS routing tables by default. To
support strict egress routing paths, operators can configure the worker to bind outbound upstream TCP connections to a
specific local IP address or network interface. This binding is managed locally at the worker level, decoupling Control
from the underlying host network topology.

**Timeout Handling**
Timeouts are governed strictly by Control's computed `deadline_unix_ms`. The Egress worker wraps the entire outbound
lifecycle—DNS dial, TLS handshake, header read, and body stream—in a single Go `context.Context` derived via
`context.WithDeadline`. This enforces a hard, unified stop across all execution phases, superseding any internal
`tls-client` default timers and guaranteeing the worker terminates resources exactly when Control expects the request to
expire.

**Error Mapping**
Egress translates raw Go and network errors into standard Straw protobuf typed errors to maintain the stable API
contract. Using `errors.As` and `errors.Is`, the worker inspects the failure chain: `net.DNSError` maps to
`dns_failure`, `syscall.ECONNREFUSED` to `connection_refused`, `context.DeadlineExceeded` to `timeout`, and `tls-client`
certificate/handshake errors to `tls_failure`. Unhandled network errors default to `upstream_failure`. Egress embeds the
raw Go error string exclusively in the protobuf `Error` details map, bridging the gap between stable external API codes
and actionable internal operator audits.

## 15. State and Storage

Straw enforces strict boundaries between durable configuration, ephemeral runtime state, high-volume operational
telemetry, and stateless execution.

### Postgres (Durable Configuration)

Postgres acts as the absolute source of truth for low-volume, high-value Control plane data.

* **Scope:** Tenants, API keys, worker credentials, routing rules, fingerprint profiles, upstream/vendor configurations,
  and injection policies.
* **Access:** Exclusively read/written by Control.
* **Integration:** Synced to ClickHouse via Change Data Capture (CDC) or ClickHouse Dictionaries to allow cross-system
  analytical queries (e.g., joining tenant config with request logs) without degrading OLTP performance.

### ClickHouse (Operational Data & Analytics)

ClickHouse handles all high-volume, append-heavy operational data, entirely isolating analytical workloads from the
Control plane database.

* **Scope:** Request metadata, audit logs, timing metrics, and payload capture metadata.
* **Lifecycle:** Data retention relies on ClickHouse native TTLs, dropping rows automatically without custom cron jobs.

### Redis (Ephemeral Runtime State)

Redis operates as a shared, highly volatile coordination layer. It is not a source of truth.

* **Scope:** Sticky session affinity, rate limit counters, backpressure state, worker availability snapshots, and the
  dynamic MITM certificate cache.
* **Eviction Policy:** Configured strictly to `volatile-lru`. Every key must have an explicit TTL.
* **Memory Boundary:** Enforces a hard `maxmemory` cap. If a tenant floods the MITM cert cache, Redis evicts the oldest
  keys to survive. Cache misses trigger regenerating the cert, fetching it from S3, or establishing a new routing
  session.

### S3-Compatible Storage (Blobs & Large Payloads)

S3 handles all unbounded or oversized binary data that must bypass NATS or survive beyond the Redis cache.

* **Scope:** Request/response bodies exceeding the configured NATS message limits (e.g., > 1 MiB) and durable,
  dynamically generated per-host MITM certificates.
* **Lifecycle:** A strict 3-day retention window enforced entirely via native bucket lifecycle rules. Control and Egress
  write the data; the storage backend handles garbage collection automatically.

### Control Instances (In-Memory State)

Control instances remain mostly stateless but heavily cache durable state to guarantee sub-millisecond routing
decisions.

* **Scope:** Evaluates traffic against a cached tenant route snapshot.
* **Updates:** Configuration writes to Postgres trigger a Redis invalidation, prompting Control to reload the snapshot.
* **Outage Behavior:** If Postgres and Redis experience a total outage, Control instances prioritize availability and
  continue routing traffic using their stale local memory snapshots.

### Egress Workers (Stateless Execution)

Egress workers carry zero writable persistent local disk state.

* **Scope:** Completely stateless binaries.
* **Provisioning:** Configured entirely via CLI parameters, static files, and environment variables. Upstream proxies
  and specific network bindings are injected locally at startup, and workers fetch everything else from Control via
  NATS.

## 16. Authentication and Authorization

### 16.1 Client Authentication

* **Credential Type:** Static API keys managed as secure hashes within Postgres. Straw rejects dynamic token
  lifecycles (JWT/OIDC) to minimize client-side operational overhead for automated headless scrapers.
* **Ingress Mapping:** REST clients must authenticate via the `Authorization: Bearer <key>` header. Proxy clients (HTTP,
  MITM, and raw CONNECT) must use the standard `Proxy-Authorization` header.
* **Tenant Isolation:** Every API key is strictly bound to a concrete `(user_id, tenant_id)` pair at creation time.
  Users spanning multiple tenants must utilize distinct API keys for each tenant context. Control resolves tenant
  context implicitly from the validated token string; headers forcing external tenant declarations are prohibited.
* **Caching and Revocation:** To maintain sub-millisecond Control-side routing paths, active key metadata is cached
  in-memory across distributed Control nodes. Key revocations or user role changes trigger immediate Postgres writes, a
  configuration version bump, and an event-driven invalidation broadcast via Redis pub/sub. Control nodes intercepting
  the invalidation signal evict the cached credential immediately.

### 16.2 Internal Executor Authentication

* **Mechanism:** Long-lived Egress workers and Provider Adapters authenticate via pre-provisioned, cryptographically
  signed tokens generated by the Admin API.
* **Token Verification:** During the NATS handshake, the worker submits this token inside the
  `RegisterRequest.signed_token` field. Control performs stateless, in-memory cryptographic verification of the
  signature before querying Redis or Postgres. This boundary insulates the persistent metadata tier from connection
  exhaustion during worker crash loops.
* **Scope Validation:** Validated tokens must explicitly map to a recognized `tenant_id` and an existing `pool_scope`
  defined in the Postgres configuration. If valid, Control registers the capability matrix and assigns a transient
  runtime `session_id`.

### 16.3 Role-Based Access Control (RBAC)

* **Binding Profile:** Roles are attached directly to the `User` entity within a specific tenant context. Generated API
  keys inherit the exact RBAC capabilities of the parent user.
* **Predefined Baseline Roles:** Access controls are governed strictly by three immutable system roles. Custom or
  granular per-key permission flags are deferred.
* **Admin:** Retains complete control over the tenant perimeter. Authorized to manage tenants, user configurations, API
  keys, worker credentials, resource quotas, and toggle global payload-capture policies.
* **Operator:** Grants permission to modify operational routing logic. Authorized to configure routing rules,
  upstream/vendor endpoints, browser fingerprint profiles, and HTTP header/cookie injection rules. Operators cannot
  manage identities, alter credentials, or touch payload-capture configurations.
* **Viewer:** Read-only access to operational telemetry. Authorized to audit active routing configurations, inspect
  executor health matrices, and query ClickHouse operational datasets. Viewers are blocked from mutating system state or
  executing request transport operations.

## 17. Rate Limits and Quotas

Straw enforces strict abuse, overload, billing, and compliance controls at the Control ingress layer before dispatching
requests to Egress workers.

**Rate Limits (Short-Term Throughput)**

* **Dimensions:** Evaluated per request across four axes: global `tenant_id`, `tenant_id` + `api_key`, `tenant_id` +
  `target_host`, and `tenant_id` + `ip_type`.
* **Algorithm:** Redis Sliding Window Log using Sorted Sets (`ZSET`). Logs exact request timestamps for strict accuracy.
* **Enforcement:** Immediate drop. Breaches return a typed `rate_limit_exceeded` error and HTTP `429 Too Many Requests`.
  No request queueing or connection throttling is performed.

**Quotas (Long-Term Volume)**

* **Metrics:** Tracks total request count and total bandwidth (bytes).
* **Cadence:** Fixed monthly resets.
* **Storage:** Evaluated using Redis Fixed Window Counters (`INCRBY`) with a 30-day TTL. Redis state is periodically
  flushed to Postgres to ensure durable billing records survive ephemeral cache evictions.
* **Enforcement:** Evaluated at request start and end. If a bandwidth quota is breached during an active request, the
  in-flight request finishes. Subsequent requests are strictly blocked with a typed `quota_exhausted` error until the
  reset.

**Destination Deny Rules and Redaction**

* **Deny Rules:** Configured both globally by administrators and locally per tenant. Blocks outbound transport to
  restricted IPs, domains, or network ranges.
* **Redaction:** Evaluated continuously at the Control layer. When a redaction rule triggers, Control dynamically strips
  the offending headers or payload bytes from the stream and forwards the remaining request. Redaction does not drop the
  entire request.

## 18. Error Handling

Canonical failure behavior, error taxonomy, and response semantics.

### 18.1 Error Envelope

All error responses — across REST API, HTTP forward proxy, CONNECT tunnel, and MITM proxy — use a single unified `ErrorResponse` message defined in the protobuf contract.

```protobuf
message ErrorResponse {
  ErrorCategory category    = 1;   // Which category the error belongs to
  ErrorCode code            = 2;   // Specific error code within the category
  string message            = 3;   // Human-readable description
  bool retryable            = 4;   // Whether the SDK/client should retry
  uint64 retry_after_ms     = 5;   // Suggested wait before retry (0 if N/A)
  string request_id         = 6;   // Correlation ID for tracing
  uint32 upstream_status    = 7;   // Origin HTTP status (only if egress_involved)
  TimeoutType timeout_type  = 8;   // Which layer timed out (only if timeout error)
}
```

- `worker_id` is **never** exposed to clients. Worker identifiers belong in internal logs and audit trails only.
- Fields 7–8 are populated only when relevant; they are zero/empty otherwise.
- `retry_after_ms` is non-zero only for rate-limit and quota errors.

### 18.2 Error Categories

Errors are grouped into five categories, each with its own protobuf `ErrorCategory` enum value:

| Category | Scope | Description |
|---|---|---|
| `CLIENT` | `control_local` | Errors originating in Control before any worker involvement: auth, permissions, rate limits, quotas, request validation, deny rules. |
| `ROUTING` | `control_local` | Errors where Control cannot find a valid path: no workers, no matching route, sticky session failure. |
| `TRANSPORT` | `egress_involved` | Errors from the Control↔Egress transport layer: worker timeout, worker disconnect, NATS failure. |
| `EGRESS` | `egress_involved` | Errors from the outbound leg: origin HTTP errors, TLS failure, connection refused, DNS failure, body too large. |
| `STREAMING` | `egress_involved` | Errors that occur mid-stream during large upload or download. |

Each `ErrorCode` references its parent `ErrorCategory`. The `control_local` vs `egress_involved` scope is implicit in the category but may be surfaced in internal logs for debugging.

### 18.3 Error Code Registry

Error codes are numbered in category ranges for structural clarity.

**Client errors (1–99)** — `control_local`, never retryable:

| Code | Name | Description | HTTP |
|---|---|---|---|
| 1 | `auth_failure` | Invalid or expired API key or worker token | 401 |
| 2 | `tenant_not_found` | API key references a deleted tenant | 401 |
| 3 | `insufficient_permissions` | API key lacks required RBAC role | 403 |
| 4 | `rate_limit_exceeded` | Request rate exceeded the sliding window | 429 |
| 5 | `quota_exhausted` | Monthly bandwidth or request count exceeded | 429 |
| 6 | `invalid_request` | Malformed request body, missing required fields | 400 |
| 7 | `destination_denied` | Target IP/domain blocked by deny rules | 403 |
| 8 | `header_injection_failed` | Configured header/cookie injection could not be applied | 400 |

**Routing errors (100–199)** — `control_local`, typically not retryable:

| Code | Name | Description | HTTP |
|---|---|---|---|
| 100 | `no_workers_available` | No registered workers for the target pool | 503 |
| 101 | `no_matching_route` | No routing rule matches the request | 404 |
| 102 | `all_upstreams_failed` | All upstream proxies/vendors returned errors | 502 |
| 103 | `sticky_session_failed` | Sticky session worker is unavailable | 503 |

**Transport errors (200–299)** — `egress_involved`, some retryable:

| Code | Name | Description | HTTP | Retryable |
|---|---|---|---|---|
| 200 | `worker_timeout` | Worker did not reply within NATS request/reply window | 504 | Yes |
| 201 | `worker_disconnected` | Worker lost connection mid-request | 502 | Yes |
| 202 | `nats_cluster_unavailable` | NATS transport failure (mapped to `worker_timeout` or `control_internal_error` externally) | 504 | Yes |
| 203 | `control_internal_error` | Unexpected failure within Control | 500 | No |

**Egress errors (300–399)** — `egress_involved`, retryable depending on code:

| Code | Name | Description | HTTP | Retryable |
|---|---|---|---|---|
| 300 | `upstream_http_error` | Origin returned a 4xx or 5xx | Passthrough | No |
| 301 | `upstream_tls_failure` | TLS handshake with origin failed | 502 | Yes |
| 302 | `upstream_connection_refused` | Origin refused the connection | 502 | Yes |
| 303 | `upstream_dns_failure` | DNS resolution failed on the worker | 502 | Yes |
| 304 | `upstream_body_too_large` | Origin rejected body size | 413 | No |

**Streaming errors (400–499)** — `egress_involved`:

| Code | Name | Description | HTTP | Retryable |
|---|---|---|---|---|
| 400 | `stream_upload_aborted` | Client upload stream was interrupted | 502 | Yes |
| 401 | `stream_download_aborted` | Worker download stream was interrupted | 502 | Yes |

### 18.4 Error-to-Transport Mapping

**REST API** — Returns a JSON body with the full `ErrorResponse` envelope:
```json
{
  "category": "CLIENT",
  "code": "rate_limit_exceeded",
  "message": "Request rate limit exceeded for tenant abc123",
  "retryable": true,
  "retry_after_ms": 5000,
  "request_id": "req-abc123",
  "upstream_status": 0,
  "timeout_type": "TIMEOUT_TYPE_UNSPECIFIED"
}
```

**HTTP forward proxy, CONNECT tunnel, MITM proxy** — Returns an HTTP response with the status code from the mapping below. The response body includes a compact JSON error envelope for client parsing. MITM proxy mirrors the HTTP forward proxy behavior exactly.

| ErrorCode | REST API JSON | HTTP/CONNECT/MITM |
|---|---|---|
| `auth_failure`, `tenant_not_found` | 401 Unauthorized | 401 Unauthorized |
| `insufficient_permissions` | 403 Forbidden | 403 Forbidden |
| `rate_limit_exceeded`, `quota_exhausted` | 429 Too Many Requests | 429 Too Many Requests |
| `invalid_request`, `header_injection_failed` | 400 Bad Request | 400 Bad Request |
| `destination_denied` | 403 Forbidden | 403 Forbidden |
| `no_workers_available`, `sticky_session_failed` | 503 Service Unavailable | 503 Service Unavailable |
| `no_matching_route` | 404 Not Found | 404 Not Found |
| `all_upstreams_failed`, `upstream_tls_failure`, `upstream_dns_failure` | 502 Bad Gateway | 502 Bad Gateway |
| `worker_timeout`, `nats_cluster_unavailable` | 504 Gateway Timeout | 504 Gateway Timeout |
| `control_internal_error` | 500 Internal Server Error | 500 Internal Server Error |
| `upstream_http_error` | 200 + `upstream_status` field | `upstream_status` (passthrough) |
| `upstream_connection_refused` | 502 Bad Gateway | 502 Bad Gateway |
| `upstream_body_too_large` | 413 Payload Too Large | 413 Payload Too Large |
| `stream_upload_aborted`, `stream_download_aborted` | 502 Bad Gateway | 502 Bad Gateway |

### 18.5 Timeout Semantics

A single `timeout_exceeded` error code is used for all timeout scenarios, with a `TimeoutType` enum distinguishing the layer:

| TimeoutType | Description |
|---|---|
| `CONNECT_TIMEOUT` | Worker could not establish outbound connection to origin |
| `REQUEST_TIMEOUT` | Origin did not send first response byte within timeout |
| `IDLE_TIMEOUT` | No data received from origin for a configurable idle period |
| `WORKER_TIMEOUT` | Worker did not reply to Control's NATS request within timeout |
| `UPLOAD_TIMEOUT` | Client did not finish uploading within timeout |
| `DOWNLOAD_TIMEOUT` | Worker did not finish downloading within timeout |

Timeout durations are configurable per-request or per-tenant, with an expected default in the 30–60 second range.

### 18.6 Egress-to-Control Error Reporting

Egress workers forward raw HTTP responses (status code, headers, body summary) back to Control via the NATS reply channel. Control is the sole authority for mapping raw responses to error codes. This keeps Egress implementations simple and portable — a custom Go or Rust Egress worker does not need to encode Straw's error semantics.

When Egress reports an error, the flow is:
1. Worker detects the failure (e.g., upstream TLS handshake failure).
2. Worker sends a `WorkerReport` message to Control containing the raw HTTP status, headers, body summary, and connection error message.
3. Control maps the raw data to the appropriate `ErrorCode` based on its error taxonomy.
4. Control populates the `ErrorResponse` envelope and returns it to the client.

If Control detects the error itself (e.g., no workers available, auth failure, NATS timeout), it generates the `ErrorResponse` directly without involving Egress.

### 18.7 Retry Semantics

Each error code has a `retryable` flag defined in the protobuf. Control does **not** auto-retry on behalf of the client — the SDK is responsible for retry logic. The `retryable` flag tells the SDK whether a retry is expected to succeed.

- **Retryable:** `worker_timeout`, `worker_disconnected`, `upstream_tls_failure`, `upstream_connection_refused`, `upstream_dns_failure`, `stream_upload_aborted`, `stream_download_aborted`. SDKs should implement exponential backoff with jitter.
- **Not retryable:** `auth_failure`, `tenant_not_found`, `insufficient_permissions`, `rate_limit_exceeded` (use `retry_after_ms`), `quota_exhausted`, `invalid_request`, `destination_denied`, `upstream_http_error`, `upstream_body_too_large`, `control_internal_error`.

For rate-limit errors, `retry_after_ms` provides a concrete wait duration. For other retryable errors, the SDK determines the backoff strategy.

### 18.8 Logging and Audit

Errors are logged at different levels based on category:

| Level | Categories | Purpose |
|---|---|---|
| `ERROR` | Transport, Egress, System | Operational incidents requiring investigation |
| `WARN` | Client (except auth), Routing | Expected operational noise (rate limits, no workers) |

## 19. Observability

Straw's observability stack follows a layered architecture: ClickHouse serves as the primary data lake for request-level records, structured logs, traces, and metrics rollups; Prometheus provides real-time metrics for alerting and SLO tracking; Loki aggregates structured logs for log-level querying; Jaeger/Tempo handles distributed tracing; and Grafana unifies all data sources into dashboards and alerting rules.

### 19.1 Data Flow

```
Egress → NATS → Control → Fluent Bit/Vector → ClickHouse
Egress → NATS → Control → Prometheus (/metrics scrape)
Egress → NATS → Control → Loki
Egress → NATS → Control → Jaeger/Tempo
Prometheus → Grafana (real-time dashboards, alerting)
ClickHouse → Grafana (deep analysis, historical queries)
Loki → Grafana (log-level querying)
Jaeger/Tempo → Grafana (distributed tracing)
```

Egress workers emit only to NATS. Control is the sole observability aggregator — it centralizes all telemetry, enriches it with routing context, and pushes it to the appropriate backend. This keeps Egress implementations simple and portable.

Control pushes telemetry through a lightweight sidecar (Fluent Bit or Vector) that buffers and batches writes to ClickHouse, providing backpressure handling and retry semantics. Prometheus scrapes Control's `/metrics` endpoint directly. Loki receives structured logs via HTTP push from Control. Jaeger/Tempo receives trace spans via gRPC/HTTP from Control.

### 19.2 ClickHouse Schema

ClickHouse stores four tables with materialized views for pre-aggregated metrics.

**Table: `requests`** — every request as a row (the main data lake, TTL: ~90 days)

| Column | Type | Description |
|---|---|---|
| `request_id` | String | Propagated from client through Control to Egress |
| `trace_id` | String | Distributed trace identifier |
| `tenant_id` | String | Tenant identifier |
| `api_key_id` | String | API key used for authentication |
| `timestamp_ingest` | DateTime64 | When Control ingested the request |
| `timestamp_request` | DateTime64 | When the client sent the request |
| `timestamp_response` | DateTime64 | When the response was received |
| `timestamp_total_ms` | Float64 | Total request latency |
| `method` | String | HTTP method |
| `url` | String | Request URL |
| `user_agent` | String | Client user agent |
| `fingerprint_profile` | String | Browser fingerprint profile used |
| `routing_rule` | String | Matching routing rule name |
| `selected_worker` | String | Egress worker selected for egress |
| `upstream_proxy` | String | Upstream proxy used (if any) |
| `country` | String | Target country code |
| `region` | String | Target region code |
| `ip_type` | String | Residential, datacenter, mobile |
| `sticky_session_id` | String | Sticky session identifier |
| `session_id` | String | Session identifier |
| `tags` | Array(String) | Tags used for routing match |
| `injected_header_count` | UInt32 | Number of injected headers |
| `error_code` | String | Straw error code |
| `error_category` | String | transport, egress, client, routing, system |
| `upstream_status` | UInt16 | Raw upstream HTTP status code |
| `client_status` | UInt16 | HTTP status returned to client |
| `timeout_type` | String | CONNECT_TIMEOUT, REQUEST_TIMEOUT, IDLE_TIMEOUT, WORKER_TIMEOUT, UPLOAD_TIMEOUT, DOWNLOAD_TIMEOUT |
| `worker_status` | String | alive, draining, disconnected |
| `retry_count` | UInt32 | Number of retries attempted |
| `is_retry` | UInt8 | Whether this is a retried request |
| `request_body_size` | UInt64 | Size of request body in bytes |
| `response_body_size` | UInt64 | Size of response body in bytes |
| `body_captured` | UInt8 | Whether payload capture was enabled |
| `latency_connect_ms` | Float64 | Time to establish outbound connection |
| `latency_upload_ms` | Float64 | Time to upload request body |
| `latency_download_ms` | Float64 | Time to download response body |
| `latency_routing_ms` | Float64 | Time spent on Control-side routing decision |

**Table: `logs`** — structured log events (TTL: ~30 days)

| Column | Type | Description |
|---|---|---|
| `timestamp` | DateTime64 | Log event timestamp |
| `service` | String | control, egress |
| `level` | String | DEBUG, INFO, WARN, ERROR, FATAL |
| `message` | String | Log message |
| `request_id` | String | Correlated request ID (nullable) |
| `tenant_id` | String | Correlated tenant ID (nullable) |
| `trace_id` | String | Correlated trace ID (nullable) |
| `worker_id` | String | Worker identifier (nullable) |
| `error_code` | String | Straw error code (nullable) |
| `extra` | Map(String, String) | Additional structured fields |

**Table: `traces`** — distributed trace spans (TTL: ~7 days)

| Column | Type | Description |
|---|---|---|
| `trace_id` | String | Distributed trace identifier |
| `span_id` | String | Span identifier |
| `parent_span_id` | String | Parent span identifier (nullable) |
| `service` | String | control, egress |
| `operation` | String | Span operation name |
| `start_time` | DateTime64 | Span start time |
| `duration_ms` | Float64 | Span duration |
| `status` | String | OK, ERROR, UNSET |
| `tags` | Map(String, String) | Key-value metadata (request_id, tenant_id, error_code, etc.) |

**Table: `events`** — operational events (TTL: ~90 days)

| Column | Type | Description |
|---|---|---|
| `timestamp` | DateTime64 | Event timestamp |
| `event_type` | String | worker_registered, worker_deregistered, worker_draining, config_updated, admin_action, worker_disconnected, nats_outage, rate_limit_triggered |
| `tenant_id` | String | Affected tenant (nullable) |
| `worker_id` | String | Affected worker (nullable) |
| `admin_user` | String | Admin user who triggered the event (nullable) |
| `details` | String | Human-readable event description |
| `extra` | Map(String, String) | Additional structured fields |

**Materialized View: `metrics_rollups`** — pre-aggregated metrics (TTL: ~180 days)

Hourly and daily rollups of request rate, error rate, and latency percentiles, partitioned by tenant_id, routing_rule, selected_worker, error_code, and country. These are computed via ClickHouse materialized views triggered by inserts into the `requests` table.

### 19.3 Prometheus Metrics

Prometheus collects real-time metrics via a `/metrics` endpoint on Control. Services expose counters, histograms, and gauges for alerting and SLO tracking.

**Counters** (monotonically increasing):

| Metric | Labels | Description |
|---|---|---|
| `straw_requests_total` | `method`, `url`, `tenant_id`, `error_code`, `error_category`, `routing_rule`, `selected_worker`, `upstream_proxy`, `country`, `region`, `ip_type`, `fingerprint_profile` | Total requests processed |
| `straw_requests_retried_total` | `method`, `tenant_id`, `error_code` | Total retried requests |
| `straw_worker_registered_total` | `worker_id` | Worker registration events |
| `straw_worker_deregistered_total` | `worker_id` | Worker deregistration events |
| `straw_worker_drain_started_total` | `worker_id` | Worker drain events |
| `straw_worker_disconnected_total` | `worker_id` | Worker disconnection events |
| `straw_upstream_proxy_connected_total` | `proxy_name` | Upstream proxy connection events |
| `straw_upstream_proxy_disconnected_total` | `proxy_name` | Upstream proxy disconnection events |
| `straw_payload_captured_total` | `tenant_id` | Payload capture events (opt-in) |
| `straw_nats_messages_sent_total` | `message_type` | NATS messages sent by Control |
| `straw_nats_messages_received_total` | `message_type` | NATS messages received by Control |
| `straw_nats_errors_total` | `error_type` | NATS transport errors |

**Histograms** (latency distributions):

| Metric | Labels | Description |
|---|---|---|
| `straw_request_duration_seconds` | `method`, `tenant_id`, `error_code`, `routing_rule` | Total request latency |
| `straw_routing_duration_seconds` | `method`, `tenant_id` | Control-side routing decision latency |
| `straw_connect_duration_seconds` | `method`, `tenant_id` | Outbound connection establishment latency |
| `straw_upload_duration_seconds` | `method`, `tenant_id` | Request body upload latency |
| `straw_download_duration_seconds` | `method`, `tenant_id` | Response body download latency |
| `straw_nats_request_duration_seconds` | `message_type` | NATS request/reply latency |
| `straw_request_body_size_bytes` | `method`, `tenant_id` | Request body size distribution |
| `straw_response_body_size_bytes` | `method`, `tenant_id`, `error_code` | Response body size distribution |

**Gauges** (current state):

| Metric | Labels | Description |
|---|---|---|
| `straw_active_connections` | `tenant_id`, `protocol` (http, connect, mitm) | Active client connections |
| `straw_active_requests` | `tenant_id`, `method` | Currently in-flight requests |
| `straw_workers_available` | `worker_id`, `country`, `region`, `ip_type` | Workers available for routing |
| `straw_workers_draining` | `worker_id` | Workers in drain state |
| `straw_nats_subscriptions` | `subscription_type` | Active NATS subscriptions |
| `straw_nats_connection_status` | `cluster_node` | NATS connection health (1=connected, 0=disconnected) |
| `straw_clickhouse_write_queue_depth` | | Pending writes to ClickHouse |
| `straw_clickhouse_write_errors_total` | | Failed ClickHouse writes |
| `straw_rate_limit_remaining` | `tenant_id` | Remaining rate limit quota |
| `straw_quota_remaining` | `tenant_id` | Remaining quota |

### 19.4 Structured Logs

All services emit JSON-structured logs. Control enriches logs with `request_id`, `tenant_id`, and `trace_id` where available.

**Log levels and categories:**

| Level | Categories | Purpose |
|---|---|---|
| `ERROR` | Transport failures, Egress worker crashes, NATS disconnections, ClickHouse write failures | Operational incidents requiring investigation |
| `WARN` | Rate limits, no workers available, timeout warnings, retry attempts | Expected operational noise |
| `INFO` | Request processing, worker registration/deregistration, config updates, routing decisions | Operational visibility |
| `DEBUG` | Routing rule matching details, NATS message payloads, header injection details | Debugging and troubleshooting |

**Log enrichment:** Every log entry includes `service`, `request_id` (where applicable), `tenant_id` (where applicable), `trace_id` (where applicable), and a structured `extra` field for domain-specific context.

### 19.5 Distributed Tracing

Tracing follows OpenTelemetry conventions. Control generates trace spans for each request lifecycle stage:

1. **Client receive** — request received from client entrypoint
2. **Auth** — API key authentication and tenant resolution
3. **Routing** — routing rule evaluation, worker selection
4. **NATS request** — request forwarded to Egress via NATS
5. **NATS reply** — response received from Egress via NATS
6. **Response send** — response returned to client

Each span includes tags for `request_id`, `tenant_id`, `error_code` (if applicable), `selected_worker`, and `routing_rule`. Spans are exported to Jaeger/Tempo for trace correlation and latency visualization.

Request IDs are propagated through all NATS messages, enabling log-to-trace correlation even when spans are incomplete.

### 19.6 SLOs and Targets

| SLO | Target | Measurement |
|---|---|---|
| **Availability** | 99.99% of requests succeed (excluding upstream failures) | `straw_requests_total` where `error_code` is not an internal error, divided by total requests |
| **Latency — p50** | < 100 ms for Control-side routing overhead | `straw_routing_duration_seconds` p50 |
| **Latency — p99** | < 500 ms for Control-side routing overhead | `straw_routing_duration_seconds` p99 |
| **Error rate** | < 1% of requests hit `control_internal_error` or `worker_timeout` | `straw_requests_total` where `error_code` in (`control_internal_error`, `worker_timeout`), divided by total requests |
| **Worker health** | > 95% of registered workers responding within timeout | Gauge of responsive workers vs. registered workers |

### 19.7 Alerting Tiers

**Critical (P1)** — Immediate response required:

| Alert | Condition | Duration |
|---|---|---|
| Control cluster down | No health check from any Control instance | 1 minute |
| Error rate critical | `error_code` in (`control_internal_error`, `worker_timeout`) > 5% of requests | 5 minutes |
| Worker pool critical | Available workers < 20% of registered workers | 5 minutes |
| NATS cluster unavailable | `straw_nats_connection_status` = 0 for any cluster node | 1 minute |
| ClickHouse write failure | Write queue depth > 10,000 or write error rate > 10% | 5 minutes |

**Warning (P2)** — Response within hours:

| Alert | Condition | Duration |
|---|---|---|
| Error rate elevated | `error_code` in (`control_internal_error`, `worker_timeout`) > 1% of requests | 10 minutes |
| Latency SLO breach | p99 routing latency exceeds 500 ms | 15 minutes |
| Worker pool degraded | Available workers < 50% of registered workers | 10 minutes |
| Worker disconnected | Individual worker disconnected from NATS | Immediate |
| Rate limit triggered | Tenant rate limit exceeded | Immediate |

**Info (P3)** — Capacity trends, no immediate action:

| Alert | Condition | Duration |
|---|---|---|
| Worker pool trending low | Available workers < 80% of registered workers | 30 minutes |
| Latency trending upward | p95 routing latency increasing > 20% over 1 hour | 1 hour |
| Storage approaching capacity | ClickHouse/table storage > 80% of allocated capacity | Daily |
| Quota exhaustion warning | Tenant quota remaining < 10% | Immediate |

### 19.8 Grafana Dashboards

**Dashboard: System Overview**

- Request rate (RPS) over time
- Error rate by category (line chart)
- Latency percentiles (p50, p90, p95, p99)
- Active connections and in-flight requests
- Worker availability (pie chart)
- Active alerts panel

**Dashboard: Per-Tenant**

- Request volume per tenant
- Error rate per tenant
- Latency percentiles per tenant
- Quota usage and rate limit utilization
- Top error codes per tenant

**Dashboard: Routing**

- Routing rule performance (request count, error rate, latency)
- Upstream proxy health and availability
- Country/region distribution map
- IP type distribution
- Sticky session hit rate

**Dashboard: Worker Pool**

- Worker registration/deregistration timeline
- Worker health status (alive, draining, disconnected)
- Worker capacity by country/region/IP type
- Drain event history
- Worker response time distribution

**Dashboard: SLO & Alerts**

- SLO burn rate (availability, latency, error rate)
- Error budget remaining (days)
- Active alerts by tier
- Alert history and resolution time
- SLO trend over time

**Dashboard: Infrastructure**

- NATS cluster health (connection status, message latency, subscription count)
- ClickHouse ingestion lag and write queue depth
- Prometheus scrape health and target status
- Fluent Bit/Vector buffer depth and error rate
- Storage utilization per ClickHouse table

## 20. Configuration


### 20.1 Static vs Dynamic Configuration

Straw configuration is divided into two categories with distinct lifecycles, storage, and reload semantics.

**Static (Deploy-Time) Configuration**

Loaded once at process startup from config files and environment variables. Changes require a process restart.

- Stored in YAML config files with JSON schema validation and `STRAW_*` environment variable overrides.
- Shared across all instances of a component within a deployment.
- Topics: NATS connection, database URLs, TLS certificates, log level, metrics endpoint, body transport thresholds, ClickHouse connection.

**Dynamic (Runtime) Configuration**

Persisted in Postgres and managed through the Admin/Operator REST APIs. Hot-reloadable without restart.

- Loaded from Postgres on demand and cached in memory by Control instances.
- May be scoped to tenants, tags, pools, or the global system.
- Topics: routing rules, API keys, tenant settings, worker credentials, quotas, fingerprint profiles, injection policies, payload capture policy, deny rules, worker disable/drain state.

**Reload Categories**

Dynamic config changes fall into three reload categories:

| Category | Propagation | Examples |
|---|---|---|
| **Immediate** | Takes effect on the next request processed by Control. In-flight requests are unaffected. | Routing rules, injection policies, quotas, deny rules, rate limits, payload capture toggle |
| **Next Heartbeat** | Takes effect when the worker next reports health or Control detects the change on the next heartbeat cycle. | Worker credential rotation, capability overrides, worker disable/drain state |
| **Requires Restart** | Only applied at process startup. Control and Egress must be restarted for changes to take effect. | TLS certificate paths, NATS connection strings, MITM CA config, log level, metrics endpoint, body transport thresholds |

**Hot-Reload Mechanism**

When dynamic config changes, Control publishes an invalidation event to Redis pub/sub on channel `straw:config:invalidate:{tenant_id}`. All Control instances subscribe to this channel and reload the affected tenant's full config snapshot from Postgres.

- **Snapshot reload**: Control reloads the entire tenant config snapshot atomically from Postgres, not individual field patches.
- **In-flight consistency**: In-flight requests use the snapshot captured at request start. Only new requests see the updated config.
- **Grace period**: After invalidation, Control reloads immediately (no artificial delay). Thundering herd is prevented by the atomic snapshot reload and the fact that invalidation is per-tenant, not global.
- **Failure fallback**: If a Control instance fails to reload config after invalidation, it falls back to the previous in-memory snapshot and logs a warning. The instance continues serving requests with the old config while retrying the reload. A P2 alert fires if reload failure persists for more than 30 seconds.

### 20.2 Config File Format and Conventions

All components use YAML config files with `STRAW_*` environment variable overrides. JSON schema files are provided for editor auto-complete and static validation.

**Naming Convention**

- Environment variables use the `STRAW_` prefix followed by the component name and configuration key, in uppercase with underscores.
- YAML config keys use kebab-case, nested under component names.
- Environment variables override YAML values at load time. If both are present, the environment variable takes precedence.

**Schema Validation**

- **Compile-time**: JSON Schema files (`schema/control.config.json`, `schema/egress.config.json`) are shipped with the project. Editors with JSON Schema support provide auto-complete and inline validation.
- **Startup-time**: Control and Egress validate their config files against the JSON schema on startup. Invalid configs cause an immediate exit with a clear error message listing each validation failure.
- **Runtime**: Dynamic config changes submitted via the REST API are validated against the protobuf schema before acceptance. Invalid changes are rejected with a reason. Partial updates are not applied — the entire change is atomic.

**Config File Layout**

Each component has its own config file:

- Control: `control.yaml`
- Egress Worker: `egress.yaml`
- Provider Adapter: `adapter.yaml` (if used)

Config files support YAML anchors and references for reusable fragments. Environment variable substitution uses `${STRAW_*}` syntax for values that must come from the environment (e.g., secrets).

### 20.3 Static Configuration Reference

#### 20.3.1 Control Server Static Configuration

```yaml
# control.yaml
control:
  server:
    host: "0.0.0.0"
    port: 8080
    read_timeout_ms: 30000
    write_timeout_ms: 30000

  # REST API entrypoint
  api:
    host: "0.0.0.0"
    port: 8081
    max_request_body_bytes: 0  # 0 = unlimited
    request_timeout_ms: 60000

  # HTTP forward proxy entrypoint
  http_proxy:
    enabled: true
    host: "0.0.0.0"
    port: 8082

  # CONNECT tunnel proxy entrypoint
  connect_proxy:
    enabled: true
    host: "0.0.0.0"
    port: 8083
    handshake_timeout_ms: 10000

  # MITM proxy entrypoint
  mitm:
    enabled: true
    host: "0.0.0.0"
    port: 8084
    ca_key_path: "/etc/straw/mitm/ca.key"
    ca_cert_path: "/etc/straw/mitm/ca.pem"
    ca_key_password_env: "STRAW_MITM_CA_KEY_PASSWORD"
    cert_validity_days: 365
    strict_verify_outbound: true
    supported_tls_versions: ["1.2", "1.3"]

  # Inbound TLS termination (tls-client stack)
  tls:
    cert_path: "/etc/straw/tls/server.crt"
    key_path: "/etc/straw/tls/server.key"
    supported_versions: ["1.2", "1.3"]
    cipher_suites: []  # empty = tls-client defaults

  # NATS connection (all static)
  nats:
    servers:
      - "nats://nats:4222"
    user_credentials_file: "/etc/straw/nats/creds.creds"
    tls_cert_path: "/etc/straw/nats/client.crt"
    tls_key_path: "/etc/straw/nats/client.key"
    tls_ca_path: "/etc/straw/nats/ca.crt"
    reconnect_attempts: 10
    reconnect_wait_ms: 2000
    ping_interval_ms: 30000
    max_ping_failures: 3
    flush_interval_ms: 100
    buffer_size_mb: 32
    max_reconnect_buffer_mb: 256

  # Database connections
  database:
    postgres:
      dsn_env: "STRAW_POSTGRES_DSN"
      max_open_conns: 20
      max_idle_conns: 5
      conn_max_lifetime_minutes: 30
    redis:
      url_env: "STRAW_REDIS_URL"
      max_open_conns: 10
      conn_max_lifetime_minutes: 10
    clickhouse:
      url: "http://clickhouse:8123"
      database: "straw"
      username: "straw"
      password_env: "STRAW_CLICKHOUSE_PASSWORD"
      max_conns: 10
      conn_max_lifetime_minutes: 30
      async_write: true
      write_batch_size: 1000
      write_flush_interval_ms: 1000

  # Body transport
  body_transport:
    large_body_threshold_bytes: 1048576  # 1MB
    object_storage:
      enabled: false
      endpoint: "https://s3.amazonaws.com"
      bucket: "straw-bodies"
      region: "us-east-1"
      access_key_env: "STRAW_S3_ACCESS_KEY"
      secret_key_env: "STRAW_S3_SECRET_KEY"
      body_retention_days: 1
    direct_stream:
      enabled: false
      endpoint: "http://body-stream:9090"
      stream_timeout_ms: 300000

  # Observability (all static)
  observability:
    logging:
      level: "info"
      format: "json"
      output: ["stdout"]
      structured_fields:
        - "request_id"
        - "tenant_id"
        - "trace_id"
        - "worker_id"
    metrics:
      enabled: true
      path: "/metrics"
      host: "0.0.0.0"
      port: 9090
      custom_prefix: "straw_"
    tracing:
      enabled: false
      exporter: "jaeger"  # jaeger | tempo
      endpoint: "http://jaeger:14268/api/traces"
      sampling_rate: 0.1
      propagate_trace_context: true
      headers:
        - "traceparent"
        - "tracestate"

  # Config management
  config:
    schema_dir: "/etc/straw/schemas"
    drift_detection: true
    drift_check_interval_seconds: 60
```

#### 20.3.2 Egress Worker Static Configuration

```yaml
# egress.yaml
egress:
  worker_id: "egress-001"

  # NATS connection (all static, same structure as Control)
  nats:
    servers:
      - "nats://nats:4222"
    user_credentials_file: "/etc/straw/nats/creds.creds"
    tls_cert_path: "/etc/straw/nats/client.crt"
    tls_key_path: "/etc/straw/nats/client.key"
    tls_ca_path: "/etc/straw/nats/ca.crt"
    reconnect_attempts: 10
    reconnect_wait_ms: 2000
    ping_interval_ms: 30000
    max_ping_failures: 3

  # Worker capabilities (advertised on registration)
  capabilities:
    pool_names: ["default"]
    tags: ["datacenter", "us-west"]
    countries: ["US"]
    regions: ["us-west-1"]
    ip_types: ["datacenter"]
    supported_ingress_modes: ["rest", "http_proxy", "connect", "mitm"]

  # Browser fingerprint (tls-client preset)
  fingerprint:
    # Preset name must match a tls-client built-in preset
    # Examples: "chrome_120", "firefox_121", "safari_17"
    # See tls-client preset documentation for the full list
    default_preset: "chrome_120"

  # Outbound TLS
  outbound_tls:
    strict_verify: true
    ca_bundle_path: "/etc/straw/tls/ca-bundle.crt"
    supported_versions: ["1.2", "1.3"]

  # Upstream proxy chaining (optional)
  upstream_proxy:
    enabled: false
    type: "http"  # http | socks5
    host: "proxy.example.com"
    port: 8080
    username_env: "STRAW_UPSTREAM_PROXY_USERNAME"
    password_env: "STRAW_UPSTREAM_PROXY_PASSWORD"

  # Body transport (same structure as Control)
  body_transport:
    large_body_threshold_bytes: 1048576
    object_storage:
      enabled: false
      endpoint: "https://s3.amazonaws.com"
      bucket: "straw-bodies"
      region: "us-east-1"
      access_key_env: "STRAW_S3_ACCESS_KEY"
      secret_key_env: "STRAW_S3_SECRET_KEY"
    direct_stream:
      enabled: false
      endpoint: "http://body-stream:9090"
      stream_timeout_ms: 300000

  # Heartbeat
  heartbeat:
    interval_ms: 5000
    timeout_ms: 15000  # Control considers worker dead after this without heartbeat

  # TLS for inbound (if Egress exposes a local health endpoint)
  tls:
    cert_path: "/etc/straw/tls/server.crt"
    key_path: "/etc/straw/tls/server.key"

  # Observability
  observability:
    logging:
      level: "info"
      format: "json"
      output: ["stdout"]
```

#### 20.3.3 Provider Adapter Static Configuration

```yaml
# adapter.yaml
adapter:
  worker_id: "adapter-001"

  # NATS connection (same structure as Control/Egress)
  nats:
    servers:
      - "nats://nats:4222"
    user_credentials_file: "/etc/straw/nats/creds.creds"

  # Provider configuration
  providers:
    - name: "bright_data"
      type: "bright_data"
      api_endpoint: "https://api.brightdata.com"
      api_key_env: "STRAW_BRIGHT_DATA_KEY"
      accounts:
        - id: "account-1"
          session_limit: 100
          max_concurrency: 50
        - id: "account-2"
          session_limit: 100
          max_concurrency: 50
      pool_names: ["bright-data-pool"]
      tags: ["residential"]
      countries: []  # empty = all
      ip_types: ["residential"]

    - name: "custom_upstream"
      type: "upstream_proxy"
      upstream_proxy:
        type: "http"
        host: "proxy.provider.com"
        port: 443
        username_env: "STRAW_UPSTREAM_USERNAME"
        password_env: "STRAW_UPSTREAM_PASSWORD"
      pool_names: ["upstream-pool"]
      tags: ["upstream"]
      countries: []
      ip_types: ["datacenter"]

  # Per-account load balancing
  load_balancing:
    strategy: "round_robin"  # round_robin | least_loaded | random
    account_rotation_enabled: true

  # Heartbeat
  heartbeat:
    interval_ms: 5000
    timeout_ms: 15000

  # Observability
  observability:
    logging:
      level: "info"
      format: "json"
      output: ["stdout"]
```

### 20.4 NATS Configuration Reference

NATS configuration is first-class and entirely static. All NATS settings are documented here, including the subject topology.

#### 20.4.1 Connection Parameters

| Parameter | YAML Key | Env Var | Default | Description |
|---|---|---|---|---|
| Servers | `nats.servers` | `STRAW_NATS_URLS` (comma-separated) | `nats://localhost:4222` | NATS server connection URLs |
| Credentials File | `nats.user_credentials_file` | `STRAW_NATS_CREDS_FILE` | — | Path to NATS NKey/JWT credentials file |
| TLS Cert | `nats.tls_cert_path` | `STRAW_NATS_TLS_CERT` | — | Client TLS certificate path |
| TLS Key | `nats.tls_key_path` | `STRAW_NATS_TLS_KEY` | — | Client TLS private key path |
| TLS CA | `nats.tls_ca_path` | `STRAW_NATS_TLS_CA` | — | CA certificate for server verification |
| Reconnect Attempts | `nats.reconnect_attempts` | `STRAW_NATS_RECONNECT_ATTEMPTS` | `10` | Max reconnect attempts before giving up |
| Reconnect Wait | `nats.reconnect_wait_ms` | `STRAW_NATS_RECONNECT_WAIT_MS` | `2000` | Wait between reconnect attempts (ms) |
| Ping Interval | `nats.ping_interval_ms` | `STRAW_NATS_PING_INTERVAL_MS` | `30000` | NATS server ping interval (ms) |
| Max Ping Failures | `nats.max_ping_failures` | `STRAW_NATS_MAX_PING_FAILURES` | `3` | Consecutive ping failures before disconnect |
| Flush Interval | `nats.flush_interval_ms` | `STRAW_NATS_FLUSH_INTERVAL_MS` | `100` | Max time between NATS flushes (ms) |
| Buffer Size | `nats.buffer_size_mb` | `STRAW_NATS_BUFFER_SIZE_MB` | `32` | NATS client send buffer size (MB) |
| Max Reconnect Buffer | `nats.max_reconnect_buffer_mb` | `STRAW_NATS_MAX_RECONNECT_BUFFER_MB` | `256` | Max buffer size during reconnect (MB) |

#### 20.4.2 Subject Topology

All NATS subjects follow the pattern `straw.v1.{topic}.{scope}.{action}`.

| Subject | Direction | Payload | Purpose |
|---|---|---|---|
| `straw.v1.register.>` | Worker → Control | `RegisterRequest` | Worker registration |
| `straw.v1.register.reply.{worker_id}` | Control → Worker | `RegisterAck` | Registration acknowledgement |
| `straw.v1.heartbeat.{worker_id}` | Worker → Control | `HeartbeatRequest` | Worker health report |
| `straw.v1.heartbeat.reply.{worker_id}` | Control → Worker | `HeartbeatAck` | Heartbeat acknowledgement |
| `straw.v1.dispatch.{pool_name}` | Control → Workers (queue group) | `AssignRequest` | Work assignment (queue group per pool) |
| `straw.v1.dispatch.reply.{request_id}` | Worker → Control | `AssignAck` | Assignment acknowledgement |
| `straw.v1.stream.c2e.{request_id}` | Worker → Control (stream) | `StreamFrame` | Client-to-executor upload / tunnel bytes |
| `straw.v1.stream.e2c.{request_id}` | Control → Worker (stream) | `StreamFrame` | Executor-to-Control response / tunnel bytes |
| `straw.v1.cancel.{request_id}` | Worker → Control | `StreamFrame` (cancel) | Request cancellation |
| `straw.v1.config.invalidate.{tenant_id}` | Control → Control (Redis) | — | Config invalidation broadcast |

#### 20.4.3 Queue Groups

Pool-based load balancing uses NATS queue groups:

- Each executor pool maps to a NATS queue group: `straw.dispatch.pool.{pool_name}`
- Workers join the queue group by subscribing to the dispatch subject with the pool name as the queue group
- When Control publishes to `straw.v1.dispatch.{pool_name}`, NATS delivers the message to exactly one worker in the queue group
- New workers automatically join the queue group on registration
- Worker removal (heartbeat timeout, disable) causes NATS to redeliver pending messages to other queue members

#### 20.4.4 NATS Message Cleanup

- NATS does not retain old messages after delivery. Each `AssignRequest` is a one-shot request/reply.
- `StreamFrame` messages are transient and delivered in real-time; no persistence is configured.
- No stream retention or disk storage is needed for the default NATS deployment.

### 20.5 Dynamic Configuration Reference

#### 20.5.1 Tenant Configuration

Each tenant is a top-level config entity with the following fields:

| Field | Type | Description |
|---|---|---|
| `id` | string | Unique tenant identifier (set at creation) |
| `name` | string | Human-readable tenant name |
| `status` | enum | `active`, `suspended`, `deleted` |
| `created_at` | unix_ms | Tenant creation timestamp |
| `updated_at` | unix_ms | Last config update timestamp |
| `config_version` | uint64 | Monotonically increasing config version number |
| `rate_limits` | object | Tenant-level rate limit dimensions (see §20.8) |
| `quotas` | object | Tenant-level quota limits (see §20.8) |
| `deny_rules` | array | Tenant-level destination deny rules (see §20.8) |
| `payload_capture_enabled` | bool | Global payload capture toggle for this tenant |
| `default_fingerprint_profile` | string | Default fingerprint preset name |

**Config Versioning**

Every dynamic config change increments the `config_version` field atomically in Postgres. The version is used for:

- **Optimistic concurrency**: Updates include the expected `config_version`. If the current version differs, the update is rejected with a `conflict` error, and the client must reload and retry.
- **Rollback**: Each config change is stored as an immutable audit record with its `config_version`. Admins can rollback to any previous version, creating a new version that reverts the changes.
- **Audit trail**: All config changes are logged to ClickHouse with `config_version`, `changed_by` (user_id), `action` (create/update/rollback), `field_path`, `old_value`, `new_value`, and `timestamp`.

#### 20.5.2 API Key Configuration

Each API key is a child resource under a tenant:

| Field | Type | Description |
|---|---|---|
| `id` | string | Unique API key identifier |
| `tenant_id` | string | Parent tenant |
| `key_hash` | string | SHA-256 hash of the API key (never stored in plaintext) |
| `key_prefix` | string | First 8 characters of the key (for identification; shown only at creation) |
| `user_id` | string | Owning user |
| `role` | enum | `admin`, `operator`, `viewer` |
| `status` | enum | `active`, `revoked` |
| `created_at` | unix_ms | Key creation timestamp |
| `revoked_at` | unix_ms | Key revocation timestamp (null if active) |
| `last_used_at` | unix_ms | Last successful authentication timestamp |

**Key Rotation**

- Multiple active keys are supported simultaneously. During rotation, both old and new keys remain valid.
- Revocation is immediate: the key hash is marked `revoked` in Postgres, the config version is bumped, and a Redis pub/sub invalidation is published. Control evicts the key from its in-memory cache immediately.
- In-flight requests authenticated with a revoked key complete normally. New requests using the revoked key are rejected with `invalid_api_key`.
- Keys are never deleted; they are soft-revoked and retained for audit purposes.

#### 20.5.3 Routing Rules Configuration

Each routing rule is a tenant-scoped config entity:

| Field | Type | Description |
|---|---|---|
| `id` | string | Unique rule identifier |
| `tenant_id` | string | Parent tenant |
| `priority` | int32 | Evaluation priority (lower number = higher priority). Rules evaluated in ascending priority order. |
| `enabled` | bool | Whether the rule is active. Disabled rules are skipped during evaluation. |
| `match_conditions` | object | Match criteria (all required fields must match) |
| `target_pool_id` | string | Target executor pool (Egress or Provider Adapter) |
| `fallback_pool_ids` | array | Ordered list of fallback pool IDs. Applied in order if the primary pool has no eligible executors. |
| `sticky_session_ttl_seconds` | int32 | Sticky session TTL for this rule. 0 = no sticky sessions. |
| `allow_sticky_fallback` | bool | If true, allow falling back to a different executor when the sticky session target is unavailable. |
| `config_version` | uint64 | Config version at time of this rule's creation |

**Match Conditions Schema**

```json
{
  "tags": ["string"],
  "country": "ISO-3166-alpha-2",
  "region": "string",
  "ip_type": "datacenter | residential | mobile | isp | unknown",
  "ingress_type": "rest | http_proxy | connect | mitm",
  "target_host": "string"
}
```

- `tags`: All requested tags must be present on the target executor.
- `country`: Strict ISO-3166 alpha-2 country code match.
- `region`: Optional executor-advertised region match.
- `ip_type`: Must match the executor's advertised IP type.
- `ingress_type`: Optional match for the client's ingress mode.
- `target_host`: Exact host or suffix domain match (e.g., `*.example.com`).

**Evaluation Order**

1. Rules are sorted by `priority` (ascending).
2. For each enabled rule, match conditions are evaluated against the request hints.
3. If all match conditions are satisfied, the rule's `target_pool_id` is selected.
4. If the target pool has no eligible executors, fallback pools in `fallback_pool_ids` are tried in order.
5. If no rule matches, the request fails with `route_no_match`.
6. If all pools are unavailable, the request fails with `route_unavailable`.
7. There is no implicit default route — admins must create a catch-all rule if desired.

**Rule Matching and Client Hints**

Any hint the client provides is a hard constraint. Missing hints mean no preference. Fallback may relax admin preferences but never client hints.

#### 20.5.4 Fingerprint Profiles Configuration

Fingerprint profiles are fixed `tls-client` built-in presets. Operators select by preset name.

| Field | Type | Description |
|---|---|---|
| `preset_name` | string | `tls-client` preset identifier (e.g., `chrome_120`, `firefox_121`, `safari_17`) |
| `display_name` | string | Human-readable name (e.g., "Chrome 120 on Windows") |

**Built-in Presets**

The following `tls-client` presets are available (subject to the `tls-client` version in use):

| Preset Name | Display Name | Browser | Version | Platform |
|---|---|---|---|---|
| `chrome_120` | Chrome 120 | Chrome | 120.0.0.0 | Windows 10 |
| `chrome_121` | Chrome 121 | Chrome | 121.0.0.0 | Windows 10 |
| `chrome_123` | Chrome 123 | Chrome | 123.0.0.0 | macOS 14 |
| `chrome_131` | Chrome 131 | Chrome | 131.0.0.0 | Android 14 |
| `firefox_120` | Firefox 120 | Firefox | 120.0 | Windows 10 |
| `firefox_121` | Firefox 121 | Firefox | 121.0 | Windows 10 |
| `firefox_125` | Firefox 125 | Firefox | 125.0 | macOS 14 |
| `safari_16` | Safari 16 | Safari | 16.0 | macOS 13 |
| `safari_17` | Safari 17 | Safari | 17.0 | iOS 17 |

**Selection Flow**

1. Client includes `fingerprint_profile` field in the request with a preset name.
2. If not specified, the tenant's `default_fingerprint_profile` is used.
3. Control validates the preset name against the known preset list.
4. Control sends the preset name to Egress via the `fingerprint_preset` protobuf enum.
5. Egress maps the enum to the corresponding `tls-client` preset and applies it immediately before making the outbound request.
6. If the preset is unsupported or the enum value is unknown, Egress returns an `unsupported_fingerprint` error.

**Fingerprint Mutability**

Preset definitions are immutable — they come from `tls-client` and cannot be modified. Operators can change the `default_fingerprint_profile` per tenant, which takes effect on the next request (immediate category).

#### 20.5.5 Header and Cookie Injection Policies

Each injection policy is a tenant-scoped config entity:

| Field | Type | Description |
|---|---|---|
| `id` | string | Unique policy identifier |
| `tenant_id` | string | Parent tenant |
| `name` | string | Human-readable policy name |
| `enabled` | bool | Whether the policy is active |
| `match_conditions` | object | Conditions for applying this policy |
| `operations` | array | Ordered list of header/cookie operations |
| `config_version` | uint64 | Config version at time of creation |

**Match Conditions**

```json
{
  "tags": ["string"],
  "target_host": "string"
}
```

- `tags`: All requested tags must be present.
- `target_host`: Exact host or suffix domain match.

**Operations Schema**

Operations are executed in array order for deterministic behavior:

```json
{
  "type": "add_header | set_header | remove_header | add_cookie | set_cookie",
  "name": "string",
  "value": "string"
}
```

- `add_header`: Appends a header (may result in duplicates).
- `set_header`: Sets a header, overwriting any existing value.
- `remove_header`: Removes a header by name.
- `add_cookie`: Appends a cookie to the `Cookie` header.
- `set_cookie`: Sets a cookie value (overwrites existing cookie by name).

**Straw Header Stripping**

The following headers are stripped by Straw before reaching the target and are never injected or forwarded:

| Header | Reason |
|---|---|
| `X-Straw-Request-Id` | Internal request correlation |
| `X-Straw-Tenant-Id` | Internal tenant identification |
| `X-Straw-Trace-Id` | Internal trace correlation |
| `X-Straw-Worker-Id` | Internal worker identification (never exposed to clients) |
| `X-Straw-Routing-Rule` | Internal routing metadata |
| `X-Straw-Fingerprint` | Internal fingerprint instruction |
| `Proxy-Authorization` (on outbound) | Straw manages its own auth; client proxy auth is not forwarded |

#### 20.5.6 Worker Credential Configuration

Worker credentials are generated by the Admin API and distributed to Egress workers.

**Credential Structure**

| Field | Type | Description |
|---|---|---|
| `credential_id` | string | Unique public identifier (used in NATS handshake) |
| `tenant_id` | string | Tenant scope |
| `pool_scope` | array | Pool names this credential can register to |
| `signing_algorithm` | string | `Ed25519` (recommended) |
| `public_key` | string | Base64-encoded public key (shown at creation) |
| `private_key` | string | Base64-encoded private key (shown **once** at creation, never stored again) |
| `status` | enum | `active`, `revoked` |
| `created_at` | unix_ms | Credential creation timestamp |
| `revoked_at` | unix_ms | Revocation timestamp (null if active) |

**Signing Algorithm**

Credentials use Ed25519 for cryptographic signing. The private key signs a token containing `credential_id`, `tenant_id`, `pool_scope`, and a timestamp. Control verifies the signature statelessly using the stored public key before querying Postgres or Redis.

**Credential Lifecycle**

1. **Create**: Admin calls `POST /api/v1/config/worker-credentials`. Control generates an Ed25519 key pair, stores the public key and metadata in Postgres, and returns the full credential (including the private key) to the admin.
2. **Distribute**: The admin copies the private key to the Egress worker's config file or sets it via `STRAW_WORKER_PRIVATE_KEY`. The private key is never transmitted over the network in plaintext after this point.
3. **Register**: On startup, the Egress worker signs a registration token with its private key and sends it to Control via the NATS `RegisterRequest`.
4. **Verify**: Control verifies the signature statelessly using the stored public key. If valid, the worker is registered and a `session_id` is assigned.
5. **Rotate**: If a credential is compromised or rotated:
    - Admin calls `POST /api/v1/config/worker-credentials/{id}/revoke`. The credential is marked `revoked` in Postgres.
    - Admin calls `POST /api/v1/config/worker-credentials` to generate a new credential.
    - The new private key is distributed to the Egress worker (config update or env var change).
    - The worker restarts and re-registers with the new credential.
    - Old requests authenticated with the revoked credential complete normally. New registrations are rejected with `REJECTED_AUTH`.
6. **Revoke**: Immediate revocation. The credential is marked `revoked`, the config version is bumped, and a Redis invalidation is published. Control rejects new registrations with the revoked credential.

#### 20.5.7 Payload Capture Configuration

| Field | Type | Description |
|---|---|---|
| `enabled` | bool | Whether payload capture is active (tenant-level or global) |
| `capture_request_headers` | bool | Capture inbound request headers |
| `capture_request_body` | bool | Capture inbound request body |
| `capture_response_headers` | bool | Capture outbound response headers |
| `capture_response_body` | bool | Capture outbound response body |
| `redaction_rules` | array | Automatic redaction rules for sensitive data |
| `by_tag` | object | Per-tag capture overrides: `{ "tag_name": { "enabled": bool, ... } }` |

**Capture Scope**

- Global toggle: Admin sets a system-wide `payload_capture_enabled` flag.
- Per-tenant override: Each tenant has its own `payload_capture_enabled` flag that overrides the global setting.
- Per-tag override: Within a tenant, specific tags can enable or disable capture independently.

**Redaction Rules**

Automatic redaction is applied before storage. Redaction rules match on header names or body content patterns:

| Rule Type | Match | Action |
|---|---|---|
| Header name | Exact header name (e.g., `Authorization`, `Cookie`) | Redact value to `[REDACTED]` |
| Body pattern | Regex pattern (e.g., `\d{3}-\d{2}-\d{4}` for SSN) | Redact matched content |
| Field path | JSON path (e.g., `$.password`, `$.credit_card`) | Redact field value |

**Storage**

Captured payloads are stored in ClickHouse in the `payloads` table (see §20.10). Retention is controlled by the ClickHouse retention policy.

**Audit**

All payload capture enable/disable actions are logged to ClickHouse with `changed_by`, `scope` (global/tenant/tag), `old_value`, `new_value`, and `timestamp`.

**Role Restriction**

Only the `admin` role can toggle payload capture. The `operator` and `viewer` roles cannot modify this setting.

#### 20.5.8 Rate Limits and Quotas Configuration

**Rate Limits**

Rate limits are configured per tenant with four dimensions. Each dimension has a configurable limit:

| Dimension | Config Key | Example |
|---|---|---|
| Global tenant | `rate_limits.global.requests_per_minute` | `100` |
| Tenant + API key | `rate_limits.key.requests_per_minute` | `50` |
| Tenant + target host | `rate_limits.host.requests_per_minute` | `200` |
| Tenant + IP type | `rate_limits.ip_type.requests_per_minute` | `150` |

- **Algorithm**: Redis Sliding Window Log using Sorted Sets (`ZSET`).
- **Burst allowance**: None. Strict hard-cap with no burst.
- **Enforcement**: Immediate drop. Returns `rate_limit_exceeded` error and HTTP `429 Too Many Requests`.

**Quotas**

Quotas are configured per tenant with monthly resets:

| Config Key | Example | Description |
|---|---|---|
| `quotas.monthly_requests` | `100000` | Max requests per month |
| `quotas.monthly_bandwidth_bytes` | `53687091200` | Max bandwidth per month (50 GB) |

- **Reset cadence**: Fixed monthly resets on the 1st of each month.
- **Enforcement**: Evaluated at request start and end. If breached during an active request, the request finishes. Subsequent requests are blocked with `quota_exhausted` error until reset.

**Destination Deny Rules**

Deny rules block outbound transport to restricted IPs, domains, or network ranges:

| Field | Type | Description |
|---|---|---|
| `id` | string | Unique rule identifier |
| `tenant_id` | string | Parent tenant (empty = global admin rule) |
| `type` | enum | `ip_cidr`, `domain`, `full_url` |
| `value` | string | CIDR block, domain pattern, or full URL |
| `enabled` | bool | Whether the rule is active |
| `config_version` | uint64 | Config version at creation |

- Deny rules are evaluated before routing. If a match is found, the request is rejected immediately with `destination_denied`.
- Global deny rules (empty `tenant_id`) apply to all tenants. Per-tenant deny rules apply only to that tenant.

### 20.6 TLS and MITM Configuration

#### 20.6.1 MITM CA Configuration

The MITM CA is used to intercept and decrypt HTTPS traffic for decoded request/response handling.

| Config Key | Env Var | Description |
|---|---|---|
| `control.mitm.ca_key_path` | — | Path to the MITM CA private key file |
| `control.mitm.ca_cert_path` | — | Path to the MITM CA public certificate file |
| `control.mitm.ca_key_password_env` | `STRAW_MITM_CA_KEY_PASSWORD` | Env var containing the CA key password (if encrypted) |
| `control.mitm.cert_validity_days` | `STROW_MITM_CERT_VALIDITY_DAYS` | Default validity period for dynamically generated intermediate certificates |

- The CA key and certificate are loaded at startup. Changes require a Control restart.
- Intermediate certificates are generated on-demand for each SNI encountered. They are signed by the MITM CA and inherit the `cert_validity_days` TTL.
- Intermediate certificates are cached in memory. Expired certificates are regenerated on the next request.

#### 20.6.2 Inbound TLS (Control)

Control uses the `tls-client` stack for inbound TLS termination to handle HTTP/2 ALPN and SNI negotiation that matches client expectations.

| Config Key | Env Var | Description |
|---|---|---|
| `control.tls.cert_path` | `STRAW_TLS_CERT` | Server TLS certificate path |
| `control.tls.key_path` | `STRAW_TLS_KEY` | Server TLS private key path |
| `control.tls.supported_versions` | `STRAW_TLS_VERSIONS` | Supported TLS versions (e.g., `["1.2", "1.3"]`) |
| `control.tls.cipher_suites` | — | Cipher suites (empty = `tls-client` defaults) |

- Certificate paths are static (restart required).
- `tls-client` handles JA3/JA4 fingerprint matching for inbound connections, ensuring proxy clients' TLS fingerprints are not flagged as non-browser.

#### 20.6.3 Outbound TLS (Egress)

Egress workers control outbound TLS via `tls-client`.

| Config Key | Env Var | Description |
|---|---|---|
| `egress.outbound_tls.strict_verify` | `STRAW_OUTBOUND_TLS_STRICT_VERIFY` | Enforce strict certificate verification (default: true) |
| `egress.outbound_tls.ca_bundle_path` | `STRAW_OUTBOUND_TLS_CA_BUNDLE` | Path to custom CA bundle for private/internal certificates |
| `egress.outbound_tls.supported_versions` | `STRAW_OUTBOUND_TLS_VERSIONS` | Supported outbound TLS versions |

- `strict_verify` is true by default. Setting it to false is not recommended for production.
- The CA bundle allows Egress to verify certificates from private/internal CAs.

#### 20.6.4 CA Trust Distribution

Operators must distribute the MITM CA certificate to their scraping clients so clients trust intercepted HTTPS traffic:

- The CA certificate is available at `control.mitm.ca_cert_path` (typically `/etc/straw/mitm/ca.pem`).
- Distribution methods:
    - **Docker**: Mount the CA cert into the client container and set `SSL_CERT_FILE` or `NODE_EXTRA_CA_CERTS`.
    - **Headless browsers**: Import the CA into the system trust store or browser profile.
    - **HTTP clients**: Configure the client to trust the CA (e.g., `curl --cacert /path/to/ca.pem`, `requests.verify=/path/to/ca.pem`).
- Straw does not automate CA distribution. This is an operator responsibility.

### 20.7 Large-Body Transport Configuration

#### 20.7.1 Transport Selection Flow

| Body Size | Transport | Config Key |
|---|---|---|
| ≤ `large_body_threshold_bytes` | NATS `StreamFrame` messages | `control.body_transport.large_body_threshold_bytes` |
| > `large_body_threshold_bytes` | Object storage or direct streaming | `control.body_transport.object_storage.enabled` or `direct_stream.enabled` |

If both object storage and direct streaming are enabled, object storage is preferred. Direct streaming is used as a fallback if object storage is unavailable.

#### 20.7.2 Object Storage Configuration

| Config Key | Env Var | Default | Description |
|---|---|---|---|
| `body_transport.object_storage.enabled` | `STROW_BODY_OBJECT_STORAGE_ENABLED` | `false` | Enable object storage for large bodies |
| `body_transport.object_storage.endpoint` | `STRAW_BODY_S3_ENDPOINT` | — | S3-compatible API endpoint |
| `body_transport.object_storage.bucket` | `STRAW_BODY_S3_BUCKET` | — | Bucket name |
| `body_transport.object_storage.region` | `STRAW_BODY_S3_REGION` | — | Region |
| `body_transport.object_storage.access_key_env` | `STRAW_S3_ACCESS_KEY` | — | Env var for access key |
| `body_transport.object_storage.secret_key_env` | `STRAW_S3_SECRET_KEY` | — | Env var for secret key |
| `body_transport.object_storage.body_retention_days` | `STRAW_BODY_RETENTION_DAYS` | `1` | Days to retain stored bodies |

#### 20.7.3 Direct Streaming Configuration

| Config Key | Env Var | Default | Description |
|---|---|---|---|
| `body_transport.direct_stream.enabled` | `STRAW_BODY_DIRECT_STREAM_ENABLED` | `false` | Enable direct streaming for large bodies |
| `body_transport.direct_stream.endpoint` | `STRAW_BODY_STREAM_ENDPOINT` | — | Streaming server endpoint URL |
| `body_transport.direct_stream.stream_timeout_ms` | `STRAW_BODY_STREAM_TIMEOUT_MS` | `300000` | Max time for a stream to complete (ms) |

### 20.8 Observability Configuration

All observability settings are static (restart required).

#### 20.8.1 Logging

| Config Key | Env Var | Default | Description |
|---|---|---|---|
| `observability.logging.level` | `STRAW_LOG_LEVEL` | `info` | Log level: `debug`, `info`, `warn`, `error` |
| `observability.logging.format` | `STRAW_LOG_FORMAT` | `json` | Log format: `json`, `text` |
| `observability.logging.output` | `STRAW_LOG_OUTPUT` | `["stdout"]` | Output destinations: `stdout`, `stderr`, `file:/path/to/log` |
| `observability.logging.structured_fields` | — | (see §20.3.1) | Required structured fields in every log entry |

#### 20.8.2 Metrics

| Config Key | Env Var | Default | Description |
|---|---|---|---|
| `observability.metrics.enabled` | `STRAW_METRICS_ENABLED` | `true` | Enable Prometheus metrics |
| `observability.metrics.path` | `STRAW_METRICS_PATH` | `/metrics` | HTTP path for metrics endpoint |
| `observability.metrics.host` | `STRAW_METRICS_HOST` | `0.0.0.0` | Bind address |
| `observability.metrics.port` | `STRAW_METRICS_PORT` | `9090` | Bind port |
| `observability.metrics.custom_prefix` | `STRAW_METRICS_PREFIX` | `straw_` | Metric name prefix |

#### 20.8.3 Tracing

| Config Key | Env Var | Default | Description |
|---|---|---|---|
| `observability.tracing.enabled` | `STRAW_TRACING_ENABLED` | `false` | Enable distributed tracing |
| `observability.tracing.exporter` | `STRAW_TRACING_EXPORTER` | `jaeger` | Trace exporter: `jaeger`, `tempo` |
| `observability.tracing.endpoint` | `STRAW_TRACING_ENDPOINT` | — | Trace collector endpoint URL |
| `observability.tracing.sampling_rate` | `STRAW_TRACING_SAMPLING_RATE` | `0.1` | Sampling rate (0.0 to 1.0) |
| `observability.tracing.propagate_trace_context` | `STRAW_TRACING_PROPAGATE` | `true` | Propagate trace context headers |
| `observability.tracing.headers` | — | (see §20.3.1) | Trace context headers to propagate |

### 20.9 ClickHouse Schema

ClickHouse stores operational analytics, audit logs, and payload captures.

#### 20.9.1 Database Layout

| Database | Purpose |
|---|---|
| `audit` | Configuration change audit trail |
| `requests` | Request-level operational logs |
| `workers` | Worker health and lifecycle events |
| `payloads` | Captured request/response payloads |
| `metrics` | Aggregated metrics for dashboards |

#### 20.9.2 Table Schemas

**`audit.config_changes`** — Configuration change audit trail

```sql
CREATE TABLE audit.config_changes
(
    id           UInt64,           -- Auto-incrementing unique ID
    tenant_id    LowCardinality(String),
    config_type  LowCardinality(String),  -- tenant | api_key | routing_rule | injection_policy | worker_credential | quota | deny_rule | payload_capture
    action       LowCardinality(String),  -- create | update | rollback | delete
    config_version UInt64,
    changed_by   String,           -- user_id of the admin/operator
    field_path   String,           -- Dot-notation path of changed field (e.g., "routing_rules[0].priority")
    old_value    String,           -- JSON-encoded previous value
    new_value    String,           -- JSON-encoded new value
    timestamp    DateTime64(3, 'UTC')
)
ENGINE = MergeTree
PARTITION BY toYYYYMM(timestamp)
ORDER BY (tenant_id, config_type, timestamp, id)
TTL timestamp + INTERVAL 90 DAY;
```

**`requests.requests`** — Request-level operational logs

```sql
CREATE TABLE requests.requests
(
    request_id       String,
    tenant_id        LowCardinality(String),
    trace_id         String,
    ingress_type     LowCardinality(String),
    method           LowCardinality(String),
    target_host      String,
    target_url       String,
    fingerprint_profile String,
    selected_worker  String,
    selected_pool    String,
    routing_rule     String,
    country          LowCardinality(String),
    region           LowCardinality(String),
    ip_type          LowCardinality(String),
    error_code       LowCardinality(String),
    error_category   LowCardinality(String),
    upstream_status  UInt16,
    request_size_bytes    UInt64,
    response_size_bytes   UInt64,
    total_duration_ms     UInt32,
    routing_duration_ms   UInt32,
    egress_duration_ms    UInt32,
    attempt               UInt8,
    timestamp             DateTime64(3, 'UTC')
)
ENGINE = MergeTree
PARTITION BY toYYYYMM(timestamp)
ORDER BY (tenant_id, timestamp, request_id)
TTL timestamp + INTERVAL 30 DAY;
```

**`workers.worker_events`** — Worker health and lifecycle events

```sql
CREATE TABLE workers.worker_events
(
    worker_id      String,
    event_type     LowCardinality(String),  -- register | heartbeat | drain | disable | dead | unregister
    health         LowCardinality(String),  -- ready | degraded | unhealthy
    session_id     String,
    active_requests    UInt32,
    max_concurrency    UInt32,
    available_capacity UInt32,
    draining          UInt8,
    reason           String,
    timestamp         DateTime64(3, 'UTC')
)
ENGINE = MergeTree
PARTITION BY toYYYYMM(timestamp)
ORDER BY (worker_id, timestamp, event_type)
TTL timestamp + INTERVAL 90 DAY;
```

**`payloads.captured`** — Captured request/response payloads

```sql
CREATE TABLE payloads.captured
(
    request_id       String,
    tenant_id        LowCardinality(String),
    capture_scope    LowCardinality(String),  -- global | tenant | tag
    request_headers  String,           -- JSON-encoded request headers
    request_body     String,           -- JSON or raw body (redacted if applicable)
    response_headers String,           -- JSON-encoded response headers
    response_body    String,           -- JSON or raw body (redacted if applicable)
    redacted_fields  Array(String),    -- List of fields that were redacted
    captured_at      DateTime64(3, 'UTC')
)
ENGINE = MergeTree
PARTITION BY toYYYYMM(captured_at)
ORDER BY (tenant_id, captured_at, request_id)
TTL captured_at + INTERVAL 7 DAY;
```

**`metrics.aggregated`** — Aggregated metrics for dashboards

```sql
CREATE TABLE metrics.aggregated
(
    metric_name      String,
    tenant_id        LowCardinality(String),
    dimensions       Map(String, String),
    value            Float64,
    timestamp        DateTime64(3, 'UTC')
)
ENGINE = AggregatingMergeTree()
PARTITION BY toYYYYMM(timestamp)
ORDER BY (metric_name, tenant_id, dimensions, toStartOfHour(timestamp))
TTL timestamp + INTERVAL 30 DAY;
```

#### 20.9.3 Write Path

Control writes to ClickHouse asynchronously via a buffered write queue:

- Writes are batched up to `write_batch_size` (default: 1000) or flushed every `write_flush_interval_ms` (default: 1000ms).
- If ClickHouse is unavailable, writes are queued in memory up to a configurable limit. If the queue is full, the oldest entries are dropped and a P1 alert fires.
- Control never blocks on ClickHouse writes. Write failures are logged and counted as metrics.

### 20.10 Config Migration and Upgrade

#### 20.10.1 Schema Versioning

Each config file declares a `config_version` field at the top level:

```yaml
config_version: "v1"
```

- Control and Egress reject config files with incompatible `config_version` at startup, exiting with a clear error message: `unsupported config_version: v2, supported: v1`.
- The error message includes a migration guide link or inline instructions for upgrading.
- Older config formats are accepted with deprecation warnings for up to 2 minor versions before removal.

#### 20.10.2 Backward Compatibility

- Config files with older `config_version` values are accepted. A warning is logged at startup: `deprecated config_version: v1, expected v2`.
- Removed config keys are silently ignored (treated as absent) for up to 2 minor versions.
- New config keys with defaults are accepted without error.

#### 20.10.3 Zero-Downtime Upgrades

- Rolling deploys are supported: old and new Control instances coexist during upgrade.
- New Control instances load their config on startup and join the NATS cluster.
- Dynamic config changes are backward-compatible: Control instances with different versions can coexist as long as they share the same config schema version.
- If a config schema change is incompatible, the deployment must be coordinated: all instances are updated before the new config version is activated.

#### 20.10.4 Config Drift Detection

When `config.drift_detection` is enabled (default: true), Control runs a periodic drift check:

- **Interval**: Configurable via `config.drift_check_interval_seconds` (default: 60 seconds).
- **Mechanism**: Each Control instance computes a hash of its loaded static config and compares it against the hashes from other Control instances.
- **Detection**: If hashes differ, drift is detected. The differing keys are reported in the logs and a P2 alert fires.
- **Resolution**: Drift is resolved by ensuring all Control instances use the same config file. Operators should use a shared config volume or centralized config management (e.g., ConfigMap in Kubernetes).
- **Scope**: Drift detection only compares static config. Dynamic config is always sourced from Postgres, so it cannot drift between Control instances.

#### 20.10.5 Migration Scripts

For config schema version changes, migration scripts are provided:

- **CLI tool**: `straw config migrate --from v1 --to v2 --input control.yaml --output control.yaml`
- The tool validates the output against the target schema before writing.
- Manual migration steps are documented in the release notes for each version change.

### 20.11 Docker Compose and Local Development

#### 20.11.1 docker-compose.yml

```yaml
version: "3.9"

services:
  # Infrastructure
  nats:
    image: nats:2.10
    ports:
      - "4222:4222"
      - "8222:8222"  # NATS monitoring

  postgres:
    image: postgres:16
    environment:
      POSTGRES_DB: straw
      POSTGRES_USER: straw
      POSTGRES_PASSWORD: ${STRAW_POSTGRES_PASSWORD}
    ports:
      - "5432:5432"
    volumes:
      - pgdata:/var/lib/postgresql/data

  redis:
    image: redis:7-alpine
    ports:
      - "6379:6379"

  clickhouse:
    image: clickhouse/clickhouse-server:24
    environment:
      CLICKHOUSE_DB: straw
      CLICKHOUSE_USER: straw
      CLICKHOUSE_DEFAULT_ACCESS_MANAGEMENT: 1
    ports:
      - "8123:8123"
      - "9000:9000"
    volumes:
      - chdata:/var/lib/clickhouse

  # Straw components
  control:
    image: straw/control:latest
    ports:
      - "8080:8080"  # API
      - "8081:8081"  # REST
      - "8082:8082"  # HTTP proxy
      - "8083:8083"  # CONNECT proxy
      - "8084:8084"  # MITM proxy
      - "9090:9090"  # Metrics
    volumes:
      - ./config/control.yaml:/etc/straw/control.yaml:ro
      - ./config/mitm:/etc/straw/mitm:ro
      - ./config/tls:/etc/straw/tls:ro
      - ./config/nats:/etc/straw/nats:ro
      - ./schemas:/etc/straw/schemas:ro
    environment:
      STRAW_POSTGRES_DSN: postgresql://straw:${STRAW_POSTGRES_PASSWORD}@postgres:5432/straw?sslmode=disable
      STRAW_REDIS_URL: redis://redis:6379
      STRAW_CLICKHOUSE_PASSWORD: ${STRAW_CLICKHOUSE_PASSWORD}
    depends_on:
      - nats
      - postgres
      - redis
      - clickhouse

  egress:
    image: straw/egress:latest
    ports:
      - "9091:9090"  # Metrics
    volumes:
      - ./config/egress.yaml:/etc/straw/egress.yaml:ro
      - ./config/nats:/etc/straw/nats:ro
      - ./config/tls:/etc/straw/tls:ro
    environment:
      STRAW_WORKER_ID: egress-local-001
      STRAW_WORKER_PRIVATE_KEY: ${STRAW_WORKER_PRIVATE_KEY}
    depends_on:
      - nats

volumes:
  pgdata:
  chdata:
```

#### 20.11.2 Config File Layout

```
config/
  control.yaml          # Control static config
  egress.yaml           # Egress static config
  adapter.yaml          # Provider Adapter static config (optional)
  mitm/
    ca.key              # MITM CA private key
    ca.pem              # MITM CA certificate
  tls/
    server.crt          # Control inbound TLS certificate
    server.key          # Control inbound TLS private key
    ca-bundle.crt       # Egress outbound CA bundle
  nats/
    creds.creds         # NATS credentials file
schemas/
  control.config.json   # JSON schema for control.yaml
  egress.config.json    # JSON schema for egress.yaml
  adapter.config.json   # JSON schema for adapter.yaml
```

#### 20.11.3 Override Mechanism

Local development overrides are applied via `docker-compose.override.yml`:

```yaml
version: "3.9"

services:
  control:
    environment:
      STRAW_LOG_LEVEL: debug
      STRAW_TRACING_ENABLED: "true"
      STRAW_TRACING_SAMPLING_RATE: "1.0"

  egress:
    environment:
      STRAW_LOG_LEVEL: debug
```

#### 20.11.4 Default Config Templates

Templates are shipped in the `templates/` directory for each component:

```
templates/
  control.yaml.template   # Control config template (all defaults)
  egress.yaml.template    # Egress config template (all defaults)
  adapter.yaml.template   # Adapter config template (all defaults)
```

Operators copy the template to `config/` and customize:

```bash
cp templates/control.yaml.template config/control.yaml
cp templates/egress.yaml.template config/egress.yaml
```

### 20.12 Config Management API Surface

All config management endpoints are versioned under `/api/v1/config/`.

#### 20.12.1 Tenant Endpoints

| Method | Path | Role | Description |
|---|---|---|---|
| `POST` | `/api/v1/config/tenants` | Admin | Create a new tenant |
| `GET` | `/api/v1/config/tenants` | Admin, Operator, Viewer | List all tenants |
| `GET` | `/api/v1/config/tenants/{id}` | Admin, Operator, Viewer | Get tenant config |
| `PUT` | `/api/v1/config/tenants/{id}` | Admin | Update tenant config |
| `DELETE` | `/api/v1/config/tenants/{id}` | Admin | Soft-delete a tenant |

#### 20.12.2 API Key Endpoints

| Method | Path | Role | Description |
|---|---|---|---|
| `POST` | `/api/v1/config/api-keys` | Admin | Create an API key |
| `GET` | `/api/v1/config/api-keys` | Admin, Operator | List API keys for a tenant |
| `GET` | `/api/v1/config/api-keys/{id}` | Admin, Operator | Get API key details |
| `POST` | `/api/v1/config/api-keys/{id}/revoke` | Admin | Revoke an API key |

#### 20.12.3 Routing Rule Endpoints

| Method | Path | Role | Description |
|---|---|---|---|
| `POST` | `/api/v1/config/routing-rules` | Admin, Operator | Create a routing rule |
| `GET` | `/api/v1/config/routing-rules` | Admin, Operator, Viewer | List routing rules for a tenant |
| `GET` | `/api/v1/config/routing-rules/{id}` | Admin, Operator, Viewer | Get a routing rule |
| `PUT` | `/api/v1/config/routing-rules/{id}` | Admin, Operator | Update a routing rule |
| `DELETE` | `/api/v1/config/routing-rules/{id}` | Admin, Operator | Delete a routing rule |

#### 20.12.4 Fingerprint Profile Endpoints

| Method | Path | Role | Description |
|---|---|---|---|
| `GET` | `/api/v1/config/fingerprint-profiles` | Admin, Operator, Viewer | List available fingerprint presets |
| `GET` | `/api/v1/config/fingerprint-profiles/{preset_name}` | Admin, Operator, Viewer | Get a fingerprint preset details |

#### 20.12.5 Injection Policy Endpoints

| Method | Path | Role | Description |
|---|---|---|---|
| `POST` | `/api/v1/config/injection-policies` | Admin, Operator | Create an injection policy |
| `GET` | `/api/v1/config/injection-policies` | Admin, Operator, Viewer | List injection policies for a tenant |
| `GET` | `/api/v1/config/injection-policies/{id}` | Admin, Operator, Viewer | Get an injection policy |
| `PUT` | `/api/v1/config/injection-policies/{id}` | Admin, Operator | Update an injection policy |
| `DELETE` | `/api/v1/config/injection-policies/{id}` | Admin, Operator | Delete an injection policy |

#### 20.12.6 Worker Credential Endpoints

| Method | Path | Role | Description |
|---|---|---|---|
| `POST` | `/api/v1/config/worker-credentials` | Admin | Generate a new worker credential |
| `GET` | `/api/v1/config/worker-credentials` | Admin | List worker credentials for a tenant |
| `POST` | `/api/v1/config/worker-credentials/{id}/revoke` | Admin | Revoke a worker credential |
| `POST` | `/api/v1/config/worker/disable` | Admin | Disable a worker |
| `POST` | `/api/v1/config/worker/enable` | Admin | Re-enable a disabled worker |
| `POST` | `/api/v1/config/worker/drain` | Admin | Start draining a worker |
| `POST` | `/api/v1/config/worker/undrain` | Admin | Stop draining a worker |

#### 20.12.7 Quota and Rate Limit Endpoints

| Method | Path | Role | Description |
|---|---|---|---|
| `GET` | `/api/v1/config/quotas` | Admin, Operator, Viewer | Get quota usage for a tenant |
| `PUT` | `/api/v1/config/quotas` | Admin | Update quota limits for a tenant |
| `GET` | `/api/v1/config/rate-limits` | Admin, Operator, Viewer | Get rate limit config for a tenant |
| `PUT` | `/api/v1/config/rate-limits` | Admin | Update rate limit config for a tenant |

#### 20.12.8 Deny Rule Endpoints

| Method | Path | Role | Description |
|---|---|---|---|
| `POST` | `/api/v1/config/deny-rules` | Admin | Create a deny rule |
| `GET` | `/api/v1/config/deny-rules` | Admin, Operator, Viewer | List deny rules (global + tenant) |
| `PUT` | `/api/v1/config/deny-rules/{id}` | Admin | Update a deny rule |
| `DELETE` | `/api/v1/config/deny-rules/{id}` | Admin | Delete a deny rule |

#### 20.12.9 Payload Capture Endpoints

| Method | Path | Role | Description |
|---|---|---|---|
| `GET` | `/api/v1/config/payload-capture` | Admin, Operator, Viewer | Get payload capture status |
| `PUT` | `/api/v1/config/payload-capture` | Admin | Update payload capture settings |

#### 20.12.10 Config Versioning Endpoints

| Method | Path | Role | Description |
|---|---|---|---|
| `GET` | `/api/v1/config/changes` | Admin, Operator, Viewer | List config change history |
| `GET` | `/api/v1/config/changes/{version_id}` | Admin, Operator, Viewer | Get details of a specific config version |
| `POST` | `/api/v1/config/rollback` | Admin | Rollback to a previous config version |

### 20.13 Environment Variable Reference

Comprehensive reference of all `STRAW_*` environment variables.

#### 20.13.1 Control Server

| Variable | Description | Default | YAML Key |
|---|---|---|---|
| `STRAW_POSTGRES_DSN` | Postgres connection string | — | `database.postgres.dsn` |
| `STRAW_REDIS_URL` | Redis connection URL | — | `database.redis.url` |
| `STRAW_CLICKHOUSE_PASSWORD` | ClickHouse password | — | `database.clickhouse.password` |
| `STRAW_NATS_URLS` | Comma-separated NATS server URLs | `nats://localhost:4222` | `nats.servers` |
| `STRAW_NATS_CREDS_FILE` | NATS credentials file path | — | `nats.user_credentials_file` |
| `STRAW_NATS_TLS_CERT` | NATS client TLS cert path | — | `nats.tls_cert_path` |
| `STRAW_NATS_TLS_KEY` | NATS client TLS key path | — | `nats.tls_key_path` |
| `STRAW_NATS_TLS_CA` | NATS CA cert path | — | `nats.tls_ca_path` |
| `STRAW_NATS_RECONNECT_ATTEMPTS` | Max NATS reconnect attempts | `10` | `nats.reconnect_attempts` |
| `STRAW_NATS_RECONNECT_WAIT_MS` | NATS reconnect wait interval (ms) | `2000` | `nats.reconnect_wait_ms` |
| `STRAW_NATS_PING_INTERVAL_MS` | NATS ping interval (ms) | `30000` | `nats.ping_interval_ms` |
| `STRAW_NATS_MAX_PING_FAILURES` | Max consecutive NATS ping failures | `3` | `nats.max_ping_failures` |
| `STRAW_MITM_CA_KEY_PASSWORD` | MITM CA key password | — | `mitm.ca_key_password_env` |
| `STROW_MITM_CERT_VALIDITY_DAYS` | MITM cert validity in days | `365` | `mitm.cert_validity_days` |
| `STRAW_TLS_CERT` | Server TLS certificate path | — | `tls.cert_path` |
| `STRAW_TLS_KEY` | Server TLS private key path | — | `tls.key_path` |
| `STRAW_TLS_VERSIONS` | Comma-separated TLS versions | `1.2,1.3` | `tls.supported_versions` |
| `STRAW_LOG_LEVEL` | Log level | `info` | `observability.logging.level` |
| `STRAW_LOG_FORMAT` | Log format | `json` | `observability.logging.format` |
| `STRAW_LOG_OUTPUT` | Comma-separated log outputs | `stdout` | `observability.logging.output` |
| `STRAW_METRICS_ENABLED` | Enable metrics | `true` | `observability.metrics.enabled` |
| `STRAW_METRICS_PATH` | Metrics HTTP path | `/metrics` | `observability.metrics.path` |
| `STRAW_METRICS_HOST` | Metrics bind address | `0.0.0.0` | `observability.metrics.host` |
| `STRAW_METRICS_PORT` | Metrics bind port | `9090` | `observability.metrics.port` |
| `STRAW_METRICS_PREFIX` | Metric name prefix | `straw_` | `observability.metrics.custom_prefix` |
| `STRAW_TRACING_ENABLED` | Enable distributed tracing | `false` | `observability.tracing.enabled` |
| `STRAW_TRACING_EXPORTER` | Trace exporter | `jaeger` | `observability.tracing.exporter` |
| `STRAW_TRACING_ENDPOINT` | Trace collector endpoint | — | `observability.tracing.endpoint` |
| `STRAW_TRACING_SAMPLING_RATE` | Tracing sampling rate | `0.1` | `observability.tracing.sampling_rate` |
| `STRAW_BODY_S3_ENDPOINT` | S3-compatible endpoint | — | `body_transport.object_storage.endpoint` |
| `STRAW_BODY_S3_BUCKET` | S3 bucket name | — | `body_transport.object_storage.bucket` |
| `STRAW_BODY_S3_REGION` | S3 region | — | `body_transport.object_storage.region` |
| `STRAW_S3_ACCESS_KEY` | S3 access key | — | `body_transport.object_storage.access_key_env` |
| `STRAW_S3_SECRET_KEY` | S3 secret key | — | `body_transport.object_storage.secret_key_env` |
| `STRAW_BODY_RETENTION_DAYS` | Body retention in days | `1` | `body_transport.object_storage.body_retention_days` |
| `STRAW_BODY_OBJECT_STORAGE_ENABLED` | Enable object storage | `false` | `body_transport.object_storage.enabled` |
| `STRAW_BODY_DIRECT_STREAM_ENABLED` | Enable direct streaming | `false` | `body_transport.direct_stream.enabled` |
| `STRAW_BODY_STREAM_ENDPOINT` | Direct stream endpoint URL | — | `body_transport.direct_stream.endpoint` |
| `STRAW_BODY_STREAM_TIMEOUT_MS` | Direct stream timeout (ms) | `300000` | `body_transport.direct_stream.stream_timeout_ms` |
| `STRAW_BODY_LARGE_BODY_THRESHOLD_BYTES` | Large body size threshold | `1048576` | `body_transport.large_body_threshold_bytes` |

#### 20.13.2 Egress Worker

| Variable | Description | Default | YAML Key |
|---|---|---|---|
| `STRAW_WORKER_ID` | Unique worker identifier | — | `worker_id` |
| `STRAW_WORKER_PRIVATE_KEY` | Ed25519 private key for credential signing | — | (credential private key) |
| `STRAW_NATS_URLS` | Comma-separated NATS server URLs | `nats://localhost:4222` | `nats.servers` |
| `STRAW_NATS_CREDS_FILE` | NATS credentials file path | — | `nats.user_credentials_file` |
| `STRAW_NATS_TLS_CERT` | NATS client TLS cert path | — | `nats.tls_cert_path` |
| `STRAW_NATS_TLS_KEY` | NATS client TLS key path | — | `nats.tls_key_path` |
| `STRAW_NATS_TLS_CA` | NATS CA cert path | — | `nats.tls_ca_path` |
| `STRAW_NATS_RECONNECT_ATTEMPTS` | Max NATS reconnect attempts | `10` | `nats.reconnect_attempts` |
| `STRAW_NATS_RECONNECT_WAIT_MS` | NATS reconnect wait interval (ms) | `2000` | `nats.reconnect_wait_ms` |
| `STRAW_NATS_PING_INTERVAL_MS` | NATS ping interval (ms) | `30000` | `nats.ping_interval_ms` |
| `STRAW_NATS_MAX_PING_FAILURES` | Max consecutive NATS ping failures | `3` | `nats.max_ping_failures` |
| `STRAW_OUTBOUND_TLS_STRICT_VERIFY` | Enforce outbound TLS verification | `true` | `outbound_tls.strict_verify` |
| `STRAW_OUTBOUND_TLS_CA_BUNDLE` | Custom CA bundle path | — | `outbound_tls.ca_bundle_path` |
| `STRAW_OUTBOUND_TLS_VERSIONS` | Comma-separated TLS versions | `1.2,1.3` | `outbound_tls.supported_versions` |
| `STRAW_UPSTREAM_PROXY_USERNAME` | Upstream proxy username | — | `upstream_proxy.username` |
| `STRAW_UPSTREAM_PROXY_PASSWORD` | Upstream proxy password | — | `upstream_proxy.password` |
| `STRAW_LOG_LEVEL` | Log level | `info` | `observability.logging.level` |
| `STRAW_LOG_FORMAT` | Log format | `json` | `observability.logging.format` |
| `STRAW_LOG_OUTPUT` | Comma-separated log outputs | `stdout` | `observability.logging.output` |

#### 20.13.3 Provider Adapter

| Variable | Description | Default | YAML Key |
|---|---|---|---|
| `STRAW_WORKER_ID` | Unique adapter identifier | — | `worker_id` |
| `STRAW_BRIGHT_DATA_KEY` | Bright Data API key | — | `providers[0].api_key_env` |
| `STRAW_UPSTREAM_USERNAME` | Upstream proxy username | — | `providers[*].upstream_proxy.username` |
| `STRAW_UPSTREAM_PROXY_PASSWORD` | Upstream proxy password | — | `providers[*].upstream_proxy.password` |
| `STRAW_NATS_URLS` | Comma-separated NATS server URLs | `nats://localhost:4222` | `nats.servers` |
| `STRAW_NATS_CREDS_FILE` | NATS credentials file path | — | `nats.user_credentials_file` |
| `STRAW_LOG_LEVEL` | Log level | `info` | `observability.logging.level` |
| `STRAW_LOG_FORMAT` | Log format | `json` | `observability.logging.format` |

#### 20.13.4 Shared Variables

| Variable | Description | Default | Used By |
|---|---|---|---|
| `STRAW_POSTGRES_PASSWORD` | Postgres password | — | Control |
| `STRAW_CLICKHOUSE_PASSWORD` | ClickHouse password | — | Control |
| `STRAW_MITM_CA_KEY_PASSWORD` | MITM CA key password | — | Control |
| `STRAW_S3_ACCESS_KEY` | S3 access key | — | Control, Egress |
| `STRAW_S3_SECRET_KEY` | S3 secret key | — | Control, Egress |

## 21. Deployment

How it runs outside code.

## 22. Security

Threat model and protections.

## 23. Testing

Checks needed to trust the system.

## 24. Operational Behavior

Expected behavior during incidents.

## 25. Open Decisions

Unresolved choices that block implementation or affect architecture.

## 26. Implementation Order

Build sequence and dependencies between parts.

## 27. Risks

Known technical, security, operational, and legal risks.

## 28. Future Work

Useful ideas that are not required for the current planned scope.
