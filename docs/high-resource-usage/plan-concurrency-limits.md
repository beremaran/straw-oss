# Implementation Plan: Lower Concurrency Limits

## Problem Statement

High concurrency limits cause excessive resource usage:

- Endpoint worker: 100 concurrent tasks (default)
- Each task holds memory for request/response bodies
- Under load, 100 simultaneous HTTP connections with large bodies consume significant memory

## Current Configuration

From `internal/config/config.go`:

```go
v.SetDefault("concurrency_limit", 100)
```

From `internal/endpoint/consumer/consumer.go`:

```go
const DefaultConcurrencyLimit = 100
```

## Proposed Changes

### [MODIFY] [config.go](file:///home/beremaran/projects/straw-proxy/internal/config/config.go)

Lower default concurrency and add relay server concurrency settings:

```go
// Endpoint defaults
v.SetDefault("concurrency_limit", 25) // Reduced from 100

// Add server-side concurrency limit
v.SetDefault("max_concurrent_requests", 50) // New setting for relay server
```

### [MODIFY] [consumer.go](file:///home/beremaran/projects/straw-proxy/internal/endpoint/consumer/consumer.go)

Update default constant:

```go
const DefaultConcurrencyLimit = 25 // Reduced from 100
```

### [MODIFY] [ServerConfig](file:///home/beremaran/projects/straw-proxy/internal/config/config.go)

Add concurrency configuration for relay server:

```go
type ServerConfig struct {
    // ... existing fields ...
    MaxConcurrentRequests int `mapstructure:"max_concurrent_requests"`
}
```

### [MODIFY] [relay.go](file:///home/beremaran/projects/straw-proxy/internal/server/handlers/relay.go)

Add request semaphore to limit concurrent in-flight requests at relay server level.

## Configuration Guidance

| Deployment | Endpoint Limit | Relay Limit | Notes |
|------------|---------------|-------------|-------|
| Low Memory (512MB) | 10 | 25 | Conservative for limited resources |
| Standard (2GB) | 25 | 50 | Balanced default |
| High Memory (8GB+) | 50-100 | 100-200 | For high-throughput scenarios |

## Verification Plan

### Automated Tests

- Run load tests with reduced limits
- Verify queue backpressure works correctly (requests wait rather than fail)

### Metrics to Monitor

- `endpoint_tasks_in_flight` should stay at or below limit
- Memory usage should be more stable under load
