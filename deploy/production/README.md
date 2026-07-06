# Production Compose Template

This is the initial production template target for P1 task 19. It uses Compose because the repository already ships
Compose builds and configs; Kubernetes/Swarm templates can be added later if there is an owner decision.

## Files

- `compose.yml` defines Control, Egress, NATS, Postgres, Redis, ClickHouse, Prometheus, Blackbox, Grafana, and an
  optional Postgres backup job.
- `control.json` and `egress.json` are non-dev service configs. Replace `egress.worker_id`,
  `egress.credential_id`, and `egress.allowed_pools` before running real workers.
- `.env.example` lists the required runtime secrets and sizing knobs.

## Ports

Only shipped ingress surfaces are published:

| Port | Service | Purpose |
|------|---------|---------|
| 8080 | Control | REST/config/admin API |
| 8081 | Control | HTTP forward proxy |
| 8082 | Control | raw CONNECT proxy |
| 9090 | Control | metrics and health/readiness |
| 9091 | Prometheus | optional observability profile |
| 3000 | Grafana | optional observability profile |

NATS, Postgres, Redis, ClickHouse, and Egress health are backend-network only.

## Operator Responsibilities

- Back up Postgres and test restores. The `backup` profile writes SQL dumps to `postgres_backups`, but scheduling and
  off-host retention are operator-owned.
- Set ClickHouse retention and disk sizing for request metadata and log volume.
- Size Redis memory with `REDIS_MAXMEMORY`; the template uses `noeviction` so overload is explicit.
- Run NATS as highly available infrastructure for production. This template is single-node and does not define
  regional leaf nodes, superclusters, or separate clusters.
- Terminate TLS at a load balancer or reverse proxy in front of Control/proxy ports, or mount certificates into a
  deployment-specific sidecar. The Straw binaries in this repo do not terminate TLS directly.
- Store `.env` values in a real secret manager. Do not commit generated `.env` files or worker private keys.
- Keep `edge` and `backend` networks isolated; only Control and optional observability UIs should have published ports.
- Operate Prometheus/Grafana/Blackbox when the `observability` profile is enabled.

## Validation

```sh
cp deploy/production/.env.example deploy/production/.env
$EDITOR deploy/production/.env
make production-deploy-check
docker compose --env-file deploy/production/.env -f deploy/production/compose.yml config
```

Regional NATS and managed disaster recovery are intentionally unsupported by this template.
