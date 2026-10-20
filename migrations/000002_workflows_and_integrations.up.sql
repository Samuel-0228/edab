-- Credentials: safe storage of integration credentials with encryption at rest
CREATE TABLE IF NOT EXISTS credentials (
    id VARCHAR(36) PRIMARY KEY,
    name VARCHAR(255) NOT NULL,
    integration VARCHAR(64) NOT NULL,
    encrypted_data BYTEA NOT NULL,
    nonce BYTEA NOT NULL,
    key_id VARCHAR(64) NOT NULL DEFAULT 'default',
    masked_fields JSONB NOT NULL DEFAULT '{}'::jsonb,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX IF NOT EXISTS idx_credentials_integration ON credentials(integration);

-- Workflows: declarative automation pipelines
CREATE TABLE IF NOT EXISTS workflows (
    id VARCHAR(36) PRIMARY KEY,
    name VARCHAR(255) NOT NULL,
    description TEXT NOT NULL DEFAULT '',
    enabled BOOLEAN NOT NULL DEFAULT true,
    trigger_type VARCHAR(64) NOT NULL,
    trigger_config JSONB NOT NULL DEFAULT '{}'::jsonb,
    conditions JSONB NOT NULL DEFAULT '{}'::jsonb,
    transformations JSONB NOT NULL DEFAULT '{}'::jsonb,
    actions JSONB NOT NULL DEFAULT '[]'::jsonb,
    max_retries INT NOT NULL DEFAULT 3,
    timeout_seconds INT NOT NULL DEFAULT 30,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX IF NOT EXISTS idx_workflows_enabled ON workflows(enabled);
CREATE INDEX IF NOT EXISTS idx_workflows_trigger_type ON workflows(trigger_type);

-- Executions: history and lifecycle of workflow execution runs
CREATE TABLE IF NOT EXISTS executions (
    id VARCHAR(36) PRIMARY KEY,
    workflow_id VARCHAR(36) NOT NULL REFERENCES workflows(id) ON DELETE CASCADE,
    event_id VARCHAR(36) NOT NULL REFERENCES events(id) ON DELETE CASCADE,
    status VARCHAR(32) NOT NULL DEFAULT 'pending',
    attempts INT NOT NULL DEFAULT 0,
    max_attempts INT NOT NULL DEFAULT 3,
    next_attempt_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    started_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    completed_at TIMESTAMPTZ,
    duration_ms BIGINT NOT NULL DEFAULT 0,
    error TEXT,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    CONSTRAINT uq_executions_workflow_event UNIQUE (workflow_id, event_id)
);

CREATE INDEX IF NOT EXISTS idx_executions_status_next ON executions(status, next_attempt_at);
CREATE INDEX IF NOT EXISTS idx_executions_workflow_id ON executions(workflow_id);
CREATE INDEX IF NOT EXISTS idx_executions_event_id ON executions(event_id);
CREATE INDEX IF NOT EXISTS idx_executions_created_at ON executions(created_at DESC);

-- Execution Steps: fine-grained audit trail of each stage (trigger, condition, transform, action)
CREATE TABLE IF NOT EXISTS execution_steps (
    id VARCHAR(36) PRIMARY KEY,
    execution_id VARCHAR(36) NOT NULL REFERENCES executions(id) ON DELETE CASCADE,
    step_type VARCHAR(32) NOT NULL,
    name VARCHAR(255) NOT NULL,
    status VARCHAR(32) NOT NULL,
    input JSONB,
    output JSONB,
    error TEXT,
    duration_ms BIGINT NOT NULL DEFAULT 0,
    started_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    completed_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX IF NOT EXISTS idx_execution_steps_exec_id ON execution_steps(execution_id);

-- Dead Letter: failed or unrecoverable workflow executions
CREATE TABLE IF NOT EXISTS dead_letter (
    id VARCHAR(36) PRIMARY KEY,
    execution_id VARCHAR(36) NOT NULL REFERENCES executions(id) ON DELETE CASCADE,
    workflow_id VARCHAR(36) NOT NULL REFERENCES workflows(id) ON DELETE CASCADE,
    event_id VARCHAR(36) NOT NULL REFERENCES events(id) ON DELETE CASCADE,
    action_id VARCHAR(64),
    error TEXT NOT NULL,
    payload JSONB,
    attempts INT NOT NULL DEFAULT 0,
    discarded BOOLEAN NOT NULL DEFAULT false,
    failed_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX IF NOT EXISTS idx_dead_letter_workflow_id ON dead_letter(workflow_id);
CREATE INDEX IF NOT EXISTS idx_dead_letter_discarded ON dead_letter(discarded);
CREATE INDEX IF NOT EXISTS idx_dead_letter_failed_at ON dead_letter(failed_at DESC);

-- Webhooks: generic inbound webhook endpoints with verification tokens
CREATE TABLE IF NOT EXISTS webhooks (
    id VARCHAR(36) PRIMARY KEY,
    name VARCHAR(255) NOT NULL,
    secret_token VARCHAR(255) NOT NULL DEFAULT '',
    rate_limit_per_minute INT NOT NULL DEFAULT 60,
    enabled BOOLEAN NOT NULL DEFAULT true,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX IF NOT EXISTS idx_webhooks_enabled ON webhooks(enabled);
