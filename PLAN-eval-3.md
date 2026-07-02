I reviewed `PLAN.md` for consistency, coverage, and completeness. Overall: the plan is architecturally strong, but not
yet implementation-ready. The biggest gaps are contract consistency, Phase 1 scope control, error semantics, and
security/data-plane edge cases.

## Summary assessment

| Area             | Assessment                                                                                                                                            |
|------------------|-------------------------------------------------------------------------------------------------------------------------------------------------------|
| **Consistency**  | **Needs work.** Several names, responsibilities, and protocol flows conflict across sections.                                                         |
| **Coverage**     | **Broad.** It covers architecture, routing, worker discovery, NATS, protobuf, MITM, config, auth, observability, deployment, testing, and operations. |
| **Completeness** | **Good outline, incomplete contract.** The plan explains the intended system well, but several parts are not precise enough to build against yet.     |

## Highest-priority consistency issues

### 1. Error taxonomy is inconsistent across sections

The plan uses multiple names for the same error classes:

| Concept                     | Names used                                                                                       |
|-----------------------------|--------------------------------------------------------------------------------------------------|
| No route                    | `route_no_match`, `no_matching_route`                                                            |
| No available route/executor | `route_unavailable`, `no_workers_available`, `sticky_session_failed`                             |
| Worker loss                 | `worker-lost`, `worker_disconnected`                                                             |
| NATS failure                | `transport_unavailable`, `nats_cluster_unavailable`, `worker_timeout`                            |
| Validation failure          | `validation_error`, `invalid_request`                                                            |
| Timeout                     | `worker_timeout`, `timeout_exceeded` + `TimeoutType`                                             |
| Egress failures             | `dns_failure`, `tls_failure`, `upstream_failure`, `upstream_dns_failure`, `upstream_tls_failure` |

This will create ambiguity in protobuf enums, SDK retry behavior, logs, metrics, and client-facing errors. Fix this
before implementation. Define one canonical `ErrorCode` enum and require every section to use it.

### 2. Assignment protocol conflicts with protobuf contract

The request lifecycle says Control sends a normalized assignment containing method, URL, headers, routing metadata,
fingerprint/injection policy, deadline, and body stream/reference.

Later, the protobuf section says `AssignRequest` only reserves capacity and does **not** carry the full HTTP request;
the actual request is sent later in `RequestStart`.

Pick one model. I recommend the two-step model:

1. `AssignRequest`: reserve executor capacity only.
2. `AssignAck`: accept/reject.
3. `RequestStart`: method, URL, headers, route metadata, fingerprint, injection policy, body metadata.
4. `DataFrame` / `BodyRef`: body transfer.

Then update the lifecycle section to match.

### 3. Control and Egress both appear to own error mapping

The Egress Execution section says Egress maps raw Go/network errors into Straw typed errors. The Error Handling section
says Control is the sole authority and maps raw worker reports into `ErrorCode`.

Choose one authority boundary.

Best option: Egress should emit typed **low-level failure facts** from a constrained enum, and Control should map them
to client-facing policy. For example:

```text
Egress reports: upstream_dns_failure, upstream_tls_failure, connection_refused
Control decides: HTTP status, retryable flag, public message, metrics category
```

Also, the referenced `WorkerReport` message is not present in the protobuf contract. Either add it or remove that flow.

## Scope and Phase 1 issues

### 4. Provider Adapter scope conflicts with the non-goals

The non-goals say Phase 1 does not include marketplace/vendor integrations or provider account automation, but the main
architecture makes Provider Adapters first-class Phase 1 executors for direct vendor/upstream execution.

This can be resolved by narrowing the wording:

> Phase 1 supports operator-configured Provider Adapters as static executors. It does not support marketplace discovery,
> automatic account provisioning, billing reconciliation, or provider economics.

Without that clarification, Phase 1 reads larger than intended.

### 5. SDKs, CLI, UI, many generated language packages, MITM, NATS, workers, config API, observability, deployment, and chaos testing is a large Phase 1

The plan has the shape of a mature platform, not an MVP. The implementation order only lists protobuf/NATS, Control, and
Egress, but the goals include SDKs, CLI, UI, full config management, protobuf packages for many languages, ClickHouse
dashboards, Docker Swarm, Kubernetes, MITM, payload capture, direct streaming, S3, and Provider Adapters.

I would split into:

| Phase                   | Recommended contents                                                                                |
|-------------------------|-----------------------------------------------------------------------------------------------------|
| **P0 / Vertical slice** | REST transport, one Control, one Go Egress, NATS request/reply, basic routing, auth, basic errors   |
| **P1**                  | HTTP proxy, CONNECT, worker discovery/health, backpressure, quotas/rate limits, ClickHouse metadata |
| **P2**                  | MITM, payload capture, S3/direct large-body transport, SDK/CLI                                      |
| **P3**                  | Provider Adapters, UI, Kubernetes, advanced observability, chaos suite                              |

MITM should not be in the first implementation slice unless it is the core differentiator.

## Data-plane completeness gaps

### 6. Large-body transport is under-specified

The plan says large bodies use S3-compatible object storage or direct streaming, but it does not fully define:

* who uploads the body,
* when upload occurs relative to route assignment,
* how executors authenticate to body references,
* cleanup behavior on cancellation,
* partial upload/download failure semantics,
* backpressure behavior for direct streaming,
* whether S3 `BodyRef` is tenant-isolated,
* whether object keys are guess-resistant,
* whether bodies are encrypted at rest,
* whether response bodies can be written by Egress and read by Control concurrently.

This is one of the highest-risk implementation areas because it crosses Control, Egress, storage, auth, cancellation,
retry, and quota accounting.

### 7. Request replayability is too optimistic

The plan treats `GET`, `HEAD`, `OPTIONS`, `PUT`, and `DELETE` as replayable/idempotent, or replayable if the body is
seekable S3. That is protocol-theoretically defensible, but unsafe operationally. Many real endpoints treat `PUT` and
`DELETE` as side-effecting, and retries can cause damage.

Recommended rule:

* Default automatic fallback/retry only before any outbound bytes reach the target.
* After outbound execution starts, retry only when the client explicitly marks the request replayable.
* SDKs can default `GET`, `HEAD`, `OPTIONS` to replayable, but not `PUT`/`DELETE` without opt-in.

### 8. Backpressure protocol needs startup details

The NATS credit-frame model is good, but incomplete. Define:

* initial credit window,
* max in-flight bytes per direction,
* max outstanding frames,
* frame idle timeout behavior,
* whether credit applies to compressed or raw byte count,
* how `BodyRef` interacts with credit,
* what happens when Control has sent client response headers but downstream stalls.

Without these details, different workers may implement incompatible flow control.

## HTTP and proxy semantics gaps

### 9. MITM versus raw CONNECT listener semantics need precision

The plan says HTTPS proxying defaults to MITM, while raw CONNECT is an explicit separate listener or mode. That needs
exact operational definition:

* separate ports?
* same port with policy?
* header flag?
* tenant config?
* per-request mode?
* what status code is returned for a CONNECT on the MITM listener?
* how does a standard proxy client know it is in MITM mode?

This matters because most HTTPS proxy clients initiate with `CONNECT`. MITM still usually starts from a CONNECT, then
Control returns `200`, terminates TLS, and generates a leaf certificate. The current wording partly blurs “CONNECT as a
method” and “raw CONNECT tunnel mode.”

### 10. HTTP/2 support is underspecified

The plan claims HTTP/2 support at ingress and egress. Add coverage for:

* request IDs per HTTP/2 stream,
* stream cancellation mapping,
* flow-control interaction with NATS credits,
* header pseudo-fields,
* trailers,
* connection-level errors affecting multiple streams,
* whether MITM supports HTTP/2 from client to Control,
* whether Egress can downgrade/upgrade between HTTP/1.1 and HTTP/2.

### 11. Origin 4xx/5xx should probably not be Straw errors

The plan sometimes treats `upstream_http_error` as an error envelope, but elsewhere says successful upstream responses
return raw upstream status, headers, body, and trailers.

For proxy semantics, origin `404`, `403`, `429`, and `500` should normally pass through as upstream responses, not
become Straw errors. Straw errors should mean Straw failed to transport the request.

Recommended distinction:

```text
origin_status = 404  -> normal upstream response, not ErrorResponse
straw_error = route_unavailable -> ErrorResponse
straw_error = upstream_tls_failure -> ErrorResponse
```

You can still log upstream 4xx/5xx as outcome metadata.

## State, storage, and operations gaps

### 12. Redis fail-open conflicts with abuse controls and quota accuracy

Operational behavior says Redis outage causes rate limits and quotas to fail open. That may be acceptable for
availability, but it conflicts with the stated abuse/overload controls and durable quota intent.

You need an explicit policy:

| Failure                               | Recommended behavior                                                                |
|---------------------------------------|-------------------------------------------------------------------------------------|
| Rate limit Redis unavailable          | fail open or fail closed per tenant/system setting                                  |
| Quota Redis unavailable               | likely fail closed for paid/abuse-sensitive tenants, fail open for internal tenants |
| Sticky session unavailable            | degrade only if sticky fallback allowed                                             |
| Worker availability Redis unavailable | use local snapshot with short TTL, then fail safe                                   |

Also, using Redis with `volatile-lru` for all ephemeral state is risky. Rate-limit counters, quota counters, worker
availability, and MITM cert cache should not share the same eviction class.

### 13. Quotas are not durably reliable as written

Monthly quotas are tracked with Redis fixed-window counters and periodically flushed to Postgres. If Redis evicts keys
or is lost before flush, quota data is wrong.

Either make quotas explicitly approximate, or store quota events/counters durably first. A good compromise is:

* Redis for fast admission control,
* ClickHouse/Postgres for durable usage events,
* reconciliation job to correct Redis counters,
* fail policy when Redis and durable usage disagree.

### 14. ClickHouse audit retention conflicts with “audit” language

Config change audit tables have TTLs. That may be fine, but call them operational audit logs, not durable compliance
audit logs. If the product needs real auditability, move immutable audit history to longer retention or object storage.

## Security and abuse coverage gaps

### 15. Destination deny rules need DNS/IP resolution rules

Blocking domains, URLs, and CIDRs is not enough unless you define:

* DNS resolution timing,
* CNAME handling,
* private IP / link-local / metadata IP blocking,
* IPv6,
* DNS rebinding,
* redirects to denied destinations,
* CONNECT target validation,
* SNI versus Host mismatch,
* IDNA/punycode normalization.

This is critical for SSRF-style abuse prevention.

### 16. MITM certificate storage needs key-material policy

The plan mentions generated cert caching/storage. It should explicitly define whether private keys are stored, where,
encrypted how, with what TTL, and who can access them. Redis should not casually store reusable private key material
unless encrypted and scoped.

### 17. Payload redaction conflicts with “does not mutate body”

The non-goals and lifecycle emphasize transport neutrality and no body mutation. Later, rate limits/quotas says
redaction dynamically strips offending headers or payload bytes from the stream and forwards the remaining request. That
is content-aware mutation.

Resolve by separating two concepts:

* **Forwarding path redaction**: mutates live traffic. This is a major feature and contradicts transport neutrality.
* **Capture redaction**: redacts only what is stored in ClickHouse/payload capture. This fits the current architecture.

I recommend Phase 1 only supports capture redaction, not live payload redaction.

## Completeness gaps to add before implementation

Add short sections or tables for:

1. Canonical `ErrorCode` enum and HTTP mapping.
2. Exact public REST request/response schema.
3. Exact proxy metadata header schema.
4. CONNECT/MITM listener behavior.
5. Large-body protocol.
6. Backpressure and credit initialization.
7. Retry/replay policy.
8. Deny-rule normalization and DNS/IP safety.
9. Tenant isolation model for NATS subjects and object storage.
10. Minimal viable Phase 1 cut.
11. Migration path from single Control instance to horizontal Control.
12. Worker capability trust model and future active verification hook.
13. Test matrix mapped to each contract, not just broad test categories.

## Recommended next edit pass

Do this in order:

1. **Normalize terminology**: one name for every error, ingress type, executor type, route state, and timeout.
2. **Lock the Control/Egress contract**: make NATS + protobuf the source of truth; update lifecycle text to match.
3. **Reduce Phase 1**: separate MVP, Phase 1, Phase 2, Future Work.
4. **Clarify security boundaries**: MITM key material, deny rules, Redis failure, payload capture/redaction.
5. **Turn vague capabilities into acceptance criteria**: especially HTTP/2, large bodies, streaming, cancellation,
   retries, and observability.

The document is a solid architecture plan, but it currently mixes product vision, Phase 1 scope, implementation detail,
and future-state platform features. Tightening those boundaries will make it much easier to implement without contract
churn.
