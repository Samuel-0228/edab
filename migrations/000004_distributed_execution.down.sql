-- 000004_distributed_execution.down.sql
-- Revert distributed execution columns and indexes

DROP INDEX IF EXISTS idx_executions_lease;
ALTER TABLE executions
    DROP COLUMN IF EXISTS lease_expires_at,
    DROP COLUMN IF EXISTS locked_by;

DROP INDEX IF EXISTS idx_delivery_jobs_lease;
ALTER TABLE delivery_jobs
    DROP COLUMN IF EXISTS lease_expires_at,
    DROP COLUMN IF EXISTS locked_by;
