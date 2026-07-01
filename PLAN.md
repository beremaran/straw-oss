# Straw Proxy Plan

## 1. Purpose

What Straw is, who uses it, and what problem it solves.

## 2. Goals

Concrete capabilities the system must provide.

## 3. Non-Goals

Things intentionally outside scope so future decisions do not drift.

## 4. System Overview

High-level architecture: Control, Egress, NATS, storage, and request flow.

## 5. Components

Responsibilities of each major service.

- Control Server: ingress, auth, routing, proxy handling, NATS coordination.
- Egress Worker: executes outbound requests and reports capabilities.
- NATS: message transport between Control and Egress.
- Redis: ephemeral state, rate limits, sessions, queues, or remove if unused.
- Postgres: durable state such as users, keys, workers, logs, config, or remove if unused.

## 6. Client Interfaces

How clients talk to Control.

- REST API: explicit request/response endpoint behavior.
- HTTP Proxy: forward proxy behavior for plain HTTP.
- CONNECT Tunnel: raw TCP tunnel behavior for HTTPS.
- MITM Proxy: intercepted HTTPS behavior and certificate requirements.

## 7. Request Lifecycle

End-to-end flow from client request to egress response.

- REST Request Flow: how API calls become egress jobs.
- Plain HTTP Proxy Flow: parsing, routing, forwarding, response streaming.
- CONNECT Tunnel Flow: tunnel setup, routing, byte forwarding.
- MITM Flow: TLS interception, HTTP reconstruction, upstream request.
- Response Flow: status, headers, body, trailers, errors.
- Streaming Bodies: upload/download streaming rules.
- Cancellation and Client Disconnects: how work is stopped.

## 8. Routing Model

How Control chooses an Egress worker.

- Worker Capabilities: what workers advertise.
- Tags: custom labels and matching behavior.
- Country and Region: geo targeting semantics.
- IP Type: datacenter/residential/mobile/etc.
- Sticky Sessions: whether clients can reuse the same worker/IP.
- Fallback Rules: what happens when preferred routing fails.
- No-Match Behavior: exact error returned when no worker qualifies.

## 9. Worker Discovery and Health

How workers join, stay alive, and leave.

- Registration: startup capability announcement.
- Heartbeats: liveness interval and payload.
- Load Reporting: capacity, active jobs, queue depth.
- Draining: graceful removal from routing.
- Failure Detection: when Control considers a worker dead.

## 10. NATS Protocol

Messaging shape between services.

- Subjects: naming convention for requests, replies, registration, heartbeat.
- Queue Groups: worker load balancing model.
- Request/Reply Pattern: correlation and reply handling.
- Timeouts: Control and worker timeout rules.
- Retries: what can be retried safely.
- Backpressure: what happens under overload.
- Message Size Limits: body size strategy and limits.

## 11. Protobuf Contracts

Schemas shared by Control and Egress.

- Request Message: method, URL, headers, body, routing metadata.
- Response Message: status, headers, body, timing.
- Error Message: typed failures and retryability.
- Worker Registration Message: worker identity and capabilities.
- Worker Heartbeat Message: health and load.
- Routing Metadata: tags, geo, IP type, session ID.
- Versioning Strategy: backward-compatible schema changes.

## 12. HTTP Semantics

Which HTTP behavior is preserved.

- Methods: supported methods.
- Headers: pass-through, stripped, rewritten, generated.
- Cookies: pass-through and session behavior.
- Redirects: follow or return upstream redirects.
- Compression: preserve, decode, or recompress.
- Trailers: support or explicitly reject.
- Connection Reuse: pooling and keep-alive behavior.
- WebSockets: supported modes or not.
- HTTP/2: inbound and outbound support.
- TLS Behavior: SNI, ALPN, verification, client certs.

## 13. MITM Design

HTTPS interception details.

- CA Generation: how root/intermediate CA is created.
- Certificate Storage: where generated certs live.
- Per-Host Certificates: cache and generation rules.
- Client Trust Setup: how clients trust the CA.
- TLS Termination: inbound TLS handling.
- Upstream TLS: outbound TLS handling.
- Security Boundaries: what data is exposed and to whom.

## 14. Egress Execution

How workers perform outbound requests.

- `tls-client` Integration: exact library and call boundary.
- Browser Fingerprints: available profiles and selection.
- Proxy Chaining: whether egress can use another upstream proxy.
- DNS Resolution: local, remote, custom resolver behavior.
- Source IP Selection: how worker IP is chosen.
- Timeout Handling: dial, TLS, header, body, total timeout.
- Error Mapping: translating execution failures to Control errors.

## 15. State and Storage

What is persisted or cached.

- Postgres Data Model: durable tables and ownership.
- Redis Usage: ephemeral keys and TTLs.
- Session State: sticky sessions and routing affinity.
- Worker State: registration snapshots and health.
- Request Logs: metadata, payload policy, retention.
- Retention: cleanup rules.

## 16. Authentication and Authorization

Who can access what.

- Client Authentication: API/client identity.
- API Keys or Tokens: issuance, hashing, revocation.
- Worker Authentication: worker identity and trust.
- NATS Authentication: service credentials.
- Admin Access: privileged operations.
- Tenant Isolation: cross-tenant safety rules.

## 17. Rate Limits and Quotas

How abuse and overload are controlled.

- Per-Client Limits: client request caps.
- Per-Worker Limits: worker capacity caps.
- Global Limits: system-wide protection.
- Burst Handling: short spikes.
- Limit Error Responses: status codes and error payloads.

## 18. Error Handling

Canonical failure behavior.

- Client Errors: bad input and auth failures.
- Routing Errors: no worker or invalid route hints.
- Worker Errors: crashes, rejection, capacity.
- Upstream Errors: DNS, TLS, timeout, refused connection.
- NATS Errors: publish/request/reply failures.
- Timeout Errors: which timeout fired.
- Error Codes: stable internal and external codes.

## 19. Observability

How the system is operated.

- Logs: structure, fields, redaction.
- Metrics: latency, volume, errors, worker health.
- Tracing: request spans across Control/NATS/Egress.
- Request IDs: propagation rules.
- Health Endpoints: liveness/readiness.
- Debug Endpoints: restricted diagnostics.

## 20. Configuration

Runtime configuration surface.

- Control Config: ports, NATS, auth, limits.
- Egress Config: worker ID, capabilities, fingerprints.
- NATS Config: URLs and credentials.
- Redis Config: URL and key namespace.
- Postgres Config: DSN and migrations.
- Secrets: source and rotation.
- Environment Variables: supported env names.

## 21. Deployment

How it runs outside code.

- Local Development: docker-compose and dev commands.
- Docker Compose: local dependency topology.
- Production Topology: service layout.
- Scaling Control: horizontal behavior.
- Scaling Egress: pool growth and regional workers.
- Rolling Deploys: compatibility during deploy.
- Draining and Shutdown: graceful stop behavior.

## 22. Security

Threat model and protections.

- Trust Boundaries: client/control/worker/NATS/storage boundaries.
- Secret Handling: storage and logging rules.
- Certificate Handling: MITM CA protection.
- Request Data Handling: sensitive payload policy.
- Abuse Prevention: SSRF, spam, forbidden destinations.
- Audit Logging: security-relevant events.

## 23. Testing

Checks needed to trust the system.

- Unit Tests: routing, config, parsing, error mapping.
- Protobuf Contract Tests: compatibility and required fields.
- NATS Integration Tests: request/reply and worker registration.
- Proxy Integration Tests: REST, HTTP proxy, CONNECT.
- MITM Tests: cert generation and intercepted HTTPS.
- Load Tests: capacity and backpressure.
- Failure Tests: worker loss, NATS outage, timeout paths.

## 24. Operational Behavior

Expected behavior during incidents.

- Startup: dependency checks and readiness.
- Graceful Shutdown: stop accepting, finish active work.
- Worker Loss: reroute or fail active requests.
- NATS Outage: degradation and recovery.
- Redis Outage: features affected.
- Postgres Outage: features affected.
- Partial Degradation: what still works.

## 25. Open Decisions

Unresolved choices that block implementation or affect architecture.

## 26. Implementation Order

Build sequence and dependencies between parts.

## 27. Risks

Known technical, security, operational, and legal risks.

## 28. Future Work

Useful ideas that are not required for the current planned scope.