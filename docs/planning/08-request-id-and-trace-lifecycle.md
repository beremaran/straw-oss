## 8. Request ID and Trace Lifecycle

Control generates `request_id` as soon as a request reaches any ingress path. Clients may supply `X-Straw-Request-Id`
only as an idempotency/correlation hint in a future phase; P0 ignores client-supplied request IDs and always generates
its own.

`request_id` is propagated through:

- REST response envelopes,
- ErrorResponse envelopes,
- NATS Envelope,
- ClickHouse records,
- logs,
- metrics exemplars where supported,
- traces.

Trace behavior:

- If inbound HTTP includes valid W3C `traceparent`, Control extracts trace context and starts a child span.
- If no valid trace context exists, Control starts a new trace.
- NATS Envelope carries `trace_id` and optionally serialized trace context.
- Egress spans use the received trace context.
- Outbound target requests do not receive tracing headers unless an explicit injection policy allows it.
