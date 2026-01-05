# Implementation Plan: Task Queue Management Improvements

## Problem Statement

Tasks are held in memory while waiting for semaphore slots. Under high load, this causes memory accumulation.

From `internal/endpoint/consumer/consumer.go`:

```go
func (c *Consumer) handleMessage(ctx context.Context, body []byte) error {
    select {
    case c.semaphore <- struct{}{}:
        // Got a slot
    case <-ctx.Done():
        return ctx.Err()
    }
    // Task body held in memory while waiting
    c.wg.Add(1)
    go func() {
        // ...
    }()
    return nil
}
```

## Proposed Changes

### Option 1: Leverage NATS/Broker-Level Backpressure (Recommended)

Configure the NATS consumer with a pull-based approach and max pending limit:

#### [MODIFY] [nats.go](file:///home/beremaran/projects/straw-proxy/internal/broker/nats.go)

```go
// Configure JetStream consumer with max pending
consumerConfig := nats.ConsumerConfig{
    MaxAckPending: concurrencyLimit, // Only fetch what we can process
}
```

This way, the broker holds queued messages rather than our memory.

### Option 2: Reject with Requeue When Full

If semaphore is full, return an error to trigger message requeue:

#### [MODIFY] [consumer.go](file:///home/beremaran/projects/straw-proxy/internal/endpoint/consumer/consumer.go)

```go
func (c *Consumer) handleMessage(ctx context.Context, body []byte) error {
    select {
    case c.semaphore <- struct{}{}:
        // Got a slot, proceed
    default:
        // No slot available, return error to requeue
        return ErrAtCapacity
    }
    // ... rest of processing
}

var ErrAtCapacity = errors.New("consumer at capacity, requeue message")
```

### Option 3: Bounded Queue with Timeout

Add a bounded internal queue with timeout:

```go
type Consumer struct {
    // ... existing fields
    taskQueue    chan []byte
    queueTimeout time.Duration
}

func (c *Consumer) handleMessage(ctx context.Context, body []byte) error {
    select {
    case c.taskQueue <- body:
        return nil
    case <-time.After(c.queueTimeout):
        return ErrQueueFull
    case <-ctx.Done():
        return ctx.Err()
    }
}
```

## Comparison

| Option | Memory Impact | Complexity | Message Loss Risk |
|--------|--------------|------------|-------------------|
| Broker Backpressure | Best | Medium | None |
| Reject + Requeue | Good | Low | Low (retry) |
| Bounded Queue | Medium | Medium | Timeout = reject |

## Metrics to Add

```go
var TasksQueued = prometheus.NewGauge(...)
var TasksRejected = prometheus.NewCounter(...)
```

## Verification Plan

### Automated Tests

- Flood endpoint with more tasks than concurrency limit
- Verify memory stays bounded
- Verify tasks eventually complete (no loss)

### Manual Verification

- Monitor `endpoint_tasks_in_flight` under load
- Check NATS/broker queue depth reflects backpressure
