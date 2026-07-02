# Postgres Migrations

P0 durable control-plane migrations live here. Task 04 owns the first schema.

Rerun behavior is guarded by idempotent `IF NOT EXISTS` DDL and the local check script re-applies the full migration set twice against a clean database.

Local verification:

```sh
./scripts/check-postgres-migrations.sh
```
