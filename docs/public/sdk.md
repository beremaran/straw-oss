# Go SDK Integration Guide

Straw provides a native, lightweight Go SDK (`github.com/beremaran/straw/v2/sdk`) for egress request forwarding. It handles HTTP envelope serialization, custom HTTP request construction, validation header injection, error deserialization, and binary stream frame parsing.

---

## Installation

Add the Straw Go SDK to your project's dependencies:

```bash
go get github.com/beremaran/straw/v2/sdk
```

---

## Initializing the Client

Construct a new `Client` by passing the Straw Control plane base URL and a tenant-scoped API key (typically carrying the `requester` or `tenant_admin` role).

```go
import "github.com/beremaran/straw/v2/sdk"

client := sdk.NewClient("http://localhost:8080", "sk_example_requester_secret")
```

### Configuring a Custom HTTP Client
By default, the client uses `http.DefaultClient`. You can configure a custom client (e.g. to set connection limits or timeouts) using the `WithHTTPClient` option:

```go
customHTTPClient := &http.Client{
    Timeout: 30 * time.Second,
}

client := sdk.NewClient(
    "http://localhost:8080", 
    "sk_example_requester_secret",
    sdk.WithHTTPClient(customHTTPClient),
)
```

---

## 1. Blocking Request Forwarding (`Do`)

The `Do` method performs a standard blocking request. The client blocks until the Control plane dispatches the request to an Egress worker, executes it, and returns the response envelope containing the upstream status and body.

### Example: GET Request with Custom Headers

```go
package main

import (
	"context"
	"encoding/base64"
	"fmt"
	"log"
	"time"

	"github.com/beremaran/straw/v2/sdk"
)

func main() {
	client := sdk.NewClient("http://localhost:8080", "sk_example_requester_secret")

	// base64 encode headers value
	userAgentEncoded := base64.StdEncoding.EncodeToString([]byte("My Custom Agent"))

	req := sdk.Request{
		Method: "GET",
		URL:    "https://api.github.com/users/octocat",
		Headers: []sdk.Header{
			{Name: "User-Agent", ValueBase64: userAgentEncoded},
		},
		TimeoutMs: 10000, // 10s timeout
	}

	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()

	resp, err := client.Do(ctx, req)
	if err != nil {
		log.Fatalf("forwarding failed: %v", err)
	}

	fmt.Printf("Request Trace ID: %s\n", resp.RequestID)
	fmt.Printf("Upstream HTTP Status: %d\n", resp.Status)
	fmt.Printf("Egress Time: %dms (Total: %dms)\n", resp.Timing.EgressMs, resp.Timing.TotalMs)

	// Decode the base64-encoded response body
	if resp.Body.Mode == "inline_base64" {
		bodyBytes, err := base64.StdEncoding.DecodeString(resp.Body.DataBase64)
		if err != nil {
			log.Fatalf("failed to decode body: %v", err)
		}
		fmt.Printf("Response Body: %s\n", string(bodyBytes))
	}
}
```

---

## 2. Streaming Request Forwarding (`DoStream`)

For requests where the response payload is large or streamed dynamically, use `DoStream`. This endpoint establishes a binary stream connection with the Control plane using the `application/vnd.straw.request-stream.v1+binary` protocol.

The response is returned as a sequence of typed binary frames. The Go SDK handles the deserialization of these frames automatically.

### Binary Stream Frame Types
The stream returns `StreamFrame` objects. Each frame has a `Type` corresponding to one of the following constants:

1. **`StreamFrameMetadata` (`1`)**: The first successful frame containing the request trace ID, upstream HTTP status, and initial headers.
2. **`StreamFrameBody` (`2`)**: A chunk of raw upstream response bytes. Multiple body frames may be sent sequentially.
3. **`StreamFrameTrailers` (`3`)**: Carrying trailing HTTP headers returned by the upstream.
4. **`StreamFrameEnd` (`4`)**: The terminal frame containing performance timing stats.
5. **`StreamFrameError` (`5`)**: A late-stage error envelope sent if execution fails after headers have already been flushed.

### Example: Streaming Response Execution

```go
package main

import (
	"context"
	"errors"
	"fmt"
	"io"
	"log"
	"time"

	"github.com/beremaran/straw/v2/sdk"
)

func main() {
	client := sdk.NewClient("http://localhost:8080", "sk_example_requester_secret")

	req := sdk.Request{
		Method: "GET",
		URL:    "https://speed.cloudflare.com/__down?bytes=10000000", // 10MB test file
	}

	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()

	stream, err := client.DoStream(ctx, req)
	if err != nil {
		log.Fatalf("stream start failed: %v", err)
	}
	defer stream.Close()

	var totalBytes int

	for {
		frame, err := stream.Next()
		if err != nil {
			if errors.Is(err, io.EOF) {
				fmt.Println("\nStream ended successfully.")
				break
			}
			log.Fatalf("stream read error: %v", err)
		}

		switch frame.Type {
		case sdk.StreamFrameMetadata:
			fmt.Printf("Request ID: %s\n", frame.RequestID)
			fmt.Printf("Upstream Status: %d\n", frame.Metadata.Status)
			fmt.Println("Headers:")
			for _, h := range frame.Metadata.Headers {
				fmt.Printf("  %s: %s\n", h.Name, h.ValueBase64)
			}
		case sdk.StreamFrameBody:
			totalBytes += len(frame.Body)
			fmt.Printf("\rDownloaded: %d bytes...", totalBytes)
		case sdk.StreamFrameTrailers:
			fmt.Println("\nTrailers:")
			for _, h := range frame.Trailers.Headers {
				fmt.Printf("  %s: %s\n", h.Name, h.ValueBase64)
			}
		case sdk.StreamFrameEnd:
			fmt.Printf("\nDone. Duration: %dms\n", frame.End.Timing.TotalMs)
		case sdk.StreamFrameError:
			log.Fatalf("\nStream execution failed midway: %s (code: %s)", 
				frame.Error.Message, frame.Error.Code)
		}
	}
}
```

---

## 3. Error Handling

When an API call returns a non-200 status code, the SDK returns an `*sdk.APIError` containing a structured `ErrorResponse`. 

You can extract the error details to inspect the failure reason:

```go
resp, err := client.Do(ctx, req)
if err != nil {
    var apiErr *sdk.APIError
    if errors.As(err, &apiErr) {
        fmt.Printf("HTTP Status: %d\n", apiErr.HTTPStatus)
        fmt.Printf("Error Code: %s\n", apiErr.Response.Code)
        fmt.Printf("Error Category: %s\n", apiErr.Response.Category)
        fmt.Printf("Message: %s\n", apiErr.Response.Message)
        fmt.Printf("Is Retryable: %v\n", apiErr.Response.Retryable)
        fmt.Printf("Request Trace ID: %s\n", apiErr.Response.RequestID)
        
        if len(apiErr.Response.Details) > 0 {
            fmt.Println("Details:")
            for k, v := range apiErr.Response.Details {
                fmt.Printf("  %s: %s\n", k, v)
            }
        }
    } else {
        fmt.Printf("Generic network error: %v\n", err)
    }
}
```
