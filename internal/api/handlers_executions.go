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
		go func() {
			ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
			defer cancel()
			wf, err := h.workflows.GetByID(ctx, exec.WorkflowID)
			if err != nil {
				slog.ErrorContext(ctx, "retry failed to load workflow",
					slog.String("execution_id", exec.ID),
					slog.String("workflow_id", exec.WorkflowID),
					slog.String("error", err.Error()),
				)
				return
			}
			ev, err := h.events.GetByID(ctx, exec.EventID)
			if err != nil {
				slog.ErrorContext(ctx, "retry failed to load event",
					slog.String("execution_id", exec.ID),
					slog.String("event_id", exec.EventID),
					slog.String("error", err.Error()),
				)
				return
			}
			if _, err := h.workflowEngine.ExecuteWorkflow(ctx, wf, ev, exec); err != nil {
				slog.ErrorContext(ctx, "retry execution failed",
					slog.String("execution_id", exec.ID),
					slog.String("workflow_id", exec.WorkflowID),
					slog.String("error", err.Error()),
				)
			}
		}()
	}

	respondJSON(w, http.StatusOK, exec)
}
