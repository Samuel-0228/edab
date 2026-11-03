-- 000008_governance_and_policy_engine.up.sql

-- 1. Governance Policies Table
CREATE TABLE IF NOT EXISTS governance_policies (
    tenant_id VARCHAR(64) PRIMARY KEY,
    allowed_connectors JSONB NOT NULL DEFAULT '[]'::jsonb,
    allowed_outbound_domains JSONB NOT NULL DEFAULT '[]'::jsonb,
    max_workflow_retries INT NOT NULL DEFAULT 5,
    max_payload_size_bytes BIGINT NOT NULL DEFAULT 1048576, -- 1MB default
    data_retention_days INT NOT NULL DEFAULT 90,
    require_approval_irreversible BOOLEAN NOT NULL DEFAULT false,
    mask_pii BOOLEAN NOT NULL DEFAULT true,
    mask_secrets BOOLEAN NOT NULL DEFAULT true,
    blocked_keywords JSONB NOT NULL DEFAULT '[]'::jsonb,
    allowed_environments JSONB NOT NULL DEFAULT '[]'::jsonb,
    enforcement_mode VARCHAR(32) NOT NULL DEFAULT 'enforce', -- 'enforce' or 'audit'
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX IF NOT EXISTS idx_governance_policies_mode ON governance_policies (enforcement_mode);

-- 2. Enhance Audit Logs with Cryptographic Chaining & Tamper-Evidence
ALTER TABLE audit_logs ADD COLUMN IF NOT EXISTS sequence_num BIGINT DEFAULT 0;
ALTER TABLE audit_logs ADD COLUMN IF NOT EXISTS prev_hash VARCHAR(64) DEFAULT '';
ALTER TABLE audit_logs ADD COLUMN IF NOT EXISTS entry_hash VARCHAR(64) DEFAULT '';

CREATE INDEX IF NOT EXISTS idx_audit_logs_chain ON audit_logs (tenant_id, sequence_num ASC);
