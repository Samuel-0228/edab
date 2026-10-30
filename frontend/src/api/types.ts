export type ExecutionStatus =
  'pending' | 'running' | 'success' | 'failed' | 'retrying' | 'cancelled';

export interface ExecutionStep {
  id: string;
  execution_id: string;
  step_type: 'trigger' | 'condition' | 'transformation' | 'action';
  name: string;
  status: 'success' | 'skipped' | 'failed';
  input?: unknown;
  output?: unknown;
  error?: string;
  duration_ms: number;
  started_at: string;
  completed_at: string;
}

export interface Execution {
  id: string;
  workflow_id: string;
  event_id: string;
  status: ExecutionStatus;
  attempts: number;
  max_attempts: number;
  next_attempt_at: string;
  started_at: string;
  completed_at?: string;
  duration_ms: number;
  error?: string;
  steps?: ExecutionStep[];
  created_at: string;
  updated_at: string;
}

export interface TriggerConfig {
  type: string; // 'telegram' | 'github' | 'webhook' | 'rss' | 'discord' | 'schedule' | 'all'
  event_types?: string[];
  config?: Record<string, unknown>;
}

export interface ConditionRule {
  field: string;
  operator: string;
  value: unknown;
}

export interface TransformationConfig {
  type: 'passthrough' | 'template' | 'mapping';
  template?: string;
  mapping?: Record<string, string>;
}

export interface ActionConfig {
  integration: string;
  action_type: string;
  credential_id?: string;
  config: Record<string, unknown>;
}

export interface Workflow {
  id: string;
  name: string;
  description?: string;
  enabled: boolean;
  trigger: TriggerConfig;
  conditions?: ConditionRule | null;
  transformations?: TransformationConfig | null;
  actions: ActionConfig[];
  max_retries: number;
  created_at?: string;
  updated_at?: string;
}

export interface WorkflowCreateInput {
  name: string;
  description?: string;
  enabled?: boolean;
  trigger: TriggerConfig;
  conditions?: ConditionRule | null;
  transformations?: TransformationConfig | null;
  actions: ActionConfig[];
  max_retries?: number;
}

export interface WorkflowTestInput {
  event_type?: string;
  source_id?: string;
  payload?: Record<string, unknown>;
  metadata?: Record<string, unknown>;
}

export interface WorkflowTestResponse {
  status: 'success' | 'error';
  error?: string;
  execution: Execution;
}

export interface DeadLetterRecord {
  id: string;
  execution_id: string;
  workflow_id: string;
  event_id: string;
  error: string;
  attempts: number;
  failed_at: string;
  discarded?: boolean;
  discarded_at?: string;
}

export interface EventRecord {
  id: string;
  source_id: string;
  external_event_id: string;
  event_type: string;
  payload: Record<string, unknown>;
  metadata?: Record<string, unknown>;
  created_at: string;
}

export interface EventIngestInput {
  source_id: string;
  external_event_id?: string;
  event_type: string;
  payload: Record<string, unknown>;
  metadata?: Record<string, unknown>;
}

export interface Webhook {
  id: string;
  name: string;
  event_type: string;
  secret_token?: string;
  enabled: boolean;
  created_at: string;
  updated_at: string;
}

export interface WebhookCreateInput {
  name: string;
  event_type: string;
  secret_token?: string;
  enabled?: boolean;
}

export interface Credential {
  id: string;
  name: string;
  integration: string;
  key_id: string;
  masked_fields: Record<string, string>;
  created_at: string;
  updated_at: string;
}

export interface CredentialCreateInput {
  name: string;
  integration: string;
  values: Record<string, string>;
}

export interface IntegrationCapability {
  type: string;
  name: string;
  description: string;
}

export interface IntegrationCredentialField {
  key: string;
  name: string;
  required: boolean;
}

export interface IntegrationSchemaProperty {
  type: string;
  title?: string;
  description?: string;
  enum?: string[];
  default?: unknown;
  items?: unknown;
}

export interface IntegrationJSONSchema {
  type: string;
  required?: string[];
  properties?: Record<string, IntegrationSchemaProperty>;
}

export interface IntegrationSchemaResponse {
  id: string;
  name: string;
  kind: string;
  capabilities: string[];
  required_credential_type?: string;
  config_schema: IntegrationJSONSchema;
  schemas?: Record<string, IntegrationJSONSchema>;
}

export interface IntegrationMetadata {
  id: string;
  name: string;
  description: string;
  kind?: string;
  capabilities?: string[];
  required_credential_type?: string;
  config_schema?: IntegrationJSONSchema;
  schemas?: Record<string, IntegrationJSONSchema>;
  triggers: IntegrationCapability[];
  actions: IntegrationCapability[];
  credentials: IntegrationCredentialField[];
}

export interface OverviewResponse {
  status: string;
  active_workflows: number;
  total_workflows: number;
  executions_total: number;
  executions_success: number;
  executions_failed: number;
  success_rate: number;
  dead_letter_count: number;
  connected_credentials: number;
  registered_adapters: number;
  recent_executions: Execution[];
  catalog: IntegrationMetadata[];
  uptime_seconds: number;
}

export interface HealthResponse {
  status: string;
}

export interface ReadyResponse {
  status: string;
  registered_sources: string[];
  registered_destinations: string[];
  registered_actions: string[];
}

export interface ParsedMetric {
  name: string;
  type?: string;
  help?: string;
  value: number;
  labels?: Record<string, string>;
}

export type AppRoute =
  | 'overview'
  | 'workflows'
  | 'builder'
  | 'integrations'
  | 'credentials'
  | 'executions'
  | 'failures'
  | 'events'
  | 'webhooks'
  | 'logs'
  | 'schedules'
  | 'health'
  | 'settings';
