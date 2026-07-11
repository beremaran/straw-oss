## 4. Canonical Architecture

Control is the only public-facing runtime service. All clients enter through Control. Control owns:

- ingress protocols,
- optional deployment-scoped client authentication,
- routing decisions,
- destination policy checks,
- worker selection,
- request IDs and trace propagation,
- NATS dispatch,
- static configuration loading,
- Prometheus metrics and structured logs.

Executors own outbound execution. They receive resolved per-request instructions from Control and report constrained
results or failures to Control.

```mermaid
flowchart LR
  Clients[REST, SDK, CLI, or proxy clients]
  Control[Control service]
  NATS[NATS]

  subgraph Executors
    Egress[Official Egress workers]
    Custom[Custom SDK workers]
  end

  Targets[Target sites]
  Vendors[Upstream proxies]

  Clients --> Control
  Control <--> NATS
  NATS <--> Egress
  NATS <--> Custom
  Egress --> Targets
  Egress --> Vendors
  Custom --> Targets
  Custom --> Vendors
```

NATS is the only required backing service. Operators may collect `/metrics` and structured logs using their existing
observability stack; Straw does not require a database or bundled analytics platform.
