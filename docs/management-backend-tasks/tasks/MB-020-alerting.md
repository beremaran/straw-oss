# MB-020: Alert Rules, Evaluator, Events, And Delivery

Status: done
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

- [x] Add migrations and domain models.
- [x] Add repositories for rules and events.
- [x] Add rule handlers and event handlers.
- [x] Add evaluator for supported metrics and conditions.
- [x] Add notification delivery integration with cooldowns.
- [x] Add tests for rule validation, event lifecycle, acknowledgement, and delivery suppression.

## Done Criteria

- [x] Alert rules can be created, updated, listed, fetched, and disabled/deleted.
- [x] Evaluator can fire, acknowledge, resolve, and record alert events.
- [x] Delivery uses configured notification channels and respects cooldown.
- [x] Unsupported metrics and invalid conditions are rejected.
