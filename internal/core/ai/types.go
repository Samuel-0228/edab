package ai

import (
	"encoding/json"
	"time"
)

// TaskType defines the category of AI assistance task.
type TaskType string

const (
	TaskDraftWorkflow         TaskType = "draft_workflow"
	TaskMapFields             TaskType = "map_fields"
	TaskGenerateConditions    TaskType = "generate_conditions"
	TaskSuggestTransformation TaskType = "suggest_transformation"
	TaskExplainFailure        TaskType = "explain_failure"
)

// ProposalStatus defines the human-in-the-loop review state.
type ProposalStatus string

const (
	StatusPendingApproval ProposalStatus = "pending_approval"
	StatusAccepted        ProposalStatus = "accepted"
	StatusRejected        ProposalStatus = "rejected"
)

// ValidationResult captures deterministic safety and structural validation of an AI proposal.
type ValidationResult struct {
	Valid    bool     `json:"valid"`
	Errors   []string `json:"errors,omitempty"`
	Warnings []string `json:"warnings,omitempty"`
}

// Proposal represents a structured, non-autonomous AI proposal requiring human review before application.
type Proposal struct {
	ID          string           `json:"id"`
	TenantID    string           `json:"tenant_id"`
	Task        TaskType         `json:"task"`
	Status      ProposalStatus   `json:"status"`
	Prompt      string           `json:"prompt"`
	Explanation string           `json:"explanation"`
	Content     json.RawMessage  `json:"content"`
	Validation  ValidationResult `json:"validation"`
	Warnings    []string         `json:"warnings,omitempty"`
	ModelUsed   string           `json:"model_used"`
	TokensUsed  int              `json:"tokens_used"`
	CreatedAt   time.Time        `json:"created_at"`
	DecidedAt   *time.Time       `json:"decided_at,omitempty"`
	DecidedBy   *string          `json:"decided_by,omitempty"`
}

// Request Types

type DraftWorkflowRequest struct {
	Prompt            string         `json:"prompt"`
	AllowedConnectors []string       `json:"allowed_connectors,omitempty"`
	Context           map[string]any `json:"context,omitempty"`
}

type MapFieldsRequest struct {
	SourceSchema  map[string]any `json:"source_schema,omitempty"`
	TargetSchema  map[string]any `json:"target_schema,omitempty"`
	SamplePayload map[string]any `json:"sample_payload,omitempty"`
	Description   string         `json:"description,omitempty"`
}

type GenerateConditionsRequest struct {
	Criteria      string         `json:"criteria"`
	SamplePayload map[string]any `json:"sample_payload,omitempty"`
}

type SuggestTransformationRequest struct {
	InputPayload  map[string]any `json:"input_payload"`
	DesiredOutput map[string]any `json:"desired_output,omitempty"`
	Goal          string         `json:"goal,omitempty"`
}

type ExplainFailureRequest struct {
	WorkflowID  string          `json:"workflow_id"`
	ExecutionID string          `json:"execution_id"`
	StepID      string          `json:"step_id,omitempty"`
	StepType    string          `json:"step_type,omitempty"`
	Error       string          `json:"error"`
	StepInput   any             `json:"step_input,omitempty"`
	StepOutput  any             `json:"step_output,omitempty"`
	Config      json.RawMessage `json:"config,omitempty"`
}

// AIStatusResponse reports engine AI configuration, provider status, and tenant eligibility.
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
