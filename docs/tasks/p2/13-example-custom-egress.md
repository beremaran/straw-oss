# 13 - Example Custom Egress Implementation

Status: not started

## Objective

Ship one example custom Egress implementation built purely on `sdk/egress` — a static-response executor that answers
every assignment with a fixed page — proving that a third party can implement an Egress node (including
provider-forwarding ones) with only the public SDK, and documenting the operator obligations that come with custom
execution.

## Context (gap being closed)

The 2026-07-07 decision `P2 Provider Adapter Baseline` (superseded entry in `docs/planning/32-open-decisions.md`)
replaced the named static provider adapter with "one example custom Egress implementation" — vendor-neutral, since
Straw no longer names providers. `docs/planning/02-phase-boundaries.md` (P2 list) requires it. Nothing outside
`cmd/egress` implements the SDK today, so the SDK's public seam is unproven until this exists. This task depends on
task 24's conformance/live verification (`docs/tasks/p2/24-egress-sdk-conformance-and-live-verification.md`), which
proves task 12's `sdk/egress` package and task 22's official-worker rebase.

## Required Planning Docs

- `docs/planning/05-component-boundaries.md` (Egress SDK and Custom Egress Implementations — operator obligations)
- `docs/planning/16-egress-execution.md` (Executor-delegated mode: equivalent destination policy, constrained facts)
- `docs/planning/27-security-controls.md` (Executor-delegated resolution; public-safe error facts)

## Prerequisites

- Task 24 completed (`sdk/egress` public package, official-worker rebase, enum rename, and conformance/live
  verification are done).

## Out of Scope

- No named provider integration (Bright Data, Scrape.do, etc.) and no provider credentials handling.
- No marketplace, billing, or account provisioning.
- No changes to `sdk/egress` beyond what a real third-party consumer could make (i.e., none; API gaps found here
  require a new owning task or a stop).

## Expected Files

- Add: `examples/egress-static/main.go` — the example worker binary on `sdk/egress`.
- Add: `examples/egress-static/README.md` — how to run it, plus the operator obligations: equivalent
  destination-policy enforcement and constrained, public-safe error facts (citing the planning docs above).
- Test: an integration test registering the example executor against the test NATS harness and serving one
  assignment end to end.

## Steps

- [ ] Read all required planning docs.
- [ ] Implement the static-response `Executor` and the example worker `main.go` using only `sdk/egress` and the
      standard library.
- [ ] Write the README with run instructions and the operator-obligation section.
- [ ] Add the integration test: register, receive an assignment, return the static page, and assert the response
      reaches the Control side of the test harness.
- [ ] Verify the example imports no `internal/*` packages.
- [ ] Optionally run the example against the compose stack and drive a request through Control to it.
- [ ] Run focused tests, then `make check`.
- [ ] Write a handoff note.

## Tests

- `go test ./examples/...`
- `make check`

## Acceptance Criteria

- `examples/egress-static` builds and imports no `internal/*` packages (proven by `go list -deps` or grep).
- The integration test proves a request dispatched to the example executor returns the static page through the
  normal stream protocol.
- The README names both operator obligations with planning-doc citations.
- No vendor/provider names appear anywhere in the example.

## Handoff Notes

- Record any friction a third-party implementer would hit with the SDK API (missing hooks, unclear types); if real,
  create an owning task for it.

## Stop Conditions

- Stop if the example cannot be built without importing `internal/*` packages — that is an SDK API gap, not something
  to patch around here.
- Stop if a deferral would have no owning task file.
