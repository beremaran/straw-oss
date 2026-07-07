# Straw

Straw is a secure, distributed HTTP/HTTPS egress proxy control plane and worker system. It lets platform teams route, inspect, validate, throttle, and audit outbound request traffic from internal workloads before it reaches the public internet.

**Full documentation: https://beremaran.github.io/straw/docs**

## Quick Start

```bash
cd deploy/docker
STRAW_BOOTSTRAP_SYSTEM_ADMIN_KEY=sk_example_admin_local docker compose up -d --build
curl -fsS http://localhost:9090/readyz && echo "Control plane ready"
```

See the [Quickstart Guide](docs/public/quickstart.md) for minting an API key and sending your first request.

## Repository Layout

- `cmd/control`, `cmd/egress`, `cmd/straw` — Control plane, Egress worker, and CLI entrypoints.
- `internal/` — Control, Egress, config, NATS/Postgres/Redis, and CLI implementation packages.
- `sdk/` — Public Go client SDK ([SDK guide](docs/public/sdk.md)).
- `deploy/docker/` — Docker Compose stack for local development.
- `docs/public/` — Source of the public documentation site (rendered by `website/`).
- `docs/planning/`, `docs/tasks/` — Internal architecture and task-tracking docs for contributors; not part of the public site.

## Development

Coding agents and contributors working in this repo should start at [`CLAUDE.md`](CLAUDE.md), which indexes the internal planning docs and task boards.

Run the full check suite before sending changes:

```bash
make check
```
