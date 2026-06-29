# MB-019: Notification Channels And Preferences

Status: not-started
Phase: 5
Depends on: MB-001, MB-006
Search tags: notification_channels, notification_preferences, webhook, email, slack, secret_ref

## Objective

Manage notification delivery channels and per-user notification preferences.

## Scope

- Add `notification_channels` and `notification_preferences` migrations.
- Add channel list, create, patch, delete/disable, and test endpoints.
- Add current-user preference read and patch endpoints.
- Support webhook, email, and Slack webhook channel types.
- Store secrets as secret references or encrypted values; never return them.

## Repo Touchpoints

- `internal/infra/postgres/migrations/*.sql`
- `internal/domain/*notification*.go`
- `internal/infra/postgres/*notification*_repo.go`
- `internal/server/admin/handlers/*notification*.go`
- `internal/server/admin/server.go`
- `internal/config/config.go`

## Implementation Tasks

- [ ] Add migrations and domain models.
- [ ] Add repositories for channels and preferences.
- [ ] Add handlers with permission checks from the spec.
- [ ] Add secret redaction and `has_secret` response behavior.
- [ ] Add delivery test path for supported channel types.

## Done Criteria

- [ ] Channels can be managed without exposing stored secrets.
- [ ] Test notification endpoint attempts delivery and reports result.
- [ ] Current user can read and update preferences.
- [ ] Mutating channel operations write audit events.
