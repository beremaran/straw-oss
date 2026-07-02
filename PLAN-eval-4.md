Read the whole thing (3,233 lines) rather than sampling it, since a plan this detailed can hide contradictions between
sections that look fine in isolation. It's a genuinely strong architecture doc — but it has real internal-consistency
problems, and a few of them are the kind that would actively mislead an implementer, not just cosmetic. Here's
everything I found, worst first.

## Three things that would break an implementation

**1. Two incompatible NATS dispatch designs.** §10 is explicit and reasoned: *"Control sends each assignment to the
exact executor session subject, not to a pool queue group, because Control already tracks sticky sessions, capacity,
health, draining, cooldowns, and fallback... NATS queue groups are used for shared Control service handling, not
executor load balancing."* Subjects are per-session: `straw.v1.executor.<worker_id>.<session_id>.assign`.

§20.4.2/20.4.3 describes the opposite mechanism: assignments go to `straw.v1.dispatch.{pool_name}` as a **queue group**,
where *"NATS delivers the message to exactly one worker in the queue group."* That means NATS — not Control — picks the
worker. You cannot have both: queue-group fan-out is incompatible with Control doing least-loaded selection,
sticky-session pinning to a specific worker, or excluding a draining worker from just this one assignment (§8, §9).
Whoever builds this will implement one of the two and silently break the other design's guarantees.

Smaller tells that these were written independently: the cancel message direction is reversed (§7: *"Control sends a
best-effort request-scoped `cancel` message..."* vs. §20.4.2's table:
`straw.v1.cancel.{request_id} | Worker → Control`), and even within §20.4 itself the pool subject is spelled two
different ways — `straw.v1.dispatch.{pool_name}` in the table (20.4.2) vs. `straw.dispatch.pool.{pool_name}` in the
prose right below it (20.4.3).

**2. Two incompatible ClickHouse schemas.** §19.2 defines four flat tables in one database: `requests` (TTL ~90 days, ~
34 columns), `logs`, `traces`, `events`, plus a `metrics_rollups` materialized view. §20.9 defines five separate
*databases* — `audit`, `requests`, `workers`, `payloads`, `metrics` — with a `requests.requests` table that has a
different, smaller column set and an explicit `TTL timestamp + INTERVAL 30 DAY` (not 90). `logs` and `traces` have no
equivalent anywhere in §20.9's schema — they just disappear. Neither section references the other. And control.yaml (
§20.3.1) connects to a single `database: "straw"`, which doesn't match either model, but especially not §20.9's
five-database layout. This needs one owner and one schema.

**3. The headline latency target contradicts itself.** Goals (§2) sets it plainly: *"Keep Control-side routing and
coordination overhead under 500 ms."* The SLO table and alerting rules agree (p99 < 500ms, alert at 500ms). But §15 and
§16.1 both claim Control is built *"to guarantee sub-millisecond routing decisions"* / *"to maintain sub-millisecond
Control-side routing paths,"* and §25 (Open Decisions) worries about billing metering *"degrading sub-millisecond
routing performance."* Sub-millisecond and 500ms are a 500x gap — pick the real number, because it drives very different
architecture (in-memory-only vs. tolerating a Postgres round trip, etc.).

## The error registry in §18.3 isn't actually canonical

Several error identifiers get used, with specific semantics, elsewhere in the doc — but never appear in the "Error Code
Registry" that's supposed to be the source of truth:

| Name used in prose        | Where                                          | Status in §18.3 registry                                                                                                                                                                   |
|---------------------------|------------------------------------------------|--------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------|
| `route_no_match`          | §8, §20.5.3                                    | Missing — closest is `no_matching_route`, mapped to **404**, not the **421** §8 explicitly specifies                                                                                       |
| `route_unavailable`       | §8, §20.5.3                                    | Missing entirely                                                                                                                                                                           |
| worker-loss / worker-lost | §2, §7, §9, §11                                | Missing — closest is `worker_disconnected`, different name                                                                                                                                 |
| `timeout_exceeded`        | §18.1, §18.5                                   | Missing — registry instead has a standalone `worker_timeout`, and 5 of the 6 `TimeoutType` values (CONNECT/REQUEST/IDLE/UPLOAD/DOWNLOAD_TIMEOUT) have no code that could ever produce them |
| `unsupported_fingerprint` | §11, §14, §20.5.4 (named three separate times) | Missing entirely                                                                                                                                                                           |
| `invalid_api_key`         | §20.5.2                                        | Missing — closest is `auth_failure`                                                                                                                                                        |
| `conflict`                | §20.5.1                                        | Missing entirely                                                                                                                                                                           |

On top of that, §18.2 defines exactly five categories (CLIENT, ROUTING, TRANSPORT, EGRESS, STREAMING), but §18.8's
logging table and §19.2's ClickHouse `error_category` column both use an undefined **"System"** category and both drop *
*STREAMING** entirely. This isn't one typo, it's a pattern — the registry needs to actually be built from a sweep of
every error name used in the other 27 sections.

## Reference and naming bugs

- Tenant config (§20.5.1) points `rate_limits`, `quotas`, and `deny_rules` all to *"(see §20.8)"* — §20.8 is
  Observability Configuration. The real section is §20.5.8.
- §20.5.7 points captured payloads to *"the `payloads` table (see §20.10)"* — §20.10 is Config Migration. The table (
  `payloads.captured`) is actually in §20.9.
- `STROW_MITM_CERT_VALIDITY_DAYS` and `STROW_BODY_OBJECT_STORAGE_ENABLED` (lines 2444, 2508, 3012) — should be `STRAW_`.
  Every other one of the ~50 env vars is spelled correctly.
- §6 states the REST API is *"versioned under `/v1`"* with examples like `/v1/config/*`. Every concrete endpoint in
  §20.12 (and the call in §20.5.6) uses `/api/v1/config/...` instead. Pick one base path.
- Adapter env vars (§20.13.3): `STRAW_UPSTREAM_USERNAME` paired with `STRAW_UPSTREAM_PROXY_PASSWORD` — inconsistent with
  each other, and neither matches the adapter.yaml example in §20.3.3, which uses `STRAW_UPSTREAM_USERNAME`/
  `STRAW_UPSTREAM_PASSWORD` (no "PROXY" in either).

## Numbers that don't match their own defaults

- §15: *"S3 data uses a strict 3-day retention window."* §20.7.2 and §20.13.1: `STRAW_BODY_RETENTION_DAYS` default is *
  *`1`**.
- §9 defines a clean two-stage model: 15s = unavailable, 30s = dead. The only config value that exists for this,
  `egress.yaml`'s `heartbeat.timeout_ms: 15000`, is commented `# Control considers worker dead after this` — it's using
  the *unavailable* threshold's value and mislabeling it as *dead*. The 30s dead threshold has **no config key anywhere
  ** in the document — I grepped for `30000` in this context and it doesn't exist.
- Same problem for three other defaults the narrative repeatedly calls out as "default X, configurable": assignment ack
  timeout (2s, §10), duplicate-session grace window (10s, §9), and cooldown trigger/duration (3 failures/60s, 30s
  cooldown, §9). None of these has a config key, YAML field, or env var anywhere in the 1,400-line Configuration
  section — which is otherwise exhaustive enough to give NATS ping intervals their own env var.

## Two things I'd want a second technical opinion on

- **`tls-client` for *inbound* TLS termination.** §13 and §20.6.2 both say Control uses `tls-client` on the inbound leg
  specifically to handle *"JA3/JA4 fingerprint matching... ensuring proxy clients' TLS fingerprints are not flagged as
  non-browser."* JA3/JA4 characterizes the **client's** ClientHello — Control is the *server* on this leg. A client's
  fingerprint is produced by the client's own TLS stack; Control's choice of server-side library can't change what
  fingerprint an incoming scraper's HTTP client emits. `tls-client` (bogdanfinn) is also specifically an outbound HTTP
  client library, not a TLS listener implementation — which is exactly how it's correctly used for Egress in §14. I'd
  bet this got copy-pasted from the Egress section without translating the logic for the server role. Worth a straight
  answer before it's built.
- **Leaf vs. intermediate certificate.** §13 correctly calls the per-domain, dynamically-generated cert a *"leaf
  certificate."* §20.6.1 calls the same artifact an *"intermediate certificate"* three times. These are not
  interchangeable — an intermediate is itself a signing CA. If that's literal, minting a new sub-CA per SNI is a serious
  security anti-pattern; if it's just wrong terminology (almost certainly), fix it everywhere. Related: §20.6.1 says
  these are *"cached in memory"* full stop, which quietly drops the disk/S3 durable tier and the
  Redis-shared-cache-with-singleflight-locking tier that §13 spent real design effort on specifically for horizontal
  scaling. As written, every horizontally scaled Control instance would regenerate its own certs independently.

## Smaller inconsistencies worth a reconciliation pass

- **Routing fallback is described two different ways.** §8: fallback tries another executor in the same rule, *then*
  falls through to *lower-priority rules*. §20.5.3's "Evaluation Order" never mentions falling through to a second rule
  at all — it evaluates one matching rule and cascades only through that rule's own `fallback_pool_ids`. Different
  mechanisms; only one should exist.
- **REST error envelope for upstream errors.** §7 says REST returns upstream status/headers/body directly, same as HTTP
  proxy and MITM, with no Straw envelope. §18.4's mapping table says REST returns `upstream_http_error` as *"
  200 + `upstream_status` field"* — that's an envelope, not passthrough. Contradicts §7 and treats REST differently from
  the other two decoded modes for no stated reason.
- **"Billing" leaks into Phase 1.** §17 opens with *"Straw enforces strict abuse, overload, billing, and compliance
  controls"* and later calls quota counters *"durable billing records."* Meanwhile Non-Goals and Open Decisions both
  explicitly state billing is deferred entirely to Phase 2. Either reword §17 to "usage tracking," or billing scope has
  quietly crept in without anyone reconciling it against the rest of the doc.
- **MITM CA distribution endpoint.** §13 gates the CA-cert download behind Admin RBAC — but every client using MITM
  needs this cert to avoid TLS errors, not just admins. Worth reconsidering the access tier. Separately, that same
  paragraph has a visible text-corruption artifact: *"...they must trust this configured CA. Control\nControl exposes a
  dedicated HTTP endpoint..."* and later *"...requires Admin RBAC.\nprovisioning for scrapers and local environments"* —
  that last fragment doesn't attach grammatically to anything before it. This reads like an unresolved edit collision
  and should be rewritten regardless of what the access-control answer ends up being.

## Coverage and completeness

- **Testing doesn't cover what Goals promises.** §2 commits to test coverage for *"routing, config, parsing, protobuf
  compatibility, NATS request/reply, worker registration, REST/proxy/CONNECT/MITM flows, worker loss, NATS outage,
  timeout paths, backpressure, and load behavior."* §23 delivers on roughly half of that list (worker loss, NATS outage,
  protobuf boundaries, routing logic). There's no config-validation testing, no per-entrypoint test plan, no
  worker-registration testing, no backpressure testing, and — notably, given how much the doc leans on specific latency
  SLOs — no load/performance testing methodology at all.
- **SDK/CLI/UI has no design section.** It's a named Phase 1 Goal, but the only description anywhere is one line in §6:
  *"thin clients over the REST request, admin, and config APIs."* No CLI command surface, no UI feature list. Every
  other Phase 1 component (MITM, Egress execution, worker discovery) gets a dedicated section; this doesn't.
- **Provider Adapter is architecturally first-class but absent from Goals.** §4/§5/§8/§9/§10 treat it as a full peer to
  Egress Workers — same NATS protocol, own config file, own RBAC scope, own pool concept. But §2's Goals bullets
  consistently say "Egress" alone where the architecture clearly means "Egress and Provider Adapter" (fingerprint
  simulation, regional pools, versioned contracts, "one official Egress implementation" with no equivalent commitment
  for adapters). Worth an explicit line in Goals, plus a decision on whether a real Bright Data adapter is a Phase 1
  deliverable or just the adapter protocol.
- **No regional NATS topology**, despite "regional Egress pools" being an explicit Goal — the NATS config (§20.4.1)
  assumes what looks like a single cluster; nothing addresses leaf nodes/superclusters for geographically distributed
  workers.
- **No backup/DR story for Postgres**, the one genuine source of truth. §24 covers outage *behavior* (stale-snapshot
  fallback) but not backup or recovery from actual data loss. Given the doc explicitly disclaims managed DR as an
  operator responsibility elsewhere (e.g., CA distribution), a one-line equivalent disclaimer here would close the gap
  cheaply.

## Bottom line

The individual design decisions are mostly good and the depth is unusual for this stage — RBAC, the routing-dimension
model, the protobuf envelope, and the quota/rate-limit split are all internally solid. But this reads like it was
written in multiple passes that never got reconciled against each other, and that shows up hardest in exactly the places
an implementer would trust most: the NATS transport contract and the ClickHouse schema are each defined twice,
incompatibly. I'd treat those two as blocking — resolve them before anyone starts building — and use the error-code
table above as a checklist for the §18 cleanup.