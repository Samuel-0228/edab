package ai

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/google/uuid"
)

// ServiceConfig holds tuning and safety limits for the AI service.
type ServiceConfig struct {
	Enabled             bool
	Provider            Provider
	Store               Store
	MaxTokensPerRequest int
	Timeout             time.Duration
	MonthlyTokenLimit   int
}

// Service manages controlled AI generation, sanitization, validation, and human approval tracking.
type Service struct {
	mu            sync.RWMutex
	enabled       bool
	provider      Provider
	store         Store
	maxTokens     int
	timeout       time.Duration
	monthlyLimit  int
	optOutTenants map[string]bool
	tenantUsage   map[string]int
}

// NewService constructs a new controlled AI assistance service.
func NewService(cfg ServiceConfig) *Service {
	if cfg.Provider == nil {
		cfg.Provider = NewMockProvider()
	}
	if cfg.Store == nil {
		cfg.Store = NewMemoryStore()
	}
	if cfg.MaxTokensPerRequest <= 0 {
		cfg.MaxTokensPerRequest = 2000
	}
	if cfg.Timeout <= 0 {
		cfg.Timeout = 30 * time.Second
	}
	if cfg.MonthlyTokenLimit <= 0 {
		cfg.MonthlyTokenLimit = 100000
	}

	return &Service{
		enabled:       cfg.Enabled,
		provider:      cfg.Provider,
		store:         cfg.Store,
		maxTokens:     cfg.MaxTokensPerRequest,
		timeout:       cfg.Timeout,
		monthlyLimit:  cfg.MonthlyTokenLimit,
		optOutTenants: make(map[string]bool),
		tenantUsage:   make(map[string]int),
	}
}

// Store returns the underlying proposal storage.
func (s *Service) Store() Store {
	return s.store
}

// Status reports current AI configuration and tenant eligibility.
func (s *Service) Status(tenantID string) AIStatusResponse {
	s.mu.RLock()
	defer s.mu.RUnlock()

	return AIStatusResponse{
		Enabled:        s.enabled,
		Provider:       s.provider.Name(),
		Model:          "gpt-4o",
		TenantID:       tenantID,
		OptedOut:       s.optOutTenants[tenantID],
		MaxTokens:      s.maxTokens,
		TimeoutSeconds: int(s.timeout.Seconds()),
		AvailableTasks: []string{
			string(TaskDraftWorkflow),
			string(TaskMapFields),
			string(TaskGenerateConditions),
			string(TaskSuggestTransformation),
			string(TaskExplainFailure),
		},
	}
}

// SetTenantOptOut enables or disables AI assistance for a specific tenant.
func (s *Service) SetTenantOptOut(tenantID string, optOut bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.optOutTenants[tenantID] = optOut
}

// SetEnabled allows runtime toggling of the global AI feature switch.
func (s *Service) SetEnabled(enabled bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.enabled = enabled
}

func (s *Service) checkAllowed(tenantID string) error {
	s.mu.RLock()
	defer s.mu.RUnlock()

	if !s.enabled {
		return fmt.Errorf("%w: AI assistance is currently disabled by server configuration", ErrAIForbidden)
	}
	if s.optOutTenants[tenantID] {
		return fmt.Errorf("%w: tenant %s has opted out of AI assistance", ErrAIForbidden, tenantID)
	}
	if s.monthlyLimit > 0 && s.tenantUsage[tenantID] >= s.monthlyLimit {
		return fmt.Errorf("ai token usage limit exceeded for tenant %s", tenantID)
	}
	return nil
}

func (s *Service) recordUsage(tenantID string, tokens int) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.tenantUsage[tenantID] += tokens
}

// DraftWorkflow generates a proposed workflow definition from a natural language brief.
func (s *Service) DraftWorkflow(ctx context.Context, tenantID, _ string, req DraftWorkflowRequest) (*Proposal, error) {
	if err := s.checkAllowed(tenantID); err != nil {
		return nil, err
	}

	sanitizedPrompt := SanitizeString(req.Prompt)
	sanitizedContext := SanitizePromptInput(req.Context)

	ctxTimeout, cancel := context.WithTimeout(ctx, s.timeout)
	defer cancel()

	systemPrompt := "You are Kerob Workflow AI. Generate a secure, valid workflow JSON definition. Never generate shell commands, exec actions, or SSRF URLs."
	userPrompt := fmt.Sprintf("Draft a workflow for the following requirement: %s\nAllowed Connectors: %v\nContext: %v", sanitizedPrompt, req.AllowedConnectors, sanitizedContext)

	res, err := s.provider.Generate(ctxTimeout, TaskDraftWorkflow, systemPrompt, userPrompt)
	if err != nil {
		return nil, fmt.Errorf("ai provider error: %w", err)
	}
	s.recordUsage(tenantID, res.TokensUsed)

	validation := ValidateProposal(TaskDraftWorkflow, res.Content, req.AllowedConnectors)

	proposal := &Proposal{
		ID:          uuid.NewString(),
		TenantID:    tenantID,
		Task:        TaskDraftWorkflow,
		Status:      StatusPendingApproval,
		Prompt:      sanitizedPrompt,
		Explanation: res.Explanation,
		Content:     res.Content,
		Validation:  validation,
		ModelUsed:   res.Model,
		TokensUsed:  res.TokensUsed,
		CreatedAt:   time.Now().UTC(),
	}

	return s.store.CreateProposal(ctx, proposal)
}

// MapFields generates a schema-to-schema field mapping proposal.
func (s *Service) MapFields(ctx context.Context, tenantID, _ string, req MapFieldsRequest) (*Proposal, error) {
	if err := s.checkAllowed(tenantID); err != nil {
		return nil, err
	}

	sanitizedSample := SanitizePromptInput(req.SamplePayload)
	sanitizedDesc := SanitizeString(req.Description)

	ctxTimeout, cancel := context.WithTimeout(ctx, s.timeout)
	defer cancel()

	systemPrompt := "You are Kerob Schema AI. Propose a JSON key-to-key field mapping. Keys must be destination field names and values source field paths."
	userPrompt := fmt.Sprintf("Source Schema: %v\nTarget Schema: %v\nSample: %v\nDescription: %s", req.SourceSchema, req.TargetSchema, sanitizedSample, sanitizedDesc)

	res, err := s.provider.Generate(ctxTimeout, TaskMapFields, systemPrompt, userPrompt)
	if err != nil {
		return nil, fmt.Errorf("ai provider error: %w", err)
	}
	s.recordUsage(tenantID, res.TokensUsed)

	validation := ValidateProposal(TaskMapFields, res.Content, nil)

	proposal := &Proposal{
		ID:          uuid.NewString(),
		TenantID:    tenantID,
		Task:        TaskMapFields,
		Status:      StatusPendingApproval,
		Prompt:      sanitizedDesc,
		Explanation: res.Explanation,
		Content:     res.Content,
		Validation:  validation,
		ModelUsed:   res.Model,
		TokensUsed:  res.TokensUsed,
		CreatedAt:   time.Now().UTC(),
	}

	return s.store.CreateProposal(ctx, proposal)
}

// GenerateConditions generates structured, declarative conditions from criteria.
func (s *Service) GenerateConditions(ctx context.Context, tenantID, _ string, req GenerateConditionsRequest) (*Proposal, error) {
	if err := s.checkAllowed(tenantID); err != nil {
		return nil, err
	}

	sanitizedCriteria := SanitizeString(req.Criteria)
	sanitizedSample := SanitizePromptInput(req.SamplePayload)

	ctxTimeout, cancel := context.WithTimeout(ctx, s.timeout)
	defer cancel()

	systemPrompt := "You are Kerob Condition AI. Generate a structured declarative Condition JSON matching the criteria."
	userPrompt := fmt.Sprintf("Criteria: %s\nSample Payload: %v", sanitizedCriteria, sanitizedSample)

	res, err := s.provider.Generate(ctxTimeout, TaskGenerateConditions, systemPrompt, userPrompt)
	if err != nil {
		return nil, fmt.Errorf("ai provider error: %w", err)
	}
	s.recordUsage(tenantID, res.TokensUsed)

	validation := ValidateProposal(TaskGenerateConditions, res.Content, nil)

	proposal := &Proposal{
		ID:          uuid.NewString(),
		TenantID:    tenantID,
		Task:        TaskGenerateConditions,
		Status:      StatusPendingApproval,
		Prompt:      sanitizedCriteria,
		Explanation: res.Explanation,
		Content:     res.Content,
		Validation:  validation,
		ModelUsed:   res.Model,
		TokensUsed:  res.TokensUsed,
		CreatedAt:   time.Now().UTC(),
	}

	return s.store.CreateProposal(ctx, proposal)
}

// SuggestTransformation suggests declarative transformation rules.
func (s *Service) SuggestTransformation(ctx context.Context, tenantID, _ string, req SuggestTransformationRequest) (*Proposal, error) {
	if err := s.checkAllowed(tenantID); err != nil {
		return nil, err
	}

	sanitizedInput := SanitizePromptInput(req.InputPayload)
	sanitizedDesired := SanitizePromptInput(req.DesiredOutput)
	sanitizedGoal := SanitizeString(req.Goal)

	ctxTimeout, cancel := context.WithTimeout(ctx, s.timeout)
	defer cancel()

	systemPrompt := "You are Kerob Transformation AI. Suggest a declarative Transformation JSON definition."
	userPrompt := fmt.Sprintf("Goal: %s\nInput: %v\nDesired Output: %v", sanitizedGoal, sanitizedInput, sanitizedDesired)

	res, err := s.provider.Generate(ctxTimeout, TaskSuggestTransformation, systemPrompt, userPrompt)
	if err != nil {
		return nil, fmt.Errorf("ai provider error: %w", err)
	}
	s.recordUsage(tenantID, res.TokensUsed)

	validation := ValidateProposal(TaskSuggestTransformation, res.Content, nil)

	proposal := &Proposal{
		ID:          uuid.NewString(),
		TenantID:    tenantID,
		Task:        TaskSuggestTransformation,
		Status:      StatusPendingApproval,
		Prompt:      sanitizedGoal,
		Explanation: res.Explanation,
		Content:     res.Content,
		Validation:  validation,
		ModelUsed:   res.Model,
		TokensUsed:  res.TokensUsed,
		CreatedAt:   time.Now().UTC(),
	}

	return s.store.CreateProposal(ctx, proposal)
}

// ExplainFailure analyzes an execution failure and provides root-cause diagnosis and remediation proposals.
func (s *Service) ExplainFailure(ctx context.Context, tenantID, _ string, req ExplainFailureRequest) (*Proposal, error) {
	if err := s.checkAllowed(tenantID); err != nil {
		return nil, err
	}

	sanitizedErr := SanitizeString(req.Error)
	sanitizedInput := SanitizePromptInput(req.StepInput)
	sanitizedOutput := SanitizePromptInput(req.StepOutput)
	sanitizedConfig := SanitizeJSON(req.Config)

	ctxTimeout, cancel := context.WithTimeout(ctx, s.timeout)
	defer cancel()

	systemPrompt := "You are Kerob Failure Analyst AI. Diagnose the failed step and propose actionable remediation steps in JSON."
	userPrompt := fmt.Sprintf("Workflow: %s\nStep: %s (%s)\nError: %s\nInput: %v\nOutput: %v\nConfig: %s",
		req.WorkflowID, req.StepID, req.StepType, sanitizedErr, sanitizedInput, sanitizedOutput, string(sanitizedConfig))

	res, err := s.provider.Generate(ctxTimeout, TaskExplainFailure, systemPrompt, userPrompt)
	if err != nil {
		return nil, fmt.Errorf("ai provider error: %w", err)
	}
	s.recordUsage(tenantID, res.TokensUsed)

	validation := ValidateProposal(TaskExplainFailure, res.Content, nil)

	proposal := &Proposal{
		ID:          uuid.NewString(),
		TenantID:    tenantID,
		Task:        TaskExplainFailure,
		Status:      StatusPendingApproval,
		Prompt:      fmt.Sprintf("Diagnose step %s failure: %s", req.StepID, sanitizedErr),
		Explanation: res.Explanation,
		Content:     res.Content,
		Validation:  validation,
		ModelUsed:   res.Model,
		TokensUsed:  res.TokensUsed,
		CreatedAt:   time.Now().UTC(),
	}

	return s.store.CreateProposal(ctx, proposal)
}

// GetProposal retrieves a proposal by ID.
func (s *Service) GetProposal(ctx context.Context, tenantID, id string) (*Proposal, error) {
	return s.store.GetProposal(ctx, tenantID, id)
}

// ListProposals lists recent proposals for a tenant.
func (s *Service) ListProposals(ctx context.Context, tenantID string, limit, offset int) ([]*Proposal, error) {
	return s.store.ListProposals(ctx, tenantID, limit, offset)
}

// AcceptProposal records explicit human approval of an AI proposal.
func (s *Service) AcceptProposal(ctx context.Context, tenantID, id, actorID string) (*Proposal, error) {
	p, err := s.store.GetProposal(ctx, tenantID, id)
	if err != nil {
		return nil, err
	}
	if !p.Validation.Valid {
		return nil, fmt.Errorf("cannot accept an invalid AI proposal with safety violations: %v", p.Validation.Errors)
	}
	return s.store.UpdateProposalStatus(ctx, tenantID, id, StatusAccepted, actorID)
}

// RejectProposal records explicit human rejection of an AI proposal.
func (s *Service) RejectProposal(ctx context.Context, tenantID, id, actorID string) (*Proposal, error) {
	return s.store.UpdateProposalStatus(ctx, tenantID, id, StatusRejected, actorID)
}
