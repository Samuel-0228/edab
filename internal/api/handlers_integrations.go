package api

import (
	"net/http"

	"kerob/internal/adapters"
)

// IntegrationSchemaResponse wraps the JSON schema for an integration configuration.
type IntegrationSchemaResponse struct {
	ID                     string                    `json:"id"`
	Name                   string                    `json:"name"`
	Kind                   string                    `json:"kind"`
	Capabilities           []string                  `json:"capabilities"`
	RequiredCredentialType string                    `json:"required_credential_type,omitempty"`
	ConfigSchema           map[string]any            `json:"config_schema"`
	Schemas                map[string]map[string]any `json:"schemas,omitempty"`
}

func (h *Handlers) ListIntegrations(w http.ResponseWriter, r *http.Request) {
	catalog := adapters.Catalog()
	respondJSON(w, http.StatusOK, catalog)
}

func (h *Handlers) GetIntegration(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	manifest, ok := adapters.GetManifest(id)
	if !ok {
		respondError(w, http.StatusNotFound, "integration not found")
		return
	}
	respondJSON(w, http.StatusOK, manifest)
}

func (h *Handlers) GetIntegrationSchema(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	manifest, ok := adapters.GetManifest(id)
	if !ok {
		respondError(w, http.StatusNotFound, "integration not found")
		return
	}

	opType := r.URL.Query().Get("action_type")
	if opType == "" {
		opType = r.URL.Query().Get("type")
	}

	activeSchema, _ := adapters.GetSchema(id, opType)

	resp := IntegrationSchemaResponse{
		ID:                     manifest.ID,
		Name:                   manifest.Name,
		Kind:                   manifest.Kind,
		Capabilities:           manifest.Capabilities,
		RequiredCredentialType: manifest.RequiredCredentialType,
		ConfigSchema:           activeSchema,
		Schemas:                manifest.Schemas,
	}

	respondJSON(w, http.StatusOK, resp)
}
