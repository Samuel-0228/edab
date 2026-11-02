-- 000004_distributed_execution.up.sql
-- Add durable job leases, worker locks, and lease expiration indexes for distributed execution

-- Delivery jobs: worker lease and expiration tracking
ALTER TABLE delivery_jobs
    ADD COLUMN IF NOT EXISTS locked_by VARCHAR(255),
    ADD COLUMN IF NOT EXISTS lease_expires_at TIMESTAMPTZ;

CREATE INDEX IF NOT EXISTS idx_delivery_jobs_lease ON delivery_jobs(status, lease_expires_at)
    WHERE status = 'processing';

-- Executions: worker lease and expiration tracking
ALTER TABLE executions
    ADD COLUMN IF NOT EXISTS locked_by VARCHAR(255),
    ADD COLUMN IF NOT EXISTS lease_expires_at TIMESTAMPTZ;

CREATE INDEX IF NOT EXISTS idx_executions_lease ON executions(status, lease_expires_at)
    WHERE status = 'running';
