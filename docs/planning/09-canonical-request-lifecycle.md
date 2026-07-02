## 9. Canonical Request Lifecycle

### P0 REST Decoded Flow

1. Control receives REST request.
2. Control generates `request_id`.
3. Control authenticates the API key and derives `tenant_id`.
4. Control authorizes data-plane execution.
5. Control validates method, URL, headers, body mode, routing hints, timeout, replayability, and P0 feature flags.
6. Control rejects URL userinfo and applies metadata/logging redaction decisions.
7. Control applies rate-limit and quota admission checks according to configured Redis fail policy.
8. Control applies destination deny rules at the URL/host level.
9. Control captures an immutable tenant route/config snapshot.
10. Control selects a route and exact executor session.
11. Control sends `AssignRequest` to the exact executor assignment subject.
12. Executor replies with `AssignAck`.
13. After accept, Control and Executor subscribe to request-scoped `c2e` and `e2c` subjects.
14. Control sends `RequestStart` over the request-scoped `c2e` subject.
15. Control streams request body frames or sends `BodyRef` according to transport mode. P0 supports only NATS
    `DataFrame` bodies derived from inline REST bodies.
16. Executor validates destination policy after DNS resolution and immediately before connect.
17. Executor sends `OutboundStartFrame` before DNS/connect or before delegating to an upstream proxy.
18. Executor performs outbound request.
19. Executor sends `ResponseStart`, response `DataFrame`s, optional `TrailersFrame`, and `EndFrame`, or sends
    `ErrorFrame`.
20. Control maps executor facts/errors into a public response or public ErrorResponse.
21. Control writes final metadata asynchronously to ClickHouse using P0 metadata redaction rules.

### Cancellation

Control sends `CancelFrame` when:

- client disconnects,
- request deadline expires,
- admin cancellation occurs,
- Control shutdown abandons the request,
- fallback makes an accepted attempt obsolete before the attempt reaches its no-replay boundary.

Cancellation is best effort. The deadline is authoritative even if cancel is missed.

### Terminal Rule

Every protocol-compliant accepted assignment that remains connected until completion must end with exactly one terminal
frame:

- `EndFrame`,
- `ErrorFrame`, or
- `CancelledFrame`.

If the worker process dies, Control loses the request-scoped NATS path, NATS becomes unavailable, or the deadline
expires
before a terminal frame arrives, Control synthesizes the terminal request outcome as one of:

- `worker_disconnected`,
- `transport_unavailable`,
- `assignment_timeout`,
- `timeout_exceeded`,
- `stream_upload_aborted`,
- `stream_download_aborted`.

After a terminal frame, synthesized terminal outcome, or deadline, both sides close request-scoped subscriptions and
ignore late frames. Repeated late/protocol-invalid frames contribute to worker cooldown.

### Replay and Fallback Boundary

For P0, Control uses the conservative replay boundary:

- fallback is allowed after assignment reject before `RequestStart`,
- fallback is allowed after assignment timeout before `RequestStart`,
- fallback is allowed after executor loss before `RequestStart`,
- after `RequestStart`, fallback is forbidden unless `replayable=true`.

Egress sends `OutboundStartFrame` before DNS/connect. In P1/P2, Control may use that finer-grained signal to allow some
pre-connect fallback after `RequestStart`, but P0 does not depend on that optimization.

If `replayable=true`, fallback after `RequestStart` is still forbidden once Control has sent a client-visible successful
response envelope or begun a streaming response in a future ingress mode.
