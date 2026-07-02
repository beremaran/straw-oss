## 28. Deployment

### Local Development

Docker Compose includes:

- Control,
- Egress,
- NATS,
- Postgres,
- Redis,
- ClickHouse.

Control exposes:

| Port | Purpose               |
|-----:|-----------------------|
| 8080 | REST/config/admin API |
| 9090 | Metrics               |

P1/P2 may add:

| Port | Purpose            |
|-----:|--------------------|
| 8081 | HTTP forward proxy |
| 8082 | raw CONNECT proxy  |
| 8083 | MITM proxy         |

Do not map unused ports in P0 examples.

### Production

Initial production may use Docker Swarm/Compose or Kubernetes, but P0 only requires a clear containerized deployment.
Production operators are responsible for:

- Postgres backups,
- ClickHouse retention/storage sizing,
- Redis memory sizing,
- NATS HA deployment,
- TLS certificates,
- secret management,
- network isolation,
- observability stack operation.

Regional Egress pools do not require regional NATS in P0. If regional NATS is added, define whether the deployment uses
NATS leaf nodes, superclusters, or separate clusters before implementation.
