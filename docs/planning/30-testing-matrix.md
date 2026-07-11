## 30. Testing Matrix

P0 must include contract-mapped tests.

| Area          | Required tests                                                                                                 |
|---------------|----------------------------------------------------------------------------------------------------------------|
| Protobuf      | `buf lint`, `buf breaking`, BodyRefFrame compiles, AssignRequest credit fields present, unknown enum rejection |
| NATS subjects | exact assignment subject, no pool queue dispatch, safe token validation, max payload validation                |
| Registration  | valid registration, invalid signature, out-of-scope pool, incompatible version, duplicate session              |
| Fingerprints  | exact signed capability list, legacy signing bytes, tenant-scoped named routing, baseline/default alias, selected/executed equality, local registry drift, no-upstream rejection, redaction, Chrome 120 conformance |
| Heartbeat     | ready/degraded/unhealthy, unavailable after 15s, dead after 30s, stale session ignored                         |
| Worker state  | global disable precedence, tenant disable isolation, draining exclusion, cooldown, duplicate replacement       |
| Routing       | priority order, hard client hints, tenant isolation, degraded pool policy, no match, sticky success/failure    |
| Assignment    | accept, reject capacity, reject draining, ack timeout, no duplicate retry                                      |
| Streaming     | sequence gaps, duplicates, out-of-order frames, offset mismatch, credit exhaustion, idle timeout               |
| Terminal      | duplicate terminal ignored/counted, late frame ignored, worker death synthesizes terminal outcome              |
| Cancellation  | client disconnect, deadline, admin cancel, late frame ignored                                                  |
| Fallback      | fallback before `RequestStart`, no fallback after `RequestStart` unless replayable, no silent replay           |
| Error mapping | every ErrorCode maps to HTTP/retry/category; origin 4xx/5xx passthrough is not ErrorResponse; ErrorFrame code outside executor-emittable set maps to `executor_internal_error` and counts toward cooldown |
| REST schema   | valid request, invalid fields, Host rejected, header duplicate preservation, inline body limit, CONNECT rejected |
| REST outcome  | upstream 404/500 returns API HTTP 200 with upstream status in envelope; Straw errors return ErrorResponse      |
| Body limits   | request body over cap returns `body_too_large`; response body over cap returns `body_too_large` with direction |
| P0 exclusions | capture_hint other than `none` rejected; unknown fields (e.g., any redirect-following field) rejected in strict mode; protobuf-defined BodyRef rejected in P0 |
| Rate limits   | dimensions, 429, retry_after, Redis fail policy, memory guardrail fallback, values above tenant ceiling rejected |
| Quotas        | request count, bandwidth accounting, admission policy, Redis loss behavior, no billing-grade claim             |
| Deny rules    | domain, CIDR, private IP, metadata IP, DNS rebinding, redirect target future test, IDNA normalization          |
| Egress policy | RequestStart carries DestinationPolicy; Egress enforces resolved-IP deny without querying Control DBs          |
| HTTP behavior | P0 rejects CONNECT; baseline retains existing HTTP/1/pooling behavior; named `chrome_120` proves its TLS/HTTP2 contract, disables HTTP/3/redirects, and has request-scoped cleanup |
| Redaction     | URL userinfo rejected; query dropped by default; auth/cookie headers absent from logs/ClickHouse               |
| Config API    | endpoint phase labels, expected version conflict, tenant version increment, config/admin path separation       |
| Invalidation  | Redis pub/sub invalidation, missed pub/sub corrected by version check, API key revocation invalidates cache    |
| ClickHouse    | async write success, outage, bounded queue drop, sanitized target_url, additive requested/selected/executed profile columns on clean and existing volumes |
| Load          | routing p50/p99, assignment latency, active request limit, worker capacity behavior                            |
| Auth          | platform key lifecycle, platform key cannot execute requests, tenant key cannot create tenants, revocation, quota write requires platform key, worker-credential create rejects foreign tenant scope |
| Audit         | actor API key recorded, injection secret values redacted in Postgres/ClickHouse, API key secret never logged   |
| Identifiers   | duplicate worker_id across tenants rejected; multi-tenant pool scope validated                                |
| HTTP validation| invalid header names rejected; CR/LF header injection rejected; URL fragment rejected; IDNA normalization    |
| Injection auth| operator cannot set Authorization/Cookie injection; tenant_admin can create audited sensitive policy          |
| Worker admin  | tenant worker actions affect only that tenant; global worker actions require system_admin; tenant-scoped worker list omits other tenants' workers and session IDs; cancel rejects foreign-tenant request_id |
| NATS ordering | subscriber flush proves RequestStart not lost; stream subject publish before subscribe fails in test harness   |
| SSRF          | local DNS validation connects only to validated IP; DNS rebinding between validation and dial is blocked       |
| Timeout       | total deadline wins over phase timeout when simultaneous; phase timeouts bounded by remaining deadline         |

Feature closure commands for the unified local stack are run from the repository root:
`make infra-up`, `make check-protos`, `make clickhouse-migrations-check`, `make check-straw`, and `cd straw && make check`.
The fixed Coles acceptance request is first-attempt only; an origin denial, challenge, or changed page is recorded as
`Open`/`Fail`, never as a substituted pass.

P1/P2 add proxy, CONNECT, MITM, BodyRef, payload capture, Egress SDK, telemetry read APIs, connection pooling,
and HTTP/2 test rows before those features ship.

P1 upstream connection pooling is specified in `docs/planning/b-upstream-connection-pooling.md`. Before implementation
ships, tests must prove the disabled default, exact pool-key reuse boundaries, tenant isolation, DNS rebinding/SSRF
validation before reuse, no second resolution by the HTTP/TLS library, eviction/shutdown behavior, and stale-connection
fallback.

P2 HTTP/2 semantics are specified in `docs/planning/c-http2-semantics.md`. Before implementation ships, tests must prove the disabled default, multiplexing, stream cancellation mapping, error mapping, flow control backpressure, pseudo-header mapping/rejection, trailers, connection-level error handling, ALPN negotiation, protocol translation, and HTTP/1.1 fallback/downgrade rules.

P2 large-body transport (BodyRef) is specified in `docs/planning/18-large-body-transport-p2.md`. Transport selection
(task `../implementation-history.md#p2-05`) must prove the Section 18 table: small bodies stay on NATS
DataFrames, bodies over `large_body_threshold_bytes` select S3 BodyRef when object storage is enabled and
DirectStreamRef when direct streaming is enabled (object storage taking precedence when both are on), no enabled
large-body transport maps to `body_too_large` with `direction`/`limit_bytes`, disabled/unavailable BodyRef variants
map to `body_ref_unavailable`, and only the resolved `stream_through_control_tee_object_storage` response-body mode
validates. The object-storage foundation (task `../implementation-history.md#p2-06`) must prove tenant/request-scoped
object keys with high-entropy nonces, rejection of identifiers that could escape the tenant prefix, SigV4 presigned
URLs bound to one object key and method with short/clamped expiry, server-side encryption signed into upload URLs, no
bucket-listing surface, retention defaults/maximums (1–3 days), and the object-storage-unavailable sentinel that maps
to `body_ref_unavailable`. The request upload flow, response tee-through-Control flow, and checksum/size verification
are proven by tasks 07–08.

P2 MITM leaf certificate behavior is specified in `docs/planning/c-mitm-leaf-certificate-design.md`. Before implementation ships, tests must prove cache miss generation, encrypted-at-rest private-key storage, cache hit reuse without regeneration, TTL bounded by certificate validity, CA and KMS/deployment-key rotation, local singleflight, Redis distributed lock coalescing, bounded generation concurrency, and per-tenant/global unique-SNI flood limits.
