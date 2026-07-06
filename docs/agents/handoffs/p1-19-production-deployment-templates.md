# Handoff

Task: `docs/tasks/p1/19-production-deployment-templates.md`

## Changed

- Added a production Compose template and non-dev Control/Egress configs under `deploy/production/`.
- Added a production deployment README covering required secrets, published ports, operator responsibilities, and unsupported assumptions.
- Added `make production-deploy-check`, which renders the production Compose template, checks shipped proxy/CONNECT ports, rejects the P2 MITM port, and runs config parsing tests.

## Acceptance Criteria Verdicts

| Criterion | Verdict | Implementation (file:line) | Proving test |
|-----------|---------|----------------------------|--------------|
| Templates deploy the documented service set without exposing unused ports. | VERIFIED | `deploy/production/compose.yml:1`, `deploy/production/compose.yml:66`, `deploy/production/compose.yml:101`, `deploy/production/compose.yml:122`, `deploy/production/compose.yml:145`, `deploy/production/compose.yml:164`; backend isolation at `deploy/production/compose.yml:181`; exposed ports at `deploy/production/compose.yml:70` | `make production-deploy-check` |
| Operator responsibilities from Section 28 are documented. | VERIFIED | `docs/planning/28-deployment.md:39`; `deploy/production/README.md:31` | Independent verifier inspection |
| Regional NATS remains out of scope unless explicitly decided. | VERIFIED | `docs/planning/28-deployment.md:50`; `deploy/production/README.md:35`; `deploy/production/README.md:52` | Independent verifier inspection |

## Planning-Doc Coverage

| Planning item | Status | Evidence / owning task |
|---------------|--------|------------------------|
| P1 includes operational deployment templates. | implemented | `deploy/production/README.md:3`; `deploy/production/compose.yml:1` |
| Production template target may be Docker Swarm/Compose or Kubernetes. | implemented | Compose chosen in `deploy/production/README.md:3` |
| Production service set includes Control, Egress, NATS, Postgres, Redis, ClickHouse. | implemented | `deploy/production/compose.yml:1`, `deploy/production/compose.yml:15`, `deploy/production/compose.yml:29`, `deploy/production/compose.yml:45`, `deploy/production/compose.yml:66`, `deploy/production/compose.yml:101` |
| Observability stack operation is an operator responsibility. | implemented | `deploy/production/compose.yml:122`, `deploy/production/compose.yml:137`, `deploy/production/compose.yml:145`; `deploy/production/README.md:41` |
| Control API, proxy, CONNECT, and metrics ports are the only published Straw service ports. | implemented | `deploy/production/compose.yml:70`; `deploy/production/README.md:14` |
| P2 MITM port 8083 must not be published. | implemented | `deploy/production/check-compose.sh:26` |
| NATS max payload must be configured above the default frame size. | implemented | `deploy/production/control.json:27`; `deploy/production/egress.json:15`; `deploy/production/compose.yml:17` |
| Postgres backups are operator-owned. | implemented | `deploy/production/compose.yml:164`; `deploy/production/postgres-backup.sh:1`; `deploy/production/README.md:31` |
| ClickHouse retention/storage sizing is operator-owned. | implemented | `deploy/production/README.md:33` |
| Redis memory sizing is operator-owned. | implemented | `deploy/production/compose.yml:4`; `deploy/production/README.md:34` |
| NATS HA is operator-owned; regional topology needs a decision. | implemented | `deploy/production/README.md:35`; `deploy/production/README.md:52` |
| TLS certificates and secret management are operator-owned. | implemented | `deploy/production/README.md:37`; `deploy/production/README.md:39`; `deploy/production/.env.example:1` |
| Network isolation is required. | implemented | `deploy/production/compose.yml:181`; `deploy/production/README.md:40` |
| Outage behavior remains existing runtime behavior, not changed by this task. | already existed | `docs/planning/29-operational-behavior.md:26` |

## Verification

```sh
make production-deploy-check
make check
```

Result:

- Postgres-backed tests: not exercised with `STRAW_TEST_POSTGRES_DSN`; diff does not touch Postgres-backed Go files or migrations.
- Live compose verification: skipped because this task adds deployment templates/docs and does not change the runtime request path.
- Completion-audit grep for `no owning task`, `no owner`, `future work`, `if needed later`, `InMemory`, `stub`, `fake`, `synthetic`, and `TODO` found no new changed-file hits except the task file's own stop-condition text.

## Reviewer Start Points

- `deploy/production/compose.yml`
- `deploy/production/README.md`
- `deploy/production/check-compose.sh`

## Remaining Work

- None.

## Blockers

- None.
