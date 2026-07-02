## 30. Testing Matrix

P0 must include contract-mapped tests.

| Area          | Required tests                                                                                                 |
|---------------|----------------------------------------------------------------------------------------------------------------|
| Protobuf      | `buf lint`, `buf breaking`, BodyRefFrame compiles, AssignRequest credit fields present, unknown enum rejection |
| NATS subjects | exact assignment subject, no pool queue dispatch, safe token validation, max payload validation                |
| Registration  | valid registration, invalid signature, out-of-scope pool, incompatible version, duplicate session              |
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
| HTTP behavior | P0 disables CONNECT, outbound HTTP/2, and upstream keep-alives unless explicit tested feature flag is enabled  |
| Redaction     | URL userinfo rejected; query dropped by default; auth/cookie headers absent from logs/ClickHouse               |
| Config API    | endpoint phase labels, expected version conflict, tenant version increment, config/admin path separation       |
| Invalidation  | Redis pub/sub invalidation, missed pub/sub corrected by version check, API key revocation invalidates cache    |
| ClickHouse    | async write success, outage, bounded queue drop, sanitized target_url                                          |
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

P1/P2 add proxy, CONNECT, MITM, BodyRef, payload capture, Provider Adapter, telemetry read APIs, connection pooling,
and HTTP/2 test rows before those features ship.
