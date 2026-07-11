-- straw P0 tenant canonical timeout and metadata-storage fields (docs/implementation-history.md#p0-46).

ALTER TABLE tenants ADD COLUMN IF NOT EXISTS default_timeout_ms bigint NOT NULL DEFAULT 60000;
ALTER TABLE tenants ADD COLUMN IF NOT EXISTS max_timeout_ms bigint NOT NULL DEFAULT 300000;
ALTER TABLE tenants ADD COLUMN IF NOT EXISTS metadata_query_storage text NOT NULL DEFAULT 'drop';
ALTER TABLE tenants ADD COLUMN IF NOT EXISTS metadata_path_storage text NOT NULL DEFAULT 'hash';

ALTER TABLE tenants DROP CONSTRAINT IF EXISTS tenants_timeout_bounds_check;
ALTER TABLE tenants ADD CONSTRAINT tenants_timeout_bounds_check
    CHECK (default_timeout_ms >= 1000 AND max_timeout_ms >= 1000 AND default_timeout_ms <= max_timeout_ms);

ALTER TABLE tenants DROP CONSTRAINT IF EXISTS tenants_metadata_query_storage_check;
ALTER TABLE tenants ADD CONSTRAINT tenants_metadata_query_storage_check
    CHECK (metadata_query_storage IN ('drop', 'hash', 'store'));

ALTER TABLE tenants DROP CONSTRAINT IF EXISTS tenants_metadata_path_storage_check;
ALTER TABLE tenants ADD CONSTRAINT tenants_metadata_path_storage_check
    CHECK (metadata_path_storage IN ('store', 'hash', 'drop'));
