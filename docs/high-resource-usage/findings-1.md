Based on the code provided, I have identified the likely causes of high CPU and memory usage.

### 1. Primary Suspect: Audit Log Middleware (Memory & CPU Spike)

**File:** `internal/server/admin/middleware/audit.go`

This file contains a critical performance anti-pattern that is likely the biggest contributor to your issues.

* **The Issue:** For *every* state-changing request (POST, PUT, DELETE), the middleware reads the **entire request body into memory**.

```go
// internal/server/admin/middleware/audit.go
var reqBody []byte
if req.Body != nil {
    reqBody, _ = io.ReadAll(req.Body) // 1. Allocates memory for full body
    // Restore body for next handlers
    req.Body = io.NopCloser(bytes.NewBuffer(reqBody)) // 2. Allocates AGAIN to restore
}

```

* **The Leak:** It then spawns an **unbounded goroutine** for every request to write to the database:

```go
go func() {
    // ...
    bodyStr := string(reqBody) // 3. Allocates a string copy of the body
    // ...
    _, _ = db.Exec(ctx, query, ...)
}()

```

* **Why it causes high usage:**
* **Memory:** If you receive a large request (e.g., 10MB), this middleware allocates at least **30MB** (10MB read + 10MB buffer restore + 10MB string cast) *per request*.
* **CPU:** The Garbage Collector (GC) has to work overtime to clean up these massive short-lived allocations.
* **Goroutine Explosion:** If your database (`admin_audit_log` table) creates backpressure (slow writes), thousands of goroutines will pile up, each holding a copy of the request body in memory.

### 2. Secondary Suspect: Response Compression (High CPU)

**File:** `internal/endpoint/publisher/publisher.go`

The endpoint worker compresses every HTTP response body before sending it back to the Relay server.

* **The Issue:**

```go
// internal/endpoint/publisher/publisher.go
if len(resp.Body) > 0 {
    compressed, err := protocol.Compress(resp.Body)
    // ...
}

```

* **Why it causes high usage:** The code uses `protocol.Compress` (likely LZMA/XZ based on standard Repomix usage). LZMA is extremely CPU-intensive. If your endpoints are scraping HTML pages (often 100KB-2MB) at high throughput, this will peg the CPU at 100%.

### 3. Connection Pooling (Memory Creep)

**File:** `internal/endpoint/transport/config.go`

* **The Issue:**

```go
MaxPoolHosts: 1000,
IdleConnTimeout: 90 * time.Second,

```

* **Why it causes high usage:** The transport layer keeps idle connections open for 90 seconds for up to 1000 different hosts. If your scraping targets are diverse (e.g., 10,000 different domains), the map entry overhead and idle connection buffers inside `net/http` will consume significant memory that is rarely released.

---

### How to Investigate & Measure (Know for Sure)

You can confirm these theories using Go's built-in `pprof` tool. You do not need to guess; the runtime provides the data.

#### 1. Enable pprof (if not already enabled)

Ensure your `main.go` imports `net/http/pprof`. It looks like your `metrics/registry.go` exposes a handler, but you should ensure the standard pprof endpoints are mounted:

```go
import _ "net/http/pprof"

// In your main setup:
go func() {
    log.Println(http.ListenAndServe("localhost:6060", nil))
}()

```

#### 2. Profile CPU Usage

Run this command while the server is under load:

```bash
go tool pprof -http=:8081 http://localhost:6060/debug/pprof/profile?seconds=30

```

* **What to look for:**
* If **Cause 1** is true: You will see `runtime.mallocgc` and `io.ReadAll` dominating the graph.
* If **Cause 2** is true: You will see `compress/lzma` or `protocol.Compress` taking up the majority of CPU time.

#### 3. Profile Memory (Heap)

Run this command:

```bash
go tool pprof -http=:8082 http://localhost:6060/debug/pprof/heap

```

* **What to look for:**
* Look for `inuse_space`.
* If **Cause 1** is true: You will see massive allocations in `middleware.AuditLog`.
* If **Cause 3** is true: You will see `net/http.(*Transport).getConn` or `transport` maps holding memory.

#### 4. Check Goroutine Count

```bash
curl http://localhost:6060/debug/pprof/goroutine?debug=1

```

* If the count is in the thousands and growing, search the output for `middleware.AuditLog.func1`. This confirms the database logging is backing up and leaking goroutines.

### Recommended Fixes

1. **Fix Audit Log:**

* **Limit Body Size:** Only log the first 1KB of the body, or exclude `body` from the log entirely for large requests.
* **Use a Worker Pool:** Instead of `go func()`, push audit tasks to a buffered channel (e.g., `auditChan <- logEntry`) and have a fixed number of workers (e.g., 5) consume and batch-insert into Postgres. This caps memory and CPU usage.

1. **Optimize Compression:**

* Switch from LZMA to **Snappy** or **Zstd** (fastest mode) for the Publisher. They offer slightly worse compression ratios but are 10-50x faster.

1. **Tune Transport:**

* Reduce `IdleConnTimeout` to `30s` and `MaxPoolHosts` to `100` if you are scraping random domains.
