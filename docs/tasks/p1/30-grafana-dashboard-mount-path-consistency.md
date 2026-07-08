# 30 - Grafana Dashboard Mount Path Test Consistency

Status: done

## Objective

Restore a green `make check` by reconciling `deploy/observability/dashboard_test.go` with the Grafana dashboard
provisioning paths currently shipped in `docker-compose.yml` and the provider config, so
`TestGrafanaProvisioningMatchesComposeMounts` asserts the real, self-consistent runtime paths instead of stale
pre-`aba1602a` strings.

## Context (gap being closed)

`make test` (and therefore `make check`) is **red on `master` right now** — verified 2026-07-08:

```
--- FAIL: TestGrafanaProvisioningMatchesComposeMounts (0.00s)
    dashboard_test.go:101: observability deployment config missing "/etc/grafana/provisioning/dashboards/straw"
```

Root cause: commit `aba1602a` ("chore: stuff", 2026-07-08 01:47) moved the Grafana dashboard file-provider path
**consistently** in two files but never updated the test that pins them:

- `deploy/observability/grafana/provisioning/dashboards/straw.yml:11` — `path: /etc/grafana/dashboards/straw`
- `docker-compose.yml:192` — `./deploy/observability/grafana/dashboards:/etc/grafana/dashboards/straw:ro`

The provider config and the compose mount **agree with each other** (dashboards land at
`/etc/grafana/dashboards/straw`, and that is where the provider looks), so the running stack is internally
consistent. The outlier is the test:

- `deploy/observability/dashboard_test.go:86` still expects `/etc/grafana/provisioning/dashboards/straw`.
- `deploy/observability/dashboard_test.go:90` still expects the old mount
  `./deploy/observability/grafana/dashboards:/etc/grafana/provisioning/dashboards/straw:ro`.

This gap was flagged in `docs/agents/handoffs/p1-29-python-client-sdk.md` (Verification section) but assigned to
`docs/tasks/p1/13-observability-dashboards.md`, which is `done`. A completed task cannot own a regression
introduced by a later, unrelated commit, so per AGENTS.md Gap Ownership this task is the real owner. p1-29's
suggested "one-line revert" is the **wrong** direction: reverting only the two config paths would re-break
provider/compose consistency to satisfy a stale test. The correct fix updates the test to the shipped paths (unless
the planning doc requires the dashboards to live under `/etc/grafana/provisioning/dashboards/straw`, in which case
revert all three consistently — see Stop Conditions).

## Required Planning Docs

- `docs/planning/23-observability.md` (the observability planning doc referenced by p1/13; this task originally
  cited a nonexistent `28-observability.md`) — to confirm whether a specific in-container dashboard path is
  mandated. Verified 2026-07-08: it mandates no container path.

## Prerequisites

- P1 task 13 (Observability dashboards) done — the test and mounts it added exist.

## Out of Scope

- No change to dashboard JSON content, datasource, Prometheus, or blackbox config.
- No new dashboards or provisioning providers.
- No change to the `grafana/provisioning/datasources` or `grafana/provisioning/dashboards` (provider config dir)
  mounts, which are unaffected by `aba1602a`.

## Expected Files

- Modify: `deploy/observability/dashboard_test.go` — update the two stale expected strings (the provider `path`
  and the dashboards mount) to `/etc/grafana/dashboards/straw`, matching `straw.yml:11` and `docker-compose.yml:192`.
- (Only if the planning doc mandates the old path) instead modify
  `deploy/observability/grafana/provisioning/dashboards/straw.yml:11` and `docker-compose.yml:192` back to
  `/etc/grafana/provisioning/dashboards/straw` — but not a mix; the three must agree.

## Steps

- [x] Read the required planning doc and confirm no specific in-container dashboard path is mandated.
- [x] Confirm current state: provider `path` and compose mount both use `/etc/grafana/dashboards/straw`; the test
      is the only file still on the old path.
- [x] Update the two `dashboard_test.go` expected strings to the shipped paths (or, if mandated, revert all three
      config sites consistently). (Also removed a third stale assertion, `profiles: ["observability"]` — commit
      `d2f0a7c7` deleted all `profiles:` lines from `docker-compose.yml`, so the test could not go green without it.)
- [x] Run `go test ./deploy/observability/` and confirm `TestGrafanaProvisioningMatchesComposeMounts` passes.
- [x] Run `make check` and confirm the full suite is green.
- [x] Update `docs/agents/handoffs/p1-29-python-client-sdk.md` to name this task as the owner (replace the "owned by
      p1/13" note).
- [x] Write a handoff note.

## Tests

- `go test ./deploy/observability/`
- `make check`

## Acceptance Criteria

- `TestGrafanaProvisioningMatchesComposeMounts` passes against the current tree.
- `make check` is green with no other change.
- The provider config (`straw.yml`), the compose mount (`docker-compose.yml`), and the test all reference the same
  in-container dashboard path — no mixed paths remain.
- The p1-29 handoff note no longer points at the `done` task p1/13 as owner.

## Handoff Notes

- Record which direction was chosen (update test vs. revert config) and the planning-doc basis for it.

## Stop Conditions

- Stop and revert all three sites to `/etc/grafana/provisioning/dashboards/straw` instead if the planning doc
  explicitly requires dashboards under the provisioning directory; do not leave the config and test on different
  paths.
- Stop if `make check` still fails for a reason unrelated to this test (e.g. the flaky
  `TestMITMHTTP2StreamCancelIsIsolated` noted in p1-29) — that is a separate concern, not this task's.
