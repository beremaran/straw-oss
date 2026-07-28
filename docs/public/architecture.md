# Architecture

Straw has three runtime components:

```mermaid
flowchart LR
  App["Application"] -->|HTTP| Control
  Control -->|NATS request/reply| NATS[("NATS")]
  NATS --> Egress["Egress worker"]
  Egress -->|HTTP/HTTPS| Dest["Destination"]
  Control -.-> Obs["Health, readiness,<br/>Prometheus metrics"]
  Control -. optional runtime config .-> JS[("JetStream KV")]
  Control -. optional HA coordination .-> Redis[("Redis")]
  Control -. optional receipts .-> OS[("Object storage")]
  Egress -. assignment-scoped download .-> OS
```

**Control** exposes the REST request API and HTTP/HTTPS proxy ingress, validates authentication and input, applies the
deployment policy, selects a healthy worker, and relays request, response, or tunnel frames. The optional runtime
profile also exposes the Admin/Config API and dashboard. Controls are interchangeable when the Redis runtime-state
profile is enabled.

**Egress** registers with Control, advertises capacity, executes outbound requests, and streams responses back. Add
workers when you need more concurrency or network locations.

**NATS** provides discovery, assignment, request/response transport, and runtime snapshot distribution. The default
profile uses Core NATS only. The optional runtime-administration profile enables file-backed JetStream KV for current
configuration and audit history.

**Redis** is optional and stores only expiring coordination state for highly available Control: worker sessions and
heartbeats, capacity, cooldowns, sticky pins, request ownership, cancellation routing, Control instance leases, and
the active configuration version. Redis never stores request or response bodies and is not the durable configuration
authority.

**Object storage** is optional. It stores durable receipt records, resumable upload parts, and verified request or
response bodies. Control streams verification and composition; NATS carries only a short-lived body reference for a
receipt request. Egress downloads through the assignment URL and verifies size and SHA-256 before use.

## Request lifecycle

```mermaid
sequenceDiagram
  participant App as Application
  participant C as Control
  participant W as Egress worker
  participant U as Destination

  App->>C: POST /api/v1/requests
  C->>C: validate token, shape, URL,<br/>headers, body size, timeout
  C->>W: assignment over NATS
  W-->>C: acknowledge
  W->>U: outbound HTTP/HTTPS request
  U-->>W: upstream response
  W-->>C: response frames over NATS
  C-->>App: status, headers, body,<br/>phase timings as one JSON response
```

1. A client posts a request to Control.
2. Control validates the bearer token, JSON shape, URL, headers, body size, and timeout.
3. Control selects an available worker from the rule's enabled pool, requiring claimed membership, exact executor
   type, required tags, allowed capabilities, health/degraded policy, available capacity, and an exact upstream-proxy
   profile claim when the pool uses one.
4. The worker acknowledges the assignment and performs the outbound request either directly or through the pool's
   configured HTTP CONNECT gateway.
5. Control returns the upstream status, headers, body, and phase timings in one JSON response.

With receipt transport, an application first uploads and verifies a request receipt. Control claims it for one
request and sends a `BodyRef` instead of body frames. For a stored response, Control writes response frames directly
to bounded object parts and returns an authorized receipt after the terminal frame.

In HA mode, NATS queue subscriptions may deliver a worker registration or heartbeat to any Control. The receiving
instance updates Redis with a session fence and TTL, so every Control routes against the same fleet. A request remains
owned by the Control holding its client connection; the shared owner record lets another instance forward an
administrative cancellation to it over NATS.

GET, HEAD, and OPTIONS requests are replayable by default in the tagged clients. Other methods are not retried unless
the caller explicitly marks them replayable. REST, absolute-form proxy, and CONNECT share the same route evaluation;
the ingress mode is an additional match/capability constraint when configured.

## Destination policy and egress safety

Control captures the deployment policy at request start and sends the resolved policy bundle with the assignment.
The selected executor pool determines one of two explicit resolution modes. An executor pool without an
`upstream_proxy` object uses direct-local resolution. A pool with an `upstream_proxy` object uses trusted upstream-proxy
remote resolution and must set `trusted_remote_resolution: true`; it never silently falls back to direct networking.

In direct-local mode, Control performs a fail-fast check for literal IPs and host rules. Egress remains authoritative:
immediately before a connection it resolves every address, validates every resolved IP, checks the configured CNAME
suffix policy, and only then dials the first validated address. A policy failure returns `destination_denied`; it never
falls back to an unvalidated address.

In trusted upstream-proxy remote mode, Control and Egress still validate the URL, hostname and suffix rules, port, and
every literal target IP. Hostname targets are sent to the statically configured proxy endpoint without local target DNS,
because the provider owns destination resolution. Consequently, Straw cannot provide direct-local-equivalent CIDR,
CNAME-chain, or private-hostname/private-address enforcement for hostname targets in this mode. Local DNS preflight is
not an equivalent safeguard: it can disagree with provider geo DNS and cannot prevent remote rebinding. The trust flag
is an explicit operator acknowledgement of this loss. Configure provider destination ACLs to deny private, metadata,
loopback, and special-use addresses whenever the provider supports them.

### Rule types and precedence

Runtime snapshots use these destination rule types:

Resolved-address and CNAME enforcement in this table applies to direct-local hostname resolution. In trusted remote
mode, literal IP rules still apply, but provider-resolved hostname addresses and CNAME hops are not visible to Straw.

| Rule type | Pattern | Enforcement |
| --- | --- | --- |
| `host` | normalized hostname | exact target-host match |
| `host_suffix` | hostname, optionally entered as `*.example.com` or `.example.com` | exact host or a dot-boundary subdomain match |
| `cname_suffix` | hostname suffix | any returned CNAME hop, not just the final address |
| `cidr` | CIDR | matching resolved address |
| `ip` | one IP address | that exact address, compiled as a host CIDR |
| `private_range` | private CIDR | matching private address; the type is an operator-facing label |
| `metadata_ip` | one recognized metadata address | exact metadata address |

`deny` is the normal action. `allow_override` is explicit and deployment-scoped:

- for `cidr`, `ip`, `private_range`, and `metadata_ip`, the matching allowed CIDR is checked before configured denies
  and built-in denies, so it is a true resolved-address override;
- for `host`, `host_suffix`, and `cname_suffix`, pair the override with the same normalized value as the deny. Control
  and Egress then compile that matching deny out of their respective host or CNAME lists;
- IPv4-mapped IPv6 addresses are always rejected and cannot be overridden.

Rules are normalized from `raw_pattern`: hostnames are lowercased and IDNA-normalized, CIDRs are masked to their
canonical network, and IPs are canonicalized. A hostname target is not treated as safe merely because its name looks
public; the resolved addresses are checked at dial time as defense against DNS changes and rebinding.

### Built-in denied destinations

Unless an explicit allowed CIDR overrides them, Egress denies literal targets and direct-local resolved addresses in
loopback, RFC1918/ULA private ranges, link-local unicast, multicast, and these additional special-use ranges:

```text
0.0.0.0/8          100.64.0.0/10       192.0.0.0/24
192.0.2.0/24       192.88.99.0/24      198.18.0.0/15
198.51.100.0/24    203.0.113.0/24     240.0.0.0/4
255.255.255.255/32
::/128             64:ff9b::/96       100::/64
::ffff:0:0/96
```

The recognized metadata addresses are `169.254.169.254`, `169.254.169.253`, `169.254.170.2`, `100.100.100.200`,
and `100.100.100.201`. Keep network-level egress controls in place as a second boundary; an operator should not rely
on application policy as the only protection for cloud metadata or private networks.

### Resolution, TLS, and redirects

Direct-local pools resolve with the worker's local DNS and validate every returned address before opening a socket.
Trusted upstream-proxy pools delegate hostname resolution as described above; literal IP validation is unchanged.
Target TLS always uses the original normalized destination hostname for SNI and certificate verification, independently
of any TLS connection to an HTTPS proxy endpoint. Straw does not follow HTTP redirects. A 3xx response is returned to
the caller and the redirected target is never fetched.

## Forward-proxy lifecycle

Absolute-form HTTP proxy requests use the decoded request pipeline, but Control streams the upstream response
directly instead of wrapping it in JSON. CONNECT assignments use the protocol's raw-tunnel mode. Egress applies the
destination policy and opens the TCP socket before its success frame causes Control to return `200 Connection
Established`; after that point NATS data and credit frames provide bounded bidirectional flow control. Tunnel bytes
remain opaque to Straw. Authenticated proxy routing hints are normalized into the same `RoutingHints` input as REST,
and the reserved `X-Straw-*` namespace is stripped before decoded forwarding or tunnel establishment. Assignment
rejection can exclude a worker and retry before the first client-visible response bytes; the first raw response header
or `200 Connection Established` is a no-replay and no-reroute boundary.

## Trust boundary

One Straw deployment is one trust and configuration boundary. Run separate deployments when workloads need isolated
credentials, policy, networks, or operators. The runtime deliberately serves a deployment rather than acting as a
hosted multi-tenant platform.

## Source map

- `cmd/control`, `internal/control`: public API, proxy ingress, and dispatch pipeline
- `cmd/egress`, `internal/egress`: official worker and HTTP executor
- `internal/natsx`: NATS connection and subjects
- [`straw-protos`](https://github.com/beremaran/straw-protos): canonical language-neutral worker protocol
- [`straw-protos-go`](https://github.com/beremaran/straw-protos-go): exact-tag Go bindings consumed by this runtime
- [`straw-protos-python`](https://github.com/beremaran/straw-protos-python): exact-tag Python bindings consumed by the
  Python SDK
- [`straw-sdk-go`](https://github.com/beremaran/straw-sdk-go): Go client and common worker SDK machinery
- [`straw-sdk-python`](https://github.com/beremaran/straw-sdk-python): Python client and common worker SDK machinery
- `deploy/local`: supported development stack
- `deploy/production`: production-pattern example
