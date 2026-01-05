# Implementation Plan: Audit Log Middleware Optimization

## Problem Statement

The `AuditLog` middleware in `internal/server/admin/middleware/audit.go` has critical performance issues:

1. **Full Body Read**: Reads entire request body into memory for *every* state-changing request
2. **Triple Allocation**: Body is allocated 3 times (read → buffer restore → string cast)
3. **Unbounded Goroutines**: Spawns a new goroutine per request with no backpressure

## Proposed Changes

### [MODIFY] [audit.go](file:///home/beremaran/projects/straw-proxy/internal/server/admin/middleware/audit.go)

#### 1. Limit Body Size Before Reading

```go
const maxAuditBodySize = 1024 // 1KB limit for audit logging

var reqBody []byte
if req.Body != nil {
    limitedReader := io.LimitReader(req.Body, maxAuditBodySize+1)
    reqBody, _ = io.ReadAll(limitedReader)
    // Check if body was truncated
    if len(reqBody) > maxAuditBodySize {
        reqBody = reqBody[:maxAuditBodySize]
    }
    // Restore full body for next handlers using TeeReader pattern or original body
    req.Body = io.NopCloser(io.MultiReader(bytes.NewReader(reqBody), req.Body))
}
```

#### 2. Implement Worker Pool Pattern

Replace unbounded goroutines with a buffered channel and worker pool:

```go
type AuditEntry struct {
    Timestamp time.Time
    Method    string
    Path      string
    Query     string
    Body      string
    IP        string
    UserAgent string
    Status    int
    Error     string
}

type AuditLogger struct {
    db       Execer
    entries  chan AuditEntry
    wg       sync.WaitGroup
    workers  int
}

func NewAuditLogger(db Execer, bufferSize, workers int) *AuditLogger {
    al := &AuditLogger{
        db:      db,
        entries: make(chan AuditEntry, bufferSize),
        workers: workers,
    }
    al.Start()
    return al
}

func (al *AuditLogger) Start() {
    for i := 0; i < al.workers; i++ {
        al.wg.Add(1)
        go al.worker()
    }
}

func (al *AuditLogger) worker() {
    defer al.wg.Done()
    for entry := range al.entries {
        ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
        _, _ = al.db.Exec(ctx, insertQuery, /* ... */)
        cancel()
    }
}
```

## Verification Plan

### Automated Tests

- Load test with large request bodies (1MB+) before and after changes
- Monitor goroutine count via `pprof`

### Manual Verification

- Check memory allocation profile shows reduced allocations in audit path
