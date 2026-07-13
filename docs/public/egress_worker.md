# Egress workers

The official Go worker is the normal choice. It registers with Control over NATS, advertises its concurrency, and
executes outbound HTTP/HTTPS requests.

## Run another official worker

Use the same NATS settings as Control and choose a unique `worker_id`:

```sh
go run ./cmd/egress -config /path/to/egress.json
```

In Compose, scale the service instead:

```sh
docker compose -f deploy/production/compose.yml up -d --scale egress=3
```

Each worker exposes `/healthz` and `/readyz` on its configured health port. Readiness becomes successful after the
worker has connected and registered.

## Place workers near destinations

Workers only need outbound access to NATS and destination hosts. They do not expose the public Control API. You can
place workers in different networks or regions, provided they can reach the deployment's secured NATS service.

## Claim executor pools

The official worker claims `default/default` when `capabilities.allowed_pools` is omitted. To populate another
configured pool, list it in the Egress JSON:

```json
{
  "config_version": "v1",
  "egress": {
    "capabilities": {
      "allowed_pools": [
        {"pool_id": "default"},
        {"pool_id": "residential-au"}
      ],
      "tags": ["residential"],
      "countries": ["AU"],
      "regions": ["ap-southeast-2"],
      "ip_types": ["residential"]
    }
  }
}
```

Each referenced pool must already exist in the deployment snapshot. Pool references are unique, default to the
single `default` deployment when `deployment_id` is omitted, and cannot name another deployment. Control rejects
invalid or duplicate claims at registration. Membership alone is not enough for assignment: the worker's executor
type must exactly match the pool, it must advertise every required pool tag, and its claimed capability values must
remain within the pool's allowed country, region, and IP-type lists. Disabled pools never receive new assignments;
degraded workers are admitted only when the pool allows them.

## Custom workers

[`straw-sdk-go/egress`](https://github.com/beremaran/straw-sdk-go) and the tagged Python SDK expose worker machinery
for specialized executors. The canonical Egress implementation in this repository exercises the Go SDK base. Custom
workers must keep protocol versions, heartbeats, assignment acknowledgements, capacity, deadlines, and stream framing
correct. Treat this as an advanced integration surface and pin exact SDK and binding tags.

The lifecycle every worker must implement:

```mermaid
sequenceDiagram
  participant W as Worker
  participant N as NATS
  participant C as Control

  W->>N: connect with worker credentials
  W->>C: register ID, protocol range,<br/>capabilities, max concurrency
  loop while running
    W->>C: heartbeat with capacity
  end
  C->>W: assignment
  W-->>C: acknowledge (final admission authority)
  C->>W: bounded request body frames
  W-->>C: response frames as credit is granted
  W-->>C: terminal outcome frame
  Note over W,C: on shutdown: stop heartbeats and admission,<br/>drain active assignments, close NATS cleanly
```

A worker registers a stable ID, protocol range, executor type, claimed pool memberships, tags/locations/IP modes, ingress modes, fingerprint
profiles, and maximum concurrency. It becomes assignable only after authenticated registration and heartbeats.
Admission must reject unsupported request modes/profiles before request start. Stream request bodies only as Control
grants bounded frames; return download credit as response bytes are consumed. Preserve ordered duplicate headers,
deadlines, attempt numbers, sequence validation, cancellation, and terminal error mapping.

Runtime-admin workers validate snapshots before acknowledging the version and continue active requests on their
original immutable snapshot. Receipt-capable workers treat assignment URLs as short-lived credentials, re-check size
and SHA-256, and never expose storage credentials. On shutdown, stop heartbeats/admission, drain active assignments
within their deadlines, publish terminal outcomes where possible, and close NATS cleanly.

Run `make conformance` and the tagged producer/consumer compatibility workflow before admission. Custom workers own
destination resolution/enforcement, TLS/proxy behavior, credential redaction, resource bounds, and upstream security
equivalent to the official Egress implementation. Protocol framing and runtime-snapshot acknowledgement remain the
least stable pre-1.0 surfaces; do not infer compatibility beyond the published matrix.

The official worker advertises all names in the [built-in fingerprint catalogue](compatibility.md#fingerprint-profile-catalogue).
Custom workers may advertise a subset, but protocol minor 1 registration rejects unknown or duplicate names. An
advertised name promises the pinned TLS/HTTP/2 behavior for that exact profile; HTTP/3 is outside the contract.
