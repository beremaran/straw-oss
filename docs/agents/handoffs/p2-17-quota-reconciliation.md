# Handoff

Task: `docs/tasks/p2/17-quota-reconciliation.md`

## Changed

- Added billing-grade quota reconciliation over canonical ClickHouse `request_events`, with request-count idempotency by `request_id`, bandwidth idempotency by `request_id:attempt`, and deterministic max-byte handling for conflicting duplicate attempt rows.
- Added durable Postgres `quota_usage_snapshots` and a `QuotaUsageSnapshotStore`; Control starts a 15-minute reconciliation loop when ClickHouse is configured and exposes the persisted current-period snapshot on `GET /api/v1/config/quotas`.
- Added quota config listing for reconciliation, focused reconciliation tests, admin display tests, and a Postgres-backed snapshot round-trip test.

## Acceptance Criteria Verdicts

Filled from the independent verifier's report (fresh agent `019f3c35-f2aa-7291-950f-e16e62b58c9f`), not from the implementer's self-assessment.

| Criterion | Verdict | Implementation (file:line) | Proving test |
|-----------|---------|----------------------------|--------------|
| Reconciliation matches the resolved accuracy decision. | VERIFIED | `internal/control/quota_reconciliation.go:17`, `internal/control/quota_reconciliation.go:107`, `internal/control/quota_reconciliation.go:120`, `internal/control/quota_usage_store.go:21`, `cmd/control/main.go:1112` | `TestQuotaReconciliationAggregatesIdempotently`, `TestQuotaReconciliationJobReconcilesRecentPeriods`, `TestPostgresQuotaUsageStoreRoundTrip` |
| Request-count quota still counts external requests, not fallback attempts. | VERIFIED | `internal/control/quota_reconciliation.go:206`, `internal/control/quota_reconciliation.go:232` | `TestQuotaReconciliationAggregatesIdempotently`, `TestQuotaReconciliationPreservesCountOnSuccess` |
| Late and duplicate events are handled deterministically. | VERIFIED | `internal/control/quota_reconciliation.go:141`, `internal/control/quota_reconciliation.go:210`, `internal/control/quota_reconciliation.go:215` | `TestQuotaReconciliationHandlesLateEventsByRecomputingPeriod`, `TestQuotaReconciliationAggregatesIdempotently`, `TestQuotaReconciliationJobReconcilesRecentPeriods` |

## Planning-Doc Coverage

Every in-phase field/endpoint/behavior from the task's cited planning-doc sections, accounted for:

| Planning item | Status | Evidence / owning task |
|---------------|--------|------------------------|
| Durable usage-event source from ClickHouse `request_events`. | implemented | `internal/control/quota_reconciliation.go:29`, `internal/control/quota_reconciliation.go:278` |
| Aggregation cadence. | implemented | `internal/control/quota_reconciliation.go:18`, `internal/control/quota_reconciliation.go:155`, `cmd/control/main.go:1112` |
| Idempotency key. | implemented | `internal/control/quota_reconciliation.go:19`, `internal/control/quota_reconciliation.go:206`, `internal/control/quota_reconciliation.go:210`, `internal/control/quota_reconciliation.go:215` |
| Late-arriving event handling. | implemented | `internal/control/quota_reconciliation.go:21`, `internal/control/quota_reconciliation.go:141`, `internal/control/quota_reconciliation_test.go:98` |
| Correction policy for Redis hot counters. | implemented | `internal/control/quota_reconciliation.go:172`, `internal/control/quota_reconciliation_test.go:136` |
| User-visible quota display semantics. | implemented | `internal/control/quota_reconciliation.go:45`, `internal/control/admin_handlers.go:890`, `internal/control/admin_handlers.go:940`, `internal/control/admin_handlers_test.go:780` |
| Durable reconciled usage snapshot for invoice/audit view. | implemented | `migrations/postgres/0001_init.sql:197`, `internal/control/quota_usage_store.go:21`, `internal/control/postgres_store_test.go:162` |
| Request-count quota counts external requests, not fallback attempts. | implemented | `internal/control/quota_reconciliation.go:206`, `internal/control/quota_reconciliation_test.go:78` |
| Bandwidth quota counts bytes per attempt. | implemented | `internal/control/quota_reconciliation.go:210`, `internal/control/quota_reconciliation.go:222`, `internal/control/quota_reconciliation_test.go:81` |
| Redis-only admission behavior remains request-path hot-counter based. | already existed / preserved | `internal/control/quota_admission_test.go:116` |

## Verification

```sh
go test ./internal/control ./cmd/control -run 'TestQuota|TestOpenRedis'
make check
make postgres-migrations-check
STRAW_TEST_POSTGRES_DSN='postgres://postgres:postgres@localhost:5432/straw_test?sslmode=disable' go test ./...
```

Result:

- Focused tests: passed.
- `make check`: passed.
- Postgres migration sanity: passed.
- Postgres-backed tests: ran against dedicated `straw_test` and passed.
- Live compose verification: skipped because this task changes reconciliation/admin quota display and durable stores, not the runtime request transport path.

## Reviewer Start Points

- `internal/control/quota_reconciliation.go`
- `internal/control/quota_usage_store.go`
- `internal/control/admin_handlers.go`
- `cmd/control/main.go`
- `migrations/postgres/0001_init.sql`

## Remaining Work

- None.

## Blockers

- None.
