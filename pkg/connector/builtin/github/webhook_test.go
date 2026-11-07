package github

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"kerob/pkg/connector"
)

func TestValidateTriggerAndNormalizeEvent(t *testing.T) {
	conn := NewConnector(Config{})

	// ValidateTrigger
	if err := conn.ValidateTrigger("github.push", nil); err != nil {
		t.Errorf("unexpected error for github.push: %v", err)
	}
	if err := conn.ValidateTrigger("gitlab.push", nil); err == nil {
		t.Error("expected error for non-github trigger")
	}

	// NormalizeEvent with issue and repository
	payload := []byte(`{
		"repository": {"full_name": "acme/corp"},
		"issue": {
			"title": "Bug found",
			"body": "Fix it",
			"number": 12,
			"html_url": "https://github.com/acme/corp/issues/12",
			"user": {"login": "mona"}
		},
		"comment": {
			"body": "I agree",
			"id": 99
		}
	}`)

	evt, err := conn.NormalizeEvent("github.issue.commented", nil, payload)
	if err != nil {
		t.Fatalf("NormalizeEvent failed: %v", err)
	}
	if evt["title"] != "Bug found" || evt["author"] != "mona" || evt["repository"] != "acme/corp" {
		t.Errorf("unexpected normalized event: %+v", evt)
	}

	// NormalizeEvent with PR
	prPayload := []byte(`{
		"pull_request": {
			"title": "New feature",
			"body": "PR description",
			"number": 5,
			"html_url": "https://github.com/acme/corp/pull/5",
			"user": {"login": "hubot"}
		}
	}`)
	evt, err = conn.NormalizeEvent("github.pull_request.opened", nil, prPayload)
	if err != nil {
		t.Fatalf("NormalizeEvent PR failed: %v", err)
	}
	if evt["title"] != "New feature" || evt["author"] != "hubot" {
		t.Errorf("unexpected normalized PR event: %+v", evt)
	}

	// Invalid payload
	_, err = conn.NormalizeEvent("github.push", nil, []byte("invalid-json"))
	if err == nil {
		t.Error("expected error for invalid json")
	}
}

func TestWebhookSetupAndTeardown(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodPost && r.URL.Path == "/repos/acme/widgets/hooks":
			w.WriteHeader(http.StatusCreated)
			_ = json.NewEncoder(w).Encode(map[string]any{
				"id": 8888,
			})
		case r.Method == http.MethodDelete && r.URL.Path == "/repos/acme/widgets/hooks/8888":
			w.WriteHeader(http.StatusNoContent)
		default:
			w.WriteHeader(http.StatusBadRequest)
		}
	}))
	defer server.Close()

	conn := NewConnector(Config{
		Client:  server.Client(),
		BaseURL: server.URL,
	})
	ctx := context.Background()

	// Missing token
	_, err := conn.SetupWebhook(ctx, connector.WebhookSetupRequest{})
	if err == nil {
		t.Error("expected error for missing token")
	}

	// Missing owner/repo
	_, err = conn.SetupWebhook(ctx, connector.WebhookSetupRequest{
		Credentials: map[string]string{"access_token": "token"},
	})
	if err == nil {
		t.Error("expected error for missing config")
	}

	// Successful setup
	info, err := conn.SetupWebhook(ctx, connector.WebhookSetupRequest{
		Credentials: map[string]string{"access_token": "token"},
		Config:      map[string]any{"owner": "acme", "repo": "widgets"},
		WebhookURL:  "https://kerob.example.com/webhook",
		Secret:      "hook-secret",
	})
	if err != nil {
		t.Fatalf("SetupWebhook failed: %v", err)
	}
	if info.ID != "8888" {
		t.Errorf("expected hook ID 8888, got %s", info.ID)
	}

	// Teardown webhook
	err = conn.TeardownWebhook(ctx, "8888", map[string]string{
		"access_token": "token",
		"owner":        "acme",
		"repo":         "widgets",
	})
	if err != nil {
		t.Fatalf("TeardownWebhook failed: %v", err)
	}

	// Teardown missing token
	if err := conn.TeardownWebhook(ctx, "8888", nil); err == nil {
		t.Error("expected error for nil creds in teardown")
	}
}

func TestVerifyWebhookSignature(t *testing.T) {
	conn := NewConnector(Config{})
	secret := "my-secret-key"
	payload := []byte(`{"action":"opened"}`)

	mac := hmac.New(sha256.New, []byte(secret))
	mac.Write(payload)
	validSig := hex.EncodeToString(mac.Sum(nil))

	headers := map[string]string{
		"X-Hub-Signature-256": "sha256=" + validSig,
	}

	// Valid signature
	if !conn.VerifyWebhookSignature(secret, headers, payload) {
		t.Error("expected valid signature verification")
	}

	// Empty secret (always true)
	if !conn.VerifyWebhookSignature("", headers, payload) {
		t.Error("expected true when secret is empty")
	}

	// Invalid signature
	badHeaders := map[string]string{
		"X-Hub-Signature-256": "sha256=invalidhexsignature0000",
	}
	if conn.VerifyWebhookSignature(secret, badHeaders, payload) {
		t.Error("expected invalid signature verification")
	}

	// Missing header
	if conn.VerifyWebhookSignature(secret, map[string]string{}, payload) {
		t.Error("expected false for missing signature header")
	}
}
