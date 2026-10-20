package api

import (
	"encoding/json"
	"errors"
	"net/http"

	"kerob/internal/core/credentials"
)

type credentialInput struct {
	ID          string            `json:"id,omitempty"`
	Name        string            `json:"name"`
	Integration string            `json:"integration"`
	Values      map[string]string `json:"values"`
}

func (h *Handlers) ListCredentials(w http.ResponseWriter, r *http.Request) {
	if h.creds == nil {
		respondJSON(w, http.StatusOK, []*credentials.Credential{})
		return
	}
	list, err := h.creds.List(r.Context())
	if err != nil {
		respondError(w, http.StatusInternalServerError, err.Error())
		return
	}
	if list == nil {
		list = []*credentials.Credential{}
	}
	respondJSON(w, http.StatusOK, list)
}

func (h *Handlers) CreateCredential(w http.ResponseWriter, r *http.Request) {
	if h.creds == nil {
		respondError(w, http.StatusInternalServerError, "credential store not initialized")
		return
	}

	var input credentialInput
	if err := json.NewDecoder(r.Body).Decode(&input); err != nil {
		respondError(w, http.StatusBadRequest, "invalid json body: "+err.Error())
		return
	}

	if input.Name == "" {
		respondError(w, http.StatusBadRequest, "name is required")
		return
	}
	if input.Integration == "" {
		respondError(w, http.StatusBadRequest, "integration is required")
		return
	}

	cred := &credentials.Credential{
		ID:          input.ID,
		Name:        input.Name,
		Integration: input.Integration,
	}

	created, err := h.creds.Create(r.Context(), cred, input.Values)
	if err != nil {
		respondError(w, http.StatusInternalServerError, err.Error())
		return
	}

	respondJSON(w, http.StatusCreated, created)
}

func (h *Handlers) GetCredential(w http.ResponseWriter, r *http.Request) {
	if h.creds == nil {
		respondError(w, http.StatusInternalServerError, "credential store not initialized")
		return
	}

	id := r.PathValue("id")
	cred, err := h.creds.GetByID(r.Context(), id)
	if errors.Is(err, credentials.ErrCredentialNotFound) {
		respondError(w, http.StatusNotFound, "credential not found")
		return
	}
	if err != nil {
		respondError(w, http.StatusInternalServerError, err.Error())
		return
	}

	respondJSON(w, http.StatusOK, cred)
}

func (h *Handlers) UpdateCredential(w http.ResponseWriter, r *http.Request) {
	if h.creds == nil {
		respondError(w, http.StatusInternalServerError, "credential store not initialized")
		return
	}

	id := r.PathValue("id")
	var input credentialInput
	if err := json.NewDecoder(r.Body).Decode(&input); err != nil {
		respondError(w, http.StatusBadRequest, "invalid json body: "+err.Error())
		return
	}

	cred := &credentials.Credential{
		ID:          id,
		Name:        input.Name,
		Integration: input.Integration,
	}

	updated, err := h.creds.Update(r.Context(), cred, input.Values)
	if errors.Is(err, credentials.ErrCredentialNotFound) {
		respondError(w, http.StatusNotFound, "credential not found")
		return
	}
	if err != nil {
		respondError(w, http.StatusInternalServerError, err.Error())
		return
	}

	respondJSON(w, http.StatusOK, updated)
}

func (h *Handlers) DeleteCredential(w http.ResponseWriter, r *http.Request) {
	if h.creds == nil {
		respondError(w, http.StatusInternalServerError, "credential store not initialized")
		return
	}

	id := r.PathValue("id")
	err := h.creds.Delete(r.Context(), id)
	if errors.Is(err, credentials.ErrCredentialNotFound) {
		respondError(w, http.StatusNotFound, "credential not found")
		return
	}
	if err != nil {
		respondError(w, http.StatusInternalServerError, err.Error())
		return
	}

	w.WriteHeader(http.StatusNoContent)
}
