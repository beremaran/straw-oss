# 32 - Python Egress SDK

Status: not started

## Objective

Add a minimal Python Egress SDK that lets a custom Python worker register with Control, heartbeat, accept one decoded
HTTP assignment, read request frames, and stream response or error frames back over the documented Core NATS protocol
without importing Straw Go internals.

## Context (gap being closed)

This task was requested directly on 2026-07-08 to add a Python Egress SDK. Current code proves the Egress SDK exists
only in Go:

- `docs/planning/02-phase-boundaries.md:75-84` places the public Egress SDK and custom Egress implementations in P2.
- `docs/planning/05-component-boundaries.md:35-48` describes the Egress SDK as a public Go SDK with custom
  implementations behind a pluggable execution seam; no non-Go Egress SDK is specified.
- `sdk/egress/types.go:38-47` defines the Go `Executor` and tenant-aware execution seam.
- `sdk/egress/runtime.go:26-45` defines the Go NATS connection surface and registration entrypoint.
- `sdk/egress/assignment.go:30-63` defines the Go assignment worker runtime.
- `docs/tasks/p1/29-python-client-sdk.md:40-43` explicitly excludes Egress SDK/custom-worker behavior from the Python
  client SDK task.
- `find . -maxdepth 4 \( -path './.git' -o -path './tmp' \) -prune -o \( -name 'pyproject.toml' -o -name 'setup.py' -o -name '*.py' \) -print`
  currently finds no Python SDK package; only `deploy/docker/kms-mock.py` exists.

## Required Planning Docs

- `docs/planning/02-phase-boundaries.md` (P2 Egress SDK scope, lines ~75-84).
- `docs/planning/05-component-boundaries.md` (Egress SDK and Custom Egress Implementations, lines ~35-48).
- `docs/planning/12-nats-protocol.md` (Envelope, subjects, assignment flow, stream ordering, and credit semantics,
  lines ~8-160).
- `docs/planning/30-testing-matrix.md` (P1/P2 feature test rows before shipping, lines ~43-44).
- `docs/planning/31-implementation-order.md` (P2 Egress SDK order, lines ~37-43).
- `docs/planning/32-open-decisions.md` (superseded Provider Adapter decision, lines ~58-70).

## Prerequisites

- P2 task 24 completed (Go Egress SDK conformance and official-worker rebase are verified).
- P2 task 13 completed (the Go example custom Egress implementation proves the public worker contract).

## Out of Scope

- Do not add the Python client/request SDK; `docs/tasks/p1/29-python-client-sdk.md` owns client-side HTTP APIs.
- Do not add raw CONNECT, BodyRef, MITM, payload capture, HTTP/2-specific behavior, provider integrations, billing, or
  marketplace features.
- Do not change Control, `cmd/egress`, or the protobuf wire contract to fit Python.
- Do not publish to PyPI or add release automation.

## Expected Files

- Add: `python/straw/egress/__init__.py`, `python/straw/egress/protocol.py`, and `python/straw/egress/runtime.py` for
  the Python Egress SDK package.
- Add or modify: `python/straw/__init__.py` only as needed for package exports, coordinating with
  `docs/tasks/p1/29-python-client-sdk.md` if that task has already created it.
- Add or modify: `buf.gen.yaml` and generated files under `python/straw/proto/straw/v1/` only if Python protobuf
  generation can use the existing protobuf source and approved tooling without changing the wire contract.
- Add: `python/tests/test_egress.py` for registration signing, subject construction, assignment admission, decoded HTTP
  response streaming, and executor error tests.
- Add: `python/README.md` or `docs/public/sdk.md` usage notes for the implemented Python Egress SDK surface.

## Steps

- [ ] Read all required planning docs.
- [ ] Mirror the Go SDK's public worker data shape in Python: identity, capabilities, capacity, assignment request/ack,
      stream frame helpers, and a small executor callable.
- [ ] Implement subject construction and safe-token validation for registration, heartbeat, assignment, and request
      stream subjects.
- [ ] Implement registration request signing and heartbeat envelope construction compatible with the protobuf contract.
- [ ] Implement the smallest Core NATS client surface needed by the SDK or use an already-approved dependency; stop if a
      new dependency is required.
- [ ] Do not wire this into `cmd/control` or `cmd/egress`; Python custom worker processes construct the Python SDK
      runtime themselves.
- [ ] Implement a decoded HTTP assignment runtime that flushes the request-scoped `c2e` subscription before acking,
      reads `RequestStart`/`DataFrame`/`CreditFrame`/`CancelFrame`, calls the executor, and publishes sequenced
      `ResponseStart`/`DataFrame`/`EndFrame` or `ErrorFrame` responses on `e2c`.
- [ ] Add Python unit/conformance tests using a fake or local NATS wire harness; do not rely on Go `internal/*` test
      helpers.
- [ ] Add minimal Python Egress SDK usage docs and operator-obligation notes for destination-policy enforcement and
      public-safe execution facts.
- [ ] Run focused Python tests, focused Go SDK tests if shared proto generation or docs changed, then `make check`.
- [ ] Write a handoff note.

## Tests

- `python3 -m unittest discover python/tests`
- `go test ./sdk/...`
- `make check`

## Acceptance Criteria

- `python/straw/egress` can build and sign a registration request, heartbeat, assignment subject, and stream subjects
  matching the Go SDK behavior, proven by Python tests against known fixtures or the Go SDK conformance harness format.
- A Python SDK worker can accept one decoded HTTP assignment and stream `ResponseStart`, one or more `DataFrame`s, and
  `EndFrame` without buffering the full response, proven by a Python conformance test.
- Python executor errors map to an `ErrorFrame` on `e2c`, proven by a Python test asserting the published frame code and
  terminal behavior.
- The Python runtime flushes the request-scoped `c2e` subscription before sending `AssignAck`, proven by a fake/wire
  NATS test that would fail if `RequestStart` can be published before subscription readiness.
- `rg -n "github.com/beremaran/straw/v2/internal|\\.\\./internal|internal/" python` returns no matches for the Python
  Egress SDK.
- `docs/tasks/p1/29-python-client-sdk.md` still excludes Egress SDK/custom-worker behavior, and this task is named on the
  P2 board as the owner for Python Egress SDK work.

## Handoff Notes

- Record the Python package path, public executor shape, protobuf generation choice, NATS client choice, exact Python
  test command used, and any incompatibility found between the Go SDK contract and a non-Go implementation.

## Stop Conditions

- Stop if implementing the Python Egress SDK requires changing the protobuf/NATS wire contract.
- Stop if a new Python runtime dependency is required for the smallest usable SDK and the user has not approved it.
- Stop if package layout conflicts with `docs/tasks/p1/29-python-client-sdk.md` and no local pattern resolves it.
- Stop if a deferral would have no owning task file.
