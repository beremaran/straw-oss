## 4. Canonical Architecture

Control is the only public-facing runtime service. All clients enter through Control. Control owns:

- ingress protocols,
- authentication,
- authorization,
- tenant resolution,
- routing decisions,
- quota/rate-limit admission,
- destination policy checks,
- worker selection,
- request IDs and trace propagation,
- NATS dispatch,
- config APIs,
- durable config access,
- observability aggregation.

Executors own outbound execution. Executors are not allowed to query Postgres, Redis, or ClickHouse. They receive
resolved per-request instructions from Control and report constrained results/failures back to Control.

```mermaid
flowchart LR
  subgraph Clients
    REST[REST clients]
    SDK[SDKs]
    CLI[CLI]
    UI[UI]
    Proxy[HTTP / CONNECT / MITM clients]
  end

  Control[Control\nIngress, auth, routing, config, coordination]
  PG[(Postgres\nDurable config)]
  Redis[(Redis\nEphemeral runtime state)]
  CH[(ClickHouse\nOperational analytics)]
  NATS[NATS\nCore request/reply + transient streams]

  subgraph Executors
    Egress[Egress Workers]
    Custom[Custom Egress implementations\nP2: built on the Egress SDK]
  end

  Body[Large-body transport\nP2: object storage or direct stream]
  Targets[Target sites]
  Vendors[Upstream proxies / vendors]

  REST --> Control
  SDK --> Control
  CLI --> Control
  UI --> Control
  Proxy --> Control
  Control <--> PG
  Control <--> Redis
  Control --> CH
  Control <--> NATS
  NATS <--> Egress
  NATS <--> Custom
  Control -. P2 .- Body
  Egress -. P2 .- Body
  Custom -. P2 .- Body
  Egress --> Targets
  Egress --> Vendors
  Custom --> Vendors
  Custom --> Targets
```
