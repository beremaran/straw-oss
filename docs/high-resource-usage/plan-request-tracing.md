# Implementation Plan: Enhanced Request Tracing

## Current State

OpenTelemetry tracing is already integrated via `internal/observability/tracing/provider.go`. The relay handler uses `otel.Tracer("router")` for spans.

## Problem Statement

Current tracing is minimal:

- Only covers router matching in relay handler
- Missing spans for: TLS handshake, HTTP execution, response processing, compression
- No integration with endpoint workers

## Proposed Changes

### [MODIFY] [client.go](file:///home/beremaran/projects/straw-proxy/internal/endpoint/http/client.go)

Add tracing to HTTP client:

```go
import "go.opentelemetry.io/otel"

func (c *Client) Do(ctx context.Context, req *protocol.Request) (*protocol.Response, error) {
    tracer := otel.Tracer("endpoint.http")
    ctx, span := tracer.Start(ctx, "http.execute")
    defer span.End()
    
    span.SetAttributes(
        attribute.String("http.method", req.Method),
        attribute.String("http.url", req.URL),
        attribute.String("fingerprint", req.Fingerprint),
    )
    
    // ... existing logic with child spans
}
```

### [MODIFY] [dial.go](file:///home/beremaran/projects/straw-proxy/internal/endpoint/tls/dial.go)

Add tracing to TLS handshake:

```go
func Dial(ctx context.Context, network, addr, fingerprint string) (net.Conn, error) {
    tracer := otel.Tracer("endpoint.tls")
    ctx, span := tracer.Start(ctx, "tls.handshake")
    defer span.End()
    
    span.SetAttributes(
        attribute.String("tls.fingerprint", fingerprint),
        attribute.String("net.peer.name", addr),
    )
    
    // ... existing logic
}
```

### [MODIFY] [publisher.go](file:///home/beremaran/projects/straw-proxy/internal/endpoint/publisher/publisher.go)

Add tracing to result publishing:

```go
func (p *Publisher) Publish(ctx context.Context, resp *protocol.Response, replyTo string) error {
    tracer := otel.Tracer("endpoint.publisher")
    ctx, span := tracer.Start(ctx, "publish.result")
    defer span.End()
    
    span.SetAttributes(
        attribute.String("request_id", resp.RequestID),
        attribute.Int("status_code", resp.StatusCode),
        attribute.Bool("compressed", resp.BodyCompressed),
    )
    
    // ... existing logic
}
```

### [MODIFY] [relay.go](file:///home/beremaran/projects/straw-proxy/internal/server/handlers/relay.go)

Expand existing tracing with more spans:

```go
func (h *RelayHandler) Handle(c echo.Context) error {
    ctx := c.Request().Context()
    tracer := otel.Tracer("relay")
    ctx, span := tracer.Start(ctx, "relay.handle")
    defer span.End()
    
    // Rate limit check span
    ctx, rlSpan := tracer.Start(ctx, "relay.ratelimit")
    // ... rate limit logic
    rlSpan.End()
    
    // Filter check span
    ctx, filterSpan := tracer.Start(ctx, "relay.filter")
    // ... filter logic
    filterSpan.End()
    
    // Execution span
    ctx, execSpan := tracer.Start(ctx, "relay.execute")
    result, err := h.executor.Execute(ctx, req, rule, sessionID, preferredEndpointID)
    execSpan.End()
    
    // ...
}
```

## Span Hierarchy

```
relay.handle
├── router.resolve
├── relay.ratelimit
├── relay.filter
└── relay.execute
    └── orchestrator.publish
        └── [endpoint receives via NATS]
            ├── tls.handshake
            ├── http.execute
            └── publish.result
```

## Configuration

Add environment variable to control tracing verbosity:

```bash
OTEL_TRACE_SAMPLING_RATE=0.1  # Sample 10% of requests in production
```

## Verification Plan

### Automated Tests

- Verify spans are created and contain expected attributes
- Test with tracing disabled (`OTEL_SDK_DISABLED=true`)

### Manual Verification

- Send test request and view trace in Jaeger/Zipkin
- Verify end-to-end trace correlation across services
