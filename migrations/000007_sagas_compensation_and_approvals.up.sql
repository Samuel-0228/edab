-- 000007_sagas_compensation_and_approvals.up.sql
-- Add workflow approvals table for durable human approvals with token security, timeouts, and escalation

CREATE TABLE IF NOT EXISTS workflow_approvals (
    id VARCHAR(36) PRIMARY KEY,
    tenant_id VARCHAR(36) NOT NULL DEFAULT 'default',
    execution_id VARCHAR(36) NOT NULL REFERENCES executions(id) ON DELETE CASCADE,
    step_id VARCHAR(255) NOT NULL,
    workflow_id VARCHAR(36) NOT NULL,
    title VARCHAR(255) NOT NULL,
    description TEXT NOT NULL DEFAULT '',
    token VARCHAR(64) NOT NULL UNIQUE,
    token_hash VARCHAR(64) NOT NULL UNIQUE,
    status VARCHAR(32) NOT NULL DEFAULT 'pending',
    approver_roles JSONB NOT NULL DEFAULT '[]'::jsonb,
    approver_users JSONB NOT NULL DEFAULT '[]'::jsonb,
    approved_by VARCHAR(255),
    decision_note TEXT,
    expires_at TIMESTAMPTZ NOT NULL,
    timeout_action VARCHAR(32) NOT NULL DEFAULT 'reject',
    escalation_roles JSONB NOT NULL DEFAULT '[]'::jsonb,
    escalation_contact VARCHAR(255) NOT NULL DEFAULT '',
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    CONSTRAINT uq_workflow_approvals_exec_step UNIQUE (execution_id, step_id)
);

CREATE INDEX IF NOT EXISTS idx_workflow_approvals_tenant ON workflow_approvals(tenant_id);
CREATE INDEX IF NOT EXISTS idx_workflow_approvals_status_expires ON workflow_approvals(status, expires_at);
CREATE INDEX IF NOT EXISTS idx_workflow_approvals_token ON workflow_approvals(token);
CREATE INDEX IF NOT EXISTS idx_workflow_approvals_token_hash ON workflow_approvals(token_hash);
CREATE INDEX IF NOT EXISTS idx_workflow_approvals_execution ON workflow_approvals(execution_id);
