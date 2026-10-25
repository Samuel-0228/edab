package api

import (
	"encoding/json"
	"errors"
	"net/http"
	"strings"

	"kerob/internal/core/event"
	"kerob/internal/core/workflow"
)

func (h *Handlers) ListWorkflows(w http.ResponseWriter, r *http.Request) {
	if h.workflows == nil {
		respondJSON(w, http.StatusOK, []*workflow.Workflow{})
		return
	}
	list, err := h.workflows.List(r.Context())
	if err != nil {
		respondError(w, http.StatusInternalServerError, err.Error())
		return
	}
	if list == nil {
		list = []*workflow.Workflow{}
	}
	respondJSON(w, http.StatusOK, list)
}

func (h *Handlers) CreateWorkflow(w http.ResponseWriter, r *http.Request) {
	if h.workflows == nil {
		respondError(w, http.StatusInternalServerError, "workflow store not initialized")
		return
	}

	var wf workflow.Workflow
	if err := json.NewDecoder(r.Body).Decode(&wf); err != nil {
		respondErrorWithCode(w, http.StatusBadRequest, "MALFORMED_JSON", "invalid json body: "+err.Error(), nil)
		return
	}

	if valErrs := ValidateWorkflowInput(&wf, false); len(valErrs) > 0 {
		respondValidationError(w, r, valErrs)
		return
	}

	created, err := h.workflows.Create(r.Context(), &wf)
	if err != nil {
		respondError(w, http.StatusBadRequest, err.Error())
		return
	}

	respondJSON(w, http.StatusCreated, created)
}

func (h *Handlers) GetWorkflow(w http.ResponseWriter, r *http.Request) {
	if h.workflows == nil {
		respondError(w, http.StatusInternalServerError, "workflow store not initialized")
		return
	}

	id := r.PathValue("id")
	wf, err := h.workflows.GetByID(r.Context(), id)
	if errors.Is(err, workflow.ErrWorkflowNotFound) {
		respondError(w, http.StatusNotFound, "workflow not found")
		return
	}
	if err != nil {
		respondError(w, http.StatusInternalServerError, err.Error())
		return
	}

	respondJSON(w, http.StatusOK, wf)
}

func (h *Handlers) UpdateWorkflow(w http.ResponseWriter, r *http.Request) {
	if h.workflows == nil {
		respondError(w, http.StatusInternalServerError, "workflow store not initialized")
		return
	}

	id := r.PathValue("id")
	var wf workflow.Workflow
	if err := json.NewDecoder(r.Body).Decode(&wf); err != nil {
		respondErrorWithCode(w, http.StatusBadRequest, "MALFORMED_JSON", "invalid json body: "+err.Error(), nil)
		return
	}
	wf.ID = id

	if valErrs := ValidateWorkflowInput(&wf, false); len(valErrs) > 0 {
		respondValidationError(w, r, valErrs)
		return
	}

	updated, err := h.workflows.Update(r.Context(), &wf)
	if errors.Is(err, workflow.ErrWorkflowNotFound) {
		respondError(w, http.StatusNotFound, "workflow not found")
		return
	}
	if err != nil {
		respondError(w, http.StatusBadRequest, err.Error())
		return
	}

	respondJSON(w, http.StatusOK, updated)
}

func (h *Handlers) PatchWorkflow(w http.ResponseWriter, r *http.Request) {
	if h.workflows == nil {
		respondError(w, http.StatusInternalServerError, "workflow store not initialized")
		return
	}

	id := r.PathValue("id")
	wf, err := h.workflows.GetByID(r.Context(), id)
	if errors.Is(err, workflow.ErrWorkflowNotFound) {
		respondError(w, http.StatusNotFound, "workflow not found")
		return
	}
	if err != nil {
		respondError(w, http.StatusInternalServerError, err.Error())
		return
	}

	var patchInput struct {
		Name        *string `json:"name,omitempty"`
		Description *string `json:"description,omitempty"`
		Enabled     *bool   `json:"enabled,omitempty"`
	}
	if err := json.NewDecoder(r.Body).Decode(&patchInput); err != nil {
		respondErrorWithCode(w, http.StatusBadRequest, "MALFORMED_JSON", "invalid json body: "+err.Error(), nil)
		return
	}

	if patchInput.Name != nil {
		trimmed := strings.TrimSpace(*patchInput.Name)
		if trimmed == "" {
			respondValidationError(w, r, []FieldError{{Field: "name", Message: "workflow name cannot be empty"}})
			return
		}
		if len(trimmed) > 128 {
			respondValidationError(w, r, []FieldError{{Field: "name", Message: "workflow name cannot exceed 128 characters"}})
			return
		}
		wf.Name = trimmed
	}
	if patchInput.Description != nil {
		wf.Description = *patchInput.Description
	}
	if patchInput.Enabled != nil {
		wf.Enabled = *patchInput.Enabled
	}

	updated, err := h.workflows.Update(r.Context(), wf)
	if err != nil {
		respondError(w, http.StatusInternalServerError, err.Error())
		return
	}

	respondJSON(w, http.StatusOK, updated)
}

func (h *Handlers) DeleteWorkflow(w http.ResponseWriter, r *http.Request) {
	if h.workflows == nil {
		respondError(w, http.StatusInternalServerError, "workflow store not initialized")
		return
	}

	id := r.PathValue("id")
	err := h.workflows.Delete(r.Context(), id)
	if errors.Is(err, workflow.ErrWorkflowNotFound) {
		respondError(w, http.StatusNotFound, "workflow not found")
		return
	}
	if err != nil {
		respondError(w, http.StatusInternalServerError, err.Error())
		return
	}

	w.WriteHeader(http.StatusNoContent)
}

type testWorkflowInput struct {
	EventType string          `json:"event_type,omitempty"`
	SourceID  string          `json:"source_id,omitempty"`
	Payload   json.RawMessage `json:"payload,omitempty"`
	Metadata  json.RawMessage `json:"metadata,omitempty"`
}

// TestWorkflow executes a workflow on-demand with synthetic or provided event payload.
func (h *Handlers) TestWorkflow(w http.ResponseWriter, r *http.Request) {
	if h.workflows == nil || h.workflowEngine == nil {
		respondError(w, http.StatusInternalServerError, "workflow engine or store not initialized")
		return
	}

	id := r.PathValue("id")
	wf, err := h.workflows.GetByID(r.Context(), id)
	if errors.Is(err, workflow.ErrWorkflowNotFound) {
		respondError(w, http.StatusNotFound, "workflow not found")
		return
	}
	if err != nil {
		respondError(w, http.StatusInternalServerError, err.Error())
		return
	}

	var input testWorkflowInput
	if r.Body != nil && r.ContentLength > 0 {
		if err := json.NewDecoder(r.Body).Decode(&input); err != nil {
			respondValidationError(w, r, []FieldError{{Field: "body", Message: "invalid JSON body: " + err.Error()}})
			return
		}
	}

	eventType := input.EventType
	if eventType == "" {
		if len(wf.Trigger.EventTypes) > 0 && wf.Trigger.EventTypes[0] != "*" {
			eventType = wf.Trigger.EventTypes[0]
		} else {
			eventType = wf.Trigger.Type + ".test"
		}
	}

	sourceID := input.SourceID
	if sourceID == "" {
		sourceID = "manual_test_" + wf.Trigger.Type
	}

	payload := input.Payload
	if len(payload) == 0 {
		payload = json.RawMessage(`{"text":"Kerob manual test message","title":"Test Event","author":"operator"}`)
	}

	ev, err := event.New(sourceID, "test_"+wf.ID, eventType, payload, input.Metadata)
	if err != nil {
		respondError(w, http.StatusBadRequest, "failed to construct test event: "+err.Error())
		return
	}

	// Create test execution
	exec := &workflow.Execution{
		WorkflowID: wf.ID,
		EventID:    ev.ID,
		Status:     workflow.ExecutionPending,
	}

	executed, err := h.workflowEngine.ExecuteWorkflow(r.Context(), wf, ev, exec)
	if err != nil {
		respondJSON(w, http.StatusOK, map[string]any{
			"status":    "error",
			"error":     err.Error(),
			"execution": executed,
		})
		return
	}

	respondJSON(w, http.StatusOK, map[string]any{
		"status":    "success",
		"execution": executed,
	})
}
