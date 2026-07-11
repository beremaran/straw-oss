# Production deployment pattern

This directory is an example, not a turnkey production platform. It demonstrates the minimum service topology and
security boundaries for running Straw with Docker Compose:

- one Control service;
- one or more Egress workers;
- one authenticated NATS service on an internal network;
- Control API and metrics bound to loopback for a host reverse proxy or load balancer.

## Try the template

```sh
cp deploy/production/.env.example deploy/production/.env
$EDITOR deploy/production/.env
docker compose --env-file deploy/production/.env -f deploy/production/compose.yml config
docker compose --env-file deploy/production/.env -f deploy/production/compose.yml up -d --build
```

Send `Authorization: Bearer <STRAW_AUTH_TOKEN>` with every Control request.

Scale workers independently:

```sh
docker compose --env-file deploy/production/.env \
  -f deploy/production/compose.yml up -d --scale egress=3
```

## Adapt it before production

- Pin images to reviewed immutable tags or digests.
- Put a TLS-terminating reverse proxy or load balancer in front of Control.
- Store `STRAW_AUTH_TOKEN` and the NATS password in your secret manager rather than a file.
- Run NATS in the topology and availability model your environment requires.
- Restrict metrics to your monitoring network.
- Set CPU, memory, file-descriptor, and log-retention limits.
- Add backups only for infrastructure you add; Straw itself has no durable database.
- Use separate NATS accounts/credentials when stronger Control/worker separation is required.

Validate the checked-in example with:

```sh
make production-deploy-check
```
