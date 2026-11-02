package api

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"time"

	"kerob/internal/core/auth"
	"kerob/internal/core/workflow"
)

// ListApprovals lists pending and historical human approvals.
func (h *Handlers) ListApprovals(w http.ResponseWriter, r *http.Request) {
	if h.approvals == nil {
		respondJSON(w, http.StatusOK, []*workflow.Approval{})
		return
	}
	if !h.requirePermission(w, r, auth.PermApprovalsRead) {
		return
	}
	identity := auth.GetIdentity(r.Context())

	status := workflow.ApprovalStatus(r.URL.Query().Get("status"))
	workflowID := r.URL.Query().Get("workflow_id")
	executionID := r.URL.Query().Get("execution_id")

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

	filter := workflow.ApprovalFilter{
		TenantID:    identity.TenantID,
		WorkflowID:  workflowID,
		ExecutionID: executionID,
		Status:      status,
		Limit:       limit,
		Offset:      offset,
	}

	if !h.multiTenancy {
		filter.TenantID = ""
	}

	list, err := h.approvals.List(r.Context(), filter)
	if err != nil {
		respondError(w, http.StatusInternalServerError, err.Error())
		return
	}
	if list == nil {
		list = []*workflow.Approval{}
	}

	respondJSON(w, http.StatusOK, list)
}

// GetApproval retrieves a single approval by ID.
func (h *Handlers) GetApproval(w http.ResponseWriter, r *http.Request) {
	if h.approvals == nil {
		respondError(w, http.StatusInternalServerError, "approval store not initialized")
		return
	}
	if !h.requirePermission(w, r, auth.PermApprovalsRead) {
		return
	}
	identity := auth.GetIdentity(r.Context())

	id := r.PathValue("id")
	app, err := h.approvals.GetByID(r.Context(), id)
	if errors.Is(err, workflow.ErrApprovalNotFound) {
		respondError(w, http.StatusNotFound, "approval not found")
		return
	}
	if err != nil {
		respondError(w, http.StatusInternalServerError, err.Error())
		return
	}

	if h.multiTenancy && app.TenantID != "" && app.TenantID != identity.TenantID {
		h.recordAudit(r, "authorization.cross_tenant_denied", "approval", id, map[string]any{
			"target_tenant": app.TenantID,
		})
		respondErrorWithCode(w, http.StatusForbidden, "CROSS_TENANT_DENIED", "cannot access approval belonging to another tenant", nil)
		return
	}

	respondJSON(w, http.StatusOK, app)
}

type ApprovalDecisionBody struct {
	Note     string `json:"note"`
	Decision string `json:"decision,omitempty"` // "approve" or "reject"
	Approver string `json:"approver,omitempty"`
}

// ApproveApproval records an approval grant from an authorized operator.
func (h *Handlers) ApproveApproval(w http.ResponseWriter, r *http.Request) {
	h.handleApprovalDecision(w, r, workflow.ApprovalApproved)
}

// RejectApproval records an approval rejection and triggers saga compensation if configured.
func (h *Handlers) RejectApproval(w http.ResponseWriter, r *http.Request) {
	h.handleApprovalDecision(w, r, workflow.ApprovalRejected)
}

func (h *Handlers) handleApprovalDecision(w http.ResponseWriter, r *http.Request, targetStatus workflow.ApprovalStatus) {
	if h.approvals == nil {
		respondError(w, http.StatusInternalServerError, "approval store not initialized")
		return
	}
	if !h.requirePermission(w, r, auth.PermApprovalsWrite) {
		return
	}
	identity := auth.GetIdentity(r.Context())

	id := r.PathValue("id")
	app, err := h.approvals.GetByID(r.Context(), id)
	if errors.Is(err, workflow.ErrApprovalNotFound) {
		respondError(w, http.StatusNotFound, "approval not found")
		return
	}
	if err != nil {
		respondError(w, http.StatusInternalServerError, err.Error())
		return
	}

	if h.multiTenancy && app.TenantID != "" && app.TenantID != identity.TenantID {
		respondErrorWithCode(w, http.StatusForbidden, "CROSS_TENANT_DENIED", "cannot decide approval for another tenant", nil)
		return
	}

	// Verify user is authorized according to approver_roles and approver_users
	userRoles := []string{string(identity.Role)}
	if !app.CanApprove(identity.UserID, userRoles) {
		respondErrorWithCode(w, http.StatusForbidden, "UNAUTHORIZED_APPROVER", "user does not possess required role or identity to decide this approval", nil)
		return
	}

	var body ApprovalDecisionBody
	_ = json.NewDecoder(r.Body).Decode(&body)

	decidedBy := identity.UserID
	if decidedBy == "" {
		decidedBy = "authorized_user"
	}

	updated, err := h.approvals.UpdateDecision(r.Context(), id, targetStatus, decidedBy, body.Note)
	if errors.Is(err, workflow.ErrApprovalAlreadyClosed) {
		respondErrorWithCode(w, http.StatusConflict, "ALREADY_CLOSED", "approval has already been decided or closed", nil)
		return
	}
	if err != nil {
		respondError(w, http.StatusInternalServerError, err.Error())
		return
	}

	auditAction := "approval.approved"
	if targetStatus == workflow.ApprovalRejected {
		auditAction = "approval.rejected"
	}
	h.recordAudit(r, auditAction, "approval", app.ID, map[string]any{
		"execution_id": app.ExecutionID,
		"step_id":      app.StepID,
		"workflow_id":  app.WorkflowID,
		"decision":     string(targetStatus),
		"note":         body.Note,
	})

	// Process Execution transition
	if targetStatus == workflow.ApprovalApproved {
		// Reset execution status to pending with immediate attempt timestamp so worker resumes
		if h.executions != nil {
			_ = h.executions.UpdateStatus(r.Context(), app.ExecutionID, workflow.ExecutionPending, nil, 0)
		}
		// Trigger resume asynchronously if engine is available
		if h.workflowEngine != nil && h.workflows != nil && h.events != nil {
			go func() {
				ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
				defer cancel()
				exec, gErr := h.executions.GetByID(ctx, app.ExecutionID)
				if gErr != nil || exec == nil {
					return
				}
				wf, wErr := h.workflows.GetByID(ctx, exec.WorkflowID)
				if wErr != nil || wf == nil {
					return
				}
				ev, eErr := h.events.GetByID(ctx, exec.EventID)
				if eErr != nil || ev == nil {
					return
				}
				_, _ = h.workflowEngine.ExecuteWorkflow(ctx, wf, ev, exec)
			}()
		}
	} else if targetStatus == workflow.ApprovalRejected {
		// Trigger saga compensation or fail execution
		if h.workflowEngine != nil && h.workflows != nil {
			exec, _ := h.executions.GetByID(r.Context(), app.ExecutionID)
			if exec != nil {
				wf, _ := h.workflows.GetByID(r.Context(), exec.WorkflowID)
				if wf != nil && h.workflowEngine.HasCompensableSteps(wf, exec) {
					reason := fmt.Sprintf("human approval rejected by %s: %s", decidedBy, body.Note)
					_, _ = h.workflowEngine.CompensateExecution(r.Context(), exec.ID, reason)
				} else {
					errStr := fmt.Sprintf("approval rejected by %s: %s", decidedBy, body.Note)
					_ = h.executions.UpdateStatus(r.Context(), exec.ID, workflow.ExecutionFailed, &errStr, 0)
				}
			}
		}
	}

	respondJSON(w, http.StatusOK, updated)
}

// GetApprovalByToken retrieves public approval preview using a secure cryptographic token.
func (h *Handlers) GetApprovalByToken(w http.ResponseWriter, r *http.Request) {
	if h.approvals == nil {
		respondError(w, http.StatusInternalServerError, "approval store not initialized")
		return
	}
	token := r.PathValue("token")
	if len(token) < 16 {
		respondError(w, http.StatusBadRequest, "invalid token format")
		return
	}

	tokenHash := workflow.HashApprovalToken(token)
	app, err := h.approvals.GetByTokenHash(r.Context(), tokenHash)
	if errors.Is(err, workflow.ErrApprovalNotFound) {
		respondError(w, http.StatusNotFound, "approval not found or token expired")
		return
	}
	if err != nil {
		respondError(w, http.StatusInternalServerError, err.Error())
		return
	}

	// Safe public preview (no secrets or internal credentials)
	preview := map[string]any{
		"id":             app.ID,
		"title":          app.Title,
		"description":    app.Description,
		"workflow_id":    app.WorkflowID,
		"status":         app.Status,
		"approver_roles": app.ApproverRoles,
		"expires_at":     app.ExpiresAt,
		"created_at":     app.CreatedAt,
		"is_expired":     time.Now().UTC().After(app.ExpiresAt),
	}

	respondJSON(w, http.StatusOK, preview)
}

// DecideApprovalByToken processes an approval or rejection using the secure cryptographic token.
func (h *Handlers) DecideApprovalByToken(w http.ResponseWriter, r *http.Request) {
	if h.approvals == nil {
		respondError(w, http.StatusInternalServerError, "approval store not initialized")
		return
	}
	token := r.PathValue("token")
	if len(token) < 16 {
		respondError(w, http.StatusBadRequest, "invalid token format")
		return
	}

	tokenHash := workflow.HashApprovalToken(token)
	app, err := h.approvals.GetByTokenHash(r.Context(), tokenHash)
	if errors.Is(err, workflow.ErrApprovalNotFound) {
		respondError(w, http.StatusNotFound, "approval token not found")
		return
	}
	if err != nil {
		respondError(w, http.StatusInternalServerError, err.Error())
		return
	}

	if app.Status != workflow.ApprovalPending && app.Status != workflow.ApprovalEscalated {
		respondErrorWithCode(w, http.StatusConflict, "ALREADY_DECIDED", "approval has already been decided", nil)
		return
	}
	if time.Now().UTC().After(app.ExpiresAt) {
		respondErrorWithCode(w, http.StatusGone, "EXPIRED", "approval token has expired", nil)
		return
	}

	var body ApprovalDecisionBody
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		respondError(w, http.StatusBadRequest, "invalid JSON payload")
		return
	}

	var targetStatus workflow.ApprovalStatus
	switch strings.ToLower(body.Decision) {
	case "approve", "approved":
		targetStatus = workflow.ApprovalApproved
	case "reject", "rejected":
		targetStatus = workflow.ApprovalRejected
	default:
		respondError(w, http.StatusBadRequest, "decision must be 'approve' or 'reject'")
		return
	}

	decidedBy := body.Approver
	if decidedBy == "" {
		decidedBy = "token_holder"
	}

	updated, err := h.approvals.UpdateDecision(r.Context(), app.ID, targetStatus, decidedBy, body.Note)
	if err != nil {
		respondError(w, http.StatusInternalServerError, err.Error())
		return
	}

	h.recordAudit(r, "approval.token_decision", "approval", app.ID, map[string]any{
		"execution_id": app.ExecutionID,
		"step_id":      app.StepID,
		"decision":     string(targetStatus),
		"approver":     decidedBy,
	})

	if targetStatus == workflow.ApprovalApproved {
		if h.executions != nil {
			_ = h.executions.UpdateStatus(r.Context(), app.ExecutionID, workflow.ExecutionPending, nil, 0)
		}
		if h.workflowEngine != nil && h.workflows != nil && h.events != nil {
			go func() {
				ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
				defer cancel()
				exec, _ := h.executions.GetByID(ctx, app.ExecutionID)
				if exec == nil {
					return
				}
				wf, _ := h.workflows.GetByID(ctx, exec.WorkflowID)
				if wf == nil {
					return
				}
				ev, _ := h.events.GetByID(ctx, exec.EventID)
				if ev == nil {
					return
				}
				_, _ = h.workflowEngine.ExecuteWorkflow(ctx, wf, ev, exec)
			}()
		}
	} else if targetStatus == workflow.ApprovalRejected {
		if h.workflowEngine != nil && h.workflows != nil {
			exec, _ := h.executions.GetByID(r.Context(), app.ExecutionID)
			if exec != nil {
				wf, _ := h.workflows.GetByID(r.Context(), exec.WorkflowID)
				if wf != nil && h.workflowEngine.HasCompensableSteps(wf, exec) {
					reason := fmt.Sprintf("token approval rejected by %s: %s", decidedBy, body.Note)
					_, _ = h.workflowEngine.CompensateExecution(r.Context(), exec.ID, reason)
				} else {
					errStr := fmt.Sprintf("token approval rejected by %s: %s", decidedBy, body.Note)
					_ = h.executions.UpdateStatus(r.Context(), exec.ID, workflow.ExecutionFailed, &errStr, 0)
				}
			}
		}
	}

	respondJSON(w, http.StatusOK, updated)
}

type CompensateRequestBody struct {
	Reason string `json:"reason"`
}

// CompensateExecution triggers operator-initiated saga rollback for an execution.
func (h *Handlers) CompensateExecution(w http.ResponseWriter, r *http.Request) {
	if h.workflowEngine == nil {
		respondError(w, http.StatusInternalServerError, "workflow engine not initialized")
		return
	}
	if !h.requirePermission(w, r, auth.PermExecutionsCompensate) {
		return
	}
	identity := auth.GetIdentity(r.Context())

	id := r.PathValue("id")
	exec, err := h.executions.GetByID(r.Context(), id)
	if errors.Is(err, workflow.ErrExecutionNotFound) {
		respondError(w, http.StatusNotFound, "execution not found")
		return
	}
	if err != nil {
		respondError(w, http.StatusInternalServerError, err.Error())
		return
	}

	if h.multiTenancy && exec.TenantID != "" && exec.TenantID != identity.TenantID {
		respondErrorWithCode(w, http.StatusForbidden, "CROSS_TENANT_DENIED", "cannot compensate execution belonging to another tenant", nil)
		return
	}

	var body CompensateRequestBody
	_ = json.NewDecoder(r.Body).Decode(&body)
	if body.Reason == "" {
		body.Reason = fmt.Sprintf("Operator manual rollback requested by %s", identity.UserID)
	}

	compensated, err := h.workflowEngine.CompensateExecution(r.Context(), id, body.Reason)
	if err != nil {
		respondError(w, http.StatusInternalServerError, fmt.Sprintf("compensation failed: %v", err))
		return
	}

	h.recordAudit(r, "execution.compensated", "execution", id, map[string]any{
		"workflow_id": exec.WorkflowID,
		"reason":      body.Reason,
		"status":      string(compensated.Status),
	})

	respondJSON(w, http.StatusOK, compensated)
}
