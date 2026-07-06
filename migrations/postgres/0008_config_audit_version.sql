ALTER TABLE config_audit_source
    ADD COLUMN IF NOT EXISTS config_version bigint;

CREATE INDEX IF NOT EXISTS config_audit_source_tenant_version_idx
    ON config_audit_source (tenant_id, config_version, id)
    WHERE config_version IS NOT NULL;
