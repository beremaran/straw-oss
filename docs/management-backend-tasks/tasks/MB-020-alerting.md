# MB-020: Alert Rules, Evaluator, Events, And Delivery

Status: not-started
Phase: 5
Depends on: MB-019
Search tags: alert_rules, alert_events, evaluator, ack, resolved, notification delivery

## Objective

Allow management users to define alert rules, evaluate metrics, track alert events, and deliver notifications.

## Scope

- Add `alert_rules` and `alert_events` migrations.
- Add alert rule CRUD endpoints.
- Add alert event list and acknowledgement endpoint.
- Evaluate first-release metrics and conditions from the spec.
- Deliver notifications through configured channels with cooldown handling.

## Repo Touchpoints

- `internal/infra/postgres/migrations/*.sql`
- `internal/domain/*alert*.go`
- `internal/infra/postgres/*alert*_repo.go`
- `internal/server/admin/handlers/*alert*.go`
- `internal/service/*alert*.go`
- `internal/server/metrics/metrics.go`
- `internal/endpoint/metrics/metrics.go`

## Implementation Tasks

- [ ] Add migrations and domain models.
- [ ] Add repositories for rules and events.
- [ ] Add rule handlers and event handlers.
- [ ] Add evaluator for supported metrics and conditions.
- [ ] Add notification delivery integration with cooldowns.
- [ ] Add tests for rule validation, event lifecycle, acknowledgement, and delivery suppression.

## Done Criteria

- [ ] Alert rules can be created, updated, listed, fetched, and disabled/deleted.
- [ ] Evaluator can fire, acknowledge, resolve, and record alert events.
- [ ] Delivery uses configured notification channels and respects cooldown.
- [ ] Unsupported metrics and invalid conditions are rejected.
