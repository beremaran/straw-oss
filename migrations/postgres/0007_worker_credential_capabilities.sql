-- Worker credential allowed_capabilities from docs/planning/26.
ALTER TABLE worker_credentials
    ADD COLUMN IF NOT EXISTS allowed_capabilities_jsonb jsonb NOT NULL DEFAULT '{}'::jsonb;
