package connector_test

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
	"time"

	"kerob/pkg/connector"
	"kerob/pkg/connector/builtin/github"
)

// AssertConnectorContract verifies that a connector conforms to all Kerob Connector SDK specifications.
func AssertConnectorContract(t *testing.T, c connector.Connector) {
	t.Helper()

	m := c.Manifest()
	if m.ID == "" {
		t.Fatal("contract violation: manifest ID must not be empty")
	}
	if m.Name == "" {
		t.Fatalf("contract violation (%s): manifest Name must not be empty", m.ID)
	}
	if m.Version == "" {
		t.Fatalf("contract violation (%s): manifest Version must not be empty", m.ID)
	}

	// Verify capability-to-interface conformance
	if m.HasCapability(connector.CapabilityAction) {
		if _, ok := c.(connector.ActionProvider); !ok {
			t.Fatalf("contract violation (%s): declares CapabilityAction but does not implement ActionProvider", m.ID)
		}
	}

	if m.HasCapability(connector.CapabilityOAuth2) {
		if _, ok := c.(connector.OAuth2Provider); !ok {
			t.Fatalf("contract violation (%s): declares CapabilityOAuth2 but does not implement OAuth2Provider", m.ID)
		}
		if m.OAuth2Config == nil {
			t.Fatalf("contract violation (%s): declares CapabilityOAuth2 but OAuth2Config is nil", m.ID)
		}
	}

	if m.HasCapability(connector.CapabilityWebhook) {
		if _, ok := c.(connector.WebhookManager); !ok {
			t.Fatalf("contract violation (%s): declares CapabilityWebhook but does not implement WebhookManager", m.ID)
		}
	}

	if m.HasCapability(connector.CapabilityHealthCheck) {
		if _, ok := c.(connector.HealthChecker); !ok {
			t.Fatalf("contract violation (%s): declares CapabilityHealthCheck but does not implement HealthChecker", m.ID)
		}
	}

	if m.HasCapability(connector.CapabilityTrigger) {
		if _, ok := c.(connector.TriggerProvider); !ok {
			t.Fatalf("contract violation (%s): declares CapabilityTrigger but does not implement TriggerProvider", m.ID)
		}
	}

	// Verify configuration validation
	if err := c.ValidateConfig(nil); err != nil {
		// Validating nil may or may not succeed based on requirements, but should not panic
	}
}

func TestTokenSecrecyContract(t *testing.T) {
	secretAccess := "gho_super_secret_access_token_1234567890"
	secretRefresh := "ghr_super_secret_refresh_token_0987654321"

	tok := &connector.OAuth2Token{
		AccessToken:  secretAccess,
		RefreshToken: secretRefresh,
		TokenType:    "Bearer",
		ExpiresAt:    time.Now().UTC().Add(1 * time.Hour),
		Scopes:       []string{"repo", "read:user"},
	}

	// 1. Verify JSON serialization NEVER leaks raw access or refresh tokens
	bytes, err := json.Marshal(tok)
	if err != nil {
		t.Fatalf("failed to marshal token: %v", err)
	}

	jsonStr := string(bytes)
	if strings.Contains(jsonStr, secretAccess) {
		t.Fatalf("SECURITY VIOLATION: AccessToken leaked in JSON serialization: %s", jsonStr)
	}
	if strings.Contains(jsonStr, secretRefresh) {
		t.Fatalf("SECURITY VIOLATION: RefreshToken leaked in JSON serialization: %s", jsonStr)
	}

	// 2. Verify masked representations hide the secrets
	maskedAccess := tok.MaskedAccessToken()
	if strings.Contains(maskedAccess, secretAccess) || !strings.HasPrefix(maskedAccess, "••••••••") {
		t.Fatalf("AccessToken masking failed: %s", maskedAccess)
	}

	maskedRefresh := tok.MaskedRefreshToken()
	if strings.Contains(maskedRefresh, secretRefresh) || !strings.HasPrefix(maskedRefresh, "••••••••") {
		t.Fatalf("RefreshToken masking failed: %s", maskedRefresh)
	}

	// 3. Verify Credential Map serialization and round-trip parsing
	credMap := tok.ToCredentialMap()
	if credMap["access_token"] != secretAccess {
		t.Fatalf("expected access_token in credential map")
	}
	if credMap["refresh_token"] != secretRefresh {
		t.Fatalf("expected refresh_token in credential map")
	}

	restored := connector.FromCredentialMap(credMap)
	if restored.AccessToken != secretAccess {
		t.Fatalf("failed to restore access token from map")
	}
	if restored.RefreshToken != secretRefresh {
		t.Fatalf("failed to restore refresh token from map")
	}
	if restored.TokenType != "Bearer" {
		t.Fatalf("failed to restore token type")
	}
}

func TestGitHubConnectorConformance(t *testing.T) {
	conn := github.NewConnector(github.Config{
		ClientID:     "mock-client-id",
		ClientSecret: "mock-client-secret",
	})

	// Run standard conformance contract
	AssertConnectorContract(t, conn)

	// Test OAuth URL construction
	authURL := conn.GetAuthorizationURL("test-csrf-state-123", "https://app.kerob.dev/callback", nil)
	parsedURL, err := url.Parse(authURL)
	if err != nil {
		t.Fatalf("malformed auth URL: %v", err)
	}
	q := parsedURL.Query()
	if q.Get("client_id") != "mock-client-id" {
		t.Errorf("expected client_id in auth URL, got %s", q.Get("client_id"))
	}
	if q.Get("state") != "test-csrf-state-123" {
		t.Errorf("expected state in auth URL, got %s", q.Get("state"))
	}
	if q.Get("redirect_uri") != "https://app.kerob.dev/callback" {
		t.Errorf("expected redirect_uri in auth URL, got %s", q.Get("redirect_uri"))
	}
}

func TestGitHubOAuthExchangeAndRefresh(t *testing.T) {
	// Mock upstream GitHub OAuth server
	mockServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")

		if r.URL.Path == "/login/oauth/access_token" {
			_ = r.ParseForm()
			if r.FormValue("grant_type") == "refresh_token" {
				if r.FormValue("refresh_token") != "valid-refresh-token" {
					w.WriteHeader(http.StatusBadRequest)
					_ = json.NewEncoder(w).Encode(map[string]string{
						"error":             "bad_refresh_token",
						"error_description": "the refresh token is invalid",
					})
					return
				}
				_ = json.NewEncoder(w).Encode(map[string]any{
					"access_token":  "new-rotated-access-token",
					"refresh_token": "new-rotated-refresh-token",
					"token_type":    "bearer",
					"expires_in":    3600,
				})
				return
			}

			// Code exchange
			if r.FormValue("code") == "valid-auth-code" {
				_ = json.NewEncoder(w).Encode(map[string]any{
					"access_token":  "gho_test_access_token_123",
					"refresh_token": "valid-refresh-token",
					"token_type":    "bearer",
					"expires_in":    28800,
					"scope":         "repo,read:user",
				})
				return
			}

			w.WriteHeader(http.StatusBadRequest)
			_ = json.NewEncoder(w).Encode(map[string]string{
				"error":             "bad_verification_code",
				"error_description": "code is invalid or expired",
			})
			return
		}

		http.NotFound(w, r)
	}))
	defer mockServer.Close()

	conn := github.NewConnector(github.Config{
		Client:       mockServer.Client(),
		TokenURL:     mockServer.URL + "/login/oauth/access_token",
		ClientID:     "client-123",
		ClientSecret: "secret-456",
	})

	ctx := context.Background()

	// 1. Valid code exchange
	tok, err := conn.ExchangeCode(ctx, "valid-auth-code", "https://kerob.local/callback")
	if err != nil {
		t.Fatalf("ExchangeCode failed: %v", err)
	}
	if tok.AccessToken != "gho_test_access_token_123" {
		t.Fatalf("unexpected access token: %s", tok.AccessToken)
	}
	if tok.RefreshToken != "valid-refresh-token" {
		t.Fatalf("unexpected refresh token: %s", tok.RefreshToken)
	}
	if tok.Expired() {
		t.Fatalf("expected token to not be expired")
	}

	// 2. Invalid code exchange
	_, err = conn.ExchangeCode(ctx, "invalid-code", "https://kerob.local/callback")
	if err == nil {
		t.Fatalf("expected error on invalid code")
	}

	// 3. Token refresh rotation
	refreshed, err := conn.RefreshToken(ctx, "valid-refresh-token")
	if err != nil {
		t.Fatalf("RefreshToken failed: %v", err)
	}
	if refreshed.AccessToken != "new-rotated-access-token" {
		t.Fatalf("unexpected rotated access token: %s", refreshed.AccessToken)
	}
	if refreshed.RefreshToken != "new-rotated-refresh-token" {
		t.Fatalf("unexpected rotated refresh token: %s", refreshed.RefreshToken)
	}
}

func TestGitHubHealthCheckAndActions(t *testing.T) {
	mockServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")

		authHeader := r.Header.Get("Authorization")
		if authHeader != "Bearer valid-token" {
			w.WriteHeader(http.StatusUnauthorized)
			_ = json.NewEncoder(w).Encode(map[string]string{"message": "Bad credentials"})
			return
		}

		switch r.URL.Path {
		case "/user":
			w.Header().Set("X-RateLimit-Remaining", "4999")
			w.Header().Set("X-RateLimit-Limit", "5000")
			_ = json.NewEncoder(w).Encode(map[string]any{
				"login": "kerob-bot",
				"id":    12345,
			})

		case "/repos/octocat/hello-world/issues":
			if r.Method == http.MethodPost {
				var reqBody map[string]any
				_ = json.NewDecoder(r.Body).Decode(&reqBody)
				w.WriteHeader(http.StatusCreated)
				_ = json.NewEncoder(w).Encode(map[string]any{
					"number":   42,
					"html_url": "https://github.com/octocat/hello-world/issues/42",
					"title":    reqBody["title"],
					"state":    "open",
				})
				return
			}

		case "/repos/octocat/hello-world/issues/42/comments":
			if r.Method == http.MethodPost {
				w.WriteHeader(http.StatusCreated)
				_ = json.NewEncoder(w).Encode(map[string]any{
					"id":       1001,
					"html_url": "https://github.com/octocat/hello-world/issues/42#comment-1001",
				})
				return
			}

		default:
			http.NotFound(w, r)
		}
	}))
	defer mockServer.Close()

	conn := github.NewConnector(github.Config{
		Client:  mockServer.Client(),
		BaseURL: mockServer.URL,
	})

	ctx := context.Background()

	// 1. Health check with valid token
	status, err := conn.CheckHealth(ctx, map[string]string{"access_token": "valid-token"})
	if err != nil {
		t.Fatalf("health check failed: %v", err)
	}
	if status.Status != "healthy" {
		t.Fatalf("expected healthy status, got %s (%s)", status.Status, status.Message)
	}

	// 2. Health check with invalid token
	statusBad, _ := conn.CheckHealth(ctx, map[string]string{"access_token": "wrong-token"})
	if statusBad.Status != "unhealthy" {
		t.Fatalf("expected unhealthy status, got %s", statusBad.Status)
	}

	// 3. Execute Create Issue action
	actRes, err := conn.ExecuteAction(ctx, connector.ActionRequest{
		ActionType: "create_issue",
		Parameters: map[string]any{
			"owner": "octocat",
			"repo":  "hello-world",
			"title": "Bug: Pipeline stalled",
			"body":  "Investigate step 3 failure",
		},
		Credentials: map[string]string{"access_token": "valid-token"},
	})
	if err != nil {
		t.Fatalf("create_issue action failed: %v", err)
	}
	if !actRes.Success {
		t.Fatalf("expected action success")
	}
	if actRes.Output["issue_number"] != float64(42) {
		t.Fatalf("expected issue number 42, got %v", actRes.Output["issue_number"])
	}

	// 4. Execute Post Comment action
	commentRes, err := conn.ExecuteAction(ctx, connector.ActionRequest{
		ActionType: "post_comment",
		Parameters: map[string]any{
			"owner":        "octocat",
			"repo":         "hello-world",
			"issue_number": 42,
			"body":         "Investigating now",
		},
		Credentials: map[string]string{"access_token": "valid-token"},
	})
	if err != nil {
		t.Fatalf("post_comment action failed: %v", err)
	}
	if !commentRes.Success {
		t.Fatalf("expected comment success")
	}
	if commentRes.Output["comment_id"] != float64(1001) {
		t.Fatalf("expected comment id 1001, got %v", commentRes.Output["comment_id"])
	}
}

func TestGitHubWebhookSignatureVerification(t *testing.T) {
	conn := github.NewConnector(github.Config{})

	secret := "webhook-secret-key-123"
	payload := []byte(`{"action":"opened","issue":{"number":1}}`)

	// Compute valid HMAC-SHA256 signature
	// echo -n '...' | openssl dgst -sha256 -hmac '...'
	validSigHeader := "sha256=174ffb9a22f46cfc6e93e0aa0a359228800882e5ee6f590bb063ea595d2c20a4"
	// Calculate actual programmatically
	mac := githubComputeHMAC(secret, payload)

	headers := map[string]string{
		"X-Hub-Signature-256": mac,
	}

	if !conn.VerifyWebhookSignature(secret, headers, payload) {
		t.Errorf("valid signature rejected")
	}

	headers["X-Hub-Signature-256"] = validSigHeader + "tampered"
	if conn.VerifyWebhookSignature(secret, headers, payload) {
		t.Errorf("tampered signature accepted")
	}
}

func githubComputeHMAC(secret string, payload []byte) string {
	mac := hmac.New(sha256.New, []byte(secret))
	mac.Write(payload)
	return "sha256=" + hex.EncodeToString(mac.Sum(nil))
}
