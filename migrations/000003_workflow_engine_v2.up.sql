-- 000003_workflow_engine_v2.up.sql
-- Add multi-step data flow, execution step tracking, priority, concurrency, and workflow versioning

-- Executions enhancements: step tracking, priority, concurrency key, and async resume
ALTER TABLE executions
    ADD COLUMN IF NOT EXISTS current_step_id VARCHAR(64),
    ADD COLUMN IF NOT EXISTS step_states JSONB NOT NULL DEFAULT '{}'::jsonb,
    ADD COLUMN IF NOT EXISTS priority INT NOT NULL DEFAULT 0,
    ADD COLUMN IF NOT EXISTS concurrency_key VARCHAR(255),
    ADD COLUMN IF NOT EXISTS resume_at TIMESTAMPTZ;

CREATE INDEX IF NOT EXISTS idx_executions_priority_next ON executions(priority DESC, next_attempt_at ASC);
CREATE INDEX IF NOT EXISTS idx_executions_concurrency_key ON executions(concurrency_key) WHERE concurrency_key IS NOT NULL;
CREATE INDEX IF NOT EXISTS idx_executions_resume_at ON executions(resume_at) WHERE resume_at IS NOT NULL;

-- Execution steps enhancements: logical step identifier
ALTER TABLE execution_steps
    ADD COLUMN IF NOT EXISTS step_id VARCHAR(64);

CREATE INDEX IF NOT EXISTS idx_execution_steps_step_id ON execution_steps(execution_id, step_id);

-- Workflows enhancements: versioning, rate limiting, and concurrency controls
ALTER TABLE workflows
    ADD COLUMN IF NOT EXISTS version INT NOT NULL DEFAULT 1,
    ADD COLUMN IF NOT EXISTS rate_limit_per_minute INT NOT NULL DEFAULT 0,
    ADD COLUMN IF NOT EXISTS max_concurrency INT NOT NULL DEFAULT 0,
    ADD COLUMN IF NOT EXISTS concurrency_key_expression TEXT NOT NULL DEFAULT '';

-- Workflow Versions: audit trail and rollback history for workflows
CREATE TABLE IF NOT EXISTS workflow_versions (
    id VARCHAR(36) PRIMARY KEY,
    workflow_id VARCHAR(36) NOT NULL REFERENCES workflows(id) ON DELETE CASCADE,
    version INT NOT NULL,
    definition JSONB NOT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    CONSTRAINT uq_workflow_versions_wf_ver UNIQUE (workflow_id, version)
);

CREATE INDEX IF NOT EXISTS idx_workflow_versions_workflow_id ON workflow_versions(workflow_id);

-- Webhooks enhancements: signature verifiers
ALTER TABLE webhooks
    ADD COLUMN IF NOT EXISTS verifier_type VARCHAR(64) NOT NULL DEFAULT 'bearer',
    ADD COLUMN IF NOT EXISTS verifier_secret VARCHAR(255) NOT NULL DEFAULT '';
