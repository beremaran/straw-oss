-- Executor pool degraded-worker policy (docs/implementation-history.md#p0-30, docs/planning/10,
-- docs/planning/26 Executor Pool schema's allow_degraded_workers field).
ALTER TABLE executor_pools ADD COLUMN IF NOT EXISTS allow_degraded_workers boolean NOT NULL DEFAULT false;
