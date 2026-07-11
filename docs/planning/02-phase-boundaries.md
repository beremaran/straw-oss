## 2. Release boundaries

### Open-source release

The first public release includes:

- one Control service and one or more Egress workers;
- REST request transport as the primary workflow;
- HTTP forward proxy, CONNECT, and MITM only where the existing implementation remains maintainable and verified;
- optional deployment-scoped client authentication;
- static routes, worker pools, destination rules, and header operations;
- exact-session Core NATS assignment and streaming;
- the official Go worker, Go client SDK, Python client SDK, Egress SDK, and example custom worker;
- health/readiness endpoints, Prometheus metrics, and structured logs;
- a minimal local Docker Compose environment;
- production deployment patterns as examples, not a managed platform;
- focused tests and a verified end-to-end quickstart.

The release excludes:

- tenants and tenant lifecycle;
- role-based access control and API-key lifecycle APIs;
- quotas, billing, usage reconciliation, and rate-limit products;
- database-backed configuration, audit history, and rollback APIs;
- bundled Postgres, Redis, ClickHouse, object storage, Grafana, or KMS dependencies;
- multiple coordinated Control replicas;
- hosted control-plane, marketplace, or provider-account workflows;
- scraping orchestration, browsers, CAPTCHA solving, and anonymity-network behavior.

### Later work

Later work is accepted only when it improves the self-hosted proxy without weakening the local-first path. Possible
areas include richer static routing, more ingress transports, and optional telemetry exporters.
