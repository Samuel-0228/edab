package api

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"strings"

	"kerob/internal/core/workflow"
)

// ListWorkflowVersions returns the version history of a workflow.
func (h *Handlers) ListWorkflowVersions(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	if _, err := h.workflows.GetByID(r.Context(), id); err != nil {
		if errors.Is(err, workflow.ErrWorkflowNotFound) {
			respondError(w, http.StatusNotFound, "workflow not found")
			return
		}
		respondError(w, http.StatusInternalServerError, err.Error())
		return
	}

	versions, err := h.versionStore.ListVersions(r.Context(), id)
	if err != nil {
		respondError(w, http.StatusInternalServerError, fmt.Sprintf("failed to list versions: %s", err.Error()))
		return
	}
	respondJSON(w, http.StatusOK, versions)
}

// GetWorkflowVersion returns a specific version snapshot of a workflow.
func (h *Handlers) GetWorkflowVersion(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	verStr := r.PathValue("version")
	verNum, err := strconv.Atoi(verStr)
	if err != nil {
		respondError(w, http.StatusBadRequest, "invalid version parameter")
		return
	}

	version, err := h.versionStore.GetVersion(r.Context(), id, verNum)
	if errors.Is(err, workflow.ErrVersionNotFound) {
		respondError(w, http.StatusNotFound, "workflow version not found")
		return
	}
	if err != nil {
		respondError(w, http.StatusInternalServerError, err.Error())
		return
	}
	respondJSON(w, http.StatusOK, version)
}

// RollbackWorkflow rolls back a workflow to a target version.
func (h *Handlers) RollbackWorkflow(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	verStr := r.PathValue("version")
	verNum, err := strconv.Atoi(verStr)
	if err != nil {
		respondError(w, http.StatusBadRequest, "invalid version parameter")
		return
	}

	targetVer, err := h.versionStore.GetVersion(r.Context(), id, verNum)
	if errors.Is(err, workflow.ErrVersionNotFound) {
		respondError(w, http.StatusNotFound, "target workflow version not found")
		return
	}
	if err != nil {
		respondError(w, http.StatusInternalServerError, err.Error())
		return
	}

	var restoredWf workflow.Workflow
	if err := json.Unmarshal(targetVer.Definition, &restoredWf); err != nil {
		respondError(w, http.StatusInternalServerError, fmt.Sprintf("corrupt version snapshot: %s", err.Error()))
		return
	}

	restoredWf.ID = id
	updated, err := h.workflows.Update(r.Context(), &restoredWf)
	if err != nil {
		respondError(w, http.StatusInternalServerError, fmt.Sprintf("failed to update workflow during rollback: %s", err.Error()))
		return
	}

	// Record new version snapshot documenting the rollback
	snapshot, _ := json.Marshal(updated)
	_, _ = h.versionStore.RecordVersion(r.Context(), updated.ID, snapshot)

	respondJSON(w, http.StatusOK, map[string]any{
		"message":          fmt.Sprintf("workflow rolled back to version %d", verNum),
		"workflow":         updated,
		"restored_version": verNum,
	})
}

// DiffWorkflowVersions returns diff metadata between two workflow versions.
func (h *Handlers) DiffWorkflowVersions(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	fromStr := r.URL.Query().Get("from")
	toStr := r.URL.Query().Get("to")

	fromVerNum, err1 := strconv.Atoi(fromStr)
	toVerNum, err2 := strconv.Atoi(toStr)
	if err1 != nil || err2 != nil {
		respondError(w, http.StatusBadRequest, "from and to version query parameters are required")
		return
	}

	fromVer, err := h.versionStore.GetVersion(r.Context(), id, fromVerNum)
	if err != nil {
		respondError(w, http.StatusNotFound, fmt.Sprintf("version %d not found", fromVerNum))
		return
	}

	toVer, err := h.versionStore.GetVersion(r.Context(), id, toVerNum)
	if err != nil {
		respondError(w, http.StatusNotFound, fmt.Sprintf("version %d not found", toVerNum))
		return
	}

	respondJSON(w, http.StatusOK, map[string]any{
		"workflow_id":  id,
		"from_version": fromVerNum,
		"to_version":   toVerNum,
		"from":         json.RawMessage(fromVer.Definition),
		"to":           json.RawMessage(toVer.Definition),
	})
}

// ExportWorkflow exports a workflow in JSON or YAML format.
func (h *Handlers) ExportWorkflow(w http.ResponseWriter, r *http.Request) {
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

	format := strings.ToLower(r.URL.Query().Get("format"))
	if format == "yaml" || format == "yml" {
		data, err := workflow.ExportWorkflowYAML(wf)
		if err != nil {
			respondError(w, http.StatusInternalServerError, err.Error())
			return
		}
		w.Header().Set("Content-Type", "application/x-yaml")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write(data)
		return
	}

	data, err := workflow.ExportWorkflowJSON(wf)
	if err != nil {
		respondError(w, http.StatusInternalServerError, err.Error())
		return
	}
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write(data)
}

// ImportWorkflow imports a workflow from JSON or YAML definition.
func (h *Handlers) ImportWorkflow(w http.ResponseWriter, r *http.Request) {
	bodyBytes, err := io.ReadAll(io.LimitReader(r.Body, 1024*1024))
	if err != nil || len(bodyBytes) == 0 {
		respondError(w, http.StatusBadRequest, "empty request body")
		return
	}

	importedWf, err := workflow.ImportWorkflowJSON(bodyBytes)
	if err != nil {
		respondError(w, http.StatusBadRequest, fmt.Sprintf("import failed: %s", err.Error()))
		return
	}

	created, err := h.workflows.Create(r.Context(), importedWf)
	if err != nil {
		respondError(w, http.StatusInternalServerError, fmt.Sprintf("failed to create imported workflow: %s", err.Error()))
		return
	}

	snapshot, _ := json.Marshal(created)
	_, _ = h.versionStore.RecordVersion(r.Context(), created.ID, snapshot)

	respondJSON(w, http.StatusCreated, created)
}

// ListTemplates returns all built-in workflow template recipes.
func (h *Handlers) ListTemplates(w http.ResponseWriter, r *http.Request) {
	templates := workflow.BuiltinTemplates()
	respondJSON(w, http.StatusOK, templates)
}
