package sdk

import (
	"encoding/json"
	"time"
)

// ExecutionStatus represents the state of a workflow execution.
type ExecutionStatus string

const (
	ExecutionPending            ExecutionStatus = "pending"
	ExecutionRunning            ExecutionStatus = "running"
	ExecutionSuccess            ExecutionStatus = "success"
	ExecutionFailed             ExecutionStatus = "failed"
	ExecutionRetrying           ExecutionStatus = "retrying"
	ExecutionWaiting            ExecutionStatus = "waiting"
	ExecutionPaused             ExecutionStatus = "paused"
	ExecutionCancelled          ExecutionStatus = "cancelled"
	ExecutionWaitingApproval    ExecutionStatus = "waiting_approval"
	ExecutionCompensating       ExecutionStatus = "compensating"
	ExecutionCompensated        ExecutionStatus = "compensated"
	ExecutionCompensationFailed ExecutionStatus = "compensation_failed"
)

// ApprovalStatus represents the state of a human approval step.
type ApprovalStatus string

const (
	ApprovalPending   ApprovalStatus = "pending"
	ApprovalApproved  ApprovalStatus = "approved"
	ApprovalRejected  ApprovalStatus = "rejected"
	ApprovalExpired   ApprovalStatus = "expired"
	ApprovalEscalated ApprovalStatus = "escalated"
)

// Role defines an authorization role.
type Role string

const (
	RoleOwner    Role = "owner"
	RoleAdmin    Role = "admin"
	RoleOperator Role = "operator"
	RoleEditor   Role = "editor"
	RoleViewer   Role = "viewer"
)

// TriggerConfig defines the trigger event criteria for activating a workflow.
type TriggerConfig struct {
	Type       string          `json:"type"`                  // "telegram", "webhook", "discord", "rss", "github", "schedule", "all"
	SourceID   string          `json:"source_id,omitempty"`   // optional source restriction
	EventTypes []string        `json:"event_types,omitempty"` // empty or ["*"] matches all event types
	Config     json.RawMessage `json:"config,omitempty"`      // trigger-specific configuration
}

// ApprovalConfig defines settings for a human approval step.
type ApprovalConfig struct {
	Title          string   `json:"title,omitempty"`
	Description    string   `json:"description,omitempty"`
	ApproverRoles  []string `json:"approver_roles,omitempty"`
	ApproverUsers  []string `json:"approver_users,omitempty"`
	TimeoutSeconds int      `json:"timeout_seconds,omitempty"`
	EscalationRole string   `json:"escalation_role,omitempty"`
}

// ActionStep defines an executable step within a workflow pipeline.
type ActionStep struct {
	ID             string          `json:"id,omitempty"`
	Name           string          `json:"name,omitempty"`
	Type           string          `json:"type,omitempty"` // "action", "branch", "for_each", "delay", "wait", "parallel", "approval"
	Integration    string          `json:"integration,omitempty"`
	ActionType     string          `json:"action_type,omitempty"`
	CredentialID   string          `json:"credential_id,omitempty"`
	Config         json.RawMessage `json:"config,omitempty"`
	TimeoutSeconds int             `json:"timeout_seconds,omitempty"`
	OnError        string          `json:"on_error,omitempty"` // "fail", "continue", "skip_rest"
	MaxRetries     int             `json:"max_retries,omitempty"`
	BackoffSeconds int             `json:"backoff_seconds,omitempty"`

	// Sagas & Compensation
	Compensation *ActionStep `json:"compensation,omitempty"`

	// Human Approvals
	Approval *ApprovalConfig `json:"approval,omitempty"`
}

// Workflow represents a defined automation pipeline.
type Workflow struct {
	ID              string          `json:"id"`
	TenantID        string          `json:"tenant_id,omitempty"`
	ProjectID       string          `json:"project_id,omitempty"`
	Name            string          `json:"name"`
	Description     string          `json:"description,omitempty"`
	Enabled         bool            `json:"enabled"`
	Trigger         TriggerConfig   `json:"trigger"`
	Conditions      json.RawMessage `json:"conditions,omitempty"`
	Transformations json.RawMessage `json:"transformations,omitempty"`
	Actions         []ActionStep    `json:"actions"`
	MaxRetries      int             `json:"max_retries,omitempty"`
	TimeoutSeconds  int             `json:"timeout_seconds,omitempty"`
	RateLimit       int             `json:"rate_limit,omitempty"`
	MaxConcurrency  int             `json:"max_concurrency,omitempty"`
	ConcurrencyKey  string          `json:"concurrency_key,omitempty"`
	CreatedAt       time.Time       `json:"created_at"`
	UpdatedAt       time.Time       `json:"updated_at"`
}

// WorkflowCreateInput represents input payload for creating or updating a workflow.
type WorkflowCreateInput struct {
	Name            string          `json:"name"`
	Description     string          `json:"description,omitempty"`
	Enabled         *bool           `json:"enabled,omitempty"`
	Trigger         TriggerConfig   `json:"trigger"`
	Conditions      json.RawMessage `json:"conditions,omitempty"`
	Transformations json.RawMessage `json:"transformations,omitempty"`
	Actions         []ActionStep    `json:"actions"`
	MaxRetries      int             `json:"max_retries,omitempty"`
	TimeoutSeconds  int             `json:"timeout_seconds,omitempty"`
	RateLimit       int             `json:"rate_limit,omitempty"`
	MaxConcurrency  int             `json:"max_concurrency,omitempty"`
	ConcurrencyKey  string          `json:"concurrency_key,omitempty"`
}

// WorkflowVersion represents an immutable historical snapshot of a workflow.
type WorkflowVersion struct {
	ID          string          `json:"id"`
	WorkflowID  string          `json:"workflow_id"`
	Version     int             `json:"version"`
	Definition  json.RawMessage `json:"definition"`
	CreatedAt   time.Time       `json:"created_at"`
	Description string          `json:"description,omitempty"`
}

// WorkflowDiff represents diff between two workflow versions.
type WorkflowDiff struct {
	WorkflowID  string          `json:"workflow_id"`
	FromVersion int             `json:"from_version"`
	ToVersion   int             `json:"to_version"`
	From        json.RawMessage `json:"from"`
	To          json.RawMessage `json:"to"`
}

// ExecutionStep records runtime outcome and duration of a granular stage.
type ExecutionStep struct {
	ID          string          `json:"id"`
	ExecutionID string          `json:"execution_id"`
	StepType    string          `json:"step_type"` // "trigger", "condition", "transformation", "action", "approval"
	Name        string          `json:"name"`
	Status      string          `json:"status"` // "success", "skipped", "failed", "waiting_approval", "approved", "rejected"
	Input       json.RawMessage `json:"input,omitempty"`
	Output      json.RawMessage `json:"output,omitempty"`
	Error       string          `json:"error,omitempty"`
	DurationMs  int64           `json:"duration_ms"`
	StartedAt   time.Time       `json:"started_at"`
	CompletedAt time.Time       `json:"completed_at"`
}

// Execution models an active, retrying, or completed workflow execution.
type Execution struct {
	ID            string          `json:"id"`
	TenantID      string          `json:"tenant_id,omitempty"`
	WorkflowID    string          `json:"workflow_id"`
	EventID       string          `json:"event_id"`
	Status        ExecutionStatus `json:"status"`
	Attempts      int             `json:"attempts"`
	MaxAttempts   int             `json:"max_attempts"`
	NextAttemptAt time.Time       `json:"next_attempt_at,omitempty"`
	StartedAt     time.Time       `json:"started_at"`
	CompletedAt   time.Time       `json:"completed_at,omitempty"`
	DurationMs    int64           `json:"duration_ms"`
	Error         string          `json:"error,omitempty"`
	Steps         []ExecutionStep `json:"steps,omitempty"`
	CreatedAt     time.Time       `json:"created_at"`
	UpdatedAt     time.Time       `json:"updated_at"`
}

// DeadLetter represents an unrecoverable failure saved to dead-letter store.
type DeadLetter struct {
	ID          string    `json:"id"`
	TenantID    string    `json:"tenant_id,omitempty"`
	ExecutionID string    `json:"execution_id"`
	WorkflowID  string    `json:"workflow_id"`
	EventID     string    `json:"event_id"`
	Error       string    `json:"error"`
	Attempts    int       `json:"attempts"`
	FailedAt    time.Time `json:"failed_at"`
	Discarded   bool      `json:"discarded"`
	DiscardedAt time.Time `json:"discarded_at,omitempty"`
}

// EventRecord represents an ingested event stored in the event log.
type EventRecord struct {
	ID              string          `json:"id"`
	TenantID        string          `json:"tenant_id,omitempty"`
	SourceID        string          `json:"source_id"`
	ExternalEventID string          `json:"external_event_id,omitempty"`
	EventType       string          `json:"event_type"`
	Payload         json.RawMessage `json:"payload"`
	Metadata        json.RawMessage `json:"metadata,omitempty"`
	CreatedAt       time.Time       `json:"created_at"`
}

// Approval represents a human approval authorization task.
type Approval struct {
	ID             string         `json:"id"`
	TenantID       string         `json:"tenant_id,omitempty"`
	ExecutionID    string         `json:"execution_id"`
	StepID         string         `json:"step_id"`
	WorkflowID     string         `json:"workflow_id"`
	Title          string         `json:"title"`
	Description    string         `json:"description,omitempty"`
	ApproverRoles  []string       `json:"approver_roles,omitempty"`
	ApproverUsers  []string       `json:"approver_users,omitempty"`
	TimeoutSeconds int            `json:"timeout_seconds"`
	EscalationRole string         `json:"escalation_role,omitempty"`
	Token          string         `json:"token,omitempty"`
	TokenHash      string         `json:"token_hash,omitempty"`
	Status         ApprovalStatus `json:"status"`
	DecidedBy      string         `json:"decided_by,omitempty"`
	DecisionNote   string         `json:"decision_note,omitempty"`
	DecidedAt      *time.Time     `json:"decided_at,omitempty"`
	ExpiresAt      time.Time      `json:"expires_at"`
	CreatedAt      time.Time      `json:"created_at"`
	UpdatedAt      time.Time      `json:"updated_at"`
}

// Tenant represents an organizational tenant.
type Tenant struct {
	ID        string    `json:"id"`
	Name      string    `json:"name"`
	Slug      string    `json:"slug"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}

// Member represents a user membership within a tenant.
type Member struct {
	ID        string    `json:"id"`
	TenantID  string    `json:"tenant_id"`
	UserID    string    `json:"user_id"`
	Role      Role      `json:"role"`
	CreatedAt time.Time `json:"created_at"`
}

// APIToken represents a scoped API credential.
type APIToken struct {
	ID          string    `json:"id"`
	TenantID    string    `json:"tenant_id"`
	Name        string    `json:"name"`
	TokenPrefix string    `json:"token_prefix"`
	Scopes      []string  `json:"scopes"`
	ExpiresAt   time.Time `json:"expires_at,omitempty"`
	LastUsedAt  time.Time `json:"last_used_at,omitempty"`
	CreatedAt   time.Time `json:"created_at"`
	Token       string    `json:"token,omitempty"` // populated only on creation
}

// Identity reflects the authenticated user or token context.
type Identity struct {
	UserID     string   `json:"user_id"`
	Email      string   `json:"email"`
	Name       string   `json:"name"`
	TenantID   string   `json:"tenant_id"`
	TenantName string   `json:"tenant_name"`
	Role       Role     `json:"role"`
	Scopes     []string `json:"scopes"`
}

// DeployOptions specifies parameters for GitOps workflow deployment.
type DeployOptions struct {
	DryRun            bool   `json:"dry_run"`
	RequireApproval   bool   `json:"require_approval"`
	Approved          bool   `json:"approved"`
	RollbackOnFailure bool   `json:"rollback_on_failure"`
	ApprovalReason    string `json:"approval_reason,omitempty"`
	CommitSHA         string `json:"commit_sha,omitempty"`
	Environment       string `json:"environment,omitempty"`
}

// DeployAction describes the operation performed for a workflow in deployment.
type DeployAction string

const (
	DeployActionCreated   DeployAction = "created"
	DeployActionUpdated   DeployAction = "updated"
	DeployActionUnchanged DeployAction = "unchanged"
	DeployActionSkipped   DeployAction = "skipped"
)

// WorkflowDeployResult records the outcome of deploying an individual workflow.
type WorkflowDeployResult struct {
	WorkflowID  string       `json:"workflow_id"`
	Name        string       `json:"name"`
	Action      DeployAction `json:"action"`
	Version     int          `json:"version,omitempty"`
	DiffSummary string       `json:"diff_summary,omitempty"`
	Error       string       `json:"error,omitempty"`
}

// DeployResult represents the aggregate outcome of a GitOps deployment run.
type DeployResult struct {
	Success   bool                   `json:"success"`
	DryRun    bool                   `json:"dry_run"`
	Workflows []WorkflowDeployResult `json:"workflows"`
	Summary   string                 `json:"summary"`
}

// DiffResult provides structured comparison between local file and remote workflow.
type DiffResult struct {
	WorkflowID string   `json:"workflow_id"`
	Name       string   `json:"name"`
	HasChanges bool     `json:"has_changes"`
	Changes    []string `json:"changes,omitempty"`
	DiffText   string   `json:"diff_text,omitempty"`
}

// GovernancePolicy defines enterprise security, rate limiting, and retention policies.
type GovernancePolicy struct {
	TenantID                              string    `json:"tenant_id"`
	AllowedConnectors                     []string  `json:"allowed_connectors"`
	AllowedOutboundDomains                []string  `json:"allowed_outbound_domains"`
	MaxWorkflowRetries                    int       `json:"max_workflow_retries"`
	MaxPayloadSizeBytes                   int64     `json:"max_payload_size_bytes"`
	DataRetentionDays                     int       `json:"data_retention_days"`
	RequireApprovalForIrreversibleActions bool      `json:"require_approval_irreversible"`
	MaskPII                               bool      `json:"mask_pii"`
	MaskSecrets                           bool      `json:"mask_secrets"`
	BlockedKeywords                       []string  `json:"blocked_keywords"`
	AllowedEnvironments                   []string  `json:"allowed_environments"`
	EnforcementMode                       string    `json:"enforcement_mode"`
	CreatedAt                             time.Time `json:"created_at"`
	UpdatedAt                             time.Time `json:"updated_at"`
}

type PolicyViolation struct {
	Rule    string         `json:"rule"`
	Field   string         `json:"field"`
	Message string         `json:"message"`
	Details map[string]any `json:"details,omitempty"`
}

type PolicyValidationResult struct {
	Valid      bool              `json:"valid"`
	Violations []PolicyViolation `json:"violations"`
	TenantID   string            `json:"tenant_id"`
}

type AuditVerificationResult struct {
	Valid            bool      `json:"valid"`
	TenantID         string    `json:"tenant_id"`
	TotalRecords     int       `json:"total_records"`
	VerifiedAt       time.Time `json:"verified_at"`
	BrokenAtSequence *int64    `json:"broken_at_sequence,omitempty"`
	ErrorMessage     string    `json:"error_message,omitempty"`
}

type RetentionPruneResult struct {
	PrunedAuditLogs int64     `json:"pruned_audit_logs"`
	RetentionDays   int       `json:"retention_days"`
	Cutoff          time.Time `json:"cutoff"`
	TenantID        string    `json:"tenant_id"`
}

// Controlled AI Assistance Types

type AITaskType string

const (
	AITaskDraftWorkflow         AITaskType = "draft_workflow"
	AITaskMapFields             AITaskType = "map_fields"
	AITaskGenerateConditions    AITaskType = "generate_conditions"
	AITaskSuggestTransformation AITaskType = "suggest_transformation"
	AITaskExplainFailure        AITaskType = "explain_failure"
)

type AIProposalStatus string

const (
	AIStatusPendingApproval AIProposalStatus = "pending_approval"
	AIStatusAccepted        AIProposalStatus = "accepted"
	AIStatusRejected        AIProposalStatus = "rejected"
)

type AIValidationResult struct {
	Valid    bool     `json:"valid"`
	Errors   []string `json:"errors,omitempty"`
	Warnings []string `json:"warnings,omitempty"`
}

type AIProposal struct {
	ID          string             `json:"id"`
	TenantID    string             `json:"tenant_id"`
	Task        AITaskType         `json:"task"`
	Status      AIProposalStatus   `json:"status"`
	Prompt      string             `json:"prompt"`
	Explanation string             `json:"explanation"`
	Content     json.RawMessage    `json:"content"`
	Validation  AIValidationResult `json:"validation"`
	Warnings    []string           `json:"warnings,omitempty"`
	ModelUsed   string             `json:"model_used"`
	TokensUsed  int                `json:"tokens_used"`
	CreatedAt   time.Time          `json:"created_at"`
	DecidedAt   *time.Time         `json:"decided_at,omitempty"`
	DecidedBy   *string            `json:"decided_by,omitempty"`
}

type AIDraftWorkflowRequest struct {
	Prompt            string         `json:"prompt"`
	AllowedConnectors []string       `json:"allowed_connectors,omitempty"`
	Context           map[string]any `json:"context,omitempty"`
}

type AIMapFieldsRequest struct {
	SourceSchema  map[string]any `json:"source_schema,omitempty"`
	TargetSchema  map[string]any `json:"target_schema,omitempty"`
	SamplePayload map[string]any `json:"sample_payload,omitempty"`
	Description   string         `json:"description,omitempty"`
}

type AIGenerateConditionsRequest struct {
	Criteria      string         `json:"criteria"`
	SamplePayload map[string]any `json:"sample_payload,omitempty"`
}

type AISuggestTransformationRequest struct {
	InputPayload  map[string]any `json:"input_payload"`
	DesiredOutput map[string]any `json:"desired_output,omitempty"`
	Goal          string         `json:"goal,omitempty"`
}

type AIExplainFailureRequest struct {
	WorkflowID  string          `json:"workflow_id"`
	ExecutionID string          `json:"execution_id"`
	StepID      string          `json:"step_id,omitempty"`
	StepType    string          `json:"step_type,omitempty"`
	Error       string          `json:"error"`
	StepInput   any             `json:"step_input,omitempty"`
	StepOutput  any             `json:"step_output,omitempty"`
	Config      json.RawMessage `json:"config,omitempty"`
}

type AIStatusResponse struct {
	Enabled        bool     `json:"enabled"`
	Provider       string   `json:"provider"`
	Model          string   `json:"model"`
	TenantID       string   `json:"tenant_id"`
	OptedOut       bool     `json:"opted_out"`
	MaxTokens      int      `json:"max_tokens"`
	TimeoutSeconds int      `json:"timeout_seconds"`
	AvailableTasks []string `json:"available_tasks"`
}

type AIAcceptProposalResult struct {
	Proposal          *AIProposal `json:"proposal"`
	AppliedWorkflowID string      `json:"applied_workflow_id,omitempty"`
}
