-- 000003_workflow_engine_v2.down.sql
-- Revert workflow engine v2 enhancements

DROP TABLE IF EXISTS workflow_versions;

ALTER TABLE webhooks
    DROP COLUMN IF EXISTS verifier_type,
    DROP COLUMN IF EXISTS verifier_secret;

ALTER TABLE workflows
    DROP COLUMN IF EXISTS version,
    DROP COLUMN IF EXISTS rate_limit_per_minute,
    DROP COLUMN IF EXISTS max_concurrency,
    DROP COLUMN IF EXISTS concurrency_key_expression;

DROP INDEX IF EXISTS idx_execution_steps_step_id;
ALTER TABLE execution_steps
    DROP COLUMN IF EXISTS step_id;

DROP INDEX IF EXISTS idx_executions_resume_at;
DROP INDEX IF EXISTS idx_executions_concurrency_key;
DROP INDEX IF EXISTS idx_executions_priority_next;

ALTER TABLE executions
    DROP COLUMN IF EXISTS current_step_id,
    DROP COLUMN IF EXISTS step_states,
    DROP COLUMN IF EXISTS priority,
    DROP COLUMN IF EXISTS concurrency_key,
    DROP COLUMN IF EXISTS resume_at;
