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
