# Handoff

Task: `docs/tasks/p0/25-p0-test-matrix-and-compose.md`

## Changed

- `cmd/control/health.go` (new) — `/healthz` (liveness) and `/readyz` (readiness) mux on the metrics port.
  `/readyz` returns 503 once the readiness flag is cleared.
- `cmd/control/health_test.go` (new) — `TestHealthzAlwaysOK`, `TestReadyzReflectsReadiness`.
- `cmd/control/main.go` — serve the metrics/readiness server on `server.metrics_port`; clear readiness on shutdown
  drain (docs/planning/29); added `-healthcheck` flag (probes own `/readyz`, used by the distroless container
  healthcheck); wired the live ClickHouse request-metadata writer (`wireClickHouse`) into the request handler;
  extracted `openStores` and `serveControl` helpers.
- `internal/config/config.go` — added optional `ClickHouseConfig` under `database.clickhouse` (endpoint, database,
  user/password env names, queue/batch/flush tuning) with defaults. Empty endpoint = telemetry disabled (no-op).
- `internal/control/dispatcher_test.go` — `TestDispatcherNATSUnavailable` (NATS outage -> `transport_unavailable`).
- `docker-compose.yml` — full P0 stack: added `clickhouse`, `control`, `egress`; healthchecks + `depends_on:
  service_healthy` ordering; NATS switched to `nats:alpine` (scratch image lacks a probe tool); healthchecks use
  `127.0.0.1` (container `localhost` resolves to IPv6, backends bind IPv4).
- `deploy/docker/Dockerfile.control`, `Dockerfile.egress` (new) — multi-stage distroless builds.
- `deploy/docker/control.json`, `egress.json` (new) — compose config; DSN/URL/secrets via env.
- `deploy/docker/clickhouse-schema.sql` (new) — P0 tables (request_events, worker_events, config_audit_events,
  log_events) from docs/planning/22; applied on first ClickHouse boot. `payload_capture_events` (P2) omitted.
- `deploy/docker/README.md` — compose start/stop, ports, and the worker-provisioning limitation.
- `docs/agents/testing-matrix-audit.md` (new) — every row of docs/planning/30 mapped to tests.
- `docs/tasks/p0.md`, `docs/tasks/p0/25-*.md` — status -> done.

## Verification

```sh
make check
docker compose up -d --build   # then curl http://localhost:9090/readyz
```

Result:
- `make check` passes: gofmt clean, `go test ./...` green (Postgres-backed tests skip without
  `STRAW_TEST_POSTGRES_DSN`), `golangci-lint ... --max-issues-per-linter 0 --max-same-issues 0` = 0 issues.
- Compose stack brought up live: `redis`, `nats`, `postgres`, `clickhouse`, `control` all report **healthy**.
  ClickHouse init applied all 4 P0 tables. Control startup ran Postgres migrations, connected NATS with payload
  validation, connected Redis, and enabled ClickHouse telemetry. `GET /readyz` -> `ready`, `GET /healthz` -> `ok`.
  `POST /api/v1/requests` returns a structured `ErrorResponse` envelope (endpoint wired). Torn down with
  `docker compose down -v`.

## Reviewer Start Points

- `cmd/control/main.go` (`serveControl`, `serveMetricsHTTP`, `runHealthcheck`, `wireClickHouse`, `openStores`)
- `cmd/control/health.go`
- `docs/agents/testing-matrix-audit.md`
- `docker-compose.yml` + `deploy/docker/`

## Remaining Work

- **Egress auto-registration through the compose stack is not wired.** `cmd/egress/main.go` generates a random
  ed25519 keypair on every boot, so the `egress` container cannot complete registration against Control without a
  pre-seeded worker credential whose public key matches. This is a pre-existing gap that **no P0 task file owns**
  (integration tasks 16-24 covered NATS/Postgres/Redis/dispatch, not egress identity persistence). Flagged to the
  user. The end-to-end REST -> Control -> NATS -> Egress -> upstream path is proven by the in-process Go test
  `TestDispatcherControlNATSEgressRoundTrip`, which controls both the worker key and the registered credential.
- ClickHouse binary wiring is complete (`wireClickHouse` constructs `HTTPClickHouseSink` + `RequestMetadataWriter`
  against the live endpoint). The write path's row-shape/outage/drop behavior is unit-tested; asserting rows land in
  a live ClickHouse table under load is not automated (belongs with P1 telemetry read/verification work).
- `buf lint` / `buf breaking` are tool-gated (the `buf` CLI is not installed locally); protobuf compile + enum
  behavior are covered by `go test` (`api/proto/straw/v1`).
- The "Load" matrix row (p50/p99 latency, load benchmarks) has no automated `go test` row in P0 by design; recorded
  as not-applicable in the audit.

## Blockers

- None for task 25 itself. The egress-identity gap above blocks a *turnkey* compose request flow and needs a
  decision on whether to create a new task (egress identity key persistence + credential seeding) — surfaced to the
  user.
