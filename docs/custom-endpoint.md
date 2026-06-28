# Custom Endpoint Developer Guide

This guide explains how to build a custom proxy endpoint using the public Go SDK packages exported by the `straw` codebase.

With this SDK, you can build custom workers that handle requests using your own HTTP client setups, custom TLS/JA3/JA4 fingerprints, rotating proxy provider integrations, and specialized logging/metrics.

---

## 📦 1. SDK Overview

The Straw Proxy SDK exports three core packages:
1. **`pkg/endpoint`**: Core worker logic (`Consumer`, `Publisher`, `HeartbeatSender`, and `Worker` option decorators).
2. **`pkg/broker`**: Public message broker interface and NATS implementation.
3. **`pkg/protocol`**: Protocol request and response data models.

### Installation

To import the public packages into a new Go project, add the module dependency:

```bash
go get github.com/beremaran/straw@latest
```

---

## 🛠️ 2. Implementing a Custom Request Executor

The core of a custom endpoint is implementing the `endpoint.RequestExecutor` interface. This interface defines how incoming tasks from the queue are executed against the target URL:

```go
type RequestExecutor interface {
    Do(ctx context.Context, req *protocol.Request) (*protocol.Response, error)
}
```

Below is a complete implementation of a custom request executor that routes egress traffic through a rotating proxy provider (e.g. using a custom proxy URL) and maps requests back into the protocol format:

```go
package main

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"time"

	"github.com/beremaran/straw/pkg/protocol"
)

type ProxyProviderExecutor struct {
	httpClient *http.Client
}

func NewProxyProviderExecutor(proxyURL string) (*ProxyProviderExecutor, error) {
	parsedURL, err := url.Parse(proxyURL)
	if err != nil {
		return nil, fmt.Errorf("invalid proxy URL: %w", err)
	}

	transport := &http.Transport{
		Proxy: http.ProxyURL(parsedURL),
	}

	return &ProxyProviderExecutor{
		httpClient: &http.Client{
			Transport: transport,
			Timeout:   30 * time.Second,
		},
	}, nil
}

func (e *ProxyProviderExecutor) Do(ctx context.Context, req *protocol.Request) (*protocol.Response, error) {
	// 1. Translate protocol.Request to native *http.Request
	httpReq, err := http.NewRequestWithContext(ctx, req.Method, req.URL, bytes.NewReader(req.Body))
	if err != nil {
		return nil, err
	}

	// Copy headers
	for _, h := range req.Headers {
		httpReq.Header.Add(h.Key, h.Value)
	}

	startTime := time.Now()

	// 2. Perform HTTP call
	httpResp, err := e.httpClient.Do(httpReq)
	if err != nil {
		return nil, err
	}
	defer httpResp.Body.Close()

	totalTime := time.Since(startTime)

	// 3. Read body
	respBody, err := io.ReadAll(httpResp.Body)
	if err != nil {
		return nil, err
	}

	// 4. Translate native http.Response back to *protocol.Response
	respHeaders := protocol.HeaderMap{}
	for k, values := range httpResp.Header {
		for _, v := range values {
			respHeaders = append(respHeaders, protocol.Header{Key: k, Value: v})
		}
	}

	return &protocol.Response{
		RequestID:  req.ID,
		SessionID:  req.SessionID,
		StatusCode: httpResp.StatusCode,
		Headers:    respHeaders,
		Body:       respBody,
		Timing: &protocol.TimingInfo{
			Total: totalTime,
		},
	}, nil
}
```

---

## 🚀 3. Running the Worker Application

Once you have your custom executor, you can wire it up using the simplified `endpoint.Worker` API to run your custom endpoint application:

```go
package main

import (
	"context"
	"log"
	"os"
	"os/signal"
	"syscall"

	"github.com/beremaran/straw/internal/config"
	"github.com/beremaran/straw/pkg/endpoint"
)

func main() {
	// 1. Load endpoint config from the environment
	cfg, err := config.LoadEndpointConfig()
	if err != nil {
		log.Fatalf("failed to load configuration: %v", err)
	}

	// 2. Initialize your Custom Request Executor
	proxyServer := "http://user:pass@proxy-provider.com:8000"
	executor, err := NewProxyProviderExecutor(proxyServer)
	if err != nil {
		log.Fatalf("failed to create proxy executor: %v", err)
	}

	// 3. Initialize the Worker with the config and custom executor option
	w := endpoint.NewWorker(cfg, endpoint.WithRequestExecutor(executor))

	// 4. Setup graceful signal handling context
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	// 5. Start the worker (this automatically sets up the NATS broker connection, 
	//    heartbeat sender, metrics server, publisher, and consumer, blocking until canceled)
	log.Println("starting custom worker...")
	if err := w.Start(ctx); err != nil {
		log.Fatalf("worker failed or shut down with error: %v", err)
	}

	log.Println("shutdown completed gracefully")
}
```

---

## 💡 Best Practices

* **Task Authentication**: The `HMAC_SECRET` passed to `NewConsumer` must match the secret set on the orchestrator server. Any tasks with invalid signatures will be automatically rejected by the consumer to prevent unauthorized payload execution.
* **Geolocation and Capability Tagging**: When configuring the heartbeat sender or supplying `ENDPOINT_TAGS`, specify relevant tagging (e.g. `type:residential`, `region:eu`, `ISP:comcast`). The orchestrator reads these tags from the heartbeats to route matching requests to your custom worker.
* **Resource Management**: Always set standard connection timeouts inside your custom `RequestExecutor`. Failing to specify a timeout on the `http.Client` can cause goroutines to block indefinitely if a proxy server hangs.
