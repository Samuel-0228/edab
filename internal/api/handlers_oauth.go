package api

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/http"

	"kerob/internal/core/auth"
	"kerob/internal/core/oauth"
	"kerob/pkg/connector"
)

// ListConnectors returns all registered SDK connectors and their manifests.
func (h *Handlers) ListConnectors(w http.ResponseWriter, r *http.Request) {
	if !h.requirePermission(w, r, auth.PermIntegrationsRead) {
		return
	}

	reg := h.ConnectorRegistry()
	manifests := reg.ListManifests()
	respondJSON(w, http.StatusOK, manifests)
}

// GetConnector returns a single connector manifest by ID.
func (h *Handlers) GetConnector(w http.ResponseWriter, r *http.Request) {
	if !h.requirePermission(w, r, auth.PermIntegrationsRead) {
		return
	}

	id := r.PathValue("id")
	conn, ok := h.ConnectorRegistry().Get(id)
	if !ok {
		respondError(w, http.StatusNotFound, "connector not found")
		return
	}

	respondJSON(w, http.StatusOK, conn.Manifest())
}

// CheckConnectorHealth executes live connectivity and authentication verification.
func (h *Handlers) CheckConnectorHealth(w http.ResponseWriter, r *http.Request) {
	if !h.requirePermission(w, r, auth.PermIntegrationsRead) {
		return
	}

	id := r.PathValue("id")
	conn, ok := h.ConnectorRegistry().Get(id)
	if !ok {
		respondError(w, http.StatusNotFound, "connector not found")
		return
	}

	checker, ok := conn.(connector.HealthChecker)
	if !ok {
		respondError(w, http.StatusBadRequest, "connector does not support dynamic health check")
		return
	}

	credsMap := make(map[string]string)
	credID := r.URL.Query().Get("credential_id")
	if credID != "" && h.creds != nil {
		decrypted, err := h.creds.GetDecrypted(r.Context(), credID)
		if err == nil && decrypted != nil {
			credsMap = decrypted
		}
	}

	status, err := checker.CheckHealth(r.Context(), credsMap)
	if err != nil && status.Status == "" {
		status.Status = "unhealthy"
		status.Message = err.Error()
	}

	respondJSON(w, http.StatusOK, status)
}

// AuthorizeOAuth initiates an OAuth 2.0 flow and returns the authorization URL with CSRF state.
func (h *Handlers) AuthorizeOAuth(w http.ResponseWriter, r *http.Request) {
	if !h.requirePermission(w, r, auth.PermCredentialsWrite) {
		return
	}

	connectorID := r.PathValue("connector")
	identity := auth.GetIdentity(r.Context())
	redirectURI := r.URL.Query().Get("redirect_uri")

	mgr := h.OAuthManager()
	authURL, state, err := mgr.GetAuthorizeURL(r.Context(), connectorID, identity.TenantID, identity.UserID, redirectURI, nil)
	if err != nil {
		if errors.Is(err, oauth.ErrConnectorNotFound) {
			respondError(w, http.StatusNotFound, err.Error())
			return
		}
		if errors.Is(err, oauth.ErrNotOAuthProvider) {
			respondError(w, http.StatusBadRequest, err.Error())
			return
		}
		respondError(w, http.StatusInternalServerError, err.Error())
		return
	}

	h.recordAudit(r, "oauth.authorize_initiated", "connector", connectorID, map[string]any{
		"connector": connectorID,
		"state":     state,
	})

	respondJSON(w, http.StatusOK, map[string]string{
		"authorize_url": authURL,
		"state":         state,
	})
}

// CallbackOAuth handles code exchange after user authorization.
// Supports both browser redirect (GET with query params) and programmatic client POST.
func (h *Handlers) CallbackOAuth(w http.ResponseWriter, r *http.Request) {
	if !h.requirePermission(w, r, auth.PermCredentialsWrite) {
		return
	}

	connectorID := r.PathValue("connector")

	var state, code, redirectURI string
	if r.Method == http.MethodGet {
		state = r.URL.Query().Get("state")
		code = r.URL.Query().Get("code")
		redirectURI = r.URL.Query().Get("redirect_uri")
	} else {
		var reqBody struct {
			State       string `json:"state"`
			Code        string `json:"code"`
			RedirectURI string `json:"redirect_uri,omitempty"`
		}
		if err := json.NewDecoder(r.Body).Decode(&reqBody); err == nil {
			state = reqBody.State
			code = reqBody.Code
			redirectURI = reqBody.RedirectURI
		}
	}

	if state == "" || code == "" {
		respondError(w, http.StatusBadRequest, "missing state or code parameters")
		return
	}

	mgr := h.OAuthManager()
	storedCred, err := mgr.HandleCallback(r.Context(), connectorID, state, code, redirectURI)
	if err != nil {
		h.recordAudit(r, "oauth.callback_failed", "connector", connectorID, map[string]any{
			"error": err.Error(),
		})
		respondError(w, http.StatusBadRequest, fmt.Sprintf("oauth exchange failed: %s", err.Error()))
		return
	}

	h.recordAudit(r, "oauth.callback_succeeded", "credential", storedCred.ID, map[string]any{
		"connector":     connectorID,
		"credential_id": storedCred.ID,
		"name":          storedCred.Name,
	})

	respondJSON(w, http.StatusCreated, storedCred)
}

// RefreshOAuth performs refresh-token rotation for an existing OAuth credential.
func (h *Handlers) RefreshOAuth(w http.ResponseWriter, r *http.Request) {
	if !h.requirePermission(w, r, auth.PermCredentialsWrite) {
		return
	}

	connectorID := r.PathValue("connector")
	var reqBody struct {
		CredentialID string `json:"credential_id"`
	}
	if err := json.NewDecoder(r.Body).Decode(&reqBody); err != nil || reqBody.CredentialID == "" {
		respondError(w, http.StatusBadRequest, "credential_id is required")
		return
	}

	mgr := h.OAuthManager()
	updatedCred, err := mgr.RefreshToken(r.Context(), connectorID, reqBody.CredentialID)
	if err != nil {
		respondError(w, http.StatusBadRequest, fmt.Sprintf("token refresh failed: %s", err.Error()))
		return
	}

	h.recordAudit(r, "oauth.token_refreshed", "credential", updatedCred.ID, map[string]any{
		"connector":     connectorID,
		"credential_id": updatedCred.ID,
	})

	respondJSON(w, http.StatusOK, updatedCred)
}

// RevokeOAuth revokes upstream OAuth authorization and purges stored credentials.
func (h *Handlers) RevokeOAuth(w http.ResponseWriter, r *http.Request) {
	if !h.requirePermission(w, r, auth.PermCredentialsDelete) {
		return
	}

	connectorID := r.PathValue("connector")
	var reqBody struct {
		CredentialID string `json:"credential_id"`
	}
	if err := json.NewDecoder(r.Body).Decode(&reqBody); err != nil || reqBody.CredentialID == "" {
		respondError(w, http.StatusBadRequest, "credential_id is required")
		return
	}

	mgr := h.OAuthManager()
	if err := mgr.RevokeToken(r.Context(), connectorID, reqBody.CredentialID); err != nil {
		respondError(w, http.StatusBadRequest, fmt.Sprintf("revocation failed: %s", err.Error()))
		return
	}

	h.recordAudit(r, "oauth.token_revoked", "credential", reqBody.CredentialID, map[string]any{
		"connector":     connectorID,
		"credential_id": reqBody.CredentialID,
	})

	respondJSON(w, http.StatusOK, map[string]string{
		"status":  "revoked",
		"message": "Token successfully revoked and credential deleted from vault",
	})
}
