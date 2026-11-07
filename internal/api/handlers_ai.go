package api

import (
	"encoding/json"
	"errors"
	"net/http"
	"strconv"
	"strings"

	"kerob/internal/core/ai"
	"kerob/internal/core/auth"
	"kerob/internal/core/workflow"
)

// GetAIStatus returns the configuration, operational status, and quota usage of the AI subsystem.
func (h *Handlers) GetAIStatus(w http.ResponseWriter, r *http.Request) {
	if !h.requirePermission(w, r, auth.PermAIUse) && !h.requirePermission(w, r, auth.PermAIAdmin) && !h.requirePermission(w, r, auth.PermWorkflowsRead) {
		return
	}
	identity := auth.GetIdentity(r.Context())
	tenantID := identity.TenantID
	if tenantID == "" {
		tenantID = "default"
	}

	if h.aiService == nil {
		respondJSON(w, http.StatusOK, map[string]any{
			"enabled":   false,
			"opted_out": false,
			"provider":  "none",
			"model":     "",
			"message":   "AI assistance layer is not configured",
		})
		return
	}

	status := h.aiService.Status(tenantID)
	respondJSON(w, http.StatusOK, status)
}

// DraftWorkflowAI handles AI-assisted drafting of a workflow schema from natural language instructions.
func (h *Handlers) DraftWorkflowAI(w http.ResponseWriter, r *http.Request) {
	if !h.requirePermission(w, r, auth.PermAIUse) {
		return
	}
	identity := auth.GetIdentity(r.Context())
	tenantID := identity.TenantID
	if tenantID == "" {
		tenantID = "default"
	}

	var req ai.DraftWorkflowRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		respondErrorWithCode(w, http.StatusBadRequest, "MALFORMED_JSON", "invalid json: "+err.Error(), nil)
		return
	}

	proposal, err := h.aiService.DraftWorkflow(r.Context(), tenantID, identity.UserID, req)
	if err != nil {
		h.handleAIError(w, err)
		return
	}

	h.recordAudit(r, "ai.draft_workflow", "ai_proposal", proposal.ID, map[string]any{
		"task":   proposal.Task,
		"valid":  proposal.Validation.Valid,
		"prompt": proposal.Prompt,
	})

	respondJSON(w, http.StatusCreated, proposal)
}

// MapFieldsAI generates candidate payload field transformations between source schemas and target destinations.
func (h *Handlers) MapFieldsAI(w http.ResponseWriter, r *http.Request) {
	if !h.requirePermission(w, r, auth.PermAIUse) {
		return
	}
	identity := auth.GetIdentity(r.Context())
	tenantID := identity.TenantID
	if tenantID == "" {
		tenantID = "default"
	}

	var req ai.MapFieldsRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		respondErrorWithCode(w, http.StatusBadRequest, "MALFORMED_JSON", "invalid json: "+err.Error(), nil)
		return
	}

	proposal, err := h.aiService.MapFields(r.Context(), tenantID, identity.UserID, req)
	if err != nil {
		h.handleAIError(w, err)
		return
	}

	h.recordAudit(r, "ai.map_fields", "ai_proposal", proposal.ID, map[string]any{
		"task":  proposal.Task,
		"valid": proposal.Validation.Valid,
	})

	respondJSON(w, http.StatusCreated, proposal)
}

// GenerateConditionsAI generates deterministic step filter conditions from user intent.
func (h *Handlers) GenerateConditionsAI(w http.ResponseWriter, r *http.Request) {
	if !h.requirePermission(w, r, auth.PermAIUse) {
		return
	}
	identity := auth.GetIdentity(r.Context())
	tenantID := identity.TenantID
	if tenantID == "" {
		tenantID = "default"
	}

	var req ai.GenerateConditionsRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		respondErrorWithCode(w, http.StatusBadRequest, "MALFORMED_JSON", "invalid json: "+err.Error(), nil)
		return
	}

	proposal, err := h.aiService.GenerateConditions(r.Context(), tenantID, identity.UserID, req)
	if err != nil {
		h.handleAIError(w, err)
		return
	}

	h.recordAudit(r, "ai.generate_conditions", "ai_proposal", proposal.ID, map[string]any{
		"task":  proposal.Task,
		"valid": proposal.Validation.Valid,
	})

	respondJSON(w, http.StatusCreated, proposal)
}

// SuggestTransformationsAI suggests payload filters, transformations, or field rewrites.
func (h *Handlers) SuggestTransformationsAI(w http.ResponseWriter, r *http.Request) {
	if !h.requirePermission(w, r, auth.PermAIUse) {
		return
	}
	identity := auth.GetIdentity(r.Context())
	tenantID := identity.TenantID
	if tenantID == "" {
		tenantID = "default"
	}

	var req ai.SuggestTransformationRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		respondErrorWithCode(w, http.StatusBadRequest, "MALFORMED_JSON", "invalid json: "+err.Error(), nil)
		return
	}

	proposal, err := h.aiService.SuggestTransformation(r.Context(), tenantID, identity.UserID, req)
	if err != nil {
		h.handleAIError(w, err)
		return
	}

	h.recordAudit(r, "ai.suggest_transformations", "ai_proposal", proposal.ID, map[string]any{
		"task":  proposal.Task,
		"valid": proposal.Validation.Valid,
	})

	respondJSON(w, http.StatusCreated, proposal)
}

// ExplainFailureAI analyzes execution errors, dead letters, and trace logs to produce safe human explanations and remediation steps.
func (h *Handlers) ExplainFailureAI(w http.ResponseWriter, r *http.Request) {
	if !h.requirePermission(w, r, auth.PermAIUse) {
		return
	}
	identity := auth.GetIdentity(r.Context())
	tenantID := identity.TenantID
	if tenantID == "" {
		tenantID = "default"
	}

	var req ai.ExplainFailureRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		respondErrorWithCode(w, http.StatusBadRequest, "MALFORMED_JSON", "invalid json: "+err.Error(), nil)
		return
	}

	proposal, err := h.aiService.ExplainFailure(r.Context(), tenantID, identity.UserID, req)
	if err != nil {
		h.handleAIError(w, err)
		return
	}

	h.recordAudit(r, "ai.explain_failure", "ai_proposal", proposal.ID, map[string]any{
		"task":         proposal.Task,
		"execution_id": req.ExecutionID,
		"valid":        proposal.Validation.Valid,
	})

	respondJSON(w, http.StatusCreated, proposal)
}

// ListAIProposals queries all proposals generated for the caller tenant.
func (h *Handlers) ListAIProposals(w http.ResponseWriter, r *http.Request) {
	if !h.requirePermission(w, r, auth.PermAIUse) {
		return
	}
	identity := auth.GetIdentity(r.Context())
	tenantID := identity.TenantID
	if tenantID == "" {
		tenantID = "default"
	}

	limit := 50
	if l := r.URL.Query().Get("limit"); l != "" {
		if parsed, err := strconv.Atoi(l); err == nil && parsed > 0 {
			limit = parsed
		}
	}
	offset := 0
	if o := r.URL.Query().Get("offset"); o != "" {
		if parsed, err := strconv.Atoi(o); err == nil && parsed >= 0 {
			offset = parsed
		}
	}

	proposals, err := h.aiService.ListProposals(r.Context(), tenantID, limit, offset)
	if err != nil {
		respondError(w, http.StatusInternalServerError, "failed to list proposals: "+err.Error())
		return
	}

	if proposals == nil {
		proposals = []*ai.Proposal{}
	}

	respondJSON(w, http.StatusOK, proposals)
}

// GetAIProposal retrieves a single proposal by ID.
func (h *Handlers) GetAIProposal(w http.ResponseWriter, r *http.Request) {
	if !h.requirePermission(w, r, auth.PermAIUse) {
		return
	}
	identity := auth.GetIdentity(r.Context())
	tenantID := identity.TenantID
	if tenantID == "" {
		tenantID = "default"
	}

	id := r.PathValue("id")
	if id == "" {
		respondError(w, http.StatusBadRequest, "proposal id is required")
		return
	}

	p, err := h.aiService.GetProposal(r.Context(), tenantID, id)
	if err != nil {
		if errors.Is(err, ai.ErrProposalNotFound) {
			respondErrorWithCode(w, http.StatusNotFound, "NOT_FOUND", "proposal not found", nil)
			return
		}
		respondError(w, http.StatusInternalServerError, "failed to get proposal: "+err.Error())
		return
	}

	respondJSON(w, http.StatusOK, p)
}

// AcceptAIProposal accepts a proposed change and optionally creates or updates the target resource.
// Strictly requires PermWorkflowsWrite to ensure autonomous model output is never applied without human authorization.
func (h *Handlers) AcceptAIProposal(w http.ResponseWriter, r *http.Request) {
	if !h.requirePermission(w, r, auth.PermWorkflowsWrite) {
		return
	}
	identity := auth.GetIdentity(r.Context())
	tenantID := identity.TenantID
	if tenantID == "" {
		tenantID = "default"
	}

	id := r.PathValue("id")
	if id == "" {
		respondError(w, http.StatusBadRequest, "proposal id is required")
		return
	}

	type acceptOptions struct {
		CreateWorkflow bool `json:"create_workflow"`
	}
	var opts acceptOptions
	if r.Body != nil {
		_ = json.NewDecoder(r.Body).Decode(&opts)
	}
	if r.URL.Query().Get("create_workflow") == "true" {
		opts.CreateWorkflow = true
	}

	p, err := h.aiService.AcceptProposal(r.Context(), tenantID, id, identity.UserID)
	if err != nil {
		if errors.Is(err, ai.ErrProposalNotFound) {
			respondErrorWithCode(w, http.StatusNotFound, "NOT_FOUND", "proposal not found", nil)
			return
		}
		if errors.Is(err, ai.ErrProposalDecided) {
			respondErrorWithCode(w, http.StatusConflict, "ALREADY_DECIDED", "proposal already decided", nil)
			return
		}
		respondError(w, http.StatusBadRequest, "failed to accept proposal: "+err.Error())
		return
	}

	// If requested and the proposal contains a valid drafted workflow, instantiate it
	var createdWorkflowID string
	if opts.CreateWorkflow && p.Task == ai.TaskDraftWorkflow && len(p.Content) > 0 && h.workflows != nil {
		var wf workflow.Workflow
		if err := json.Unmarshal(p.Content, &wf); err == nil && wf.Name != "" {
			wf.TenantID = tenantID
			if createdWf, err := h.workflows.Create(r.Context(), &wf); err == nil && createdWf != nil {
				createdWorkflowID = createdWf.ID
			}
		}
	}

	h.recordAudit(r, "ai.proposal_accepted", "ai_proposal", p.ID, map[string]any{
		"task":                p.Task,
		"applied_workflow_id": createdWorkflowID,
	})

	respondJSON(w, http.StatusOK, map[string]any{
		"proposal":            p,
		"applied_workflow_id": createdWorkflowID,
	})
}

// RejectAIProposal rejects an AI proposed change with an optional feedback note.
func (h *Handlers) RejectAIProposal(w http.ResponseWriter, r *http.Request) {
	if !h.requirePermission(w, r, auth.PermAIUse) {
		return
	}
	identity := auth.GetIdentity(r.Context())
	tenantID := identity.TenantID
	if tenantID == "" {
		tenantID = "default"
	}

	id := r.PathValue("id")
	if id == "" {
		respondError(w, http.StatusBadRequest, "proposal id is required")
		return
	}

	p, err := h.aiService.RejectProposal(r.Context(), tenantID, id, identity.UserID)
	if err != nil {
		if errors.Is(err, ai.ErrProposalNotFound) {
			respondErrorWithCode(w, http.StatusNotFound, "NOT_FOUND", "proposal not found", nil)
			return
		}
		if errors.Is(err, ai.ErrProposalDecided) {
			respondErrorWithCode(w, http.StatusConflict, "ALREADY_DECIDED", "proposal already decided", nil)
			return
		}
		respondError(w, http.StatusInternalServerError, "failed to reject proposal: "+err.Error())
		return
	}

	h.recordAudit(r, "ai.proposal_rejected", "ai_proposal", p.ID, map[string]any{
		"task": p.Task,
	})

	respondJSON(w, http.StatusOK, p)
}

// handleAIError maps AI errors to appropriate HTTP responses.
func (h *Handlers) handleAIError(w http.ResponseWriter, err error) {
	if errors.Is(err, ai.ErrAIForbidden) {
		respondErrorWithCode(w, http.StatusForbidden, "AI_FORBIDDEN", err.Error(), nil)
		return
	}
	if strings.Contains(err.Error(), "token usage limit exceeded") {
		respondErrorWithCode(w, http.StatusTooManyRequests, "MONTHLY_LIMIT_EXCEEDED", err.Error(), nil)
		return
	}
	if strings.Contains(err.Error(), "timeout") || strings.Contains(err.Error(), "deadline exceeded") {
		respondErrorWithCode(w, http.StatusGatewayTimeout, "AI_TIMEOUT", "AI provider request timed out", nil)
		return
	}
	respondError(w, http.StatusInternalServerError, "AI assistance request failed: "+err.Error())
}
