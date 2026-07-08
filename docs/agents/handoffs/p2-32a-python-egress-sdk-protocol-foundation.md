# Handoff

Task: `docs/tasks/p2/32a-python-egress-sdk-protocol-foundation.md`

## Changed

- `python/straw/proto/straw/v1/straw_pb2.py`, `.pyi` (+ `__init__.py` package files) — generated Python protobuf
  bindings for `api/proto/straw/v1/straw.proto`, no schema changes.
- `python/straw/egress/_ed25519.py` — pure-Python Ed25519 sign/verify/public-key derivation (RFC 8032 reference
  algorithm), used because no crypto dependency was approved.
- `python/straw/egress/protocol.py` — subject construction, safe-token validation, `Envelope`
  construction/marshal/unmarshal, registration signing, heartbeat construction.
- `python/straw/egress/natsclient.py` — minimal hand-rolled Core NATS wire client (CONNECT, PUB, SUB, UNSUB,
  flush/PING-PONG, MSG parsing) over one blocking TCP socket.
- `python/straw/egress/__init__.py` — package exports for the above.
- `python/pyproject.toml` — added `protobuf>=5.26,<8` as the package's only new dependency.
- `python/README.md` — new "Egress SDK (protocol foundation)" section with usage example and operator-obligation
  note.
- `python/tests/test_egress_protocol.py`, `test_egress_ed25519.py`, `test_egress_natsclient.py` — new tests.
- `docs/tasks/p2.md`, `docs/tasks/p2/32-python-egress-sdk.md` — task split bookkeeping (see below).
- `docs/tasks/p2/32a-...md` (this task), `docs/tasks/p2/32b-...md` — new task files from the split.

## Task-split context

The original task `docs/tasks/p2/32-python-egress-sdk.md` was sized close to the entire Go Egress SDK
(`sdk/egress/`, ~3500 lines) and required a dependency decision (protobuf toolchain unavailable in this
environment: no `protoc`/`buf`, no Python `protobuf` package installed) before any code could be written. The user
was asked and approved: (a) adding `protobuf` as a new Python dependency, generated via `grpcio-tools`' bundled
`protoc`, instead of hand-rolling protobuf wire encoding; (b) splitting the task into 32a (this task: protocol
foundation) and 32b (assignment runtime, not started). Task 32 is now `Status: superseded`; 32a and 32b are the
owning tasks for all remaining scope.

## Acceptance Criteria Verdicts

Filled from the independent verifier's report (fresh sub-agent, given only the task file and diff).

| Criterion | Verdict | Implementation (file:line) | Proving test |
|-----------|---------|----------------------------|--------------|
| Builds/signs registration + heartbeat, constructs every canonical subject | VERIFIED | `python/straw/egress/protocol.py` (subject fns, `build_register_request`, `build_heartbeat`) | `python/tests/test_egress_protocol.py:37-139` |
| Dot-containing/unsafe tokens rejected | VERIFIED | `protocol.py` `validate_subject_token` | `test_egress_protocol.py:42-44,82-84` |
| Envelope/RegisterRequest/HeartbeatRequest/AssignRequest/StreamFrame round-trip without data loss | VERIFIED | `protocol.py` `marshal_envelope`/`unmarshal_envelope` | `test_egress_protocol.py:142-199` |
| `rg` internal-import grep returns no matches | VERIFIED | n/a | `rg -n "github.com/beremaran/straw/v2/internal\|\.\./internal\|internal/" python` → no matches |
| `p1/29` still excludes Egress SDK/custom-worker behavior | VERIFIED | `docs/tasks/p1/29-python-client-sdk.md:41` unchanged | manual grep by verifier |
| Generated protobuf schema byte-identical / reproducible | VERIFIED | `python/straw/proto/straw/v1/straw_pb2.py` | verifier re-ran generation command, `diff` = empty |

## Planning-Doc Coverage

| Planning item | Status | Evidence / owning task |
|---------------|--------|------------------------|
| Envelope fields + oneof payload types (`12-nats-protocol.md` Envelope) | implemented | `protocol.py` `register_envelope`/`heartbeat_envelope`; other payload types (AssignRequest, StreamFrame) constructed directly from generated `pb` messages in tests, no extra wrapper needed |
| Envelope validation-by-payload-type field rules (register/heartbeat leave `deadline_unix_ms`/`attempt` at zero-value) | implemented | `protocol.py:186-190` (`register_envelope`), `:193` (`heartbeat_envelope`) — no `deadline_unix_ms`/`attempt` set, matching the Go SDK's `Run`/`Register`/`Heartbeat` envelopes |
| Canonical subjects table (register/heartbeat/logs/assign/c2e/e2c) | implemented | `protocol.py:43-77` |
| Dot-free safe subject tokens | implemented | `protocol.py:32-38` |
| Assignment flow subscription ordering (flush before AssignAck) | out of scope | owned by `docs/tasks/p2/32b-python-egress-sdk-assignment-runtime.md` (this task ships only the `flush()` primitive in `natsclient.py`, not the assignment flow itself) |
| Stream ordering/sequencing, backpressure/credit semantics | out of scope | owned by `docs/tasks/p2/32b-python-egress-sdk-assignment-runtime.md` |
| NATS subject ACLs (inbox prefixes) | implemented (prefix helpers only) | `protocol.py:56-59` (`control_inbox_prefix`, `worker_inbox_prefix`); actual NATS credential/ACL provisioning is deployment configuration, not SDK code, and is out of scope for both 32a and 32b |
| P2 component boundaries: Egress SDK operator obligations (destination-policy enforcement, public-safe facts) | documented, not enforced in code | `python/README.md` "Operator obligations" section states the obligation explicitly; enforcement is the custom implementation's responsibility once 32b exists, matching the Go SDK's approach (the SDK does not enforce policy for custom executors either) |

## Verification

```sh
cd python && python3 -m unittest discover tests   # 37 tests, all OK
go test ./sdk/... ./api/proto/...                  # unaffected, all OK
make check                                         # fmt-check, test, lint: all green
```

- Postgres-backed tests: not exercised — this task touches no Postgres surface.
- Live compose verification: skipped — this task adds a library-only protocol layer with no runtime wiring into
  `cmd/control`/`cmd/egress` (the task explicitly forbids that wiring; Python custom workers construct this SDK
  themselves as separate processes). Nothing in this diff has a live path to drive yet — that arrives with 32b's
  assignment runtime.

## Reviewer Start Points

- `python/straw/egress/protocol.py`
- `python/straw/egress/natsclient.py`
- `python/straw/egress/_ed25519.py`
- `python/tests/test_egress_protocol.py`

## Remaining Work

- Assignment runtime (registration/heartbeat loop wired to a live session, subscription-ordering enforcement,
  stream frame reading, executor invocation, credit-based backpressure) was completed by
  `docs/tasks/p2/32b-python-egress-sdk-assignment-runtime.md`.
- Grep for `InMemory`/`stub`/`fake`/`synthetic`/`TODO` in the diff surfaced only `_FakeNATSServer` in
  `python/tests/test_egress_natsclient.py:13` — a test-only loopback NATS simulator used to exercise wire framing
  without a real NATS server binary, exactly as the task's Steps asked for ("fake or local NATS wire harness"). Not
  a production fake standing in for a real backend.

## Blockers

- None. Work is ready to commit.
