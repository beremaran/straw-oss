# Runtime Profiling Guide

This guide explains how to capture CPU, memory, and other runtime profiles from running straw-proxy services using Go's built-in `pprof` tooling.

## Available Profiles

| Profile | Endpoint | Description |
|---------|----------|-------------|
| CPU | `/debug/pprof/profile?seconds=30` | CPU usage over time |
| Heap | `/debug/pprof/heap` | Current memory allocations |
| Goroutines | `/debug/pprof/goroutine` | Active goroutine stacks |
| Allocs | `/debug/pprof/allocs` | All past memory allocations |
| Block | `/debug/pprof/block` | Goroutine blocking operations |
| Mutex | `/debug/pprof/mutex` | Mutex contention |
| Trace | `/debug/pprof/trace?seconds=5` | Execution trace |

## Accessing pprof Endpoints

### Default Ports

| Service | Metrics/pprof Port |
|---------|-------------------|
| relay-server | 9090 |
| endpoint | 9090 |

> [!NOTE]
> By default, these ports are not exposed externally in docker-compose for security reasons. See below for access methods.

---

## Method 1: Docker Compose Override (Recommended for Development)

Create a `docker-compose.override.yml` to expose the metrics port:

```yaml
# docker-compose.override.yml
services:
  relay-server:
    ports:
      - "9200:9090"  # Expose pprof/metrics
  
  endpoint-1:
    ports:
      - "9201:9090"  # pprof for endpoint-1
  
  endpoint-2:
    ports:
      - "9202:9090"  # pprof for endpoint-2
```

Then restart:

```bash
docker compose up -d
```

Now you can access directly:

```bash
# Relay server profiles
curl http://localhost:9200/debug/pprof/heap > heap.prof

# Endpoint-1 profiles
curl http://localhost:9201/debug/pprof/goroutine?debug=1
```

---

## Method 2: Temporary Port Forward with Docker

Forward a specific container's port temporarily:

```bash
# Find container ID
docker compose ps

# Forward endpoint-1's pprof port to local 9091
docker run --rm -it --network straw-proxy_straw-net \
  -p 9091:9091 \
  alpine/socat TCP-LISTEN:9091,fork TCP:endpoint-1:9090
```

In another terminal:

```bash
curl http://localhost:9091/debug/pprof/heap > heap.prof
```

---

## Method 3: Access via Docker Network

Use a temporary container on the same network:

```bash
# Interactive access
docker run --rm -it --network straw-proxy_straw-net curlimages/curl sh

# One-liner to fetch heap profile
docker run --rm --network straw-proxy_straw-net \
  curlimages/curl -s http://relay-server:9090/debug/pprof/heap > heap.prof
```

---

## Load Testing with Profiling

### Step-by-Step Workflow

1. **Start services with pprof exposed:**

   ```bash
   # Ensure docker-compose.override.yml exposes port 9090
   docker compose up -d --build
   ```

2. **Capture baseline goroutine count:**

   ```bash
   curl -s http://localhost:9090/debug/pprof/goroutine?debug=1 | head -5
   ```

3. **Start CPU profiling (in background):**

   ```bash
   # 60-second CPU profile
   curl -o cpu_during_load.prof \
     "http://localhost:9090/debug/pprof/profile?seconds=60" &
   ```

4. **Run your load test:**

   ```bash
   # Example with hey
   hey -n 10000 -c 100 -H "Authorization: Bearer YOUR_TOKEN" \
     http://localhost:8080/proxy?url=http://httpbin.org/ip

   # Or with wrk
   wrk -t4 -c100 -d60s http://localhost:8080/healthz
   ```

5. **Capture heap profile after load:**

   ```bash
   curl -o heap_after_load.prof http://localhost:9090/debug/pprof/heap
   ```

6. **Check goroutine count after load (leak detection):**

   ```bash
   curl -s http://localhost:9090/debug/pprof/goroutine?debug=1 | head -5
   ```

---

## Analyzing Profiles

### Interactive Web UI (Recommended)

```bash
# CPU profile
go tool pprof -http=:8082 cpu_during_load.prof

# Heap profile
go tool pprof -http=:8082 heap_after_load.prof
```

This opens a browser with flame graphs, call graphs, and top consumers.

### Command-Line Analysis

```bash
# Top 20 memory consumers
go tool pprof -top -cum heap.prof

# Top 20 CPU consumers
go tool pprof -top cpu.prof

# Show specific function
go tool pprof -list="handleRequest" cpu.prof
```

### Comparing Profiles (Before/After)

```bash
# Compare two heap profiles
go tool pprof -base heap_before.prof heap_after.prof

# Then use 'top' or 'list' to see differences
```

---

## Common Diagnostics

### Goroutine Leak Detection

```bash
# Get goroutine count
curl -s http://localhost:9090/debug/pprof/goroutine?debug=1 | head -1

# Full stack trace of all goroutines
curl -s http://localhost:9090/debug/pprof/goroutine?debug=2 > goroutines.txt
```

Look for goroutines stuck in:

- `chan receive` (unbuffered channel waits)
- `select` (infinite waits)
- `sync.Mutex.Lock` (deadlocks)

### Memory Leak Detection

```bash
# Capture heap before load
curl -o heap_before.prof http://localhost:9090/debug/pprof/heap

# Run load test...

# Capture heap after load
curl -o heap_after.prof http://localhost:9090/debug/pprof/heap

# Compare
go tool pprof -http=:8082 -diff_base=heap_before.prof heap_after.prof
```

### Mutex Contention

First, enable mutex profiling in the application (add to main.go if needed):

```go
runtime.SetMutexProfileFraction(1)
runtime.SetBlockProfileRate(1)
```

Then capture:

```bash
curl -o mutex.prof http://localhost:9090/debug/pprof/mutex
go tool pprof -http=:8082 mutex.prof
```

---

## Example: Diagnosing High CPU

```bash
# 1. Start profiling during high CPU
curl -o cpu.prof "http://localhost:9090/debug/pprof/profile?seconds=30"

# 2. Analyze
go tool pprof -http=:8082 cpu.prof
```

In the web UI:

- Click **Flame Graph** for visual call hierarchy
- Click **Top** for flat list of CPU consumers
- Look for unexpected functions consuming >5% CPU

---

## Example: Diagnosing High Memory

```bash
# 1. Capture heap profile
curl -o heap.prof http://localhost:9090/debug/pprof/heap

# 2. Show top allocations by size
go tool pprof -top -inuse_space heap.prof

# 3. Show allocation counts (for GC pressure)
go tool pprof -top -alloc_objects heap.prof
```

---

## Security Considerations

> [!WARNING]
> pprof endpoints expose internal application state. In production:
>
> - **Never expose port 9090 publicly**
> - Use network policies to restrict access
> - Consider adding authentication middleware
> - Use temporary port forwards only when debugging

---

## Quick Reference

```bash
# CPU profile (30 seconds)
curl -o cpu.prof "http://localhost:9090/debug/pprof/profile?seconds=30"

# Heap profile
curl -o heap.prof http://localhost:9090/debug/pprof/heap

# Goroutine count
curl -s http://localhost:9090/debug/pprof/goroutine?debug=1 | head -1

# Full goroutine dump
curl -s http://localhost:9090/debug/pprof/goroutine?debug=2 > goroutines.txt

# Analyze with web UI
go tool pprof -http=:8082 <profile.prof>

# Compare before/after
go tool pprof -http=:8082 -diff_base=before.prof after.prof
```
