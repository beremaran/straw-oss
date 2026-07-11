# Straw roadmap

Straw is a small, self-hosted HTTP/HTTPS egress proxy for developers and operators who want to run their own workers.
One deployment is one trust boundary, and NATS is the only required backing service.

## Open-source release

The repository now targets a complete first public release:

- local Compose starts NATS, Control, and Egress without provisioning;
- the request API, CLI, and Go/Python clients share one straightforward workflow;
- deployment-scoped authentication is optional locally and required in the production example;
- metrics, health endpoints, and structured logs work without an analytics database;
- production assets show security and scaling patterns without claiming turnkey operation;
- public documentation covers installation, concepts, configuration, API use, SDKs, operation, security,
  troubleshooting, contribution, and releases;
- standard license, conduct, security, contribution, changelog, and GitHub metadata are included.

## Near term

- collect feedback from first self-hosted deployments;
- tighten REST API compatibility guarantees before v1.0;
- improve release automation and publish versioned container images;
- add focused examples based on demonstrated community needs.

## Later ideas

These are not commitments and should remain optional:

- richer static routing predicates;
- additional ingress transports;
- optional telemetry exporters;
- community-maintained deployment examples.

Hosted multi-tenancy, billing, tenant administration, and mandatory databases are outside the project boundary.
