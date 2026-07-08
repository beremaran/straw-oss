# Handoff

Task: `docs/tasks/p1/30-grafana-dashboard-mount-path-consistency.md`

## Changed

- `deploy/observability/dashboard_test.go` — updated the two stale expected strings (provider `path` and compose
  dashboards mount) from `/etc/grafana/provisioning/dashboards/straw` to the shipped `/etc/grafana/dashboards/straw`
  (matching `straw.yml:11` and `docker-compose.yml:192`). Also removed a third stale assertion,
  `profiles: ["observability"]`, which the task file did not name: commit `d2f0a7c7` ("chore: stuff", 2026-07-08)
  deleted every `profiles:` line from `docker-compose.yml`, and the test cannot go green while asserting a string
  the compose file no longer contains. Same class of fix (reconcile this test with the shipped compose config), so
  in scope.
- `docs/agents/handoffs/p1-29-python-client-sdk.md` — the Verification section still named the `done` task p1/13 as
  owner and suggested reverting the two config paths; replaced with a bracketed correction naming this task as owner
  and recording that the revert suggestion was wrong (config and compose agreed; only the test was stale).
- `docs/tasks/p1/30-grafana-dashboard-mount-path-consistency.md` — fixed the Required Planning Docs reference (it
  cited the nonexistent `docs/planning/28-observability.md`; the real doc is `docs/planning/23-observability.md`),
  checked steps, status → done.
- `docs/tasks/p1.md` — board row 30 → done.

## Acceptance Criteria Verdicts

From the independent verifier (fresh sub-agent, task file + diff only; ran the tests itself uncached):

| Criterion | Verdict | Implementation (file:line) | Proving test |
|-----------|---------|----------------------------|--------------|
| `TestGrafanaProvisioningMatchesComposeMounts` passes against the current tree | VERIFIED | `deploy/observability/dashboard_test.go:86,89` | `go clean -testcache && go test ./deploy/observability/` → ok |
| `make check` green with no other change | VERIFIED | diff touches only the test + p1-29 handoff | `make check` exit 0, lint 0 issues |
| Provider config, compose mount, and test reference the same in-container path; no mixed paths | VERIFIED | `straw.yml:11`, `docker-compose.yml:192`, `dashboard_test.go:86,89`; repo-wide grep clean | `TestGrafanaProvisioningMatchesComposeMounts` |
| p1-29 handoff no longer names done task p1/13 as owner | VERIFIED | `docs/agents/handoffs/p1-29-python-client-sdk.md` (Verification + Remaining Work both name p1/30) | n/a (doc) |

## Planning-Doc Coverage

| Planning item | Status | Evidence / owning task |
|---------------|--------|------------------------|
| `docs/planning/23-observability.md` in-container dashboard path mandate | none exists — doc mandates no Grafana container path (grep empty, confirmed independently by verifier) | direction chosen: update the test to shipped paths, not revert config |

## Verification

```sh
go test ./deploy/observability/
make check
```

Result: both green (verifier reproduced with a cleaned test cache).

- Postgres-backed tests: not exercised — diff touches no Postgres surfaces (one test file and two docs).
- Live compose verification: skipped — this task changes only test assertions about static config files; there is
  no runtime request-path behavior to drive. The config/compose consistency the test pins is exactly what the
  running stack uses.

## Handoff Notes (direction chosen)

Updated the test to the shipped paths. Basis: `docs/planning/23-observability.md` mandates no specific in-container
dashboard path, and the provider config + compose mount already agreed with each other at
`/etc/grafana/dashboards/straw`; reverting the config (p1-29's suggestion) would have re-broken a self-consistent
runtime layout to satisfy a stale test.

## Reviewer Start Points

- `deploy/observability/dashboard_test.go`

## Remaining Work

- None owned by this task. Nothing is faked, stubbed, or deferred.
- One observation raised to the user in the completing session (a user decision, not a deferral of this task's
  behavior): user commit `d2f0a7c7` removed all compose `profiles:` gating, so the observability services
  (Prometheus, Grafana, blackbox) and the docs service now always start with `docker compose up` instead of being
  opt-in via `--profile observability` as p1/13 shipped them. The test previously pinned that gating; it no longer
  does. If the un-gating was unintentional, restoring it is a small compose + test change — flagged directly to the
  user for a decision rather than tasked, since the change was the user's own deliberate commit, not agent-deferred
  behavior.

## Blockers

- None. Committed in the same run.
