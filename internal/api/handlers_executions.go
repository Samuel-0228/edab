package api

import (
	"context"
	"errors"
	"log/slog"
	"net/http"
	"strconv"
	"time"

	"kerob/internal/core/auth"
	"kerob/internal/core/workflow"
)

func (h *Handlers) ListExecutions(w http.ResponseWriter, r *http.Request) {
	if h.executions == nil {
		respondJSON(w, http.StatusOK, []*workflow.Execution{})
		return
	}
	if !h.requirePermission(w, r, auth.PermExecutionsRead) {
		return
	}
	identity := auth.GetIdentity(r.Context())

	workflowID := r.URL.Query().Get("workflow_id")
	status := workflow.ExecutionStatus(r.URL.Query().Get("status"))

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

	filter := workflow.ExecutionFilter{
		WorkflowID: workflowID,
		Status:     status,
		Limit:      limit,
		Offset:     offset,
	}

	list, err := h.executions.List(r.Context(), filter)
	if err != nil {
		respondError(w, http.StatusInternalServerError, err.Error())
		return
	}
	if list == nil {
		list = []*workflow.Execution{}
	}

	if h.multiTenancy {
		filtered := make([]*workflow.Execution, 0, len(list))
		for _, item := range list {
			itemTenant := item.TenantID
			if itemTenant == "" && h.workflows != nil {
				if wf, err := h.workflows.GetByID(r.Context(), item.WorkflowID); err == nil && wf != nil {
					itemTenant = wf.TenantID
				}
			}
			if itemTenant == "" || itemTenant == identity.TenantID {
				filtered = append(filtered, item)
			}
		}
		list = filtered
	}

	respondJSON(w, http.StatusOK, list)
}

func (h *Handlers) GetExecution(w http.ResponseWriter, r *http.Request) {
	if h.executions == nil {
		respondError(w, http.StatusInternalServerError, "execution store not initialized")
		return
	}
	if !h.requirePermission(w, r, auth.PermExecutionsRead) {
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

	execTenant := exec.TenantID
	if execTenant == "" && h.workflows != nil {
		if wf, err := h.workflows.GetByID(r.Context(), exec.WorkflowID); err == nil && wf != nil {
			execTenant = wf.TenantID
		}
	}

	if h.multiTenancy && execTenant != "" && execTenant != identity.TenantID {
		respondErrorWithCode(w, http.StatusForbidden, "CROSS_TENANT_DENIED", "cannot access execution belonging to another tenant", nil)
		return
	}

	respondJSON(w, http.StatusOK, exec)
}

func (h *Handlers) RetryExecution(w http.ResponseWriter, r *http.Request) {
	if h.executions == nil {
		respondError(w, http.StatusInternalServerError, "execution store not initialized")
		return
	}
	if !h.requirePermission(w, r, auth.PermExecutionsRetry) {
		return
	}
	identity := auth.GetIdentity(r.Context())

	id := r.PathValue("id")
	existing, err := h.executions.GetByID(r.Context(), id)
	if errors.Is(err, workflow.ErrExecutionNotFound) {
		respondError(w, http.StatusNotFound, "execution not found")
		return
	}
	if err != nil {
		respondError(w, http.StatusInternalServerError, err.Error())
		return
	}

	execTenant := existing.TenantID
	if execTenant == "" && h.workflows != nil {
		if wf, err := h.workflows.GetByID(r.Context(), existing.WorkflowID); err == nil && wf != nil {
			execTenant = wf.TenantID
		}
	}

	if h.multiTenancy && execTenant != "" && execTenant != identity.TenantID {
		respondErrorWithCode(w, http.StatusForbidden, "CROSS_TENANT_DENIED", "cannot retry execution belonging to another tenant", nil)
		return
	}

	exec, err := h.executions.ResetForRetry(r.Context(), id)
	if errors.Is(err, workflow.ErrExecutionNotFound) {
		respondError(w, http.StatusNotFound, "execution not found")
		return
	}
	if err != nil {
		respondError(w, http.StatusInternalServerError, err.Error())
		return
	}

	h.recordAudit(r, "execution.retried", "execution", exec.ID, map[string]any{
		"workflow_id": exec.WorkflowID,
		"event_id":    exec.EventID,
	})

	// If engine is present, immediately execute retry asynchronously or synchronously
	if h.workflowEngine != nil && h.workflows != nil && h.events != nil {
		execClone := *exec
		go func() {
			ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
			defer cancel()
			wf, err := h.workflows.GetByID(ctx, execClone.WorkflowID)
			if err != nil {
				slog.ErrorContext(ctx, "retry failed to load workflow",
					slog.String("execution_id", execClone.ID),
					slog.String("workflow_id", execClone.WorkflowID),
					slog.String("error", err.Error()),
				)
				return
			}
			ev, err := h.events.GetByID(ctx, execClone.EventID)
			if err != nil {
				slog.ErrorContext(ctx, "retry failed to load event",
					slog.String("execution_id", execClone.ID),
					slog.String("event_id", execClone.EventID),
					slog.String("error", err.Error()),
				)
				return
			}
			if _, err := h.workflowEngine.ExecuteWorkflow(ctx, wf, ev, &execClone); err != nil {
				slog.ErrorContext(ctx, "retry execution failed",
					slog.String("execution_id", execClone.ID),
					slog.String("workflow_id", execClone.WorkflowID),
					slog.String("error", err.Error()),
				)
			}
		}()
	}

	respondJSON(w, http.StatusOK, exec)
}

func (h *Handlers) CancelExecution(w http.ResponseWriter, r *http.Request) {
	if h.executions == nil {
		respondError(w, http.StatusInternalServerError, "execution store not initialized")
		return
	}
	identity := auth.GetIdentity(r.Context())
	if !identity.HasPermission(auth.PermExecutionsCancel) && !identity.HasPermission(auth.PermExecutionsRetry) && !identity.HasPermission(auth.PermWorkflowsExecute) {
		h.recordAudit(r, "authorization.denied", "execution", r.PathValue("id"), map[string]any{
			"required_permission": auth.PermExecutionsCancel,
		})
		respondErrorWithCode(w, http.StatusForbidden, "FORBIDDEN", "permission denied: executions:cancel required", nil)
		return
	}

	id := r.PathValue("id")
	existing, err := h.executions.GetByID(r.Context(), id)
	if errors.Is(err, workflow.ErrExecutionNotFound) {
		respondError(w, http.StatusNotFound, "execution not found")
		return
	}
	if err != nil {
		respondError(w, http.StatusInternalServerError, err.Error())
		return
	}

	execTenant := existing.TenantID
	if execTenant == "" && h.workflows != nil {
		if wf, err := h.workflows.GetByID(r.Context(), existing.WorkflowID); err == nil && wf != nil {
			execTenant = wf.TenantID
		}
	}

	if h.multiTenancy && execTenant != "" && execTenant != identity.TenantID {
		h.recordAudit(r, "authorization.cross_tenant_denied", "execution", id, map[string]any{
			"target_tenant": execTenant,
		})
		respondErrorWithCode(w, http.StatusForbidden, "CROSS_TENANT_DENIED", "cannot cancel execution belonging to another tenant", nil)
		return
	}

	var exec *workflow.Execution
	if h.workflowEngine != nil {
		exec, err = h.workflowEngine.CancelExecution(r.Context(), id)
	} else {
		exec, err = h.executions.CancelExecution(r.Context(), id)
	}
	if errors.Is(err, workflow.ErrExecutionNotFound) {
		respondError(w, http.StatusNotFound, "execution not found")
		return
	}
	if errors.Is(err, workflow.ErrExecutionInvalidState) {
		respondErrorWithCode(w, http.StatusConflict, "INVALID_STATE", err.Error(), nil)
		return
	}
	if err != nil {
		respondError(w, http.StatusInternalServerError, err.Error())
		return
	}

	if h.liveHub != nil {
		h.liveHub.OnExecutionCancelled(exec)
	}

	h.recordAudit(r, "execution.cancelled", "execution", exec.ID, map[string]any{
		"workflow_id": exec.WorkflowID,
		"event_id":    exec.EventID,
	})

	respondJSON(w, http.StatusOK, exec)
}

func (h *Handlers) PauseExecution(w http.ResponseWriter, r *http.Request) {
	if h.executions == nil {
		respondError(w, http.StatusInternalServerError, "execution store not initialized")
		return
	}
	identity := auth.GetIdentity(r.Context())
	if !identity.HasPermission(auth.PermExecutionsPause) && !identity.HasPermission(auth.PermExecutionsRetry) && !identity.HasPermission(auth.PermWorkflowsExecute) {
		h.recordAudit(r, "authorization.denied", "execution", r.PathValue("id"), map[string]any{
			"required_permission": auth.PermExecutionsPause,
		})
		respondErrorWithCode(w, http.StatusForbidden, "FORBIDDEN", "permission denied: executions:pause required", nil)
		return
	}

	id := r.PathValue("id")
	existing, err := h.executions.GetByID(r.Context(), id)
	if errors.Is(err, workflow.ErrExecutionNotFound) {
		respondError(w, http.StatusNotFound, "execution not found")
		return
	}
	if err != nil {
		respondError(w, http.StatusInternalServerError, err.Error())
		return
	}

	execTenant := existing.TenantID
	if execTenant == "" && h.workflows != nil {
		if wf, err := h.workflows.GetByID(r.Context(), existing.WorkflowID); err == nil && wf != nil {
			execTenant = wf.TenantID
		}
	}

	if h.multiTenancy && execTenant != "" && execTenant != identity.TenantID {
		h.recordAudit(r, "authorization.cross_tenant_denied", "execution", id, map[string]any{
			"target_tenant": execTenant,
		})
		respondErrorWithCode(w, http.StatusForbidden, "CROSS_TENANT_DENIED", "cannot pause execution belonging to another tenant", nil)
		return
	}

	var exec *workflow.Execution
	if h.workflowEngine != nil {
		exec, err = h.workflowEngine.PauseExecution(r.Context(), id)
	} else {
		exec, err = h.executions.PauseExecution(r.Context(), id)
	}
	if errors.Is(err, workflow.ErrExecutionNotFound) {
		respondError(w, http.StatusNotFound, "execution not found")
		return
	}
	if errors.Is(err, workflow.ErrExecutionInvalidState) {
		respondErrorWithCode(w, http.StatusConflict, "INVALID_STATE", err.Error(), nil)
		return
	}
	if err != nil {
		respondError(w, http.StatusInternalServerError, err.Error())
		return
	}

	if h.liveHub != nil {
		h.liveHub.OnExecutionPaused(exec)
	}

	h.recordAudit(r, "execution.paused", "execution", exec.ID, map[string]any{
		"workflow_id": exec.WorkflowID,
		"event_id":    exec.EventID,
	})

	respondJSON(w, http.StatusOK, exec)
}

func (h *Handlers) ResumeExecution(w http.ResponseWriter, r *http.Request) {
	if h.executions == nil {
		respondError(w, http.StatusInternalServerError, "execution store not initialized")
		return
	}
	identity := auth.GetIdentity(r.Context())
	if !identity.HasPermission(auth.PermExecutionsRetry) && !identity.HasPermission(auth.PermExecutionsPause) && !identity.HasPermission(auth.PermWorkflowsExecute) {
		h.recordAudit(r, "authorization.denied", "execution", r.PathValue("id"), map[string]any{
			"required_permission": auth.PermExecutionsRetry,
		})
		respondErrorWithCode(w, http.StatusForbidden, "FORBIDDEN", "permission denied: executions:retry required", nil)
		return
	}

	id := r.PathValue("id")
	existing, err := h.executions.GetByID(r.Context(), id)
	if errors.Is(err, workflow.ErrExecutionNotFound) {
		respondError(w, http.StatusNotFound, "execution not found")
		return
	}
	if err != nil {
		respondError(w, http.StatusInternalServerError, err.Error())
		return
	}

	execTenant := existing.TenantID
	if execTenant == "" && h.workflows != nil {
		if wf, err := h.workflows.GetByID(r.Context(), existing.WorkflowID); err == nil && wf != nil {
			execTenant = wf.TenantID
		}
	}

	if h.multiTenancy && execTenant != "" && execTenant != identity.TenantID {
		h.recordAudit(r, "authorization.cross_tenant_denied", "execution", id, map[string]any{
			"target_tenant": execTenant,
		})
		respondErrorWithCode(w, http.StatusForbidden, "CROSS_TENANT_DENIED", "cannot resume execution belonging to another tenant", nil)
		return
	}

	var exec *workflow.Execution
	if h.workflowEngine != nil {
		exec, err = h.workflowEngine.ResumeExecution(r.Context(), id)
	} else {
		exec, err = h.executions.ResumeExecution(r.Context(), id)
	}
	if errors.Is(err, workflow.ErrExecutionNotFound) {
		respondError(w, http.StatusNotFound, "execution not found")
		return
	}
	if errors.Is(err, workflow.ErrExecutionInvalidState) {
		respondErrorWithCode(w, http.StatusConflict, "INVALID_STATE", err.Error(), nil)
		return
	}
	if err != nil {
		respondError(w, http.StatusInternalServerError, err.Error())
		return
	}

	if h.liveHub != nil {
		h.liveHub.OnExecutionResumed(exec)
	}

	h.recordAudit(r, "execution.resumed", "execution", exec.ID, map[string]any{
		"workflow_id": exec.WorkflowID,
		"event_id":    exec.EventID,
	})

	// If engine is present, immediately execute resume asynchronously
	if h.workflowEngine != nil && h.workflows != nil && h.events != nil {
		execClone := *exec
		go func() {
			ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
			defer cancel()
			wf, err := h.workflows.GetByID(ctx, execClone.WorkflowID)
			if err != nil {
				return
			}
			ev, err := h.events.GetByID(ctx, execClone.EventID)
			if err != nil {
				return
			}
			_, _ = h.workflowEngine.ExecuteWorkflow(ctx, wf, ev, &execClone)
		}()
	}

	respondJSON(w, http.StatusOK, exec)
}
