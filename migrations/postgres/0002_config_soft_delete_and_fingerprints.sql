-- straw P0 config store follow-up: soft-delete columns for tenant-scoped
-- config resources and seeded built-in global fingerprint profiles.

-- Soft deletion (docs/planning/25): deleted resources stay in the table for
-- audit/version history but are excluded from assembled tenant snapshots.
ALTER TABLE routing_rules ADD COLUMN IF NOT EXISTS deleted_at timestamptz;
ALTER TABLE executor_pools ADD COLUMN IF NOT EXISTS deleted_at timestamptz;
ALTER TABLE deny_rules ADD COLUMN IF NOT EXISTS deleted_at timestamptz;
ALTER TABLE injection_policies ADD COLUMN IF NOT EXISTS deleted_at timestamptz;

-- Seeded built-in global fingerprint profiles (docs/planning/21, 26): P0 rows
-- are seeded built-ins only with no write API. Idempotent via NOT EXISTS so the
-- migration set can be re-applied against a clean or existing database.
INSERT INTO fingerprint_profiles (tenant_id, name, scope_type, supported_by_worker, enabled, profile_jsonb)
SELECT NULL, v.name, 'global', true, true, jsonb_build_object('profile_ref', 'builtin:' || v.name)
FROM (VALUES ('default'), ('chrome_120'), ('firefox_121'), ('safari_17')) AS v(name)
WHERE NOT EXISTS (
    SELECT 1 FROM fingerprint_profiles fp
    WHERE fp.scope_type = 'global' AND fp.name = v.name
);
