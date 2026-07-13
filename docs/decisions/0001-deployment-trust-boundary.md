# ADR 0001: one deployment is one trust boundary

Status: accepted. Date: 2026-07-13. Owner: maintainer.

Straw is deployment-scoped. Applications, Control, workers, and optional profile services are administered as one
trust boundary. Straw will not add tenants, RBAC, quotas, billing, or an administration platform. Separate hostile
or independently governed workloads use separate deployments. This keeps authorization and failure semantics
auditable for a small self-hosted proxy.
