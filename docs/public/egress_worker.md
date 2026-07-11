---
sidebar_position: 7
---

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

## Custom workers

`sdk/egress` and `python/straw/egress` expose the worker protocol for specialized executors. Start from
`examples/egress-static`. Custom workers must keep protocol versions, heartbeats, assignment acknowledgements,
capacity, deadlines, and stream framing correct. Treat this as an advanced integration surface and pin your Straw
module/package version.
