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

To enable the optional runtime-administration profile, also set a distinct `STRAW_ADMIN_TOKEN` and combine the files:

```sh
docker compose --env-file deploy/production/.env \
  -f deploy/production/compose.yml \
  -f deploy/production/compose.runtime-admin.yml up -d --build
```

This enables file-backed JetStream. Adapt storage, replication, backup, and recovery before using the example.

To enable receipt transport, adapt `control.object-storage.json` for your S3-compatible service, set the receipt and
S3 secrets, and add `compose.object-storage.yml`. The example requests server-side encryption and proxies
assignment-scoped downloads through Control so Egress never receives long-lived storage credentials.

```sh
docker compose --env-file deploy/production/.env \
  -f deploy/production/compose.yml \
  -f deploy/production/compose.object-storage.yml config
docker compose --env-file deploy/production/.env \
  -f deploy/production/compose.yml \
  -f deploy/production/compose.object-storage.yml up -d --build
```

For the optional TLS proxy, install a managed certificate followed by its private key at
`deploy/production/secrets/straw.pem`, then set `STRAW_TLS_BIND` and `STRAW_TLS_PORT` if the defaults
(`0.0.0.0:8443`) do not suit the host:

```sh
install -m 600 /path/to/managed-certificate-and-key.pem deploy/production/secrets/straw.pem
docker compose --env-file deploy/production/.env \
  -f deploy/production/compose.yml -f deploy/production/compose.tls.yml config
docker compose --env-file deploy/production/.env \
  -f deploy/production/compose.yml -f deploy/production/compose.tls.yml up -d --build
make tls-proxy-check
```

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
- Back up JetStream when the runtime-administration profile is enabled.
- Use separate NATS accounts/credentials when stronger Control/worker separation is required.

Validate the checked-in example with:

```sh
make production-deploy-check
```
