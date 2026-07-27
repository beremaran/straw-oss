# Client SDKs

The REST API has small Go and Python clients. Both accept a base URL and optional deployment token.

## Go

```sh
go get github.com/beremaran/straw-sdk-go@v0.3.0
```

```go
package main

import (
    "context"
    "errors"
    "fmt"
    "log"
    "os"
    "time"

    straw "github.com/beremaran/straw-sdk-go"
)

func main() {
    client := straw.NewClient("http://localhost:8080", os.Getenv("STRAW_AUTH_TOKEN"))
    ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
    defer cancel()

    response, err := client.Do(ctx, straw.Request{
        Method: "GET",
        URL:    "https://example.com",
        Routing: &straw.RoutingHints{Country: "AU", Tags: []string{"residential"}, StickySessionID: "checkout-42"},
        Headers: []straw.Header{{Name: "X-Request-Source", ValueBase64: "YXBw"}},
    })
    if err != nil {
        var apiErr *straw.APIError
        if errors.As(err, &apiErr) {
            log.Printf("Straw error code=%s retryable=%t request_id=%s", apiErr.Response.Code, apiErr.Response.Retryable, apiErr.Response.RequestID)
            return
        }
        log.Fatal(err)
    }
    fmt.Println(response.Status, response.RequestID)
}
```

The exact `v0.3.0` Go tag supports the fields shown above plus `Body`, `FingerprintProfile`, `TimeoutMs`,
`Replayable`, and `ResponseBodyMode`. Header values are base64-encoded bytes. Non-2xx Straw responses are returned as
`*straw.APIError` with `HTTPStatus` and the parsed error envelope.

`Request.Routing` carries optional tags, country, region, IP type, and sticky-session ID constraints. GET, HEAD, and
OPTIONS requests become replayable by default; set `Replayable: true` only when the application operation is safe to
retry for other methods.

Create one client per base URL/token and reuse it; the client is safe for concurrent requests through Go's shared
HTTP transport. Every call accepts a context, so set a deadline and cancel abandoned work. The SDK does not retry:
inspect `APIError.Response.Retryable`, `RetryAfterMs`, and application idempotency before replay. Response bodies are
fully represented by the bounded API envelope or receipt and require no caller-owned HTTP body close. Use structured
request IDs/error codes in logs and never log tokens, headers, URLs, bodies, or signed receipt references.

For a large body, use `CreateReceipt`, one or more `UploadReceiptPart` calls, and `CompleteReceipt`; then set
`RequestBody{Mode: "receipt", ReceiptID: receipt.ReceiptID}`. Set `ResponseBodyMode: "receipt"` to store a response,
and open it with `DownloadReceipt`.

This is a one-part request receipt; larger uploads use the same calls with positive part numbers and completion still
requires the complete `1..N` set:

```go
body := []byte("request body")
sum := sha256.Sum256(body)
receipt, err := client.CreateReceipt(ctx, straw.CreateReceiptInput{
    Direction: "request", SizeBytes: int64(len(body)), SHA256Hex: hex.EncodeToString(sum[:]),
})
if err != nil { log.Fatal(err) }
if _, err = client.UploadReceiptPart(ctx, receipt.ReceiptID, 1, bytes.NewReader(body), int64(len(body)), hex.EncodeToString(sum[:])); err != nil {
    log.Fatal(err)
}
if _, err = client.CompleteReceipt(ctx, receipt.ReceiptID); err != nil { log.Fatal(err) }
response, err := client.Do(ctx, straw.Request{
    Method: "POST", URL: "https://example.com/upload", Replayable: false,
    Body: &straw.RequestBody{Mode: "receipt", ReceiptID: receipt.ReceiptID},
})
if err != nil { log.Fatal(err) }
fmt.Println(response.Status, response.RequestID)
```

Add `bytes`, `crypto/sha256`, and `encoding/hex` to the imports in this receipt example. For a response receipt,
set `ResponseBodyMode: "receipt"`, read `response.Body.ReceiptID`, and close the `io.ReadCloser` returned by
`DownloadReceipt` after copying it.

## Python

Install the exact public tag:

```sh
uv add 'straw-sdk @ git+https://github.com/beremaran/straw-sdk-python.git@v0.2.0'
```

```python
import os

from straw import APIError, Client, Header, Request, RoutingHints

client = Client("http://localhost:8080", os.getenv("STRAW_AUTH_TOKEN", ""), timeout=30.0)
try:
    response = client.do(Request(
        method="GET",
        url="https://example.com",
        routing=RoutingHints(tags=["residential"], country="AU", sticky_session_id="checkout-42"),
        headers=[Header(name="X-Request-Source", value_base64="YXBw")],
    ))
    print(response.status, response.request_id)
except APIError as exc:
    print(exc.http_status, exc.response.code, exc.response.retryable, exc.response.request_id)
```

Pass a token as the second `Client` argument. The exact `v0.2.0` package exposes `Client`, `Request`, `Header`,
`RequestBody`, `Response`, `Receipt`, and `APIError` as used above. Non-2xx Straw responses raise `straw.APIError`;
inspect `exc.http_status` and the typed `exc.response` envelope.

Reuse a client rather than creating one per request. Supply explicit request timeouts and let cancellation/errors
propagate; there is no automatic retry. A shared client may be used by concurrent application tasks according to the
tagged package API, while callers remain responsible for their own mutable request data. Log request IDs and stable
codes only. Tests can point the client at an `httptest`/local fake implementing the documented REST contract.

The Python client provides matching `create_receipt`, `upload_receipt_part`, `complete_receipt`, `get_receipt`, and
`download_receipt` methods. `RequestBody(mode="receipt", receipt_id=...)` and
`Request(response_body_mode="receipt", ...)` select the receipt paths.

Use `RoutingHints(tags=["residential"], country="AU", sticky_session_id="checkout-42")` on `Request.routing` for
the same routing contract as REST, proxy, CONNECT, and the Go SDK. GET, HEAD, and OPTIONS default to replayable; set
`replayable=True` for another method only when the operation is safe to retry.

Example request-body upload:

```python
import hashlib

body = b"request body"
digest = hashlib.sha256(body).hexdigest()
receipt = client.create_receipt("request", len(body), digest, "upload-42")
client.upload_receipt_part(receipt.receipt_id, 1, body, digest)
client.complete_receipt(receipt.receipt_id)
response = client.do(Request(
    method="POST",
    url="https://example.com/upload",
    body=RequestBody(mode="receipt", receipt_id=receipt.receipt_id),
))
print(response.status, response.request_id)
```

The Python package also contains the lower-level worker SDK. See [custom workers](egress_worker.md).

The REST clients are the most stable SDK surface. Worker SDKs follow the negotiated protocol compatibility matrix and
may change between pre-1.0 minors. Generated package documentation and source are linked from the public tagged
repositories; use only versions listed in [Compatibility and versioning](compatibility.md).

Both public clients default `GET`, `HEAD`, and `OPTIONS` to replayable; other methods remain non-replayable unless the
tagged client request type permits an explicit override. Neither client automatically retries a failed request.
