# Straw roadmap

Straw remains a small, self-hosted HTTP/HTTPS egress proxy. One deployment is one trust boundary and NATS is the only
required backing service. JetStream, Redis, and object storage are optional profiles. Hosted tenants, RBAC, quotas,
billing, administration platforms, and mandatory databases are outside the product boundary.

## Before 1.0

- prove the default and optional deployment profiles continuously against isolated owned infrastructure;
- stabilize and version the REST, configuration, receipt, administration, CLI, metrics, and worker contracts;
- exercise signed reproducible releases, compatibility order, upgrades, rollback, recovery, and support workflows;
- use real adopter evidence to prioritize maintained examples and deployment recipes;
- tighten failure-path, concurrency, cancellation, retry, idempotency, fuzz, and cross-version coverage.

## After a stable release baseline

- improve operator visibility and safe diagnostics from observed incidents;
- refine policy and routing only where self-hosted deployments demonstrate a need;
- add optional telemetry exporters and community-maintained deployment integrations without expanding required state;
- prepare a deliberate 1.0 compatibility commitment after pre-1.0 users validate the stable surfaces.

Delivered behavior belongs in [CHANGELOG.md](CHANGELOG.md), public documentation, and architecture decisions. The
[release checklist](docs/public/release-checklist.md) defines promotion criteria; this roadmap describes only future
outcomes and is not evidence that a feature has shipped.
