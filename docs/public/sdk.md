---
sidebar_position: 5
---

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

For a large body, use `CreateReceipt`, one or more `UploadReceiptPart` calls, and `CompleteReceipt`; then set
`RequestBody{Mode: "receipt", ReceiptID: receipt.ReceiptID}`. Set `ResponseBodyMode: "receipt"` to store a response,
and open it with `DownloadReceipt`.

## Python

From this repository:

```sh
uv sync --all-packages --frozen
```

```python
from straw import Client, Request

client = Client("http://localhost:8080")
response = client.do(Request(method="GET", url="https://example.com"))
print(response.status, response.request_id)
```

Pass a token as the second `Client` argument. Non-200 Straw responses raise `straw.APIError`.

The Python client provides matching `create_receipt`, `upload_receipt_part`, `complete_receipt`, `get_receipt`, and
`download_receipt` methods. `RequestBody(mode="receipt", receipt_id=...)` and
`Request(response_body_mode="receipt", ...)` select the receipt paths.

The Python package also contains the lower-level worker SDK. See [custom workers](egress_worker.md).
