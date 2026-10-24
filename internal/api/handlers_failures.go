package api

import (
	"context"
	"errors"
	"log/slog"
	"net/http"
	"time"

	"kerob/internal/core/workflow"
)

func (h *Handlers) ListFailures(w http.ResponseWriter, r *http.Request) {
	if h.deadLetters == nil {
		respondJSON(w, http.StatusOK, []*workflow.DeadLetterRecord{})
		return
	}

	includeDiscarded := r.URL.Query().Get("include_discarded") == "true"
	list, err := h.deadLetters.List(r.Context(), includeDiscarded)
	if err != nil {
		respondError(w, http.StatusInternalServerError, err.Error())
		return
	}
	if list == nil {
		list = []*workflow.DeadLetterRecord{}
	}

	respondJSON(w, http.StatusOK, list)
}

func (h *Handlers) GetFailure(w http.ResponseWriter, r *http.Request) {
	if h.deadLetters == nil {
		respondError(w, http.StatusInternalServerError, "dead-letter store not initialized")
		return
	}

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

	respondJSON(w, http.StatusOK, rec)
}

func (h *Handlers) DiscardFailure(w http.ResponseWriter, r *http.Request) {
	if h.deadLetters == nil {
		respondError(w, http.StatusInternalServerError, "dead-letter store not initialized")
		return
	}

	id := r.PathValue("id")
	err := h.deadLetters.Discard(r.Context(), id)
	if errors.Is(err, workflow.ErrDeadLetterNotFound) {
		respondError(w, http.StatusNotFound, "failure record not found")
		return
	}
	if err != nil {
		respondError(w, http.StatusInternalServerError, err.Error())
		return
	}

	respondJSON(w, http.StatusOK, map[string]string{"status": "discarded"})
}

func (h *Handlers) RetryFailure(w http.ResponseWriter, r *http.Request) {
	if h.deadLetters == nil || h.executions == nil {
		respondError(w, http.StatusInternalServerError, "stores not initialized")
		return
	}

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
