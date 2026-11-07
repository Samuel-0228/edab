export type ExecutionStatus =
  | 'pending'
  | 'running'
  | 'success'
  | 'failed'
  | 'retrying'
  | 'waiting'
  | 'paused'
  | 'cancelled'
  | 'waiting_approval'
  | 'compensating'
  | 'compensated'
  | 'compensation_failed';

export type ApprovalStatus = 'pending' | 'approved' | 'rejected' | 'expired' | 'escalated';

export type Role = 'owner' | 'admin' | 'operator' | 'editor' | 'viewer';

export interface TriggerConfig {
  type: string;
  source_id?: string;
  event_types?: string[];
  config?: Record<string, unknown>;
}

export interface ApprovalConfig {
  title?: string;
  description?: string;
  approver_roles?: string[];
  approver_users?: string[];
  timeout_seconds?: number;
  escalation_role?: string;
}

export interface ActionStep {
  id?: string;
  name?: string;
  type?: 'action' | 'branch' | 'for_each' | 'delay' | 'wait' | 'parallel' | 'approval';
  integration?: string;
  action_type?: string;
  credential_id?: string;
  config?: Record<string, unknown>;
  timeout_seconds?: number;
  on_error?: 'fail' | 'continue' | 'skip_rest';
  max_retries?: number;
  backoff_seconds?: number;
  compensation?: ActionStep;
  approval?: ApprovalConfig;
}

export interface Workflow {
  id: string;
  tenant_id?: string;
  project_id?: string;
  name: string;
  description?: string;
  enabled: boolean;
  trigger: TriggerConfig;
  conditions?: Record<string, unknown> | null;
  transformations?: Record<string, unknown> | null;
  actions: ActionStep[];
  max_retries?: number;
  timeout_seconds?: number;
  rate_limit?: number;
  max_concurrency?: number;
  concurrency_key?: string;
  created_at?: string;
  updated_at?: string;
}

export interface WorkflowCreateInput {
  name: string;
  description?: string;
  enabled?: boolean;
  trigger: TriggerConfig;
  conditions?: Record<string, unknown> | null;
  transformations?: Record<string, unknown> | null;
  actions: ActionStep[];
  max_retries?: number;
  timeout_seconds?: number;
  rate_limit?: number;
  max_concurrency?: number;
  concurrency_key?: string;
}

export interface WorkflowVersion {
  id: string;
  workflow_id: string;
  version: number;
  definition: Workflow;
  created_at: string;
  description?: string;
}

export interface WorkflowDiff {
  workflow_id: string;
  from_version: number;
  to_version: number;
  from: Workflow;
  to: Workflow;
}

export interface ExecutionStep {
  id: string;
  execution_id: string;
  step_type: 'trigger' | 'condition' | 'transformation' | 'action' | 'approval';
  name: string;
  status: 'success' | 'skipped' | 'failed' | 'waiting_approval' | 'approved' | 'rejected' | 'compensating' | 'compensated';
  input?: unknown;
  output?: unknown;
  error?: string;
  duration_ms: number;
  started_at: string;
  completed_at: string;
}

export interface Execution {
  id: string;
  tenant_id?: string;
  workflow_id: string;
  event_id: string;
  status: ExecutionStatus;
  attempts: number;
  max_attempts: number;
  next_attempt_at?: string;
  started_at: string;
  completed_at?: string;
  duration_ms: number;
  error?: string;
  steps?: ExecutionStep[];
  created_at: string;
  updated_at: string;
}

export interface DeadLetter {
  id: string;
  tenant_id?: string;
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
  tenant_id?: string;
  source_id: string;
  external_event_id?: string;
  event_type: string;
  payload: Record<string, unknown>;
  metadata?: Record<string, unknown>;
  created_at: string;
}

export interface Approval {
  id: string;
  tenant_id?: string;
  execution_id: string;
  step_id: string;
  workflow_id: string;
  title: string;
  description?: string;
  approver_roles?: string[];
  approver_users?: string[];
  timeout_seconds: number;
  escalation_role?: string;
  token?: string;
  token_hash?: string;
  status: ApprovalStatus;
  decided_by?: string;
  decision_note?: string;
  decided_at?: string;
  expires_at: string;
  created_at: string;
  updated_at: string;
}

export interface Tenant {
  id: string;
  name: string;
  slug: string;
  created_at: string;
  updated_at: string;
}

export interface Member {
  id: string;
  tenant_id: string;
  user_id: string;
  role: Role;
  created_at: string;
}

export interface APIToken {
  id: string;
  tenant_id: string;
  name: string;
  token_prefix: string;
  scopes: string[];
  expires_at?: string;
  last_used_at?: string;
  created_at: string;
  token?: string;
}

export interface Identity {
  user_id: string;
  email: string;
  name: string;
  tenant_id: string;
  tenant_name: string;
  role: Role;
  scopes: string[];
}

export interface DeployOptions {
  dryRun?: boolean;
  requireApproval?: boolean;
  approved?: boolean;
  rollbackOnFailure?: boolean;
  approvalReason?: string;
  commitSha?: string;
  environment?: string;
}

export interface WorkflowDeployResult {
  workflow_id: string;
  name: string;
  action: 'created' | 'updated' | 'unchanged' | 'skipped';
  version?: number;
  diff_summary?: string;
  error?: string;
}

export interface DeployResult {
  success: boolean;
  dry_run: boolean;
  workflows: WorkflowDeployResult[];
  summary: string;
}

export interface DiffResult {
  workflow_id: string;
  name: string;
  has_changes: boolean;
  changes: string[];
  diff_text?: string;
}

export interface GovernancePolicy {
  tenant_id: string;
  allowed_connectors: string[];
  allowed_outbound_domains: string[];
  max_workflow_retries: number;
  max_payload_size_bytes: number;
  data_retention_days: number;
  require_approval_irreversible: boolean;
  mask_pii: boolean;
  mask_secrets: boolean;
  blocked_keywords: string[];
  allowed_environments: string[];
  enforcement_mode: 'enforce' | 'audit';
  created_at?: string;
  updated_at?: string;
}

export interface PolicyViolation {
  rule: string;
  field: string;
  message: string;
  details?: Record<string, unknown>;
}

export interface PolicyValidationResult {
  valid: boolean;
  violations: PolicyViolation[];
  tenant_id: string;
}

export interface AuditVerificationResult {
  valid: boolean;
  tenant_id: string;
  total_records: number;
  verified_at: string;
  broken_at_sequence?: number;
  error_message?: string;
}

export interface RetentionPruneResult {
  pruned_audit_logs: number;
  retention_days: number;
  cutoff: string;
  tenant_id: string;
}

// --- Controlled AI Assistance Types ---

export type AITaskType =
  | 'draft_workflow'
  | 'map_fields'
  | 'generate_conditions'
  | 'suggest_transformation'
  | 'explain_failure';

export type AIProposalStatus = 'pending_approval' | 'accepted' | 'rejected';

export interface AIValidationResult {
  valid: boolean;
  errors?: string[];
  warnings?: string[];
}

export interface AIProposal {
  id: string;
  tenant_id: string;
  task: AITaskType;
  status: AIProposalStatus;
  prompt: string;
  explanation: string;
  content: unknown;
  validation: AIValidationResult;
  warnings?: string[];
  model_used: string;
  tokens_used: number;
  created_at: string;
  decided_at?: string;
  decided_by?: string;
}

export interface AIDraftWorkflowRequest {
  prompt: string;
  allowed_connectors?: string[];
  context?: Record<string, unknown>;
}

export interface AIMapFieldsRequest {
  source_schema?: Record<string, unknown>;
  target_schema?: Record<string, unknown>;
  sample_payload?: Record<string, unknown>;
  description?: string;
}

export interface AIGenerateConditionsRequest {
  criteria: string;
  sample_payload?: Record<string, unknown>;
}

export interface AISuggestTransformationRequest {
  input_payload: Record<string, unknown>;
  desired_output?: Record<string, unknown>;
  goal?: string;
}

export interface AIExplainFailureRequest {
  workflow_id: string;
  execution_id: string;
  step_id?: string;
  step_type?: string;
  error: string;
  step_input?: unknown;
  step_output?: unknown;
  config?: unknown;
}

export interface AIStatusResponse {
  enabled: boolean;
  provider: string;
  model: string;
  tenant_id: string;
  opted_out: boolean;
  max_tokens: number;
  timeout_seconds: number;
  available_tasks: string[];
}

export interface AIAcceptProposalResult {
  proposal: AIProposal;
  applied_workflow_id?: string;
}
