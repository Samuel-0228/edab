package api

import (
	"encoding/json"
	"errors"
	"net/http"

	"kerob/internal/core/auth"
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
	if !h.requirePermission(w, r, auth.PermCredentialsRead) {
		return
	}
	identity := auth.GetIdentity(r.Context())

	list, err := h.creds.List(r.Context())
	if err != nil {
		respondError(w, http.StatusInternalServerError, err.Error())
		return
	}
	if list == nil {
		list = []*credentials.Credential{}
	}

	if h.multiTenancy {
		filtered := make([]*credentials.Credential, 0, len(list))
		for _, item := range list {
			if item.TenantID == "" || item.TenantID == identity.TenantID {
				filtered = append(filtered, item)
			}
		}
		list = filtered
	}

	respondJSON(w, http.StatusOK, list)
}

func (h *Handlers) CreateCredential(w http.ResponseWriter, r *http.Request) {
	if h.creds == nil {
		respondError(w, http.StatusInternalServerError, "credential store not initialized")
		return
	}
	if !h.requirePermission(w, r, auth.PermCredentialsWrite) {
		return
	}
	identity := auth.GetIdentity(r.Context())

	var input credentialInput
	if err := json.NewDecoder(r.Body).Decode(&input); err != nil {
		respondErrorWithCode(w, http.StatusBadRequest, "MALFORMED_JSON", "invalid json body: "+err.Error(), nil)
		return
	}

	cred := &credentials.Credential{
		ID:          input.ID,
		TenantID:    identity.TenantID,
		Name:        input.Name,
		Integration: input.Integration,
	}

	if valErrs := ValidateCredentialInput(cred, input.Values); len(valErrs) > 0 {
		respondValidationError(w, r, valErrs)
		return
	}

	created, err := h.creds.Create(r.Context(), cred, input.Values)
	if err != nil {
		respondError(w, http.StatusBadRequest, err.Error())
		return
	}

	h.recordAudit(r, "credential.created", "credential", created.ID, map[string]any{
		"name":        created.Name,
		"integration": created.Integration,
	})

	respondJSON(w, http.StatusCreated, created)
}

func (h *Handlers) GetCredential(w http.ResponseWriter, r *http.Request) {
	if h.creds == nil {
		respondError(w, http.StatusInternalServerError, "credential store not initialized")
		return
	}
	if !h.requirePermission(w, r, auth.PermCredentialsRead) {
		return
	}
	identity := auth.GetIdentity(r.Context())

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

	if h.multiTenancy && cred.TenantID != "" && cred.TenantID != identity.TenantID {
		respondErrorWithCode(w, http.StatusForbidden, "CROSS_TENANT_DENIED", "cannot access credential in another tenant", nil)
		return
	}

	h.recordAudit(r, "credential.read", "credential", cred.ID, map[string]any{
		"name":        cred.Name,
		"integration": cred.Integration,
	})

	respondJSON(w, http.StatusOK, cred)
}

func (h *Handlers) UpdateCredential(w http.ResponseWriter, r *http.Request) {
	if h.creds == nil {
		respondError(w, http.StatusInternalServerError, "credential store not initialized")
		return
	}
	if !h.requirePermission(w, r, auth.PermCredentialsWrite) {
		return
	}
	identity := auth.GetIdentity(r.Context())

	id := r.PathValue("id")
	existing, err := h.creds.GetByID(r.Context(), id)
	if errors.Is(err, credentials.ErrCredentialNotFound) {
		respondError(w, http.StatusNotFound, "credential not found")
		return
	}
	if err != nil {
		respondError(w, http.StatusInternalServerError, err.Error())
		return
	}

	if h.multiTenancy && existing.TenantID != "" && existing.TenantID != identity.TenantID {
		respondErrorWithCode(w, http.StatusForbidden, "CROSS_TENANT_DENIED", "cannot modify credential in another tenant", nil)
		return
	}

	var input credentialInput
	if err := json.NewDecoder(r.Body).Decode(&input); err != nil {
		respondErrorWithCode(w, http.StatusBadRequest, "MALFORMED_JSON", "invalid json body: "+err.Error(), nil)
		return
	}

	cred := &credentials.Credential{
		ID:          id,
		TenantID:    existing.TenantID,
		Name:        input.Name,
		Integration: input.Integration,
	}

	if valErrs := ValidateCredentialInput(cred, input.Values); len(valErrs) > 0 {
		respondValidationError(w, r, valErrs)
		return
	}

	updated, err := h.creds.Update(r.Context(), cred, input.Values)
	if errors.Is(err, credentials.ErrCredentialNotFound) {
		respondError(w, http.StatusNotFound, "credential not found")
		return
	}
	if err != nil {
		respondError(w, http.StatusBadRequest, err.Error())
		return
	}

	h.recordAudit(r, "credential.updated", "credential", updated.ID, map[string]any{
		"name":        updated.Name,
		"integration": updated.Integration,
	})

	respondJSON(w, http.StatusOK, updated)
}

func (h *Handlers) DeleteCredential(w http.ResponseWriter, r *http.Request) {
	if h.creds == nil {
		respondError(w, http.StatusInternalServerError, "credential store not initialized")
		return
	}
	if !h.requirePermission(w, r, auth.PermCredentialsDelete) {
		return
	}
	identity := auth.GetIdentity(r.Context())

	id := r.PathValue("id")
	existing, err := h.creds.GetByID(r.Context(), id)
	if errors.Is(err, credentials.ErrCredentialNotFound) {
		respondError(w, http.StatusNotFound, "credential not found")
		return
	}
	if err != nil {
		respondError(w, http.StatusInternalServerError, err.Error())
		return
	}

	if h.multiTenancy && existing.TenantID != "" && existing.TenantID != identity.TenantID {
		respondErrorWithCode(w, http.StatusForbidden, "CROSS_TENANT_DENIED", "cannot delete credential in another tenant", nil)
		return
	}

	err = h.creds.Delete(r.Context(), id)
	if errors.Is(err, credentials.ErrCredentialNotFound) {
		respondError(w, http.StatusNotFound, "credential not found")
		return
	}
	if err != nil {
		respondError(w, http.StatusInternalServerError, err.Error())
		return
	}

	h.recordAudit(r, "credential.deleted", "credential", id, map[string]any{
		"name":        existing.Name,
		"integration": existing.Integration,
	})

	w.WriteHeader(http.StatusNoContent)
}
