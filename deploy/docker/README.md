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
| control    | 8083 | MITM CONNECT proxy             |
| control    | 9090 | Metrics + `/healthz` `/readyz`  |
| egress     | 8090 | `/healthz` `/readyz` (container-local; not published) |
| nats       | 4222 | client / 8222 monitoring       |
| postgres   | 5432 | Postgres                       |
| redis      | 6379 | Redis                          |
| clickhouse | 8123 | HTTP interface                 |

## Start / stop

```sh
# Generate dev-only MITM CA material before starting Control.
scripts/dev-mitm-ca.sh

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
through the `/api/v1/config/worker-credentials` admin API for any non-dev deployment.

Registration also requires a nonce and issued-at timestamp within the configured clock-skew tolerance
(`control.worker.registration_clock_skew_ms`, default 60s) and is checked against a Redis-backed nonce store for
replay protection (`docs/planning/27-security-controls.md`); this needs no extra compose configuration since Redis
is already part of the stack.

## Live dispatch round-trip

`docker-compose.yml` also seeds a complete dev routing path so a real REST -> Control -> NATS -> Egress -> upstream
request works out of the box: `STRAW_BOOTSTRAP_DEV_TENANT_ID` and `STRAW_BOOTSTRAP_DEV_POOL_ID` on the `control`
service make Control seed a dev tenant, an enabled routing rule targeting the dev pool, and scope the dev worker
credential to that (tenant, pool) (`bootstrapDevProvisioning`, `cmd/control/main.go`). `egress.json` declares the
matching membership via `egress.allowed_pools`, which the worker sends as pool refs at registration. `egress.json`
also declares a dev capability set via `egress.capabilities` (`ip_types: ["datacenter"]`,
`docs/planning/24-static-configuration.md`), so an executor pool's `allowed_ip_types`/`allowed_countries`/
`allowed_regions` restriction can actually exclude or admit the live worker — set a pool's `allowed_ip_types` to a
disjoint value (e.g. `["residential"]`) to observe `route_unavailable`. The dev credential's `allowed_capabilities`
scope is empty, which Control treats as unrestricted, so the declared claims pass registration without extra
seeding. All of this is dev-only seeding; production provisions tenants, routing, pools, and credentials through
the admin API.

To drive it end to end:

```sh
# 1. Start with an admin bootstrap key.
STRAW_BOOTSTRAP_SYSTEM_ADMIN_KEY=<your-dev-admin-key> docker compose up -d --build

# 2. Mint the dev tenant's first requester key (system_admin can bootstrap any tenant's first key).
curl -s -H "Authorization: Bearer <your-dev-admin-key>" -H 'Content-Type: application/json' \
  -d '{"role":"requester"}' \
  http://localhost:8080/api/v1/config/tenants/22222222-2222-4222-8222-222222222222/api-keys

# 3. Execute a request with the returned secret; the response envelope carries the real upstream status/body.
curl -s -H "Authorization: Bearer <requester-secret>" -H 'Content-Type: application/json' \
  -d '{"method":"GET","url":"https://example.com/","timeout_ms":15000}' \
  http://localhost:8080/api/v1/requests
```

Request metadata lands asynchronously in ClickHouse (`straw.request_events`, ~1s flush):
`curl -s http://localhost:8123/ --data-binary 'SELECT * FROM straw.request_events FORMAT Vertical'`.

The in-process proof of the same path is `TestDispatcherControlNATSEgressRoundTrip`
(`go test ./internal/control/`), which controls both the worker key and the registered credential. See
`docs/agents/testing-matrix-audit.md`.

## Live HTTP/2 MITM round-trip

The local compose stack enables the P2 MITM CONNECT proxy on `8083`, using dev-only CA material from
`.dev/mitm-ca` and a local AWS-KMS-compatible mock for encrypted leaf-bundle cache writes. Generate the CA before
starting Control:

```sh
scripts/dev-mitm-ca.sh
STRAW_BOOTSTRAP_SYSTEM_ADMIN_KEY=<your-dev-admin-key> docker compose up -d --build
curl -fsS http://localhost:9090/readyz && echo ready
```

Mint the dev tenant requester key as in the REST round-trip section, then drive one HTTP/2 MITM request:

```sh
STRAW_REQUESTER_SECRET=<requester-secret> go run ./scripts/mitm-h2-request.go
```

Expected output includes `proto=HTTP/2.0` and the upstream HTTP status. The request is an authenticated CONNECT to
Control, an inner TLS handshake signed by the dev MITM CA, and a normal Control -> NATS -> Egress stream dispatch.

## Body object lifecycle

When `control.body_transport.object_storage.enabled` is true, Control applies the S3 bucket lifecycle rule at startup
with the configured `body_retention_days` value. The rule expires BodyRef objects under the `tenant/` prefix, covering
objects orphaned by a Control crash after upload. Compose uses the same path as production; override retention in
`deploy/docker/control.json`, bounded to 1-3 days by config validation.

## Running the Postgres-backed tests

The Postgres test harness truncates identity tables between tests, so it refuses to run against any
database whose name does not end in `_test`. Never point `STRAW_TEST_POSTGRES_DSN` at the compose
stack's live `straw` database — use the dedicated `straw_test` database:

```sh
STRAW_TEST_POSTGRES_DSN='postgres://postgres:postgres@localhost:5432/straw_test?sslmode=disable' go test ./...
```

`straw_test` is created automatically by `deploy/docker/postgres-init.sql` when the `postgres_data`
volume is first initialized. On a volume that predates the init script, create it once manually:

```sh
docker compose exec postgres createdb -U postgres straw_test
```
