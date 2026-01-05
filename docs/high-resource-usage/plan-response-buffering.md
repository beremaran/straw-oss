# Implementation Plan: Per-Request Response Buffering Control

## Problem Statement

Currently, all HTTP responses are fully buffered in memory before transmission. This is problematic for large responses (e.g., 10MB+ files) as they consume significant memory.

From `internal/endpoint/http/response.go`:

```go
func readResponseBody(resp *fhttp.Response, maxSize int64) ([]byte, error) {
    limitReader := io.LimitReader(resp.Body, maxSize+1)
    rawBody, err := io.ReadAll(limitReader)  // Full buffer in memory
    // ...
}
```

## Proposed Changes

### [NEW] Add Streaming Response Option to Protocol

#### [MODIFY] [types.go](file:///home/beremaran/projects/straw-proxy/pkg/protocol/types.go)

Add field to `Request` to control buffering behavior:

```go
type Request struct {
    // ... existing fields ...
    
    // StreamResponse indicates the caller wants the response streamed
    // rather than fully buffered. Useful for large file downloads.
    StreamResponse bool `json:"stream_response,omitempty"`
    
    // MaxResponseSize limits response body size (0 = use default)
    MaxResponseSize int64 `json:"max_response_size,omitempty"`
}
```

### [MODIFY] [response.go](file:///home/beremaran/projects/straw-proxy/internal/endpoint/http/response.go)

Add streaming support:

```go
type ResponseOptions struct {
    MaxBodySize    int64
    StreamResponse bool
}

func BuildResponseWithOptions(
    requestID string,
    resp *fhttp.Response,
    timing protocol.TimingInfo,
    opts ResponseOptions,
    endpointID, sessionID string,
) (*protocol.Response, error) {
    if opts.StreamResponse {
        return buildStreamingResponse(requestID, resp, timing, endpointID, sessionID)
    }
    return BuildResponse(requestID, resp, timing, opts.MaxBodySize, endpointID, sessionID)
}

func buildStreamingResponse(...) (*protocol.Response, error) {
    // Return metadata only, body will be streamed separately
    return &protocol.Response{
        RequestID:  requestID,
        StatusCode: resp.StatusCode,
        Headers:    HeadersToProtocol(resp.Header),
        IsStreaming: true,
        // Body is nil, indicating client should fetch via streaming endpoint
    }, nil
}
```

### [MODIFY] [dto.go](file:///home/beremaran/projects/straw-proxy/internal/server/dto/relay.go)

Expose streaming option in API:

```go
type RelayRequest struct {
    // ... existing fields ...
    StreamResponse  bool  `json:"stream_response,omitempty"`
    MaxResponseSize int64 `json:"max_response_size,omitempty"`
}
```

## Memory Impact

| Response Size | Current (Full Buffer) | With Streaming |
|--------------|----------------------|----------------|
| 100KB | 100KB | ~4KB (metadata only) |
| 10MB | 10MB | ~4KB |
| 100MB | 100MB (OOM risk) | ~4KB |

## Verification Plan

### Automated Tests

- Unit test for streaming response path
- Integration test with large response bodies

### Manual Verification

- Test with 50MB+ file downloads
- Monitor memory profile during streaming
