# Investigation Report: Potential CPU and Memory Bottlenecks in Straw Proxy

## Executive Summary

Based on my analysis of the Straw Proxy codebase, I've identified several potential causes of high CPU and memory usage, along with existing monitoring capabilities and recommendations for further instrumentation. This distributed proxy system has multiple components that could contribute to resource consumption under load.

## 1. Architecture Overview

Straw Proxy is an event-driven, passive consumer proxy system with these core components:

- **Relay Server**: The "brain" that handles authentication, routing, and task distribution
- **Endpoint Workers**: The "muscle" that executes HTTP requests using TLS fingerprinting
- **Message Broker (NATS/RabbitMQ)**: Facilitates communication between components
- **Supporting Infrastructure**: Redis for caching, PostgreSQL for persistence

## 2. Potential CPU Bottlenecks

### 2.1 TLS Fingerprinting Operations

- **Location**: [`internal/endpoint/tls/dial.go`](internal/endpoint/tls/dial.go)
- **Impact**: High CPU usage during TLS handshakes with custom fingerprints using `utls` library
- **Cause**: Each request requires creating a TLS connection that mimics browser fingerprints, which is computationally expensive

### 2.2 HTTP Request Processing

- **Location**: [`internal/endpoint/http/client.go`](internal/endpoint/http/client.go)
- **Impact**: CPU-intensive processing of HTTP requests and responses
- **Cause**: Large request/response bodies, complex headers, and response processing

### 2.3 Concurrent Task Processing

- **Location**: [`internal/endpoint/consumer/consumer.go`](internal/endpoint/consumer/consumer.go:133)
- **Impact**: High CPU when processing many concurrent tasks
- **Cause**: Default concurrency limit of 100 tasks per endpoint (line 24), with each task requiring significant processing

### 2.4 Circuit Breaker Operations

- **Location**: [`internal/infra/circuitbreaker/breaker.go`](internal/infra/circuitbreaker/breaker.go)
- **Impact**: Additional CPU overhead from circuit breaker checks
- **Cause**: Mutex contention in high-throughput scenarios (lines 37, 80, 105, 121)

## 3. Potential Memory Bottlenecks

### 3.1 Connection Pool Management

- **Location**: [`internal/endpoint/transport/pool.go`](internal/endpoint/transport/pool.go)
- **Impact**: Memory growth from pooled connections
- **Cause**:
  - Pools keyed by "host:fingerprint" (line 70)
  - LRU eviction mechanism (lines 27-28)
  - Potential memory leaks if connections aren't properly closed

### 3.2 Response Buffering

- **Location**: [`internal/endpoint/http/response.go`](internal/endpoint/http/response.go)
- **Impact**: High memory usage for large responses
- **Cause**: Full response buffering in memory before transmission

### 3.3 Task Queue Management

- **Location**: [`internal/endpoint/consumer/consumer.go`](internal/endpoint/consumer/consumer.go)
- **Impact**: Memory accumulation from in-flight tasks
- **Cause**: Tasks held in memory while waiting for semaphore slots (lines 192-197)

### 3.4 Caching Layers

- **Location**:
  - [`internal/infra/redis/client.go`](internal/infra/redis/client.go)
  - [`internal/service/router/cache.go`](internal/service/router/cache.go)
- **Impact**: Memory growth from cached data
- **Cause**: Accumulation of cached routing rules, auth tokens, and session data

## 4. Existing Monitoring Capabilities

### 4.1 Prometheus Metrics

- **Location**: [`internal/observability/metrics/registry.go`](internal/observability/metrics/registry.go)
- **Available Metrics**:
  - Go runtime metrics (lines 22-24)
  - Process metrics (line 24)
  - Custom application metrics

### 4.2 Endpoint-Specific Metrics

- **Location**: [`internal/endpoint/metrics/metrics.go`](internal/endpoint/metrics/metrics.go)
- **Key Metrics**:
  - `endpoint_tasks_in_flight` (line 47): Current concurrent tasks
  - `endpoint_connections_pooled` (line 38): Active connections by host
  - `endpoint_upstream_duration_seconds` (line 10): Request latency
  - `endpoint_bytes_sent/received` (lines 81, 89): Data transfer volumes

### 4.3 Server-Side Metrics

- **Location**: [`internal/server/metrics/metrics.go`](internal/server/metrics/metrics.go)
- **Key Metrics**:
  - `relay_active_sessions` (line 75): Concurrent sessions
  - `relay_request_duration_seconds` (line 42): Request processing time
  - `relay_cache_hits/misses` (lines 59, 67): Cache effectiveness

## 5. Recommendations for Measurement and Instrumentation

### 5.1 Immediate Actions

1. **Enable Existing Metrics**:
   - Ensure metrics server is running (configured in [`cmd/relay-server/main.go:278`](cmd/relay-server/main.go:278))
   - Monitor `/metrics` endpoint for real-time data

2. **Add Memory Profiling**:
   - Enable Go's pprof profiles for heap and CPU
   - Add endpoints for `/debug/pprof/heap` and `/debug/pprof/profile`

3. **Monitor Connection Pools**:
   - Track pool size vs. max capacity
   - Monitor eviction frequency in [`pool.go:242`](internal/endpoint/transport/pool.go:242)

### 5.2 Enhanced Instrumentation

1. **Add Resource Usage Metrics**:
   - Goroutine count
   - Memory allocation rates
   - GC pause times

2. **Implement Request Tracing**:
   - Track end-to-end request latency
   - Identify bottlenecks in the request pipeline

3. **Add Database Connection Monitoring**:
   - Track connection pool usage in PostgreSQL client
   - Monitor Redis connection health

### 5.3 Performance Testing

1. **Load Testing**:
   - Use the existing [`scripts/load-test.sh`](scripts/load-test.sh) to simulate high load
   - Monitor resource usage during peak load

2. **Stress Testing**:
   - Test behavior at concurrency limits
   - Identify breaking points for memory and CPU

## 6. Configuration Optimizations

### 6.1 Connection Pool Tuning

- **Location**: [`cmd/endpoint/main.go:90-93`](cmd/endpoint/main.go:90-93)
- **Recommendations**:
  - Adjust `MaxPoolHosts` based on target diversity
  - Tune `IdleConnsPerHost` for optimal reuse
  - Set appropriate `IdleConnTimeout` for cleanup

### 6.2 Concurrency Limits

- **Location**: [`internal/endpoint/consumer/consumer.go:24`](internal/endpoint/consumer/consumer.go:24)
- **Recommendation**:
  - Adjust `DefaultConcurrencyLimit` based on endpoint capabilities
  - Consider dynamic scaling based on system resources

### 6.3 Rate Limiting

- **Location**: [`internal/service/ratelimit/limiter.go`](internal/service/ratelimit/limiter.go)
- **Impact**:
  - Proper rate limiting prevents resource exhaustion
  - Monitor Redis key expiration (lines 45, 81)

## 7. Potential Memory Leaks

### 7.1 Connection Handling

- **Risk**: Improperly closed connections in the transport pool
- **Detection**: Monitor `endpoint_connections_pooled` metric for unexpected growth
- **Mitigation**: Ensure proper connection cleanup in [`pool.go:232`](internal/endpoint/transport/pool.go:232)

### 7.2 Goroutine Leaks

- **Risk**: Goroutines not properly terminated
- **Detection**: Monitor goroutine count via runtime metrics
- **Mitigation**: Ensure proper context cancellation in all background processes

## Conclusion

The Straw Proxy system has several potential points of high CPU and memory usage, primarily related to TLS fingerprinting, connection pooling, and concurrent task processing. The existing Prometheus metrics provide a good foundation for monitoring, but additional instrumentation would help identify specific bottlenecks.

To diagnose issues with certainty:

1. Enable all existing metrics and monitor them during normal operation
2. Add memory and CPU profiling endpoints
3. Conduct load testing while monitoring resource usage
4. Focus on connection pool behavior and task queue management
5. Implement alerting for key metrics like goroutine count and memory growth

The distributed nature of the system means that issues might manifest differently in the relay server versus endpoint workers, so monitoring should be implemented at all components.
