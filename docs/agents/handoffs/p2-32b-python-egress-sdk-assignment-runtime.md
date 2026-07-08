# Handoff

Task: `docs/tasks/p2/32b-python-egress-sdk-assignment-runtime.md`

## Changed

- `python/straw/egress/runtime.py` (new): registration/heartbeat request-reply (`Runtime`), the assignment
  worker (`Worker`), stream-frame sequence validation (`_StreamValidator`, mirroring `sdk/egress/stream.go`),
  a sid-based message dispatcher over the single synchronous NATS connection (`_Dispatcher`), and
  byte-credit backpressure for the response direction (`_CreditGate`).
- `python/straw/egress/__init__.py`: exports the new runtime surface (`Runtime`, `Worker`, `DecodedRequest`,
  `DecodedResponse`, `ProtocolError`, `RegistrationError`).
- `python/tests/test_egress_runtime.py` (new): a scripted fake Core NATS server (parses SUB/PUB/UNSUB off
  the wire into an ordered record list) driving three conformance tests.
- `python/README.md`: replaced the "protocol foundation only" Egress SDK section with full runtime usage,
  the executor callable shape, and operator-obligation notes.
- `docs/tasks/p2/32b-python-egress-sdk-assignment-runtime.md`: Status → `done`, all Steps checked, Handoff
  Notes filled in.
- `docs/tasks/p2.md`: board row 32b → `done`.

Task 32's `Status: superseded` and its 32a/32b handoff pointer were already correct from the 32a run; no
change was needed this run (confirmed unchanged).

## Acceptance Criteria Verdicts

Filled from a fresh independent verifier agent (given only the task file and diff, no implementer reasoning).

| Criterion | Verdict | Implementation (file:line) | Proving test |
|-----------|---------|----------------------------|--------------|
| Stream `ResponseStart`/`DataFrame`(s)/`EndFrame` without buffering the full response | VERIFIED | `runtime.py:433-442` (`_publish_response` iterates `response.body` lazily, publishing per chunk) | `test_streams_decoded_response_without_buffering` — the executor's generator blocks on the first `DataFrame` already being on the wire before yielding its second chunk, so the test fails if the runtime buffered the whole body first |
| Executor errors → `ErrorFrame`, terminal | VERIFIED | `runtime.py:385-386` catches `Exception`, maps via `_publish_error` (`runtime.py:444`) to `ERROR_CODE_EXECUTOR_INTERNAL_ERROR` | `test_executor_error_maps_to_error_frame` — asserts exactly one frame (the error), joined thread confirms no further frames |
| `c2e` subscription flushed before `AssignAck` | VERIFIED | `runtime.py:332-335` subscribes+flushes `c2e` before calling `_reply` | `_accept_assignment` asserts SUB-before-PUB ordering via the fake server's real record index, not a final-state check |
| Credit-based backpressure (stop + resume) | VERIFIED | `_CreditGate.take`/`_await_grant` (`runtime.py:483-520`) | `test_credit_backpressure_stops_and_resumes` — confirms exactly 2 frames sent with 5 bytes of credit, asserts the serve thread is still alive (genuinely blocked) before granting more, then confirms resumption |
| `rg` internal/-import check | VERIFIED | n/a | `rg -n "github.com/beremaran/straw/v2/internal|\.\./internal|internal/" python` → no matches |
| Task 32 superseded, names 32a/32b | VERIFIED | `docs/tasks/p2/32-python-egress-sdk.md:3,105-114` | — |
| Task 29 still excludes Egress SDK | VERIFIED | `docs/tasks/p1/29-python-client-sdk.md:41` unchanged | — |

Verifier also confirmed: no Control/`cmd/egress`/protobuf/wire-contract changes; no raw CONNECT/BodyRef/MITM/
HTTP2/provider code added; `runtime.py` correctly builds on 32a's `protocol.py`/`natsclient.py` (imports,
doesn't fork).

## Planning-Doc Coverage

| Planning item | Status | Evidence / owning task |
|---------------|--------|------------------------|
| `docs/planning/12-nats-protocol.md` Assignment Flow subscription ordering | implemented | `runtime.py:216-219` (assignment subject), `runtime.py:332-335` (c2e subscribe+flush before ack) |
| Stream ordering and sequencing (`stream_seq`, `attempt`, duplicate/gap/terminal rules) | implemented | `_StreamValidator` (`runtime.py:112-165`) |
| Backpressure and credit semantics (initial credit from `AssignRequest`, `CreditFrame` grants, stop-at-zero) | implemented | `_CreditGate` (`runtime.py:483-520`); upload-side credit tracked by the same validator during request-body read |
| `docs/planning/05-component-boundaries.md` operator obligations (destination-policy enforcement, public-safe execution facts) | implemented (as a documented obligation on the custom executor, matching the Go SDK's stance) | `python/README.md` "Operator obligations" section |
| `docs/planning/30-testing-matrix.md` P2 Egress SDK test row | implemented | `python/tests/test_egress_runtime.py` |
| Raw CONNECT tunnel, BodyRef, MITM, HTTP/2 (out of scope per task) | out of scope | owned by the Go SDK (`sdk/egress`); Python SDK intentionally decoded-HTTP-only, documented in `runtime.py` module docstring and `python/README.md` |
| Per-session concurrent assignments (Go SDK behavior) | out of scope, documented simplification | `ponytail:` comment at `runtime.py:15-18`; Python SDK serves one assignment at a time per session — a worker needing concurrency uses the Go SDK |

## Verification

```sh
python3 -m unittest discover python/tests   # 40/40 pass, run 3x for flakiness, none observed
go test ./sdk/...                            # pass (unaffected — no Go files touched)
make check                                   # fmt-check, test, lint all pass, 0 lint issues
```

- Postgres-backed tests: not exercised — diff touches no Postgres surface.
- Live compose verification: skipped. This SDK is a library for external custom worker processes; there is
  no `cmd/*` wiring to drive live, and the task's Expected Files/Steps do not call for a compose-stack
  reachability test (only a fake/local NATS wire harness, which was built and used).

## Reviewer Start Points

- `python/straw/egress/runtime.py`
- `python/tests/test_egress_runtime.py`
- `python/README.md` (Egress SDK section)

## Remaining Work

- None owned by this task. Two pre-existing, out-of-scope gaps are worth naming for anyone extending this SDK
  (neither is a defect in what 32b shipped, and neither needs a new task right now — they're properties of
  the one-assignment-at-a-time, decoded-HTTP-only scope this task explicitly chose):
  - No per-session concurrency (Go SDK parity) — upgrade path is a per-request worker pool around `Worker`,
    noted in `runtime.py`'s module docstring and `python/README.md`.
  - No raw tunnel/BodyRef/MITM/HTTP2 support — explicitly out of scope per this task file; a Python worker
    needing those modes uses the Go SDK.

## Blockers

- None. Work is committed in this session (see commit history) after this handoff was written.
