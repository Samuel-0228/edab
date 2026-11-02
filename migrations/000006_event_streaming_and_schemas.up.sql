-- 000006_event_streaming_and_schemas.up.sql
-- Event schema registry, partitioned consumer offsets, and dead-letter events

-- 1. Event Schema Registry
CREATE TABLE IF NOT EXISTS event_schemas (
    id VARCHAR(36) PRIMARY KEY,
    tenant_id VARCHAR(36) REFERENCES tenants(id) ON DELETE CASCADE,
    name VARCHAR(128) NOT NULL,
    version INT NOT NULL,
    schema_type VARCHAR(32) NOT NULL DEFAULT 'json_schema',
    definition TEXT NOT NULL,
    compatibility VARCHAR(32) NOT NULL DEFAULT 'BACKWARD',
    description TEXT,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    CONSTRAINT uq_schema_tenant_name_ver UNIQUE (tenant_id, name, version)
);

CREATE INDEX IF NOT EXISTS idx_event_schemas_name_ver ON event_schemas(name, version DESC);
CREATE INDEX IF NOT EXISTS idx_event_schemas_tenant ON event_schemas(tenant_id);

-- 2. Stream Consumer Group Offsets
CREATE TABLE IF NOT EXISTS stream_consumer_offsets (
    group_id VARCHAR(128) NOT NULL,
    topic VARCHAR(128) NOT NULL,
    partition INT NOT NULL,
    offset_val BIGINT NOT NULL,
    committed_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    PRIMARY KEY (group_id, topic, partition)
);

CREATE INDEX IF NOT EXISTS idx_consumer_offsets_group ON stream_consumer_offsets(group_id);

-- 3. Dead-Letter Events Queue
CREATE TABLE IF NOT EXISTS dead_letter_events (
    id VARCHAR(36) PRIMARY KEY,
    tenant_id VARCHAR(36) REFERENCES tenants(id) ON DELETE SET NULL,
    topic VARCHAR(128) NOT NULL,
    partition INT NOT NULL,
    offset_val BIGINT NOT NULL,
    event_id VARCHAR(36),
    ordering_key VARCHAR(255),
    event_type VARCHAR(128),
    payload JSONB NOT NULL DEFAULT '{}'::jsonb,
    error_reason TEXT NOT NULL,
    retry_count INT NOT NULL DEFAULT 0,
    resolved BOOLEAN NOT NULL DEFAULT FALSE,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX IF NOT EXISTS idx_dlq_events_tenant_created ON dead_letter_events(tenant_id, created_at DESC);
CREATE INDEX IF NOT EXISTS idx_dlq_events_topic_part ON dead_letter_events(topic, partition);
CREATE INDEX IF NOT EXISTS idx_dlq_events_resolved ON dead_letter_events(resolved) WHERE resolved = false;
