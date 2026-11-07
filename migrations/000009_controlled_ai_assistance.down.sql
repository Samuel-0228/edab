-- 000009_controlled_ai_assistance.down.sql

DROP INDEX IF EXISTS idx_ai_proposals_status;
DROP INDEX IF EXISTS idx_ai_proposals_tenant;
DROP TABLE IF EXISTS ai_proposals;
