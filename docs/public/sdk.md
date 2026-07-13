# Client SDKs

The REST API has small Go and Python clients. Both accept a base URL and optional deployment token.

## Go

```sh
go get github.com/beremaran/straw-sdk-go@v0.1.0
```

```go
package main

import (
    "context"
    "fmt"
    "log"

    straw "github.com/beremaran/straw-sdk-go"
)

func main() {
    client := straw.NewClient("http://localhost:8080", "")
    response, err := client.Do(context.Background(), straw.Request{
        Method: "GET",
        URL:    "https://example.com",
    })
    if err != nil {
        log.Fatal(err)
    }
    fmt.Println(response.Status, response.RequestID)
}
```

Non-200 Straw responses are returned as `*straw.APIError` with `HTTPStatus` and the parsed error envelope.

Create one client per base URL/token and reuse it; the client is safe for concurrent requests through Go's shared
HTTP transport. Every call accepts a context, so set a deadline and cancel abandoned work. The SDK does not retry:
inspect `APIError.Response.Retryable`, `RetryAfterMs`, and application idempotency before replay. Response bodies are
fully represented by the bounded API envelope or receipt and require no caller-owned HTTP body close. Use structured
request IDs/error codes in logs and never log tokens, headers, URLs, bodies, or signed receipt references.

For a large body, use `CreateReceipt`, one or more `UploadReceiptPart` calls, and `CompleteReceipt`; then set
`RequestBody{Mode: "receipt", ReceiptID: receipt.ReceiptID}`. Set `ResponseBodyMode: "receipt"` to store a response,
and open it with `DownloadReceipt`.

## Python

Install the exact public tag:

```sh
uv add 'straw-sdk @ git+https://github.com/beremaran/straw-sdk-python.git@v0.1.0'
```

```python
from straw import Client, Request

client = Client("http://localhost:8080")
response = client.do(Request(method="GET", url="https://example.com"))
print(response.status, response.request_id)
```

Pass a token as the second `Client` argument. Non-200 Straw responses raise `straw.APIError`.

Reuse a client rather than creating one per request. Supply explicit request timeouts and let cancellation/errors
propagate; there is no automatic retry. A shared client may be used by concurrent application tasks according to the
tagged package API, while callers remain responsible for their own mutable request data. Log request IDs and stable
codes only. Tests can point the client at an `httptest`/local fake implementing the documented REST contract.

The Python client provides matching `create_receipt`, `upload_receipt_part`, `complete_receipt`, `get_receipt`, and
`download_receipt` methods. `RequestBody(mode="receipt", receipt_id=...)` and
`Request(response_body_mode="receipt", ...)` select the receipt paths.

The Python package also contains the lower-level worker SDK. See [custom workers](egress_worker.md).

The REST clients are the most stable SDK surface. Worker SDKs follow the negotiated protocol compatibility matrix and
may change between pre-1.0 minors. Generated package documentation and source are linked from the public tagged
repositories; use only versions listed in [Compatibility and versioning](compatibility.md).
