-- 000005_multi_tenancy_and_rbac.down.sql

ALTER TABLE delivery_jobs DROP COLUMN IF EXISTS tenant_id;
ALTER TABLE routes DROP COLUMN IF EXISTS tenant_id;
ALTER TABLE destinations DROP COLUMN IF EXISTS tenant_id;
ALTER TABLE sources DROP COLUMN IF EXISTS tenant_id;
ALTER TABLE dead_letter DROP COLUMN IF EXISTS tenant_id;
ALTER TABLE executions DROP COLUMN IF EXISTS project_id, DROP COLUMN IF EXISTS tenant_id;
ALTER TABLE events DROP COLUMN IF EXISTS project_id, DROP COLUMN IF EXISTS tenant_id;
ALTER TABLE webhooks DROP COLUMN IF EXISTS project_id, DROP COLUMN IF EXISTS tenant_id;
ALTER TABLE credentials DROP COLUMN IF EXISTS project_id, DROP COLUMN IF EXISTS tenant_id;
ALTER TABLE workflows DROP COLUMN IF EXISTS project_id, DROP COLUMN IF EXISTS tenant_id;

DROP TABLE IF EXISTS audit_logs;
DROP TABLE IF EXISTS api_tokens;
DROP TABLE IF EXISTS projects;
DROP TABLE IF EXISTS memberships;
DROP TABLE IF EXISTS users;
DROP TABLE IF EXISTS tenants;
