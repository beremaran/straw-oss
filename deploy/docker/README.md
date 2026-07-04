# Docker Deployment

Local docker-compose support for the full P0 vertical slice: Control, Egress, NATS, Postgres, Redis, ClickHouse.

## Files

- `Dockerfile.control`, `Dockerfile.egress` — multi-stage builds (distroless static runtime).
- `control.json`, `egress.json` — baked-in config; secrets/DSNs come from environment (see `docker-compose.yml`).
- `nats-server.conf` — sets `max_payload: 2MB` (required; the stock 1 MiB default fails Control startup validation
  against the default 1 MiB frame size).
- `clickhouse-schema.sql` — P0 tables from `docs/planning/22`, applied on first ClickHouse boot.

## Ports

| Service    | Port | Purpose                        |
|------------|------|--------------------------------|
| control    | 8080 | REST/config/admin API          |
| control    | 9090 | Metrics + `/healthz` `/readyz`  |
| nats       | 4222 | client / 8222 monitoring       |
| postgres   | 5432 | Postgres                       |
| redis      | 6379 | Redis                          |
| clickhouse | 8123 | HTTP interface                 |

## Start / stop

```sh
# Build and start the whole stack (Control waits for the four backends to be healthy).
docker compose up -d --build

# Watch Control become ready.
curl -fsS http://localhost:9090/readyz && echo ready

# Tail logs.
docker compose logs -f control egress

# Tear down (add -v to also drop data volumes).
docker compose down
docker compose down -v
```

Control's compose healthcheck runs `control -healthcheck`, which probes its own `/readyz`; `/readyz` returns 503
once graceful-shutdown drain begins (`docs/planning/29`).

## Worker provisioning

The `egress` service registers successfully out of the box. `docker-compose.yml` bakes in a **dev-only** ed25519
keypair: `STRAW_WORKER_PRIVATE_KEY_ED25519_BASE64` on the `egress` service is the persistent private key egress loads
(`cmd/egress/main.go`, `egress.private_key_ed25519_env` in `egress.json`); `STRAW_BOOTSTRAP_WORKER_CREDENTIAL_ID` and
`STRAW_BOOTSTRAP_WORKER_PUBLIC_KEY_ED25519_BASE64` on the `control` service seed a matching `worker_credentials` row
on first startup (`control.BootstrapWorkerCredentialFromEnv`, `internal/control/bootstrap.go`) if that credential ID
doesn't already exist. Both reference the same key pair, so Control's signature check against the seeded credential's
public key succeeds. **Never reuse this keypair outside local development** — generate and provision a real one
through the `/worker-credentials` admin API for any non-dev deployment.

Registration also requires a nonce and issued-at timestamp within the configured clock-skew tolerance
(`control.worker.registration_clock_skew_ms`, default 60s) and is checked against a Redis-backed nonce store for
replay protection (`docs/planning/27-security-controls.md`); this needs no extra compose configuration since Redis
is already part of the stack.

The automated proof of the end-to-end REST -> Control -> NATS -> Egress -> upstream path is the in-process Go test
`TestDispatcherControlNATSEgressRoundTrip` (run with `go test ./internal/control/`), which controls both the worker
key and the registered credential. See `docs/agents/testing-matrix-audit.md`.

To exercise the request API against the running Control (routing/admission/validation, without a live egress worker),
bootstrap an admin key by setting `STRAW_BOOTSTRAP_SYSTEM_ADMIN_KEY` before `docker compose up`, then call the admin
and `POST /api/v1/requests` endpoints on port 8080.
