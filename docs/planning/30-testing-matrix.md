## 30. Testing Matrix

P0 must include contract-mapped tests.

| Area          | Required tests                                                                                                 |
|---------------|----------------------------------------------------------------------------------------------------------------|
| Protobuf      | `buf lint`, `buf breaking`, unknown field tolerance, unknown enum rejection                                    |
| NATS subjects | exact assignment subject, no pool queue dispatch, safe token validation, max payload validation                |
| Registration  | valid registration, invalid signature, out-of-scope pool, incompatible version, duplicate session              |
| Heartbeat     | ready/degraded/unhealthy, unavailable after 15s, dead after 30s, stale session ignored                         |
| Worker state  | disabled precedence, draining exclusion, cooldown entry/exit, duplicate replacement, stale heartbeat ignored   |
| Routing       | priority order, hard client hints, tenant isolation, no match, unavailable, sticky success/failure             |
| Assignment    | accept, reject capacity, reject draining, ack timeout, no duplicate retry                                      |
| Streaming     | sequence gaps, duplicates, out-of-order frames, offset mismatch, credit exhaustion, idle timeout               |
| Terminal      | duplicate terminal ignored/counted, late frame ignored, worker death synthesizes terminal outcome              |
| Cancellation  | client disconnect, deadline, admin cancel, late frame ignored                                                  |
| Fallback      | fallback before `RequestStart`, no fallback after `RequestStart` unless replayable, no silent replay           |
| Error mapping | every ErrorCode maps to HTTP/retry/category; origin 4xx/5xx passthrough is not ErrorResponse                   |
| REST schema   | valid request, invalid fields, header duplicate preservation, inline body limit, CONNECT rejected              |
| REST outcome  | upstream 404/500 returns API HTTP 200 with upstream status in envelope; Straw errors return ErrorResponse      |
| Body limits   | request body over cap returns `body_too_large`; response body over cap returns `body_too_large` with direction |
| P0 exclusions | capture_hint other than `none` rejected; redirect-following flag rejected/unsupported; BodyRef rejected        |
| Rate limits   | dimensions, 429, retry_after, Redis fail policy, memory guardrail fallback                                     |
| Quotas        | request count, bandwidth accounting, admission policy, Redis loss behavior, no billing-grade claim             |
| Deny rules    | domain, CIDR, private IP, metadata IP, DNS rebinding, redirect target future test, IDNA normalization          |
| Egress policy | RequestStart carries DestinationPolicy; Egress enforces resolved-IP deny without querying Control DBs          |
| HTTP behavior | P0 disables CONNECT, outbound HTTP/2, and upstream keep-alives unless explicit tested feature flag is enabled  |
| Redaction     | URL userinfo rejected; query dropped by default; auth/cookie headers absent from logs/ClickHouse               |
| Config API    | expected version conflict, tenant version increment, path separation for config/admin                          |
| Invalidation  | Redis pub/sub invalidation, missed pub/sub corrected by version check, API key revocation invalidates cache    |
| ClickHouse    | async write success, outage, bounded queue drop, sanitized target_url                                          |
| Load          | routing p50/p99, assignment latency, active request limit, worker capacity behavior                            |
| Auth          | platform key cannot execute data-plane requests; tenant key cannot create tenants; revoked key invalidation    |
| Audit         | injection policy secret values redacted in Postgres audit source and ClickHouse; API key secret never logged  |
| Identifiers   | duplicate worker_id across tenants rejected; multi-tenant pool scope validated                                |
| HTTP validation| invalid header names rejected; CR/LF header injection rejected; URL fragment rejected; IDNA normalization    |
| NATS ordering | subscriber flush proves RequestStart not lost; stream subject publish before subscribe fails in test harness   |
| SSRF          | local DNS validation connects only to validated IP; DNS rebinding between validation and dial is blocked       |
| Timeout       | total deadline wins over phase timeout when simultaneous; phase timeouts bounded by remaining deadline         |

P1/P2 add proxy, CONNECT, MITM, BodyRef, payload capture, Provider Adapter, telemetry read APIs, connection pooling,
and HTTP/2 test rows before those features ship.
