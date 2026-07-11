# Standalone local infrastructure

This directory owns Straw's complete local proxy stack: Redis, NATS, PostgreSQL, ClickHouse, Control, Egress, the
development KMS endpoint, documentation, Prometheus, Blackbox, and Grafana. It intentionally contains no scraper,
browser-harvester, retailer, or application-specific service.

Run lifecycle commands from the Straw repository root:

```sh
make infra-up
make infra-status
make infra-down
make infra-reset
```

`make infra-up` creates owner-only local credentials and development MITM CA material under `deploy/local/.dev/`.
Those files are ignored by Git and must never be used in production.

Consumer repositories may include `docker-compose.yml` and attach an application service to the internal
`straw_clients` network. Straw defines the network as a generic client boundary and has no dependency on any
particular application, scraper, or browser service.
