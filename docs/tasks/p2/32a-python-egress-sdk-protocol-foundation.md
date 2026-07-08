# 32a - Python Egress SDK Protocol Foundation

Status: done

## Objective

Give the Python Egress SDK a wire-compatible protocol layer: generated Python protobuf bindings for
`api/proto/straw/v1/straw.proto`, subject construction and safe-token validation for every canonical NATS subject,
`Envelope` construction/validation by payload type, registration-request signing, heartbeat envelope construction, and
the smallest Core NATS wire client the SDK needs — all without touching the protobuf/NATS wire contract or Go/Control
runtime code. No assignment runtime is built in this task; it produces the foundation task 32b's runtime is built on.

## Context (gap being closed)

Task 32 ("Python Egress SDK") was split on 2026-07-08 after user approval because it was sized close to the entire Go
Egress SDK (`sdk/egress/`, ~3500 lines across `types.go`, `runtime.go`, `assignment.go`, `stream.go`, `bodyref.go`, plus
conformance tests) and this environment has neither `protoc`/`buf` nor an approved Python NATS client dependency. The
user approved adding `protobuf` as a new Python dependency (generated via `grpcio-tools`' bundled `protoc`, mirroring
how Go generates `.pb.go` from the same source `.proto` — `grpcio-tools` itself is a codegen-time tool, not necessarily
a shipped runtime dependency) instead of hand-rolling protobuf wire encoding.

Current code proves no Python protocol layer exists:

- `python/pyproject.toml:6` declares `dependencies = []`; `python/straw/client.py` (P1 task 29) is a stdlib-only REST
  client and explicitly excludes Egress SDK/custom-worker behavior
  (`docs/tasks/p1/29-python-client-sdk.md` Out of Scope: "Do not add Egress SDK, custom worker, BodyRef, MITM,
  payload-capture, or telemetry API clients").
- `find . -maxdepth 4 -name '*.proto' -o -name 'straw_pb2.py'` finds `api/proto/straw/v1/straw.proto` but no generated
  Python bindings anywhere in the repo.
- `sdk/egress/types.go:38-47` (public `Executor`/tenant-aware seam) and `api/proto/straw/v1/registration_sign.go`
  (registration signing) define the Go-side contract this task must be wire-compatible with; no Python equivalent
  exists.
- `buf.gen.yaml` only generates Go (`local: protoc-gen-go`, `out: api/proto`); no Python plugin is configured.

## Required Planning Docs

- `docs/planning/02-phase-boundaries.md` (P2 Egress SDK scope, line ~43 and the P2 section listing it).
- `docs/planning/05-component-boundaries.md` (Egress SDK and Custom Egress Implementations, lines ~35-48).
- `docs/planning/12-nats-protocol.md` (Envelope, canonical subjects, subject ACLs, assignment subscription-ordering
  rules — read in full; lines ~1-202).
- `docs/planning/31-implementation-order.md` (P2 Egress SDK order).
- `docs/planning/32-open-decisions.md` (superseded Provider Adapter decision, for context only).

## Prerequisites

- P2 task 24 completed (Go Egress SDK conformance verified — this task mirrors that contract).
- P2 task 13 completed (example custom Go Egress implementation proves the public worker contract shape).

## Out of Scope

- Do not implement the assignment worker runtime, streaming loop, or executor callable invocation — task 32b owns
  that.
- Do not add the Python client/request SDK; `docs/tasks/p1/29-python-client-sdk.md` owns client-side HTTP APIs.
- Do not add raw CONNECT, BodyRef, MITM, payload capture, HTTP/2-specific behavior, provider integrations, billing, or
  marketplace features.
- Do not change Control, `cmd/egress`, the protobuf schema, or any wire contract to fit Python.
- Do not publish to PyPI or add release automation.
- Do not add `grpcio` (the RPC runtime) as a shipped dependency; only `protobuf` (the generated-message runtime) and,
  if needed for codegen, `grpcio-tools` as a dev-only tool are in scope.

## Expected Files

- Add: `python/straw/proto/straw/v1/` generated Python protobuf bindings for `api/proto/straw/v1/straw.proto` (record
  the exact generation command used, e.g. `python -m grpc_tools.protoc`, in the handoff so it is reproducible).
- Modify: `python/pyproject.toml` to add `protobuf` to `dependencies`.
- Add: `python/straw/egress/__init__.py` and `python/straw/egress/protocol.py` for Envelope construction/validation,
  subject construction, safe-token validation, registration signing, and heartbeat envelope construction.
- Add: a minimal Core NATS wire client — either `python/straw/egress/natsclient.py` (hand-implemented CONNECT/PUB/SUB/
  MSG/PING-PONG framing) or a newly-justified minimal dependency (decide and record the choice and why in the
  handoff; a hand-rolled client is the default expectation given `python/pyproject.toml`'s current zero-dependency
  baseline for transport concerns).
- Add: `python/tests/test_egress_protocol.py` for subject construction, safe-token validation, Envelope round-trip
  encode/decode, and registration signing tests.

## Steps

- [x] Read all required planning docs.
- [x] Choose and record the Python protobuf generation command; generate bindings under
      `python/straw/proto/straw/v1/` from the existing `.proto` source with no schema changes.
- [x] Add `protobuf` to `python/pyproject.toml` dependencies.
- [x] Implement subject construction and dot-free safe-token validation for `straw.v1.control.register`,
      `straw.v1.control.heartbeat`, `straw.v1.control.logs`, `straw.v1.executor.<worker_id>.<session_id>.assign`,
      `straw.v1.req.<request_id>.<worker_id>.<session_id>.c2e`, and `...e2c`.
- [x] Implement `Envelope` construction with the field-population rules from `docs/planning/12-nats-protocol.md`'s
      "Envelope Validation by Payload Type" table (RegisterRequest, HeartbeatRequest, LogEvent, AssignRequest/
      StreamFrame).
- [x] Implement registration request signing and heartbeat envelope construction compatible with
      `api/proto/straw/v1/registration_sign.go` and `sdk/egress/types.go`.
- [x] Implement the smallest Core NATS wire client surface needed (connect, publish, subscribe, flush, ping/pong) or
      record the newly-approved minimal dependency; stop if neither is achievable without a heavier dependency.
- [x] Do not wire this into `cmd/control` or `cmd/egress`; Python custom worker processes construct this SDK
      themselves.
- [x] Add the tests listed in Expected Files.
- [x] Run focused Python tests, focused Go SDK/proto tests if shared proto generation changed, then `make check`.
- [x] Write a handoff note.

## Tests

- `python3 -m unittest discover python/tests`
- `go test ./sdk/... ./api/proto/...`
- `make check`

## Acceptance Criteria

- `python/straw/egress/protocol.py` builds and signs a registration request and a heartbeat envelope, and constructs
  every canonical subject listed in `docs/planning/12-nats-protocol.md`'s Canonical Subjects table, proven by
  `python/tests/test_egress_protocol.py`.
- Dot-containing or otherwise unsafe tokens are rejected by subject construction, proven by a test.
- Generated Python protobuf messages round-trip encode/decode without data loss for `Envelope`, `RegisterRequest`,
  `HeartbeatRequest`, `AssignRequest`, and `StreamFrame`, proven by a test.
- `rg -n "github.com/beremaran/straw/v2/internal|\\.\\./internal|internal/" python` returns no matches.
- `docs/tasks/p1/29-python-client-sdk.md` still excludes Egress SDK/custom-worker behavior (no change needed there;
  this criterion just confirms no scope leak occurred).
- The generated protobuf schema is byte-identical in structure to `api/proto/straw/v1/straw.proto` (no hand-edited
  message shapes), proven by re-running the recorded generation command and diffing.

## Handoff Notes

- Protobuf generation command (reproducible; verified byte-identical on re-run):
  `python -m grpc_tools.protoc -I api/proto --python_out=python/straw/proto --pyi_out=python/straw/proto api/proto/straw/v1/straw.proto`
  (via `grpcio-tools`, a dev-time-only codegen tool — not a shipped runtime dependency).
- NATS wire client: hand-rolled (`python/straw/egress/natsclient.py`), not a dependency. No NATS Python client was
  approved and the zero-dependency baseline in `python/pyproject.toml` made hand-rolling the smallest usable option;
  implements CONNECT handshake, PUB, SUB, UNSUB, flush (PING/PONG), and MSG parsing over one blocking TCP socket.
- Registration signing: pure-Python Ed25519 (`python/straw/egress/_ed25519.py`, RFC 8032 reference algorithm) because
  no crypto dependency (e.g. `cryptography`) was approved either. Cross-validated during development against the
  `cryptography` package's OpenSSL-backed Ed25519 (byte-identical output on 20+ random vectors), and the fixed
  regression fixture in `python/tests/test_egress_ed25519.py` was itself generated via `cryptography` so the test
  checks against an independent implementation, not only self-consistency.
- `registration_signing_payload()` in `protocol.py` reproduces `api/proto/straw/v1/registration_sign.go`'s
  `RegistrationSigningPayload` byte-for-byte (domain prefix, `worker_id\n`, `credential_id\n`, `executor_type\n`,
  `major.minor\n`, `len(nonce):nonce\n`, `issued_at_unix_ms`).
- No incompatibility found between the Go SDK contract and this Python implementation.
- The assignment runtime (registration/heartbeat loop, stream frame handling, executor invocation) is deferred to
  task 32b (`docs/tasks/p2/32b-python-egress-sdk-assignment-runtime.md`), which is already the owning task — not an
  unowned deferral.

## Stop Conditions

- Stop if implementing the Python protocol layer requires changing the protobuf/NATS wire contract.
- Stop if the smallest usable NATS wire client requires a dependency heavier than a minimal Core NATS client library
  and the user has not approved it.
- Stop if a deferral would have no owning task file.
