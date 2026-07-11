# Straw

Straw is a secure, distributed HTTP/HTTPS egress proxy control plane and worker system. It lets platform teams route, inspect, validate, throttle, and audit outbound request traffic from internal workloads before it reaches the public internet.

**Full documentation: https://beremaran.github.io/straw/docs**

## Quick Start

```bash
make infra-up
curl -fsS http://localhost:9090/readyz && echo "Control plane ready"
```

The command creates redacted local credentials under `deploy/local/.dev/`, starts the backing services, Control,
Egress, telemetry, and documentation stack, and provisions a tenant-scoped requester key. See the
[Quickstart Guide](docs/public/quickstart.md) for sending your first request.

## Repository Layout

- `cmd/control`, `cmd/egress`, `cmd/straw` — Control plane, Egress worker, and CLI entrypoints.
- `internal/` — Control, Egress, config, NATS/Postgres/Redis, and CLI implementation packages.
- `sdk/` — Public Go client SDK ([SDK guide](docs/public/sdk.md)).
- `deploy/docker/` — Control and Egress container build definitions.
- `deploy/local/` — Standalone local Compose stack, backing-service configuration, observability, and bootstrap scripts.
- `deploy/production/` — Production deployment templates and checks.
- `docs/public/` — Source of the public documentation site (rendered by `website/`).
- `docs/planning/`, `docs/features/`, `docs/security/`, `docs/implementation-history.md` — Architecture, durable
  capability/security records, and consolidated implementation history; not part of the public site.

## Development

Coding agents and contributors working in this repo should start at [`CLAUDE.md`](CLAUDE.md), which indexes the internal planning docs and task boards.

Run the full check suite before sending changes:

```bash
make check
```

The repository is self-contained for proxy development. Applications that consume Straw—including scraper runtimes
and browser/session harvesters—belong in their own repositories and connect through Straw's public API and SDKs.
