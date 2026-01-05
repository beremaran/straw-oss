# Implementation Plan: Circuit Breaker Mutex Optimization

## Problem Statement

The circuit breaker in `internal/infra/circuitbreaker/breaker.go` uses `sync.Mutex` for all state operations. Under high throughput, this creates contention as every request must acquire the lock to check state.

## Current Implementation

```go
type CircuitBreaker struct {
    mu          sync.Mutex  // Single lock for all operations
    state       State
    failures    uint
    lastFailure time.Time
}

func (cb *CircuitBreaker) Allow() bool {
    cb.mu.Lock()
    defer cb.mu.Unlock()
    // ... state check logic
}
```

## Proposed Changes

### Option 1: Use RWMutex (Recommended)

Most operations are reads (`Allow()` in closed state). Use `RWMutex` to allow concurrent reads:

### [MODIFY] [breaker.go](file:///home/beremaran/projects/straw-proxy/internal/infra/circuitbreaker/breaker.go)

```go
type CircuitBreaker struct {
    mu          sync.RWMutex  // Changed from sync.Mutex
    state       State
    failures    uint
    lastFailure time.Time
}

func (cb *CircuitBreaker) Allow() bool {
    cb.mu.RLock()
    state := cb.state
    lastFailure := cb.lastFailure
    cb.mu.RUnlock()
    
    switch state {
    case StateClosed:
        return true
    case StateOpen:
        if time.Since(lastFailure) > cb.resetTimeout {
            // Upgrade to write lock for state transition
            cb.mu.Lock()
            if cb.state == StateOpen { // Double-check
                cb.state = StateHalfOpen
            }
            cb.mu.Unlock()
            return true
        }
        return false
    case StateHalfOpen:
        return true
    }
    return false
}
```

### Option 2: Lock-Free with Atomic Operations

For even less contention, use atomic operations for state reads:

```go
type CircuitBreaker struct {
    mu          sync.Mutex    // Only for writes
    state       atomic.Int32  // Lock-free reads
    failures    atomic.Uint32
    lastFailure atomic.Int64  // Unix nano timestamp
}

func (cb *CircuitBreaker) Allow() bool {
    state := State(cb.state.Load())
    // Fast path: most requests go through closed state
    if state == StateClosed {
        return true
    }
    // ... handle open/half-open
}
```

## Benchmark Comparison

| Approach | Contention Level | Complexity | Recommendation |
|----------|------------------|------------|----------------|
| Current (Mutex) | High | Low | — |
| RWMutex | Medium | Low | ✓ Best balance |
| Atomics | Low | Medium | For extreme throughput |

## Verification Plan

### Automated Tests

- Existing circuit breaker tests must pass
- Add concurrent access test with `go test -race`

### Benchmarks

```bash
go test -bench=. ./internal/infra/circuitbreaker/...
```
