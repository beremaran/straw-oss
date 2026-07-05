-- Executor pool P0 capability restrictions (docs/planning/26 Executor Pool
-- schema's allowed_ip_types/allowed_countries/allowed_regions fields,
-- docs/tasks/p0/42). Empty array = unrestricted; existing rows keep current
-- (unrestricted) behavior.
ALTER TABLE executor_pools ADD COLUMN IF NOT EXISTS allowed_ip_types_jsonb jsonb NOT NULL DEFAULT '[]'::jsonb;
ALTER TABLE executor_pools ADD COLUMN IF NOT EXISTS allowed_countries_jsonb jsonb NOT NULL DEFAULT '[]'::jsonb;
ALTER TABLE executor_pools ADD COLUMN IF NOT EXISTS allowed_regions_jsonb jsonb NOT NULL DEFAULT '[]'::jsonb;
