# Load and Backpressure Testing

This is a local correctness harness, not a production benchmark.

## CI-Safe Smoke

```sh
make load-smoke
```

The smoke target runs deterministic checks for:

- routing plus assignment SLO evaluation: p50 < 100 ms and p99 < 500 ms, excluding outbound execution;
- phase timing consistency (`routing_ms + assignment_ms + egress_ms <= total_ms`);
- expected capacity/backpressure rejection detection;
- ClickHouse request-metadata row-count assertion logic against canonical `straw.request_events`.
- saturated assignment and worker capacity rejection;
- upload/download credit backpressure;
- Redis rate-limit memory guardrails and rate/quota failure policies.

## Local Compose Run

Start compose and mint a tenant requester key as described in `deploy/docker/README.md`, then run:

```sh
STRAW_LOAD_API_KEY=<requester-secret> go run ./cmd/straw-load \
  -base-url http://localhost:8080 \
  -target-url https://example.com/ \
  -requests 50 \
  -concurrency 8 \
  -clickhouse-url http://localhost:8123
```

The command fails if routing plus assignment exceeds the documented SLOs, any request fails unexpectedly, phase timings
overclaim, or live ClickHouse request-metadata rows in canonical `straw.request_events` do not match completed requests
within the async flush wait.

To exercise capacity/backpressure rejection behavior deliberately, run with tighter live worker capacity and add:

```sh
-expect-rejections
```

Do not publish laptop numbers as production capacity.
