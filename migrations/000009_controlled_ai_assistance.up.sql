-- 000009_controlled_ai_assistance.up.sql

CREATE TABLE IF NOT EXISTS ai_proposals (
    id VARCHAR(64) PRIMARY KEY,
    tenant_id VARCHAR(64) NOT NULL,
    task VARCHAR(32) NOT NULL,
    status VARCHAR(32) NOT NULL DEFAULT 'pending_approval',
    prompt TEXT NOT NULL,
    explanation TEXT NOT NULL DEFAULT '',
    content JSONB NOT NULL DEFAULT '{}'::jsonb,
    validation JSONB NOT NULL DEFAULT '{}'::jsonb,
    model_used VARCHAR(64) NOT NULL DEFAULT '',
    tokens_used INT NOT NULL DEFAULT 0,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    decided_at TIMESTAMPTZ,
    decided_by VARCHAR(64)
);

CREATE INDEX IF NOT EXISTS idx_ai_proposals_tenant ON ai_proposals (tenant_id, created_at DESC);
CREATE INDEX IF NOT EXISTS idx_ai_proposals_status ON ai_proposals (status);
