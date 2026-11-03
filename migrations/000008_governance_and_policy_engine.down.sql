-- 000008_governance_and_policy_engine.down.sql

DROP INDEX IF EXISTS idx_governance_policies_mode;
DROP TABLE IF EXISTS governance_policies;

DROP INDEX IF EXISTS idx_audit_logs_chain;
ALTER TABLE audit_logs DROP COLUMN IF EXISTS sequence_num;
ALTER TABLE audit_logs DROP COLUMN IF EXISTS prev_hash;
ALTER TABLE audit_logs DROP COLUMN IF EXISTS entry_hash;
