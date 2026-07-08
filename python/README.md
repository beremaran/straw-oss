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

## Egress SDK (protocol foundation)

`straw.egress` gives a custom Python worker the wire-compatible pieces needed to talk to Control over Core
NATS, mirroring the Go SDK (`sdk/egress`): canonical subject construction, `Envelope` build/marshal, and
signed registration/heartbeat requests. Unlike the client above, this package depends on `protobuf` (for
`straw.proto.straw.v1.straw_pb2`, generated from `api/proto/straw/v1/straw.proto`) and includes its own
minimal Core NATS wire client (`straw.egress.NATSClient`) since no NATS client dependency was available to
approve at the time this was built.

**This is the protocol foundation only.** The assignment runtime that actually serves a decoded HTTP
request (subscription-ordering, streaming a response, executor invocation) is a separate package
addition — see `docs/tasks/p2/32b-python-egress-sdk-assignment-runtime.md`. `straw.egress` alone can
register and heartbeat, but cannot yet accept an assignment.

```python
import os

from straw.egress import Capabilities, Identity, build_register_request, register_envelope, marshal_envelope

identity = Identity(
    worker_id="worker-1",
    credential_id="cred-1",
    executor_type="http",
    private_key=os.urandom(32),  # persist this seed; it is the worker's Ed25519 identity key
)
caps = Capabilities(max_concurrency=8, software_version="0.1.0")

req = build_register_request(identity, caps)  # already signed
raw = marshal_envelope(register_envelope(req))
# raw is ready to publish on straw.egress.registration_subject() over straw.egress.NATSClient
```

### Operator obligations

A custom Egress implementation built on this SDK assumes the same executor-side obligations the official
Go worker enforces: equivalent destination-policy enforcement (`docs/planning/27-security-controls.md`)
and reporting only constrained, public-safe execution facts back to Control. This SDK does not enforce
destination policy for you — that remains the custom implementation's responsibility once the assignment
runtime (32b) is in place.
