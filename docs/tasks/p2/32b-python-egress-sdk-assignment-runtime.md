# 32b - Python Egress SDK Assignment Runtime

Status: not started

## Objective

Give the Python Egress SDK a working assignment runtime built on task 32a's protocol layer: a registration/heartbeat
loop, correct subscription-before-ack assignment ordering, decoded HTTP request-frame reading, an executor callable
seam, sequenced streamed response/error publishing without buffering the full response, and credit-based backpressure
— proven by conformance tests against a fake/local NATS wire harness. Ship usage docs for the completed Python Egress
SDK (protocol + runtime) and retire task 32 as superseded.

## Context (gap being closed)

Task 32 ("Python Egress SDK") was split on 2026-07-08 after user approval into 32a (protocol foundation: generated
protobuf bindings, subjects, signing, minimal NATS wire client) and 32b (this task: the assignment runtime built on
that foundation), because the combined scope was sized close to the entire Go Egress SDK (`sdk/egress/`, ~3500 lines).
This task depends on 32a's protocol layer existing; before 32a lands, no Python code can construct a valid `Envelope`,
subject, or signed registration request to build a runtime on top of.

The Go reference contract this task must be behaviorally compatible with:

- `sdk/egress/runtime.go:26-45` — NATS connection surface and registration entrypoint.
- `sdk/egress/assignment.go:30-63` (and the full file, ~816 lines) — assignment worker runtime: subscription
  ordering, frame reading, executor invocation, response streaming.
- `sdk/egress/stream.go` — stream frame sequencing helpers.
- `sdk/egress/assignment_test.go` and `sdk/egress/conformance_wire_test.go` — the behavioral proofs (subscription-
  before-ack ordering, no full-response buffering, executor-error-to-ErrorFrame mapping) this task's Python tests
  must reproduce in Python.

## Required Planning Docs

- `docs/planning/12-nats-protocol.md` — read in full, especially "Assignment Flow (Subscription Ordering)"
  (lines ~92-107), "Stream Ordering and Sequencing" (lines ~109-133), and "Backpressure and Credit Semantics"
  (lines ~135-164).
- `docs/planning/05-component-boundaries.md` (Egress SDK and Custom Egress Implementations, lines ~35-48; operator
  obligations for destination-policy enforcement and public-safe execution facts).
- `docs/planning/30-testing-matrix.md` (P1/P2 feature test rows before shipping, lines ~43-44).

## Prerequisites

- Task 32a completed (protocol foundation: generated protobuf bindings, subject construction, Envelope construction,
  registration signing, and the NATS wire client this runtime uses).

## Out of Scope

- Do not add the Python client/request SDK; `docs/tasks/p1/29-python-client-sdk.md` owns client-side HTTP APIs.
- Do not add raw CONNECT, BodyRef, MITM, payload capture, HTTP/2-specific behavior, provider integrations, billing, or
  marketplace features.
- Do not change Control, `cmd/egress`, or the protobuf/NATS wire contract to fit Python.
- Do not publish to PyPI or add release automation.
- Do not modify task 32a's protocol layer beyond what is strictly needed to fix a genuine defect found while building
  the runtime on top of it (record any such fix in the handoff).

## Expected Files

- Add: `python/straw/egress/runtime.py` for the registration/heartbeat loop and the assignment worker runtime.
- Modify: `python/straw/egress/__init__.py` for package exports of the runtime surface.
- Add: `python/tests/test_egress_runtime.py` for assignment admission, decoded HTTP response streaming, credit
  backpressure, and executor error tests against a fake/local NATS wire harness.
- Add: `python/README.md` or `docs/public/sdk.md` usage notes for the complete Python Egress SDK surface (protocol +
  runtime).
- Modify: `docs/tasks/p2/32-python-egress-sdk.md` — mark `Status: superseded`, and update its Handoff Notes to point
  to 32a/32b as the owning tasks instead of leaving it as an open, pickable task.

## Steps

- [ ] Read all required planning docs.
- [ ] Implement the registration + heartbeat loop using 32a's `protocol.py` and NATS wire client.
- [ ] Implement the assignment flow with correct subscription ordering: subscribe to the request-scoped `c2e` subject
      and flush it before publishing `AssignAck`, per the Assignment Flow subscription-ordering rules.
- [ ] Implement reading of `RequestStart`/`DataFrame`/`CreditFrame`/`CancelFrame` on `c2e` with `stream_seq`
      validation (accept only the next expected sequence; ignore late duplicates; treat gaps/high sequence as
      protocol errors).
- [ ] Implement a small executor callable seam (decoded HTTP request in, response iterator/callable out) that a
      custom Python worker process supplies.
- [ ] Implement sequenced `ResponseStart`/`DataFrame`/`EndFrame` publishing on `e2c` without buffering the full
      response, and `ErrorFrame` publishing on executor error.
- [ ] Implement credit-based backpressure: honor initial upload/download credit and max in-flight byte limits from
      `AssignRequest`, and stop/slow sending when credit reaches zero.
- [ ] Add the tests listed in Expected Files using a fake/local NATS wire harness; do not rely on Go `internal/*`
      test helpers.
- [ ] Add Python Egress SDK usage docs (protocol + runtime) and operator-obligation notes for destination-policy
      enforcement and public-safe execution facts.
- [ ] Mark `docs/tasks/p2/32-python-egress-sdk.md` superseded by 32a/32b in its own Status/Handoff Notes.
- [ ] Run focused Python tests, focused Go SDK tests if shared behavior changed, then `make check`.
- [ ] Write a handoff note.

## Tests

- `python3 -m unittest discover python/tests`
- `go test ./sdk/...`
- `make check`

## Acceptance Criteria

- A Python SDK worker can accept one decoded HTTP assignment and stream `ResponseStart`, one or more `DataFrame`s, and
  `EndFrame` without buffering the full response, proven by a Python conformance test.
- Python executor errors map to an `ErrorFrame` on `e2c`, proven by a Python test asserting the published frame code
  and terminal behavior.
- The Python runtime flushes the request-scoped `c2e` subscription before sending `AssignAck`, proven by a fake/wire
  NATS test that would fail if `RequestStart` can be published before subscription readiness.
- Credit-based backpressure is honored: a test proves the runtime stops sending `DataFrame`s when credit is exhausted
  and resumes after a `CreditFrame` grants more.
- `rg -n "github.com/beremaran/straw/v2/internal|\\.\\./internal|internal/" python` returns no matches for the Python
  Egress SDK.
- `docs/tasks/p2/32-python-egress-sdk.md`'s `Status:` line reads `superseded` and names 32a/32b as the owning tasks.
- `docs/tasks/p1/29-python-client-sdk.md` still excludes Egress SDK/custom-worker behavior.

## Handoff Notes

- Record the executor callable's public shape, the credit/backpressure implementation approach, the exact Python test
  command used, and any incompatibility found between the Go SDK contract and the Python runtime.
- Confirm task 32's Status/Handoff Notes were updated to `superseded` in this same run.

## Stop Conditions

- Stop if implementing the runtime requires changing the protobuf/NATS wire contract or 32a's protocol layer's public
  shape in an incompatible way.
- Stop if a new Python runtime dependency beyond what 32a already introduced is required and the user has not
  approved it.
- Stop if a deferral would have no owning task file.
