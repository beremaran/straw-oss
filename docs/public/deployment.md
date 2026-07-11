---
sidebar_position: 8
---

# Deployment

## Local development

`make dev` is the supported starting point. It uses `deploy/local/docker-compose.yml` and starts exactly NATS,
Control, and one Egress worker.

Override occupied host ports without editing files:

```sh
STRAW_NATS_PORT=14222 \
STRAW_NATS_MONITOR_PORT=18222 \
STRAW_CONTROL_API_PORT=18080 \
STRAW_CONTROL_METRICS_PORT=19090 \
make dev
```

These variables affect host mappings only.

## Production pattern

`deploy/production` is an example to adapt, not a turnkey platform. It demonstrates authenticated NATS, a required
Control token, internal backend networking, loopback-bound public ports, health checks, read-only containers, and
independently scalable workers.

```sh
cp deploy/production/.env.example deploy/production/.env
$EDITOR deploy/production/.env
docker compose --env-file deploy/production/.env \
  -f deploy/production/compose.yml config
docker compose --env-file deploy/production/.env \
  -f deploy/production/compose.yml up -d --build
```

Before real use:

- terminate TLS in a reverse proxy or load balancer in front of Control;
- move secrets into your secret manager;
- pin reviewed images by immutable tag or digest;
- choose a NATS availability model appropriate to your environment;
- restrict NATS, metrics, and worker health ports to trusted networks;
- set resource, file-descriptor, and log-retention limits;
- run separate Straw deployments for separate trust or policy boundaries.

Straw has no application database to migrate or back up. Back up only infrastructure and configuration you add.

Validate the example with `make production-deploy-check`.
