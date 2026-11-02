package oauth_test

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"kerob/internal/core/credentials"
	"kerob/internal/core/oauth"
	"kerob/pkg/connector"
	"kerob/pkg/connector/builtin/github"
)

func setupTestOAuthManager(t *testing.T) (*oauth.Manager, *credentials.MemoryStore, *connector.Registry, *httptest.Server) {
	t.Helper()

	// Upstream OAuth mock server
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")

		if r.URL.Path == "/login/oauth/access_token" {
			_ = r.ParseForm()
			if r.FormValue("grant_type") == "refresh_token" {
				if r.FormValue("refresh_token") == "valid-refresh-token" {
					_ = json.NewEncoder(w).Encode(map[string]any{
						"access_token":  "gho_new_rotated_access_999",
						"refresh_token": "ghr_new_rotated_refresh_888",
						"token_type":    "bearer",
						"expires_in":    3600,
					})
					return
				}
				w.WriteHeader(http.StatusBadRequest)
				_ = json.NewEncoder(w).Encode(map[string]string{"error": "invalid_grant"})
				return
			}

			if r.FormValue("code") == "auth-code-123" {
				_ = json.NewEncoder(w).Encode(map[string]any{
					"access_token":  "gho_valid_oauth_access_777",
					"refresh_token": "valid-refresh-token",
					"token_type":    "bearer",
					"expires_in":    7200,
					"scope":         "repo,read:user",
				})
				return
			}

			w.WriteHeader(http.StatusBadRequest)
			_ = json.NewEncoder(w).Encode(map[string]string{"error": "bad_code"})
			return
		}

		if strings.HasPrefix(r.URL.Path, "/applications/") && r.Method == http.MethodDelete {
			w.WriteHeader(http.StatusNoContent)
			return
		}

		http.NotFound(w, r)
	}))

	// Registry with GitHub connector pointing to mock server
	reg := connector.NewRegistry()
	gh := github.NewConnector(github.Config{
		Client:       server.Client(),
		BaseURL:      server.URL,
		AuthURL:      server.URL + "/login/oauth/authorize",
		TokenURL:     server.URL + "/login/oauth/access_token",
		ClientID:     "test-client-id",
		ClientSecret: "test-client-secret",
	})
	_ = reg.Register(gh)

	key := credentials.DeriveKey("test-aes-key-for-oauth-tests")
	credStore := credentials.NewMemoryStore(key)

	stateMgr := oauth.NewStateManager(10 * time.Minute)
	mgr := oauth.NewManager(reg, credStore, stateMgr)

	return mgr, credStore, reg, server
}

func TestOAuthCSRFProtection(t *testing.T) {
	mgr, _, _, server := setupTestOAuthManager(t)
	defer server.Close()

	ctx := context.Background()

	// 1. Generate legitimate state
	authURL, state, err := mgr.GetAuthorizeURL(ctx, "github", "tenant-1", "user-1", "https://app.kerob.dev/callback", nil)
	if err != nil {
		t.Fatalf("GetAuthorizeURL failed: %v", err)
	}
	if state == "" {
		t.Fatal("expected non-empty state")
	}
	if !strings.Contains(authURL, "state="+state) {
		t.Fatalf("authURL missing state parameter: %s", authURL)
	}

	// 2. Reject unknown / forged state token (CSRF Attack)
	_, err = mgr.HandleCallback(ctx, "github", "forged-csrf-state-attacker", "auth-code-123", "https://app.kerob.dev/callback")
	if err == nil {
		t.Fatal("SECURITY FAILURE: forged state was accepted")
	}

	// 3. Reject replay attack (Single-use consumption)
	// First callback with valid state succeeds
	cred, err := mgr.HandleCallback(ctx, "github", state, "auth-code-123", "https://app.kerob.dev/callback")
	if err != nil {
		t.Fatalf("valid callback failed: %v", err)
	}
	if cred == nil || cred.ID == "" {
		t.Fatal("expected stored credential")
	}

	// Immediate second attempt with same state MUST fail
	_, err = mgr.HandleCallback(ctx, "github", state, "auth-code-123", "https://app.kerob.dev/callback")
	if err == nil {
		t.Fatal("SECURITY FAILURE: replayed state token was accepted")
	}
}

func TestOAuthStateExpiration(t *testing.T) {
	reg := connector.NewRegistry()
	credStore := credentials.NewMemoryStore(credentials.DeriveKey("test-key"))
	// State manager with 1 millisecond TTL
	shortStateMgr := oauth.NewStateManager(1 * time.Millisecond)
	mgr := oauth.NewManager(reg, credStore, shortStateMgr)

	gh := github.NewConnector(github.Config{
		ClientID:     "client-id",
		ClientSecret: "client-secret",
	})
	_ = reg.Register(gh)

	_, state, err := mgr.GetAuthorizeURL(context.Background(), "github", "tenant-1", "user-1", "", nil)
	if err != nil {
		t.Fatalf("GetAuthorizeURL failed: %v", err)
	}

	// Wait for state to expire
	time.Sleep(10 * time.Millisecond)

	_, err = mgr.HandleCallback(context.Background(), "github", state, "any-code", "")
	if err == nil {
		t.Fatal("SECURITY FAILURE: expired state token was accepted")
	}
	if !strings.Contains(err.Error(), "expired") {
		t.Fatalf("expected expiration error, got: %v", err)
	}
}

func TestOAuthTokenEncryptionAndSecrecy(t *testing.T) {
	mgr, credStore, _, server := setupTestOAuthManager(t)
	defer server.Close()

	ctx := context.Background()

	_, state, err := mgr.GetAuthorizeURL(ctx, "github", "tenant-alpha", "user-admin", "https://app.kerob.dev/callback", nil)
	if err != nil {
		t.Fatalf("GetAuthorizeURL failed: %v", err)
	}

	// Exchange code
	cred, err := mgr.HandleCallback(ctx, "github", state, "auth-code-123", "https://app.kerob.dev/callback")
	if err != nil {
		t.Fatalf("HandleCallback failed: %v", err)
	}

	// Verify Tenant Isolation
	if cred.TenantID != "tenant-alpha" {
		t.Fatalf("expected tenant-alpha, got %s", cred.TenantID)
	}

	// 1. Verify that raw access_token is NEVER in cred struct or JSON serialization
	rawJSON, err := json.Marshal(cred)
	if err != nil {
		t.Fatalf("json marshal failed: %v", err)
	}
	rawStr := string(rawJSON)
	if strings.Contains(rawStr, "gho_valid_oauth_access_777") {
		t.Fatalf("SECURITY VIOLATION: plaintext access token exposed in JSON: %s", rawStr)
	}
	if strings.Contains(rawStr, "valid-refresh-token") {
		t.Fatalf("SECURITY VIOLATION: plaintext refresh token exposed in JSON: %s", rawStr)
	}

	// 2. Verify masked fields exist for safe UI display
	if cred.MaskedFields["access_token"] == "" || !strings.HasPrefix(cred.MaskedFields["access_token"], "••••••••") {
		t.Fatalf("access token not properly masked: %v", cred.MaskedFields)
	}

	// 3. Verify AES-256-GCM encryption in storage
	storedCred, err := credStore.GetByID(ctx, cred.ID)
	if err != nil {
		t.Fatalf("failed to retrieve stored cred: %v", err)
	}
	if len(storedCred.EncryptedData) == 0 {
		t.Fatal("EncryptedData is empty in storage")
	}
	if strings.Contains(string(storedCred.EncryptedData), "gho_valid_oauth_access_777") {
		t.Fatal("SECURITY VIOLATION: plaintext token found in encrypted payload")
	}

	// 4. Verify decryption with vault key restores tokens safely for internal worker execution
	decrypted, err := credStore.GetDecrypted(ctx, cred.ID)
	if err != nil {
		t.Fatalf("failed to decrypt credentials: %v", err)
	}
	if decrypted["access_token"] != "gho_valid_oauth_access_777" {
		t.Fatalf("decrypted access token mismatch: %s", decrypted["access_token"])
	}
	if decrypted["refresh_token"] != "valid-refresh-token" {
		t.Fatalf("decrypted refresh token mismatch: %s", decrypted["refresh_token"])
	}
}

func TestOAuthTokenRotationAndRevocation(t *testing.T) {
	mgr, _, _, server := setupTestOAuthManager(t)
	defer server.Close()

	ctx := context.Background()

	_, state, err := mgr.GetAuthorizeURL(ctx, "github", "tenant-1", "user-1", "", nil)
	if err != nil {
		t.Fatalf("GetAuthorizeURL failed: %v", err)
	}

	cred, err := mgr.HandleCallback(ctx, "github", state, "auth-code-123", "")
	if err != nil {
		t.Fatalf("HandleCallback failed: %v", err)
	}

	// 1. Perform refresh-token rotation
	updatedCred, err := mgr.RefreshToken(ctx, "github", cred.ID)
	if err != nil {
		t.Fatalf("RefreshToken failed: %v", err)
	}

	// Check masked values updated
	if updatedCred.MaskedFields["access_token"] == "" {
		t.Fatal("expected masked access_token after rotation")
	}

	// 2. Perform revocation
	err = mgr.RevokeToken(ctx, "github", cred.ID)
	if err != nil {
		t.Fatalf("RevokeToken failed: %v", err)
	}

	// Verify credential was purged from vault
	_, err = mgr.RefreshToken(ctx, "github", cred.ID)
	if err == nil {
		t.Fatal("expected error refreshing deleted/revoked credential")
	}
}
