# 19 - Production Deployment Templates

Status: not started

## Objective

Add production deployment templates and operator docs for the P0/P1 service set.

## Required Planning Docs

- `docs/planning/28-deployment.md`
- `docs/planning/24-static-configuration.md`
- `docs/planning/29-operational-behavior.md`

## Prerequisites

- Task 02 completed.
- Task 05 completed.
- Task 15 completed or explicitly closed as not-applicable by decision.

## Out of Scope

- Do not add regional NATS topology unless it has a written owner decision.
- Do not map unused ingress ports.
- Do not implement managed disaster recovery.

## Expected Files

- Create: production deployment templates under `deploy/`.
- Create or modify: deployment documentation.
- Test: template validation checks.

## Steps

- [ ] Read all required planning docs.
- [ ] Choose the production template target explicitly (Kubernetes, Swarm, or Compose) before implementation.
- [ ] Add service definitions for Control, Egress, NATS, Postgres, Redis, ClickHouse, and observability components.
- [ ] Add secret, TLS, backup, retention, sizing, and network-isolation configuration hooks.
- [ ] Map ports 8081 and 8082 only when the corresponding ingress tasks have shipped.
- [ ] Document operator responsibilities for backups, retention, NATS HA, TLS, secrets, network isolation, and
      observability.
- [ ] Add template lint/render checks.
- [ ] Run focused deployment template checks.
- [ ] Run `make check`.
- [ ] Write a handoff note.

## Tests

- Deployment template validation checks.
- `make check`

## Acceptance Criteria

- Templates deploy the documented service set without exposing unused ports.
- Operator responsibilities from Section 28 are documented.
- Regional NATS remains out of scope unless explicitly decided.

## Handoff Notes

- List templates, required secrets, and unsupported deployment assumptions.

## Stop Conditions

- Stop if regional NATS topology is needed but undecided.
- Stop if a deferral would have no owning task file.
