# Handoff

Task: `docs/tasks/p0/04-postgres-schema.md`

## Changed

- Added `migrations/postgres/0001_init.sql` with the initial P0 Postgres schema for tenants, API keys, worker credentials, executor pools, worker admin state, tenant worker admin state, routing rules, deny rules, fingerprint profiles, injection policies, rate limits, quotas, tenant config versions, and audit source records.
- Added `scripts/check-postgres-migrations.sh` to apply the migration set against a clean local Postgres database and rerun it to guard idempotent behavior.
- Added `postgres-migrations-check` to the `Makefile` for a one-command local migration check.
- Updated `migrations/postgres/README.md` with the local verification command and rerun note.

## Verification

```sh
./scripts/check-postgres-migrations.sh
make check
```

Result:

- Both commands passed.

## Reviewer Start Points

- `/Users/beremaran/projects/straw/migrations/postgres/0001_init.sql`
- `/Users/beremaran/projects/straw/scripts/check-postgres-migrations.sh`

## Remaining Work

- None.

## Blockers

- None.

## Notes

- Exact migration command: `./scripts/check-postgres-migrations.sh`
- Application code still needs to manage `id`, `config_version`, and `updated_at` fields on writes.
- `api_keys` stores only `secret_hash`; there is no plaintext secret column in the schema.
