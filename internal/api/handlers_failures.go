package api

import (
	"errors"
	"net/http"

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
	_ = h.deadLetters.Discard(r.Context(), id)

	// Trigger immediate retry execution if engine is active
	if h.workflowEngine != nil && h.workflows != nil && h.events != nil {
		go func() {
			ctx := r.Context()
			wf, err := h.workflows.GetByID(ctx, exec.WorkflowID)
			if err != nil {
				return
			}
			ev, err := h.events.GetByID(ctx, exec.EventID)
			if err != nil {
				return
			}
			_, _ = h.workflowEngine.ExecuteWorkflow(ctx, wf, ev, exec)
		}()
	}

	respondJSON(w, http.StatusOK, exec)
}
