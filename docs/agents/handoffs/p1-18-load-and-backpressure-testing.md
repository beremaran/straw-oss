# Handoff

Task: `docs/tasks/p1/18-load-and-backpressure-testing.md`

## Changed

- `internal/loadtest/loadtest.go` — added the reusable load harness runner, SLO/phase/rejection checks, and opt-in
  ClickHouse request-metadata row assertion against canonical `straw.request_events`.
- `internal/loadtest/loadtest_test.go` — added CI-safe checks for SLO failures, expected rejections, phase timing
  consistency, and ClickHouse row-count query construction.
- `cmd/straw-load/main.go` — added a local CLI for concurrent REST request runs and optional ClickHouse row-count
  assertion.
- `Makefile` — added `make load-smoke`, including the new harness checks plus focused existing tests for Redis
  guardrails/failure policies, worker capacity, upload/download credit, and NATS stream validation.
- `docs/testing/load-and-backpressure.md` — documented CI-safe and local compose commands and the no-production-
  benchmark boundary.
- `docs/agents/handoffs/25-p0-test-matrix-and-compose.md` — closed the stale live ClickHouse load verification note.
- `docs/tasks/p1.md`, `docs/tasks/p1/18-load-and-backpressure-testing.md` — marked task 18 complete after verification.

## Acceptance Criteria Verdicts

Filled from the independent verifier's report (straw-task-runner workflow step 12), not from the implementer's
self-assessment.

| Criterion | Verdict | Implementation (file:line) | Proving test |
|-----------|---------|----------------------------|--------------|
| Reproducible load harness exists with CI-safe and local modes. | VERIFIED | `Makefile:13`, `cmd/straw-load/main.go:41`, `docs/testing/load-and-backpressure.md:5`, `docs/testing/load-and-backpressure.md:21` | `make load-smoke`; `make check` |
| SLOs and memory/backpressure guardrails are checked. | VERIFIED | `internal/loadtest/loadtest.go:18`, `internal/loadtest/loadtest.go:131`, `internal/loadtest/loadtest.go:249`, `Makefile:15`, `Makefile:16` | `make load-smoke` |
| Local mode proves rows land in live ClickHouse under load, closing handoff-25 flag. | VERIFIED | `cmd/straw-load/main.go:52`, `cmd/straw-load/main.go:74`, `internal/loadtest/loadtest.go:187`, `internal/loadtest/loadtest.go:218`, `internal/loadtest/loadtest_test.go:72` | Live `straw-load` run: 20 completed, 20 request-metadata rows |
| Results do not overclaim production capacity. | VERIFIED | `docs/testing/load-and-backpressure.md:3`, `docs/testing/load-and-backpressure.md:44`, `internal/loadtest/loadtest_test.go:36` | `make load-smoke`; `make check` |

## Planning-Doc Coverage

Every in-phase field/endpoint/behavior from the task's cited planning-doc sections, accounted for:

| Planning item | Status | Evidence / owning task |
|---------------|--------|------------------------|
| Control routing/coordination p50 < 100 ms and p99 < 500 ms, excluding outbound execution (`docs/planning/23`). | implemented | `internal/loadtest/loadtest.go:18`, `internal/loadtest/loadtest.go:273`; live run p50 1ms / p99 14ms |
| Results must not claim sub-millisecond routing or laptop runs as production guarantees (`docs/planning/23`, `docs/planning/30`). | implemented | `docs/testing/load-and-backpressure.md:3`, `docs/testing/load-and-backpressure.md:44` |
| Redis unavailable policies for rate limits and quotas (`docs/planning/20`, `docs/planning/29`). | already existed, now smoke-covered | `Makefile:15`; `internal/control/ratelimit_test.go:127`; `internal/control/quota_admission_test.go:94` |
| Rate-limit memory guardrails (`docs/planning/20`). | already existed, now smoke-covered | `Makefile:15`; `internal/control/ratelimit_test.go:81` |
| NATS byte-credit flow control and stream sequencing (`docs/planning/12`). | already existed, now smoke-covered | `Makefile:16`, `Makefile:17`; `internal/egress/loop_test.go:154`; `internal/egress/loop_test.go:184`; `internal/natsx/stream_test.go:44` |
| Active request limit and worker capacity behavior (`docs/planning/30`). | already existed, now smoke-covered | `Makefile:16`; `internal/egress/assignment_test.go:9`; `internal/egress/loop_test.go:69` |
| Canonical ClickHouse request metadata lands in `straw.request_events` (`docs/planning/22`, `docs/planning/30`). | implemented | `internal/loadtest/loadtest.go:200`, `internal/loadtest/loadtest.go:218`; live run asserted 20 rows |
| ClickHouse outage does not block request transport (`docs/planning/29`, `docs/planning/22`). | already existed | `internal/control/request_metadata.go:68`; `internal/control/telemetry_events.go:28` |
| Billing-grade quota reconciliation is out of scope (`docs/planning/20`). | out of scope | Excluded by `docs/tasks/p1/18-load-and-backpressure-testing.md`; no new billing reconciliation added |

## Verification

```sh
make load-smoke
make check
```

Result:

- `make load-smoke`: passed.
- `make check`: passed.
- Postgres-backed tests: not exercised with `STRAW_TEST_POSTGRES_DSN`; diff does not touch Postgres-backed files or
  migrations.
- Live compose verification: request path driven through local compose with a manual Redis container on the
  `straw_default` network alias `redis` because an unrelated `financetracker-redis-1` already owned host port 6379.
  Command:

```sh
go run ./cmd/straw-load \
  -base-url http://localhost:8080 \
  -target-url https://example.com/ \
  -requests 20 \
  -concurrency 4 \
  -clickhouse-url http://localhost:8123 \
  -clickhouse-wait 3s
```

Live result:

```text
requests=20 completed=20 failures=0 elapsed=642ms coordination_p50=1ms coordination_p99=14ms
clickhouse_request_metadata_rows=20
```

Hardware/runtime assumptions: local Docker Desktop on this machine, 20 requests, concurrency 4, upstream
`https://example.com/`, no production capacity claim.

## Reviewer Start Points

- `internal/loadtest/loadtest.go`
- `cmd/straw-load/main.go`
- `docs/testing/load-and-backpressure.md`
- `Makefile`

## Remaining Work

- None.

## Blockers

- None.
