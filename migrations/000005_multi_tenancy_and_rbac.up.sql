-- 000005_multi_tenancy_and_rbac.up.sql

-- 1. Tenants / Organizations
CREATE TABLE IF NOT EXISTS tenants (
    id VARCHAR(36) PRIMARY KEY,
    name VARCHAR(255) NOT NULL,
    slug VARCHAR(255) NOT NULL UNIQUE,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

-- 2. Users
CREATE TABLE IF NOT EXISTS users (
    id VARCHAR(36) PRIMARY KEY,
    email VARCHAR(255) NOT NULL UNIQUE,
    name VARCHAR(255) NOT NULL,
    password_hash TEXT NOT NULL DEFAULT '',
    is_system BOOLEAN NOT NULL DEFAULT false,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

-- 3. Memberships (User <-> Tenant with Role)
CREATE TABLE IF NOT EXISTS memberships (
    id VARCHAR(36) PRIMARY KEY,
    tenant_id VARCHAR(36) NOT NULL REFERENCES tenants(id) ON DELETE CASCADE,
    user_id VARCHAR(36) NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    role VARCHAR(32) NOT NULL DEFAULT 'viewer',
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    CONSTRAINT uq_memberships_tenant_user UNIQUE (tenant_id, user_id)
);

CREATE INDEX IF NOT EXISTS idx_memberships_tenant_id ON memberships(tenant_id);
CREATE INDEX IF NOT EXISTS idx_memberships_user_id ON memberships(user_id);

-- 4. Projects (Sub-division within a Tenant)
CREATE TABLE IF NOT EXISTS projects (
    id VARCHAR(36) PRIMARY KEY,
    tenant_id VARCHAR(36) NOT NULL REFERENCES tenants(id) ON DELETE CASCADE,
    name VARCHAR(255) NOT NULL,
    description TEXT NOT NULL DEFAULT '',
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    CONSTRAINT uq_projects_tenant_name UNIQUE (tenant_id, name)
);

CREATE INDEX IF NOT EXISTS idx_projects_tenant_id ON projects(tenant_id);

-- 5. API Tokens & Service Accounts
CREATE TABLE IF NOT EXISTS api_tokens (
    id VARCHAR(36) PRIMARY KEY,
    tenant_id VARCHAR(36) NOT NULL REFERENCES tenants(id) ON DELETE CASCADE,
    project_id VARCHAR(36) REFERENCES projects(id) ON DELETE SET NULL,
    name VARCHAR(255) NOT NULL,
    token_prefix VARCHAR(32) NOT NULL,
    token_hash VARCHAR(64) NOT NULL UNIQUE,
    role VARCHAR(32) NOT NULL DEFAULT 'operator',
    scopes JSONB NOT NULL DEFAULT '[]'::jsonb,
    last_used_at TIMESTAMPTZ,
    expires_at TIMESTAMPTZ,
    created_by VARCHAR(36) NOT NULL DEFAULT 'system',
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX IF NOT EXISTS idx_api_tokens_tenant_id ON api_tokens(tenant_id);
CREATE INDEX IF NOT EXISTS idx_api_tokens_hash ON api_tokens(token_hash);

-- 6. Audit Logs
CREATE TABLE IF NOT EXISTS audit_logs (
    id VARCHAR(36) PRIMARY KEY,
    tenant_id VARCHAR(36) NOT NULL,
    actor_id VARCHAR(255) NOT NULL,
    actor_type VARCHAR(64) NOT NULL,
    action VARCHAR(128) NOT NULL,
    resource_type VARCHAR(64) NOT NULL,
    resource_id VARCHAR(255) NOT NULL,
    details JSONB NOT NULL DEFAULT '{}'::jsonb,
    ip_address VARCHAR(64) NOT NULL DEFAULT '',
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX IF NOT EXISTS idx_audit_logs_tenant_created ON audit_logs(tenant_id, created_at DESC);
CREATE INDEX IF NOT EXISTS idx_audit_logs_resource ON audit_logs(resource_type, resource_id);
CREATE INDEX IF NOT EXISTS idx_audit_logs_action ON audit_logs(action);

-- 7. Seed Default Tenant, User, Membership, and Project for Single-Tenant Dev Compatibility
INSERT INTO tenants (id, name, slug)
VALUES ('default', 'Default Organization', 'default')
ON CONFLICT (id) DO NOTHING;

INSERT INTO users (id, email, name, is_system)
VALUES ('default-user', 'admin@local', 'Default Admin', true)
ON CONFLICT (id) DO NOTHING;

INSERT INTO memberships (id, tenant_id, user_id, role)
VALUES ('default-membership', 'default', 'default-user', 'owner')
ON CONFLICT (tenant_id, user_id) DO NOTHING;

INSERT INTO projects (id, tenant_id, name, description)
VALUES ('default', 'default', 'Default Project', 'Default single-tenant project')
ON CONFLICT (id) DO NOTHING;

-- 8. Add tenant_id and project_id to existing resource tables
ALTER TABLE workflows
    ADD COLUMN IF NOT EXISTS tenant_id VARCHAR(36) NOT NULL DEFAULT 'default',
    ADD COLUMN IF NOT EXISTS project_id VARCHAR(36);

CREATE INDEX IF NOT EXISTS idx_workflows_tenant_id ON workflows(tenant_id);

ALTER TABLE credentials
    ADD COLUMN IF NOT EXISTS tenant_id VARCHAR(36) NOT NULL DEFAULT 'default',
    ADD COLUMN IF NOT EXISTS project_id VARCHAR(36);

CREATE INDEX IF NOT EXISTS idx_credentials_tenant_id ON credentials(tenant_id);

ALTER TABLE webhooks
    ADD COLUMN IF NOT EXISTS tenant_id VARCHAR(36) NOT NULL DEFAULT 'default',
    ADD COLUMN IF NOT EXISTS project_id VARCHAR(36);

CREATE INDEX IF NOT EXISTS idx_webhooks_tenant_id ON webhooks(tenant_id);

ALTER TABLE events
    ADD COLUMN IF NOT EXISTS tenant_id VARCHAR(36) NOT NULL DEFAULT 'default',
    ADD COLUMN IF NOT EXISTS project_id VARCHAR(36);

CREATE INDEX IF NOT EXISTS idx_events_tenant_id ON events(tenant_id);

ALTER TABLE executions
    ADD COLUMN IF NOT EXISTS tenant_id VARCHAR(36) NOT NULL DEFAULT 'default',
    ADD COLUMN IF NOT EXISTS project_id VARCHAR(36);

CREATE INDEX IF NOT EXISTS idx_executions_tenant_id ON executions(tenant_id);

ALTER TABLE dead_letter
    ADD COLUMN IF NOT EXISTS tenant_id VARCHAR(36) NOT NULL DEFAULT 'default';

CREATE INDEX IF NOT EXISTS idx_dead_letter_tenant_id ON dead_letter(tenant_id);

ALTER TABLE sources
    ADD COLUMN IF NOT EXISTS tenant_id VARCHAR(36) NOT NULL DEFAULT 'default';

CREATE INDEX IF NOT EXISTS idx_sources_tenant_id ON sources(tenant_id);

ALTER TABLE destinations
    ADD COLUMN IF NOT EXISTS tenant_id VARCHAR(36) NOT NULL DEFAULT 'default';

CREATE INDEX IF NOT EXISTS idx_destinations_tenant_id ON destinations(tenant_id);

ALTER TABLE routes
    ADD COLUMN IF NOT EXISTS tenant_id VARCHAR(36) NOT NULL DEFAULT 'default';

CREATE INDEX IF NOT EXISTS idx_routes_tenant_id ON routes(tenant_id);

ALTER TABLE delivery_jobs
    ADD COLUMN IF NOT EXISTS tenant_id VARCHAR(36) NOT NULL DEFAULT 'default';

CREATE INDEX IF NOT EXISTS idx_delivery_jobs_tenant_id ON delivery_jobs(tenant_id);
