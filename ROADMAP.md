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

## Planned open-source capabilities

These capabilities belong in the open-source project. They must be additive deployment profiles: the default local
path continues to require only NATS, while operators opt into durable or highly available infrastructure when they
need the corresponding behavior. The project will not reserve these features for a separate enterprise edition.

### Runtime administration and configuration — delivered

**Outcome:** operators can inspect and change Control and Egress behavior at runtime, without editing files and
restarting the deployment.

- A deployment-scoped Admin and Config REST API covers Control settings, worker settings, pools, routing, destination
  policy, injection policy, fingerprint profiles, and the supported runtime controls.
- The web dashboard has action parity with the API and uses only the documented endpoints.
- Worker lifecycle operations cover inspect, drain, undrain, disable, enable, and safe request cancellation.
- Changes are validated before activation, versioned snapshots are atomically applied and distributed, rollout status
  is exposed, and bounded audit history supports rollback into a new version.
- ETag/`If-Match` optimistic concurrency prevents silent operator overwrites.
- The opt-in durable backend is file-backed NATS JetStream KV. This replaces the initially expected PostgreSQL choice
  so NATS remains the project's only backing service.
- The model remains deployment-scoped and adds no hosted tenants, billing, or account management.

Acceptance is covered by API/UI action parity, restart-safe configuration, documented recovery, a separately
authorized administrative surface, and an end-to-end local worker-drain example that lets active requests continue.

### Highly available Control — delivered

**Outcome:** operators can run multiple interchangeable Control instances behind a load balancer without losing
runtime coordination.

- Add a shared runtime-state interface with an open-source Redis implementation and retain the in-memory
  implementation for single-Control development.
- Coordinate worker sessions, heartbeats, capacity, cooldowns, sticky routing, in-flight request ownership and
  cancellation, configuration-version invalidation, and graceful instance handoff.
- Define TTLs, ownership fencing, idempotency, degraded behavior, and recovery after Redis or Control failure.
- Ensure no Control instance requires local affinity and no ephemeral-state loss can corrupt durable configuration.
- Provide an HA deployment example, health/readiness semantics, failure drills, and operational metrics.

Acceptance requires concurrent Control instances to route through the same worker fleet, survive the loss and return
of one Control instance, and behave predictably during a temporary shared-state outage.

### Object storage and receipt-and-check transport — delivered

**Outcome:** Straw can accept bodies larger than the inline transport limit using a durable receipt-and-check flow,
without buffering an unbounded payload in Control or NATS.

- Add an object-storage interface with an S3-compatible implementation and an optional local development profile.
- Receipt request data into object storage, record a durable receipt ID and state, and verify declared size and
  checksum before making the object eligible for assignment.
- Give Egress a short-lived, assignment-scoped reference; Egress re-checks size/checksum before using the body.
- Define status/check APIs, retries, idempotency, multipart/resumable upload behavior, cancellation, orphan cleanup,
  retention, encryption, and signed-URL or temporary-credential boundaries.
- Support stored response receipts where asynchronous or large-response workflows need them, with explicit expiry and
  download authorization.
- Keep inline base64 transport as the simplest default for ordinary requests.

Acceptance requires corrupted or incomplete objects to be rejected, references to be unusable outside their
assignment, interrupted uploads to be recoverable or cleaned up, and the full lifecycle to be observable and
documented.

## Delivery order

1. [x] Specify the deployment-scoped configuration model and API/UI action matrix.
2. [x] Add durable configuration and runtime snapshot distribution.
3. [x] Add shared runtime state and validate multi-Control HA behavior.
4. [x] Add receipt-and-check object transport on top of the versioned configuration and HA foundations.

## Later ideas

These are not commitments and should remain optional:

- richer static routing predicates;
- additional ingress transports;
- optional telemetry exporters;
- community-maintained deployment examples.

Hosted multi-tenancy, billing, tenant administration, and mandatory databases are outside the project boundary.
