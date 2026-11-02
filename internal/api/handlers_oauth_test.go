package api

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"kerob/pkg/connector"
	"kerob/pkg/connector/builtin/github"
)

func TestConnectorsAndOAuthAPI(t *testing.T) {
	// Setup mock upstream GitHub server for token exchange
	mockUpstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")

		if r.URL.Path == "/login/oauth/access_token" {
			_ = r.ParseForm()
			if r.FormValue("grant_type") == "refresh_token" {
				_ = json.NewEncoder(w).Encode(map[string]any{
					"access_token":  "gho_rotated_token_777",
					"refresh_token": "ghr_rotated_refresh_888",
					"token_type":    "bearer",
					"expires_in":    3600,
				})
				return
			}
			if r.FormValue("code") == "test-oauth-code" {
				_ = json.NewEncoder(w).Encode(map[string]any{
					"access_token":  "gho_test_access_secret_123",
					"refresh_token": "ghr_test_refresh_secret_456",
					"token_type":    "bearer",
					"expires_in":    7200,
				})
				return
			}
			w.WriteHeader(http.StatusBadRequest)
			_ = json.NewEncoder(w).Encode(map[string]string{"error": "bad_code"})
			return
		}

		if r.URL.Path == "/user" {
			authHeader := r.Header.Get("Authorization")
			if strings.Contains(authHeader, "gho_") || strings.Contains(authHeader, "valid") {
				_ = json.NewEncoder(w).Encode(map[string]any{"login": "kerob-org"})
				return
			}
			w.WriteHeader(http.StatusUnauthorized)
			return
		}

		if strings.HasPrefix(r.URL.Path, "/applications/") && r.Method == http.MethodDelete {
			w.WriteHeader(http.StatusNoContent)
			return
		}

		http.NotFound(w, r)
	}))
	defer mockUpstream.Close()

	server, _, cfg := setupTestServer()

	// Register test GitHub connector with mock upstream URL
	cReg := connector.NewRegistry()
	ghConn := github.NewConnector(github.Config{
		Client:       mockUpstream.Client(),
		BaseURL:      mockUpstream.URL,
		AuthURL:      mockUpstream.URL + "/login/oauth/authorize",
		TokenURL:     mockUpstream.URL + "/login/oauth/access_token",
		ClientID:     "mock-client-id",
		ClientSecret: "mock-client-secret",
	})
	_ = cReg.Register(ghConn)

	// Update handlers with our cReg and mock
	server.httpServer = nil // server.mux is what we test
	handlers := NewHandlersFromConfig(Config{
		Workflows:      cfg.Workflows,
		Executions:     cfg.Executions,
		DeadLetters:    cfg.DeadLetters,
		WorkflowEngine: cfg.WorkflowEngine,
		ActionReg:      cfg.ActionReg,
		Creds:          cfg.Creds,
		WebhookStore:   cfg.WebhookStore,
		ConnectorReg:   cReg,
	})
	srv := NewServer("0", handlers)

	// 1. GET /api/v1/connectors
	req1 := httptest.NewRequest(http.MethodGet, "/api/v1/connectors", nil)
	rr1 := httptest.NewRecorder()
	srv.mux.ServeHTTP(rr1, req1)
	if rr1.Code != http.StatusOK {
		t.Fatalf("ListConnectors returned %d: %s", rr1.Code, rr1.Body.String())
	}
	var manifests []connector.Manifest
	if err := json.Unmarshal(rr1.Body.Bytes(), &manifests); err != nil {
		t.Fatalf("failed to decode connectors: %v", err)
	}
	if len(manifests) == 0 {
		t.Fatal("expected at least 1 connector")
	}

	// 2. GET /api/v1/connectors/github
	req2 := httptest.NewRequest(http.MethodGet, "/api/v1/connectors/github", nil)
	rr2 := httptest.NewRecorder()
	srv.mux.ServeHTTP(rr2, req2)
	if rr2.Code != http.StatusOK {
		t.Fatalf("GetConnector returned %d", rr2.Code)
	}

	// 3. GET /api/v1/connectors/github/health
	req3 := httptest.NewRequest(http.MethodGet, "/api/v1/connectors/github/health", nil)
	rr3 := httptest.NewRecorder()
	srv.mux.ServeHTTP(rr3, req3)
	if rr3.Code != http.StatusOK {
		t.Fatalf("CheckConnectorHealth returned %d", rr3.Code)
	}

	// 4. GET /api/v1/oauth/github/authorize
	req4 := httptest.NewRequest(http.MethodGet, "/api/v1/oauth/github/authorize?redirect_uri=https://app.kerob.dev/cb", nil)
	rr4 := httptest.NewRecorder()
	srv.mux.ServeHTTP(rr4, req4)
	if rr4.Code != http.StatusOK {
		t.Fatalf("AuthorizeOAuth returned %d: %s", rr4.Code, rr4.Body.String())
	}
	var authResp map[string]string
	_ = json.Unmarshal(rr4.Body.Bytes(), &authResp)
	state := authResp["state"]
	if state == "" {
		t.Fatal("missing state in authorize response")
	}

	// 5. POST /api/v1/oauth/github/callback (Code Exchange)
	cbPayload, _ := json.Marshal(map[string]string{
		"state": state,
		"code":  "test-oauth-code",
	})
	req5 := httptest.NewRequest(http.MethodPost, "/api/v1/oauth/github/callback", bytes.NewReader(cbPayload))
	req5.Header.Set("Content-Type", "application/json")
	rr5 := httptest.NewRecorder()
	srv.mux.ServeHTTP(rr5, req5)
	if rr5.Code != http.StatusCreated {
		t.Fatalf("CallbackOAuth returned %d: %s", rr5.Code, rr5.Body.String())
	}

	// Assert token secrecy: Raw secret tokens MUST NOT be in response JSON
	respBody := rr5.Body.String()
	if strings.Contains(respBody, "gho_test_access_secret_123") {
		t.Fatalf("SECURITY VIOLATION: access token leaked in callback response: %s", respBody)
	}
	if strings.Contains(respBody, "ghr_test_refresh_secret_456") {
		t.Fatalf("SECURITY VIOLATION: refresh token leaked in callback response: %s", respBody)
	}

	var credResp map[string]any
	_ = json.Unmarshal(rr5.Body.Bytes(), &credResp)
	credID, _ := credResp["id"].(string)
	if credID == "" {
		t.Fatal("missing credential ID in callback response")
	}

	// 6. POST /api/v1/oauth/github/refresh
	refPayload, _ := json.Marshal(map[string]string{
		"credential_id": credID,
	})
	req6 := httptest.NewRequest(http.MethodPost, "/api/v1/oauth/github/refresh", bytes.NewReader(refPayload))
	req6.Header.Set("Content-Type", "application/json")
	rr6 := httptest.NewRecorder()
	srv.mux.ServeHTTP(rr6, req6)
	if rr6.Code != http.StatusOK {
		t.Fatalf("RefreshOAuth returned %d: %s", rr6.Code, rr6.Body.String())
	}

	// 7. Dynamic Health Check with stored credential
	req7 := httptest.NewRequest(http.MethodGet, "/api/v1/connectors/github/health?credential_id="+credID, nil)
	rr7 := httptest.NewRecorder()
	srv.mux.ServeHTTP(rr7, req7)
	if rr7.Code != http.StatusOK {
		t.Fatalf("Health check with credential returned %d", rr7.Code)
	}
	var healthResp connector.HealthStatus
	_ = json.Unmarshal(rr7.Body.Bytes(), &healthResp)
	if healthResp.Status != "healthy" {
		t.Fatalf("expected healthy, got %s (%s)", healthResp.Status, healthResp.Message)
	}

	// 8. POST /api/v1/oauth/github/revoke
	revPayload, _ := json.Marshal(map[string]string{
		"credential_id": credID,
	})
	req8 := httptest.NewRequest(http.MethodPost, "/api/v1/oauth/github/revoke", bytes.NewReader(revPayload))
	req8.Header.Set("Content-Type", "application/json")
	rr8 := httptest.NewRecorder()
	srv.mux.ServeHTTP(rr8, req8)
	if rr8.Code != http.StatusOK {
		t.Fatalf("RevokeOAuth returned %d: %s", rr8.Code, rr8.Body.String())
	}

	// Ensure credential deleted from store
	_, err := cfg.Creds.GetByID(context.Background(), credID)
	if err == nil {
		t.Fatal("expected credential to be deleted from store")
	}
}
