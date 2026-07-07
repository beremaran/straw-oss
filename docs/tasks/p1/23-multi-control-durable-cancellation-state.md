# 23 - Multi-Control Durable Cancellation State

Status: done

## Objective

Admin request cancellation works across more than one Control replica. When an admin cancel targets a `request_id`
whose dispatch is in-flight on a **different** Control instance than the one that received the cancel call, the
owning Control still tears the request down (publishes the request's `CancelFrame` and returns the cancelled
terminal outcome), instead of the cancel being lost because the in-flight entry lives only in the receiving
instance's process memory.

## Context (gap being closed)

The P0 audit and the task-27 handoff (`docs/agents/handoffs/27-admin-request-cancellation.md`, "Remaining Work")
flagged that P0 cancellation is single-Control only and recorded it as having **no owning task** ("if needed later
they'd be a new P1/P2 task"). This task is the owner.

Current-code evidence (single-Control, in-process only):

- `internal/control/inflight.go` — `InFlightRegistry` maps `request_id -> {tenant_id, cancel func()}` in a plain
  in-memory map guarded by a mutex; the stored `cancel func()` is a live `context.CancelFunc` that cannot cross a
  process boundary.
- `internal/control/dispatcher.go` — `Dispatch` registers `(request_id, tenant_id, cancel)` with `d.opts.InFlight`
  and `defer`-deregisters on every return; the registry is populated only by the instance running the dispatch.
- `internal/control/request_admin_handlers.go` — `CancelRequest` resolves the target via
  `InFlightRegistry.Cancel`, which returns `ErrRequestNotFound` when the `request_id` is not in *this* process's
  map — exactly the miss that happens when a sibling Control owns the request.
- `cmd/control/main.go` (`buildControlMux`) constructs **one** `control.NewInFlightRegistry()` per Control process
  and wires the same instance into both the dispatcher and the admin handler — confirming the state is per-process.

Owning task for the single-Control implementation this extends: `docs/tasks/p0/27-admin-request-cancellation.md`.

## Phase placement and decision

Placed in **P1** ("worker-loss and NATS-outage hardening beyond the P0 baseline" / operational hardening in
`docs/planning/02-phase-boundaries.md`, lines 59-73). Cross-Control coordination of an in-flight control signal is
operational hardening, not a P2 data-plane feature (MITM/BodyRef/capture) and not Future-Work "managed disaster
recovery."

The multi-Control-replica gate was resolved in `docs/planning/32-open-decisions.md` on 2026-07-07. Straw supports
multiple concurrent Control replicas sharing one request plane, with cross-instance runtime coordination in Redis
behind `server.multi_control_enabled` (default off). This task is now complete.

## Required Planning Docs

- `docs/planning/12-nats-protocol.md` — the `CancelFrame` / `c2e` subject and per-request subject conventions the
  cross-instance teardown must reuse (a sibling instance already owns the request's NATS subjects).
- `docs/planning/21-state-and-storage.md` — Redis runtime-state conventions; a shared cancellation signal belongs in
  the same runtime-state tier as other cross-instance runtime data, not a new store.
- `docs/planning/29-operational-behavior.md` — cancellation and terminal-outcome behavior the cross-instance path
  must still satisfy (no new terminal outcomes, no duplicate teardown).

## Prerequisites

- P0 task 27 completed (single-Control in-process cancellation is the thing this extends). Done.
- A committed decision that Straw runs multiple concurrent Control replicas. Done:
  `docs/planning/32-open-decisions.md` resolved it on 2026-07-07.

## Out of Scope

- No persistent request queue and no automatic retry/replay workflow (both are Future Work in
  `docs/planning/02-phase-boundaries.md`, lines 95-96).
- No change to the authorization model — `AuthorizeAdminCancel` and the role checks in `CancelRequest` stay exactly
  as P0 task 27 left them; only the *reach* of a cancel across instances changes.
- Do not introduce a new datastore; reuse the existing Redis runtime-state tier already wired into Control.
- Do not change the single-Control fast path's behavior or its tests; the local in-process cancel must remain the
  path taken when the receiving instance owns the request.

## Expected Files

- Modify: `internal/control/inflight.go` (or an adjacent new file) — add a cross-instance resolution path so a cancel
  for a `request_id` not owned locally reaches the owning instance.
- Modify: `internal/control/request_admin_handlers.go` — only if the cross-instance miss must map to a different
  outcome than the current `ErrRequestNotFound -> 400`.
- Modify: `cmd/control/main.go` — wire the shared runtime-state client into the registry so the built `cmd/control`
  binary constructs the cross-instance-capable registry.
- Test: `internal/control/inflight_test.go` and/or a new cross-instance test simulating two registries against one
  shared runtime-state backend.

## Steps

- [x] Confirm the gate: a committed multi-Control-replica decision exists. If not, stop.
- [x] Read all required planning docs.
- [x] Choose the cross-instance signal mechanism within the existing Redis runtime-state tier (e.g. a
      request_id -> owning-instance record plus a pub/sub or polled cancel signal); document the choice before coding.
- [x] Extend `InFlightRegistry` so `Cancel` on a `request_id` not owned locally publishes a cancel signal the owning
      instance consumes and applies to its local `context.CancelFunc`; keep the local fast path unchanged.
- [x] Ensure the owning instance's teardown still publishes the request's `CancelFrame` on its `c2e` subject and
      returns the cancelled terminal outcome exactly once (no duplicate teardown).
- [x] Wire the shared runtime-state client into the registry in `cmd/control/main.go` (`buildControlMux`).
- [x] Add a test with two registries over one shared backend: a cancel received by instance A tears down a request
      in-flight on instance B; and a cancel for a truly unknown `request_id` still returns the existing not-found
      outcome.
- [x] Run focused control tests, then `make check`.
- [x] Write a handoff note.

## Tests

- `go test ./internal/control`
- `make check`

## Acceptance Criteria

- A cancel delivered to a Control instance that does **not** own the in-flight `request_id` results in the owning
  instance publishing the request's `CancelFrame` and returning the cancelled terminal outcome, proven by a test
  using two registries over one shared runtime-state backend.
- The single-Control fast path (receiving instance owns the request) still cancels in-process without touching the
  shared backend, proven by the existing task-27 tests remaining green plus an assertion that the local path is
  taken.
- A cancel for a `request_id` in-flight on no instance still returns the existing not-found outcome (no false
  positive teardown), proven by a test.
- `cmd/control/main.go` constructs the cross-instance-capable registry from the shared runtime-state client (wiring
  verified in the built binary, not just a unit test).
- The task-27 handoff's "no owning P0 task ... a new P1/P2 task" note is updated to name this task (`p1/23`) as the
  owner of the gap.
- `make check` passes.

## Handoff Notes

- Record the chosen cross-instance signal mechanism and why it stays within the existing Redis runtime-state tier.
- Record that the local in-process fast path is preserved and how the test proves it is taken for locally-owned
  requests.
- Confirm the task-27 "no owning task" deferral is now closed.

## Stop Conditions

- Stop if the committed multi-Control-replica decision is removed or superseded.
- Stop if closing this gap would require a persistent request queue, a new datastore, or a retry/replay workflow
  (all out of scope / Future Work).
- Stop if a deferral would have no owning task file.
