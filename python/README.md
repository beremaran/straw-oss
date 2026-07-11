# Straw Python SDK

Minimal Python client for Straw's public REST request transport. Uses only the standard library —
no third-party runtime dependency.

Supported endpoints:

- `POST /api/v1/requests` (blocking request)
- `POST /api/v1/requests:stream` (binary frame streaming)

## Usage

```python
from straw import Client, Request, Header

client = Client("http://localhost:8080", "sk_example_requester_secret")

resp = client.do(Request(
    method="GET",
    url="https://api.github.com/users/octocat",
    headers=[Header(name="User-Agent", value_base64="TXkgQ3VzdG9tIEFnZW50")],
))

print(resp.request_id, resp.status)  # resp.status is the upstream HTTP status
```

`GET`, `HEAD`, and `OPTIONS` requests default `replayable=True` before submission; other methods do not.

### Errors

Non-200 responses raise `straw.APIError`, which carries `http_status` and the parsed canonical
`ErrorResponse` (`category`, `code`, `message`, `retryable`, `request_id`, `retry_after_ms`, `details`):

```python
from straw import APIError

try:
    client.do(req)
except APIError as err:
    print(err.http_status, err.response.category, err.response.code)
```

### Streaming

```python
stream = client.do_stream(Request(method="GET", url="https://example.com/large-file"))
try:
    for frame in stream:
        if frame.type == straw.FRAME_METADATA:
            print("status", frame.metadata.status)
        elif frame.type == straw.FRAME_BODY:
            handle_chunk(frame.body)
        elif frame.type == straw.FRAME_ERROR:
            raise RuntimeError(frame.error.message)
finally:
    stream.close()
```

`Stream` reads exactly one frame at a time from the live HTTP response, so body bytes are yielded
as they arrive instead of being buffered until the response completes.

## Running tests

```sh
python3 -m unittest discover python/tests
```

## Egress SDK

`straw.egress` lets a custom Python worker talk to Control over Core NATS and serve decoded-HTTP
assignments, mirroring the Go SDK (`sdk/egress`): canonical subject construction, `Envelope` build/marshal,
signed registration/heartbeat requests (`protocol.py`), and an assignment runtime (`runtime.py`) that
registers, heartbeats, and serves one decoded HTTP request at a time. Unlike the client above, this package
depends on `protobuf` (for `straw.proto.straw.v1.straw_pb2`, generated from `api/proto/straw/v1/straw.proto`)
and includes its own minimal Core NATS wire client (`straw.egress.NATSClient`) since no NATS client
dependency was available to approve at the time this was built.

**Scope**: decoded HTTP only — no raw CONNECT tunnel, BodyRef, MITM, or HTTP/2-specific behavior, and one
assignment in flight per session (`docs/implementation-history.md#p2-32b`). A worker
needing the Go SDK's per-session concurrency or the other request modes should use the Go SDK
(`sdk/egress`) instead.

```python
import os

from straw.egress import Capabilities, DecodedRequest, DecodedResponse, Identity, NATSClient, Runtime

identity = Identity(
    worker_id="worker-1",
    credential_id="cred-1",
    executor_type="http",
    private_key=os.urandom(32),  # persist this seed; it is the worker's Ed25519 identity key
)
caps = Capabilities(max_concurrency=1, software_version="0.1.0")


def executor(request: DecodedRequest) -> DecodedResponse:
    # Do the upstream call yourself; STREAM the response body instead of
    # buffering it (a generator is fine — the runtime pulls one chunk at a
    # time and publishes it before asking for the next).
    return DecodedResponse(status=200, headers=[("content-type", b"text/plain")], body=[b"hello from a custom worker"])


conn = NATSClient("127.0.0.1", 4222)
runtime = Runtime(conn, identity, caps)
runtime.register()  # blocks for Control's RegisterAck; retry/backoff is the caller's responsibility

worker = runtime.worker(executor)
worker.start()  # subscribes + flushes the session's assignment subject
while True:
    worker.serve_one(timeout=30.0)       # blocks for one AssignRequest, serves it fully, returns
    runtime.heartbeat(worker.active_requests())  # call periodically (e.g. every 5s) from your own loop
```

`Runtime.register()`/`Runtime.heartbeat()` are plain blocking request/reply calls — a real worker process
drives its own registration-retry and heartbeat-interval loop around them (see
`sdk/egress/runtime.go`'s `Run` for the Go reference shape); this SDK does not spawn threads for you.

### Executor callable shape

```python
Executor = Callable[[DecodedRequest], DecodedResponse]
```

- `DecodedRequest`: `method`, `url`, `headers: list[(name, bytes)]`, `body: bytes`, `attempt`.
- `DecodedResponse`: `status`, `headers: list[(name, bytes)]`, and `body: Iterable[bytes]` — return a
  generator to stream without buffering the full response; the runtime publishes each chunk (`DataFrame`)
  as it is produced, respecting Control-granted download credit, then a terminal `EndFrame`.
- Raising from the executor callable, or from iterating its `body`, is caught and published as an
  `ErrorFrame` (`ERROR_CODE_EXECUTOR_INTERNAL_ERROR`) instead of an `EndFrame` — the stream still
  terminates cleanly.

### Operator obligations

A custom Egress implementation built on this SDK assumes the same executor-side obligations the official
Go worker enforces: equivalent destination-policy enforcement (`docs/planning/27-security-controls.md`)
and reporting only constrained, public-safe execution facts back to Control (do not leak internal
infrastructure details — upstream hostnames beyond what the request already carries, internal IPs, stack
traces — into `ErrorFrame.message`/`details`). This SDK does not enforce destination policy for you; that
remains the custom implementation's responsibility inside its `executor` callable.
