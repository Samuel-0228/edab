package github

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"kerob/pkg/connector"
)

func TestNewConnectorAndManifest(t *testing.T) {
	conn := NewConnector(Config{
		ClientID:     "client-123",
		ClientSecret: "secret-456",
	})
	if conn == nil {
		t.Fatal("expected connector to be non-nil")
	}

	manifest := conn.Manifest()
	if manifest.ID != ConnectorID {
		t.Errorf("expected ID %s, got %s", ConnectorID, manifest.ID)
	}
	if len(manifest.Capabilities) == 0 {
		t.Error("expected capabilities in manifest")
	}

	if err := conn.ValidateConfig(nil); err != nil {
		t.Errorf("unexpected ValidateConfig error: %v", err)
	}
	if err := conn.ValidateConfig(map[string]any{"owner": "acme"}); err != nil {
		t.Errorf("unexpected ValidateConfig error: %v", err)
	}
}

func TestOAuthFlow(t *testing.T) {
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/login/oauth/access_token":
			_ = r.ParseForm()
			grantType := r.Form.Get("grant_type")
			code := r.Form.Get("code")
			if code == "bad-code" {
				w.WriteHeader(http.StatusOK)
				_ = json.NewEncoder(w).Encode(gitHubTokenResponse{
					Error:            "bad_verification_code",
					ErrorDescription: "Code is invalid or expired",
				})
				return
			}
			if grantType == "refresh_token" {
				w.WriteHeader(http.StatusOK)
				_ = json.NewEncoder(w).Encode(gitHubTokenResponse{
					AccessToken: "gho_refreshed_token",
					TokenType:   "Bearer",
					ExpiresIn:   3600,
				})
				return
			}
			w.WriteHeader(http.StatusOK)
			_ = json.NewEncoder(w).Encode(gitHubTokenResponse{
				AccessToken:  "gho_test_token",
				TokenType:    "Bearer",
				RefreshToken: "ghr_refresh_token",
				ExpiresIn:    3600,
				Scope:        "repo,read:user",
			})
		case "/applications/client-123/grant":
			if r.Method == http.MethodDelete {
				w.WriteHeader(http.StatusNoContent)
				return
			}
			w.WriteHeader(http.StatusBadRequest)
		default:
			http.NotFound(w, r)
		}
	})

	server := httptest.NewServer(handler)
	defer server.Close()

	conn := NewConnector(Config{
		Client:       server.Client(),
		BaseURL:      server.URL,
		AuthURL:      server.URL + "/login/oauth/authorize",
		TokenURL:     server.URL + "/login/oauth/access_token",
		ClientID:     "client-123",
		ClientSecret: "secret-456",
	})

	ctx := context.Background()

	// 1. Authorization URL
	authURL := conn.GetAuthorizationURL("state-abc", "http://localhost/cb", map[string]string{"foo": "bar"})
	if authURL == "" {
		t.Fatal("expected auth URL")
	}

	// 2. Exchange code
	token, err := conn.ExchangeCode(ctx, "valid-code", "http://localhost/cb")
	if err != nil {
		t.Fatalf("ExchangeCode failed: %v", err)
	}
	if token.AccessToken != "gho_test_token" {
		t.Errorf("unexpected access token: %s", token.AccessToken)
	}

	// 3. Exchange code error
	_, err = conn.ExchangeCode(ctx, "bad-code", "http://localhost/cb")
	if err == nil {
		t.Error("expected error for bad code")
	}

	// 4. Refresh token
	refreshed, err := conn.RefreshToken(ctx, "ghr_refresh_token")
	if err != nil {
		t.Fatalf("RefreshToken failed: %v", err)
	}
	if refreshed.AccessToken != "gho_refreshed_token" {
		t.Errorf("unexpected refreshed token: %s", refreshed.AccessToken)
	}

	// 5. Revoke token
	if err := conn.RevokeToken(ctx, "gho_test_token"); err != nil {
		t.Fatalf("RevokeToken failed: %v", err)
	}
}

func TestCheckHealth(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		token := r.Header.Get("Authorization")
		switch token {
		case "Bearer valid-token":
			w.Header().Set("X-RateLimit-Limit", "5000")
			w.Header().Set("X-RateLimit-Remaining", "4999")
			w.WriteHeader(http.StatusOK)
			_ = json.NewEncoder(w).Encode(map[string]any{"login": "octocat"})
		case "Bearer unauthorized-token":
			w.WriteHeader(http.StatusUnauthorized)
		default:
			w.WriteHeader(http.StatusInternalServerError)
		}
	}))
	defer server.Close()

	conn := NewConnector(Config{
		Client:  server.Client(),
		BaseURL: server.URL,
	})
	ctx := context.Background()

	// Empty creds
	status, err := conn.CheckHealth(ctx, nil)
	if err != nil || status.Status != "unhealthy" {
		t.Errorf("expected unhealthy for nil creds, got %s, err: %v", status.Status, err)
	}

	// Valid token
	status, err = conn.CheckHealth(ctx, map[string]string{"access_token": "valid-token"})
	if err != nil || status.Status != "healthy" {
		t.Errorf("expected healthy status, got %s, err: %v", status.Status, err)
	}
	if status.Details["authenticated_user"] != "octocat" {
		t.Errorf("expected user octocat, got %v", status.Details["authenticated_user"])
	}

	// Unauthorized token
	status, _ = conn.CheckHealth(ctx, map[string]string{"token": "unauthorized-token"})
	if status.Status != "unhealthy" {
		t.Errorf("expected unhealthy status, got %s", status.Status)
	}

	// Degraded
	status, _ = conn.CheckHealth(ctx, map[string]string{"personal_access_token": "broken-token"})
	if status.Status != "degraded" {
		t.Errorf("expected degraded status, got %s", status.Status)
	}
}

func TestExecuteAction(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.URL.Path == "/repos/acme/widgets/issues" && r.Method == http.MethodPost:
			var payload map[string]any
			_ = json.NewDecoder(r.Body).Decode(&payload)
			w.WriteHeader(http.StatusCreated)
			_ = json.NewEncoder(w).Encode(map[string]any{
				"number":   42,
				"html_url": "https://github.com/acme/widgets/issues/42",
				"title":    payload["title"],
				"state":    "open",
			})
		case r.URL.Path == "/repos/acme/widgets/issues/42/comments" && r.Method == http.MethodPost:
			w.WriteHeader(http.StatusCreated)
			_ = json.NewEncoder(w).Encode(map[string]any{
				"id":       101,
				"html_url": "https://github.com/acme/widgets/issues/42#issuecomment-101",
			})
		case r.URL.Path == "/repos/acme/widgets/issues/42/labels" && r.Method == http.MethodPost:
			w.WriteHeader(http.StatusOK)
			_ = json.NewEncoder(w).Encode([]map[string]any{
				{"name": "bug"},
			})
		default:
			w.WriteHeader(http.StatusInternalServerError)
		}
	}))
	defer server.Close()

	conn := NewConnector(Config{
		Client:  server.Client(),
		BaseURL: server.URL,
	})
	ctx := context.Background()

	// Missing token
	_, err := conn.ExecuteAction(ctx, connector.ActionRequest{})
	if err == nil {
		t.Error("expected error for missing token")
	}

	// Missing owner/repo
	_, err = conn.ExecuteAction(ctx, connector.ActionRequest{
		Credentials: map[string]string{"access_token": "token"},
	})
	if err == nil {
		t.Error("expected error for missing owner/repo")
	}

	creds := map[string]string{"access_token": "token"}

	// create_issue
	res, err := conn.ExecuteAction(ctx, connector.ActionRequest{
		ActionType:  "create_issue",
		Credentials: creds,
		Parameters: map[string]any{
			"owner":  "acme",
			"repo":   "widgets",
			"title":  "Test Issue",
			"body":   "Issue description",
			"labels": []any{"bug", "p0"},
		},
	})
	if err != nil || !res.Success {
		t.Fatalf("create_issue failed: %v", err)
	}
	if res.Output["issue_number"] != float64(42) && res.Output["issue_number"] != 42 {
		t.Errorf("expected issue 42, got %v", res.Output["issue_number"])
	}

	// post_comment
	res, err = conn.ExecuteAction(ctx, connector.ActionRequest{
		ActionType:  "post_comment",
		Credentials: creds,
		Parameters: map[string]any{
			"owner":        "acme",
			"repo":         "widgets",
			"issue_number": 42,
			"body":         "Looks great!",
		},
	})
	if err != nil || !res.Success {
		t.Fatalf("post_comment failed: %v", err)
	}

	// add_labels with string list
	res, err = conn.ExecuteAction(ctx, connector.ActionRequest{
		ActionType:  "add_labels",
		Credentials: creds,
		Parameters: map[string]any{
			"owner":        "acme",
			"repo":         "widgets",
			"issue_number": 42,
			"labels":       "bug, high-priority",
		},
	})
	if err != nil || !res.Success {
		t.Fatalf("add_labels failed: %v", err)
	}

	// Unknown action
	_, err = conn.ExecuteAction(ctx, connector.ActionRequest{
		ActionType:  "unknown_action",
		Credentials: creds,
		Parameters:  map[string]any{"owner": "acme", "repo": "widgets"},
	})
	if err == nil {
		t.Error("expected error for unknown action")
	}
}

func TestGetSchema(t *testing.T) {
	conn := NewConnector(Config{})
	for _, op := range []string{"create_issue", "post_comment", "add_labels", "other"} {
		schema, err := conn.GetSchema(op)
		if err != nil {
			t.Fatalf("GetSchema(%s) failed: %v", op, err)
		}
		if schema["type"] != "object" {
			t.Errorf("expected object type schema for %s", op)
		}
	}
}
