---
sidebar_position: 5
---

# Client SDKs

The REST API has small Go and Python clients. Both accept a base URL and optional deployment token.

## Go

```sh
go get github.com/beremaran/straw-oss/v2
```

```go
package main

import (
    "context"
    "fmt"
    "log"

    "github.com/beremaran/straw-oss/v2/sdk"
)

func main() {
    client := sdk.NewClient("http://localhost:8080", "")
    response, err := client.Do(context.Background(), sdk.Request{
        Method: "GET",
        URL:    "https://example.com",
    })
    if err != nil {
        log.Fatal(err)
    }
    fmt.Println(response.Status, response.RequestID)
}
```

Non-200 Straw responses are returned as `*sdk.APIError` with `HTTPStatus` and the parsed error envelope.

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

The Python package also contains the lower-level worker SDK. See [custom workers](egress_worker.md).
