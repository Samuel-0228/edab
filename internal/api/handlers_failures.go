package api

import (
	"context"
	"errors"
	"log/slog"
	"net/http"
	"time"

	"kerob/internal/core/auth"
	"kerob/internal/core/workflow"
)

func (h *Handlers) ListFailures(w http.ResponseWriter, r *http.Request) {
	if h.deadLetters == nil {
		respondJSON(w, http.StatusOK, []*workflow.DeadLetterRecord{})
		return
	}
	if !h.requirePermission(w, r, auth.PermDLQRead) {
		return
	}
	identity := auth.GetIdentity(r.Context())

	includeDiscarded := r.URL.Query().Get("include_discarded") == "true"
	list, err := h.deadLetters.List(r.Context(), includeDiscarded)
	if err != nil {
		respondError(w, http.StatusInternalServerError, err.Error())
		return
	}
	if list == nil {
		list = []*workflow.DeadLetterRecord{}
	}

	if h.multiTenancy {
		filtered := make([]*workflow.DeadLetterRecord, 0, len(list))
		for _, rec := range list {
			recTenant := rec.TenantID
			if recTenant == "" && h.workflows != nil {
				if wf, err := h.workflows.GetByID(r.Context(), rec.WorkflowID); err == nil && wf != nil {
					recTenant = wf.TenantID
				}
			}
			if recTenant == "" || recTenant == identity.TenantID {
				filtered = append(filtered, rec)
			}
		}
		list = filtered
	}

	respondJSON(w, http.StatusOK, list)
}

func (h *Handlers) GetFailure(w http.ResponseWriter, r *http.Request) {
	if h.deadLetters == nil {
		respondError(w, http.StatusInternalServerError, "dead-letter store not initialized")
		return
	}
	if !h.requirePermission(w, r, auth.PermDLQRead) {
		return
	}
	identity := auth.GetIdentity(r.Context())

	id := r.PathValue("id")
	rec, err := h.deadLetters.GetByID(r.Context(), id)
	if errors.Is(err, workflow.ErrDeadLetterNotFound) {
		respondError(w, http.StatusNotFound, "failure record not found")
		return
	}
	if err != nil {
		respondError(w, http.StatusInternalServerError, err.Error())
		return
	}

	recTenant := rec.TenantID
	if recTenant == "" && h.workflows != nil {
		if wf, err := h.workflows.GetByID(r.Context(), rec.WorkflowID); err == nil && wf != nil {
			recTenant = wf.TenantID
		}
	}
	if h.multiTenancy && recTenant != "" && recTenant != identity.TenantID {
		respondErrorWithCode(w, http.StatusForbidden, "CROSS_TENANT_DENIED", "cannot access failure belonging to another tenant", nil)
		return
	}

	respondJSON(w, http.StatusOK, rec)
}

func (h *Handlers) DiscardFailure(w http.ResponseWriter, r *http.Request) {
	if h.deadLetters == nil {
		respondError(w, http.StatusInternalServerError, "dead-letter store not initialized")
		return
	}
	if !h.requirePermission(w, r, auth.PermDLQRetry) {
		return
	}
	identity := auth.GetIdentity(r.Context())

	id := r.PathValue("id")
	rec, err := h.deadLetters.GetByID(r.Context(), id)
	if errors.Is(err, workflow.ErrDeadLetterNotFound) {
		respondError(w, http.StatusNotFound, "failure record not found")
		return
	}
	if err != nil {
		respondError(w, http.StatusInternalServerError, err.Error())
		return
	}

	recTenant := rec.TenantID
	if recTenant == "" && h.workflows != nil {
		if wf, err := h.workflows.GetByID(r.Context(), rec.WorkflowID); err == nil && wf != nil {
			recTenant = wf.TenantID
		}
	}
	if h.multiTenancy && recTenant != "" && recTenant != identity.TenantID {
		respondErrorWithCode(w, http.StatusForbidden, "CROSS_TENANT_DENIED", "cannot discard failure belonging to another tenant", nil)
		return
	}

	err = h.deadLetters.Discard(r.Context(), id)
	if errors.Is(err, workflow.ErrDeadLetterNotFound) {
		respondError(w, http.StatusNotFound, "failure record not found")
		return
	}
	if err != nil {
		respondError(w, http.StatusInternalServerError, err.Error())
		return
	}

	h.recordAudit(r, "dlq.discarded", "dead_letter", id, map[string]any{
		"execution_id": rec.ExecutionID,
		"workflow_id":  rec.WorkflowID,
	})

	respondJSON(w, http.StatusOK, map[string]string{"status": "discarded"})
}

func (h *Handlers) RetryFailure(w http.ResponseWriter, r *http.Request) {
	if h.deadLetters == nil || h.executions == nil {
		respondError(w, http.StatusInternalServerError, "stores not initialized")
		return
	}
	if !h.requirePermission(w, r, auth.PermDLQRetry) {
		return
	}
	identity := auth.GetIdentity(r.Context())

	id := r.PathValue("id")
	rec, err := h.deadLetters.GetByID(r.Context(), id)
	if errors.Is(err, workflow.ErrDeadLetterNotFound) {
		respondError(w, http.StatusNotFound, "failure record not found")
		return
	}
	if err != nil {
		respondError(w, http.StatusInternalServerError, err.Error())
		return
	}

	recTenant := rec.TenantID
	if recTenant == "" && h.workflows != nil {
		if wf, err := h.workflows.GetByID(r.Context(), rec.WorkflowID); err == nil && wf != nil {
			recTenant = wf.TenantID
		}
	}
	if h.multiTenancy && recTenant != "" && recTenant != identity.TenantID {
		respondErrorWithCode(w, http.StatusForbidden, "CROSS_TENANT_DENIED", "cannot retry failure belonging to another tenant", nil)
		return
	}

	// Reset corresponding execution for retry
	exec, err := h.executions.ResetForRetry(r.Context(), rec.ExecutionID)
	if err != nil {
		respondError(w, http.StatusInternalServerError, err.Error())
		return
	}

	// Mark dead letter as discarded/resolved
	if err := h.deadLetters.Discard(r.Context(), id); err != nil {
		respondError(w, http.StatusInternalServerError, "failed to resolve failure")
		return
	}

	h.recordAudit(r, "dlq.retried", "dead_letter", id, map[string]any{
		"execution_id": rec.ExecutionID,
		"workflow_id":  rec.WorkflowID,
	})

	// Trigger immediate retry execution if engine is active
	if h.workflowEngine != nil && h.workflows != nil && h.events != nil {
		go func() {
			ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
			defer cancel()
			wf, err := h.workflows.GetByID(ctx, exec.WorkflowID)
			if err != nil {
				slog.ErrorContext(ctx, "failure retry failed to load workflow", slog.String("execution_id", exec.ID), slog.String("error", err.Error()))
				return
			}
			ev, err := h.events.GetByID(ctx, exec.EventID)
			if err != nil {
				slog.ErrorContext(ctx, "failure retry failed to load event", slog.String("execution_id", exec.ID), slog.String("error", err.Error()))
				return
			}
			if _, err := h.workflowEngine.ExecuteWorkflow(ctx, wf, ev, exec); err != nil {
				slog.ErrorContext(ctx, "failure retry execution failed", slog.String("execution_id", exec.ID), slog.String("error", err.Error()))
			}
		}()
	}

	respondJSON(w, http.StatusOK, exec)
}
