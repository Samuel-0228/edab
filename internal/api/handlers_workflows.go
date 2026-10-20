package api

import (
	"encoding/json"
	"errors"
	"net/http"

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
		respondError(w, http.StatusBadRequest, "invalid json body: "+err.Error())
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
		respondError(w, http.StatusBadRequest, "invalid json body: "+err.Error())
		return
	}
	wf.ID = id

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
		respondError(w, http.StatusBadRequest, "invalid json body: "+err.Error())
		return
	}

	if patchInput.Name != nil {
		wf.Name = *patchInput.Name
	}
	if patchInput.Description != nil {
		wf.Description = *patchInput.Description
	}
	if patchInput.Enabled != nil {
		wf.Enabled = *patchInput.Enabled
	}

	updated, err := h.workflows.Update(r.Context(), wf)
	if err != nil {
		respondError(w, http.StatusBadRequest, err.Error())
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
