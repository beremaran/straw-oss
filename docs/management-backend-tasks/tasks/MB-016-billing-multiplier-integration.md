# MB-016: Billing Multiplier Integration

Status: done
Phase: 4
Depends on: MB-015
Search tags: billing estimate, cost multiplier, pricing_version, usage summaries

## Objective

Apply active cost multipliers to billing estimates and expose pricing metadata.

## Scope

- Use active multipliers by endpoint tag in usage cost-unit computation.
- Include multiplier version or `pricing_version` in `GET /management/billing/estimate`.
- Do not rewrite historical summaries by default.
- Leave historical repricing as a future explicit job.

## Repo Touchpoints

- `internal/server/admin/handlers/usage.go`
- `internal/server/dto/usage.go`
- `internal/infra/postgres/usage_repo.go`
- `internal/infra/postgres/*cost*_repo.go`
- `internal/domain/usage.go`
- `internal/server/admin/handlers/usage_test.go`

## Implementation Tasks

- [x] Add active multiplier lookup to billing estimate flow.
- [x] Apply multipliers by endpoint tag with deterministic conflict behavior.
- [x] Add pricing metadata to the response without removing current fields.
- [x] Add tests for no multiplier, matching multiplier, inactive multiplier, and pricing metadata.

## Done Criteria

- [x] Billing estimates reflect active multiplier configuration.
- [x] Existing billing response fields remain compatible.
- [x] Response identifies the multiplier version or pricing version used.
