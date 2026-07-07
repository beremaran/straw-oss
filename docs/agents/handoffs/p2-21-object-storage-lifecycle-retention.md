# Handoff

Task: `docs/tasks/p2/21-object-storage-lifecycle-retention.md`

## Changed

- `internal/objectstore/objectstore.go`: added signed `PutBucketLifecycleConfiguration` application for BodyRef object retention.
- `cmd/control/main.go`: applies the lifecycle rule when object storage is enabled before wiring the BodyRef store.
- `internal/objectstore/objectstore_test.go`: verifies lifecycle API path, SigV4 auth presence, `tenant/` prefix, configured expiration, and 1-3 day retention bounds.
- `deploy/docker/README.md` and `deploy/production/README.md`: document Control-applied lifecycle retention and operator override/permission requirements.

## Acceptance Criteria Verdicts

| Criterion | Verdict | Implementation (file:line) | Proving test |
|-----------|---------|----------------------------|--------------|
| Orphaned body objects are expired by a bucket lifecycle rule at configured retention. | VERIFIED | `internal/objectstore/objectstore.go:217`, `internal/objectstore/objectstore.go:223`, `internal/objectstore/objectstore.go:225`, `cmd/control/main.go:227` | `TestApplyLifecycleRetention` |
| Retention honored is the configured value, clamped/bounded to 1-3 days. | VERIFIED | `cmd/control/main.go:221`, `internal/objectstore/objectstore.go:74`, `internal/objectstore/objectstore.go:188`, `internal/objectstore/objectstore.go:225` | `TestNewValidation`, `TestApplyLifecycleRetention` |
| Mechanism is documented for both compose and production. | VERIFIED | `deploy/docker/README.md:100`, `deploy/docker/README.md:102`, `deploy/docker/README.md:105`, `deploy/production/README.md:42`, `deploy/production/README.md:43`, `deploy/production/README.md:44` | Documentation inspection |

Independent verifier result: PASS for all acceptance criteria; no unowned deferrals found in the diff.

## Planning-Doc Coverage

| Planning item | Status | Evidence / owning task |
|---------------|--------|------------------------|
| `docs/planning/18` S3 request flow step 9: lifecycle rules clean up orphaned objects. | implemented | `internal/objectstore/objectstore.go:217`, `cmd/control/main.go:227` |
| `docs/planning/18` S3 response flow uses object storage retention policy for stored response objects. | implemented | The same `tenant/` lifecycle rule covers request, response, and payload-capture BodyRef keys: `internal/objectstore/objectstore.go:223`. |
| `docs/planning/18` Retention: default 1 day; operators may configure up to 3 days. | already existed / implemented | Bounds already existed in `internal/objectstore/objectstore.go:74` and `internal/objectstore/objectstore.go:188`; lifecycle XML uses the configured duration at `internal/objectstore/objectstore.go:225`. |
| `docs/planning/21` Redis stores ephemeral runtime state only. | out of scope | This task changes object storage only; no Redis state is introduced. |
| `docs/planning/21` ClickHouse/Postgres storage roles and metadata redaction boundaries. | out of scope | This task does not change metadata, ClickHouse, or Postgres surfaces. |

## Verification

```sh
go test ./internal/objectstore ./cmd/control ./internal/control
make check
```

Result:

- Postgres-backed tests: not exercised; diff does not touch Postgres files, migrations, or Postgres-backed surfaces.
- Live compose verification: skipped because this task applies a bucket-level S3 lifecycle rule and the focused test verifies the signed lifecycle request against an HTTP test server. No request runtime path changed.

## Reviewer Start Points

- `internal/objectstore/objectstore.go`
- `cmd/control/main.go`
- `internal/objectstore/objectstore_test.go`
- `deploy/docker/README.md`
- `deploy/production/README.md`

## Remaining Work

- None.

## Blockers

- None.
