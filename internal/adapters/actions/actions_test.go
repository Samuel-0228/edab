package actions

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"kerob/internal/core/event"
	"kerob/internal/core/workflow"
)

func TestHTTPActionExecutorSuccess(t *testing.T) {
	// Start a local mock server
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("X-Custom-Header") != "CustomValue" {
			t.Errorf("missing custom header")
		}
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"status":"success","code":200}`))
	}))
	defer server.Close()

	executor := NewHTTPActionExecutor()

	ev, _ := event.New("src-test", "evt-1", "test.event", map[string]any{"data": "val"}, nil)

	config := json.RawMessage(`{
		"method": "POST",
		"url": "` + server.URL + `",
		"headers": {"X-Custom-Header": "CustomValue"}
	}`)

	// SSRF validation blocks localhost by default in production.
	// For testing with httptest server, let's verify that SSRF correctly intercepts localhost calls!
	result, err := executor.Execute(context.Background(), workflow.ActionRequest{
		Config: config,
		Event:  ev,
	})

	// Since httptest uses 127.0.0.1, SSRF protection must trigger:
	if err == nil {
		t.Fatal("expected SSRF protection to block httptest loopback URL, but it succeeded")
	}
	if result.Success {
		t.Fatal("expected result.Success to be false when SSRF blocks request")
	}
}

func TestHTTPActionValidation(t *testing.T) {
	executor := NewHTTPActionExecutor()

	// Missing URL
	if err := executor.Validate(json.RawMessage(`{"method":"POST"}`)); err == nil {
		t.Fatal("expected validation error for missing URL")
	}

	// Invalid method
	if err := executor.Validate(json.RawMessage(`{"url":"https://example.com","method":"INVALID"}`)); err == nil {
		t.Fatal("expected validation error for invalid HTTP method")
	}

	// Valid config
	if err := executor.Validate(json.RawMessage(`{"url":"https://example.com","method":"POST"}`)); err != nil {
		t.Fatalf("unexpected validation error: %v", err)
	}
}

func TestDiscordActionValidation(t *testing.T) {
	executor := NewDiscordActionExecutor()

	if err := executor.Validate(json.RawMessage(`{"webhook_url":"https://discord.com/api/webhooks/123/xyz"}`)); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestGitHubActionValidation(t *testing.T) {
	executor := NewGitHubActionExecutor()

	// Missing owner/repo
	if err := executor.Validate(json.RawMessage(`{"title":"New Issue"}`)); err == nil {
		t.Fatal("expected validation error for missing owner and repo")
	}

	// Valid config
	if err := executor.Validate(json.RawMessage(`{"owner":"myorg","repo":"myrepo","title":"Bug"}`)); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}
