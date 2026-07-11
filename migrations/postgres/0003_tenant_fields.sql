-- straw P0 tenant lifecycle fields (docs/implementation-history.md#p0-29).
--
-- Vocabulary reconciliation with docs/planning/26-config-management-api-surface.md:
-- the tenant status enum is `active | suspended | deleted` (migration 0001 used
-- `disabled` instead of `suspended`), and the shared soft-delete contract sets
-- `deleted_at` (migration 0001 used `soft_deleted_at`).

-- Migrate any legacy 'disabled' rows before swapping the CHECK constraint.
UPDATE tenants SET status = 'suspended' WHERE status = 'disabled';

ALTER TABLE tenants DROP CONSTRAINT IF EXISTS tenants_status_check;
ALTER TABLE tenants ADD CONSTRAINT tenants_status_check CHECK (status IN ('active', 'suspended', 'deleted'));

DO $$
BEGIN
    IF EXISTS (
        SELECT 1 FROM information_schema.columns
        WHERE table_name = 'tenants' AND column_name = 'soft_deleted_at'
    ) THEN
        ALTER TABLE tenants RENAME COLUMN soft_deleted_at TO deleted_at;
    END IF;
END $$;

ALTER TABLE tenants ADD COLUMN IF NOT EXISTS name text NOT NULL DEFAULT '';
ALTER TABLE tenants ALTER COLUMN name DROP DEFAULT;
ALTER TABLE tenants ADD COLUMN IF NOT EXISTS rate_limit_ceiling_window_seconds integer;
ALTER TABLE tenants ADD COLUMN IF NOT EXISTS rate_limit_ceiling_max_requests integer;
ALTER TABLE tenants ADD COLUMN IF NOT EXISTS config_version bigint NOT NULL DEFAULT 0;
