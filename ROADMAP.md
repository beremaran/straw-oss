# Straw roadmap

Straw is becoming a small, self-hosted HTTP/HTTPS egress proxy for developers and operators who want to route
outbound requests through their own workers. The open-source distribution serves one deployment boundary. It is not a
hosted multi-tenant control plane.

## Open-source release

**Outcome:** a new contributor can clone Straw, start the required services locally, send a request, understand the
architecture, and adapt the included production templates without learning enterprise platform concepts.

### Product boundary

- One Straw deployment has one configuration and one client trust boundary.
- NATS coordinates Control and Egress workers and remains the only required backing service.
- Client authentication is deployment-scoped and optional for loopback-only local development.
- Worker authentication is deployment-scoped and uses a simple operator-managed credential; workers do not require
  pre-provisioned database records.
- Routing, destination policy, header injection, and worker selection use a small static configuration.
- Postgres, Redis, ClickHouse, tenant APIs, RBAC, quotas, billing-grade accounting, config audit/rollback, and
  multi-Control coordination are outside the open-source product boundary.
- Prometheus metrics and structured logs remain available without an analytics database.
- REST request forwarding is the primary documented ingress. Other shipped ingress modes are advanced options only.

### Acceptance criteria

- `make dev` (or the documented equivalent) starts a usable local deployment with Docker Compose.
- The default local deployment needs no generated credentials or manual provisioning.
- A documented curl request succeeds against the local deployment.
- Control and Egress have useful defaults; configuration errors identify the field and remedy without enforcing
  infrastructure that is not enabled.
- The default Compose file contains only Straw, NATS, and services required by enabled features.
- Production deployment assets are explicitly labeled templates and demonstrate secrets, health checks, persistence,
  TLS termination, and scaling patterns without claiming to be a turnkey production platform.
- Public docs explain what Straw is, its limits, installation, quickstart, configuration, request API, SDKs, worker
  operation, deployment patterns, troubleshooting, security, architecture, and contribution.
- The repository includes a license, code of conduct, contribution guide, security policy, release guidance, and
  useful issue/PR metadata expected of a public open-source project.
- `make check` passes from a clean checkout.

### Remaining work

- Replace tenant/platform API-key authentication with deployment-scoped authentication.
- Replace database-backed configuration snapshots and worker credentials with static deployment configuration.
- Remove tenant, RBAC, quota, audit, rollback, multi-Control, Postgres, Redis, ClickHouse, and object-storage code and
  dependencies that no longer have a caller.
- Simplify configuration defaults and error handling for local use.
- Reduce and verify the local Compose stack.
- Convert production deployment files into clear, minimal examples.
- Rebuild the public documentation and project community files around the shipped simplified behavior.
- Run live quickstart examples, full checks, and a final public-release hygiene review.

## Later ideas

These are not commitments and must not add complexity to the open-source release path:

- additional ingress transports;
- richer static routing predicates;
- optional external telemetry exporters;
- community-maintained deployment examples.
