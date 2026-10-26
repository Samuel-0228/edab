package actions

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"testing"

	"kerob/internal/core/event"
	"kerob/internal/core/workflow"
	"kerob/internal/telegram"
)

type roundTripperFunc func(req *http.Request) (*http.Response, error)

func (f roundTripperFunc) RoundTrip(req *http.Request) (*http.Response, error) {
	return f(req)
}

func mockHTTPClient(handler func(req *http.Request) (*http.Response, error)) *http.Client {
	return &http.Client{
		Transport: roundTripperFunc(handler),
	}
}

func jsonResponse(status int, body any) *http.Response {
	b, _ := json.Marshal(body)
	return &http.Response{
		StatusCode: status,
		Body:       io.NopCloser(bytes.NewReader(b)),
		Header:     make(http.Header),
	}
}

func TestHTTPActionValidation(t *testing.T) {
	executor := NewHTTPActionExecutor()

	if executor.Name() != "Generic HTTP Action" || executor.Type() != "http" {
		t.Fatalf("unexpected name or type: %s %s", executor.Name(), executor.Type())
	}

	// Missing URL
	if err := executor.Validate(json.RawMessage(`{"method":"POST"}`)); err == nil {
		t.Fatal("expected validation error for missing URL")
	}

	// Invalid method
	if err := executor.Validate(json.RawMessage(`{"url":"https://example.com","method":"INVALID"}`)); err == nil {
		t.Fatal("expected validation error for invalid HTTP method")
	}

	// Malformed JSON
	if err := executor.Validate(json.RawMessage(`{malformed`)); err == nil {
		t.Fatal("expected validation error for malformed json")
	}

	// Valid config
	if err := executor.Validate(json.RawMessage(`{"url":"https://example.com","method":"POST"}`)); err != nil {
		t.Fatalf("unexpected validation error: %v", err)
	}
}

func TestHTTPActionExecution(t *testing.T) {
	ev, _ := event.New("src-test", "evt-1", "test.event", map[string]any{"data": "val"}, nil)

	// 1. Success with custom headers, query params, and body
	var capturedReq *http.Request
	client := mockHTTPClient(func(req *http.Request) (*http.Response, error) {
		capturedReq = req
		return jsonResponse(http.StatusOK, map[string]string{"result": "created"}), nil
	})

	executor := NewHTTPActionExecutorWithClient(client)

	cfg := json.RawMessage(`{
		"method": "POST",
		"url": "https://8.8.8.8/v1/resource",
		"query_params": {"ref": "main", "active": "true"},
		"headers": {"X-Custom-Trace": "trace-123"},
		"body": "{\"custom\":\"payload\"}"
	}`)

	res, err := executor.Execute(context.Background(), workflow.ActionRequest{
		Config: cfg,
		Event:  ev,
		Credentials: map[string]string{
			"api_key": "secret-api-token",
		},
	})
	if err != nil {
		t.Fatalf("expected success, got error: %v", err)
	}
	if !res.Success {
		t.Fatalf("expected res.Success to be true")
	}
	if capturedReq.Header.Get("Authorization") != "Bearer secret-api-token" {
		t.Errorf("expected Bearer token, got: %s", capturedReq.Header.Get("Authorization"))
	}
	if capturedReq.Header.Get("X-Custom-Trace") != "trace-123" {
		t.Errorf("missing custom header")
	}
	if !strings.Contains(capturedReq.URL.RawQuery, "ref=main") {
		t.Errorf("expected query params in url: %s", capturedReq.URL.String())
	}

	// 2. HTTP error response (status 404 / 500)
	clientErr := mockHTTPClient(func(req *http.Request) (*http.Response, error) {
		return jsonResponse(http.StatusNotFound, map[string]string{"error": "not found"}), nil
	})
	executorErr := NewHTTPActionExecutorWithClient(clientErr)

	res, err = executorErr.Execute(context.Background(), workflow.ActionRequest{
		Config: cfg,
		Event:  ev,
	})
	if err == nil || res.Success {
		t.Fatalf("expected error on 404 response")
	}

	// 3. Client network error
	clientNetErr := mockHTTPClient(func(req *http.Request) (*http.Response, error) {
		return nil, fmt.Errorf("network connection reset")
	})
	executorNetErr := NewHTTPActionExecutorWithClient(clientNetErr)
	_, err = executorNetErr.Execute(context.Background(), workflow.ActionRequest{
		Config: cfg,
		Event:  ev,
	})
	if err == nil {
		t.Fatalf("expected error on network reset")
	}
}

func TestDiscordActionValidationAndExecution(t *testing.T) {
	ev, _ := event.New("src-test", "evt-1", "test.event", map[string]any{"data": "val"}, nil)

	// Success with injected client
	var postedBody []byte
	client := mockHTTPClient(func(req *http.Request) (*http.Response, error) {
		postedBody, _ = io.ReadAll(req.Body)
		return jsonResponse(http.StatusNoContent, nil), nil
	})

	executor := NewDiscordActionExecutorWithClient(client)
	if executor.Name() != "Discord" || executor.Type() != "discord" {
		t.Fatalf("unexpected name or type: %s %s", executor.Name(), executor.Type())
	}

	cfg := json.RawMessage(`{
		"webhook_url": "https://8.8.8.8/api/webhooks/123/xyz",
		"content": "Alert from test",
		"username": "Kerob Alert",
		"avatar_url": "https://8.8.8.8/logo.png",
		"embed": {
			"title": "CPU Overload",
			"description": "Host node-1 usage > 90%",
			"color": 16711680,
			"fields": [{"name": "Usage", "value": "95%", "inline": true}]
		}
	}`)

	res, err := executor.Execute(context.Background(), workflow.ActionRequest{
		Config: cfg,
		Event:  ev,
	})
	if err != nil {
		t.Fatalf("failed to execute discord action: %v", err)
	}
	if !res.Success {
		t.Fatalf("expected res.Success to be true")
	}
	if !strings.Contains(string(postedBody), "CPU Overload") {
		t.Errorf("expected embed title in posted discord payload: %s", string(postedBody))
	}

	// Discord error response (status 400)
	client400 := mockHTTPClient(func(req *http.Request) (*http.Response, error) {
		return jsonResponse(http.StatusBadRequest, map[string]string{"message": "invalid embed"}), nil
	})
	exec400 := NewDiscordActionExecutorWithClient(client400)
	_, err = exec400.Execute(context.Background(), workflow.ActionRequest{
		Config: cfg,
		Event:  ev,
	})
	if err == nil {
		t.Fatalf("expected error on 400 discord response")
	}

	// Missing webhook URL
	_, err = executor.Execute(context.Background(), workflow.ActionRequest{
		Config: json.RawMessage(`{"content":"hello"}`),
		Event:  ev,
	})
	if err == nil {
		t.Fatal("expected error when webhook_url missing")
	}
}

func TestGitHubActionValidationAndExecution(t *testing.T) {
	ev, _ := event.New("src-test", "evt-1", "test.event", map[string]any{"data": "val"}, nil)

	// Success with injected client
	client := mockHTTPClient(func(req *http.Request) (*http.Response, error) {
		if !strings.HasPrefix(req.Header.Get("Authorization"), "token ") {
			t.Errorf("missing token in authorization header: %s", req.Header.Get("Authorization"))
		}
		return jsonResponse(http.StatusCreated, map[string]any{
			"id":       101,
			"number":   42,
			"html_url": "https://github.com/myorg/myrepo/issues/42",
		}), nil
	})

	executor := NewGitHubActionExecutorWithClient(client)
	if executor.Name() != "GitHub" || executor.Type() != "github" {
		t.Fatalf("unexpected name or type: %s %s", executor.Name(), executor.Type())
	}

	cfg := json.RawMessage(`{
		"owner": "myorg",
		"repo": "myrepo",
		"title": "Bug in automation",
		"body": "Detailed report",
		"labels": ["bug", "automated"],
		"api_base_url": "https://8.8.8.8"
	}`)

	res, err := executor.Execute(context.Background(), workflow.ActionRequest{
		Config: cfg,
		Event:  ev,
		Credentials: map[string]string{
			"token": "ghp_mocktesttoken12345678901234567890",
		},
	})
	if err != nil {
		t.Fatalf("failed to execute github action: %v", err)
	}
	if !res.Success {
		t.Fatalf("expected res.Success to be true")
	}
	if res.ExternalID != "issue#42" {
		t.Errorf("expected ExternalID 'issue#42', got: %s", res.ExternalID)
	}

	// GitHub API 403 Forbidden
	client403 := mockHTTPClient(func(req *http.Request) (*http.Response, error) {
		return jsonResponse(http.StatusForbidden, map[string]string{"message": "Resource not accessible by personal access token"}), nil
	})
	exec403 := NewGitHubActionExecutorWithClient(client403)
	_, err = exec403.Execute(context.Background(), workflow.ActionRequest{
		Config: cfg,
		Event:  ev,
	})
	if err == nil {
		t.Fatalf("expected error on github 403")
	}

	malformed := NewGitHubActionExecutorWithClient(mockHTTPClient(func(req *http.Request) (*http.Response, error) {
		return &http.Response{
			StatusCode: http.StatusCreated,
			Body:       io.NopCloser(strings.NewReader(`{"id":"not-an-issue-response"}`)),
			Header:     make(http.Header),
		}, nil
	}))
	_, err = malformed.Execute(context.Background(), workflow.ActionRequest{
		Config:      cfg,
		Event:       ev,
		Credentials: map[string]string{"token": "test-token"},
	})
	if err == nil {
		t.Fatal("expected malformed GitHub success response to fail")
	}
}

func TestHTTPActionCancellationIsPropagated(t *testing.T) {
	ev, _ := event.New("src-test", "evt-1", "test.event", nil, nil)
	client := mockHTTPClient(func(req *http.Request) (*http.Response, error) {
		<-req.Context().Done()
		return nil, req.Context().Err()
	})
	executor := NewHTTPActionExecutorWithClient(client)
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	_, err := executor.Execute(ctx, workflow.ActionRequest{
		Config: json.RawMessage(`{"url":"https://8.8.8.8/slow","method":"GET"}`),
		Event:  ev,
	})
	if err == nil || !strings.Contains(err.Error(), "context canceled") {
		t.Fatalf("expected cancellation error, got %v", err)
	}
}

func TestTelegramActionExecutor(t *testing.T) {
	mockAPI := telegram.NewMockAPI()
	executor := NewTelegramActionExecutor(mockAPI)

	if executor.Name() != "Telegram" || executor.Type() != "telegram" {
		t.Fatalf("unexpected name or type: %s %s", executor.Name(), executor.Type())
	}

	// Validate config
	if err := executor.Validate(json.RawMessage(`{}`)); err == nil {
		t.Fatal("expected error when chat_id missing")
	}
	if err := executor.Validate(json.RawMessage(`{"chat_id":"-10012345"}`)); err != nil {
		t.Fatalf("unexpected validation error: %v", err)
	}

	ev, _ := event.New("src-test", "evt-1", "test.event", map[string]any{"data": "val"}, nil)

	// Execute send message
	cfg := json.RawMessage(`{"chat_id":"-10012345","text":"Alert message"}`)
	res, err := executor.Execute(context.Background(), workflow.ActionRequest{
		Config: cfg,
		Event:  ev,
	})
	if err != nil {
		t.Fatalf("failed to execute telegram action: %v", err)
	}
	if !res.Success {
		t.Fatalf("expected success result")
	}

	// Execute with missing API client
	nilExec := NewTelegramActionExecutor(nil)
	_, err = nilExec.Execute(context.Background(), workflow.ActionRequest{
		Config: cfg,
		Event:  ev,
	})
	if err == nil {
		t.Fatal("expected error when Telegram API is uninitialized")
	}

	// Validate malformed config
	if err := executor.Validate(json.RawMessage(`{invalid`)); err == nil {
		t.Fatal("expected error on malformed json")
	}

	// Execute with invalid json
	if _, err := executor.Execute(context.Background(), workflow.ActionRequest{
		Config: json.RawMessage(`{invalid`),
		Event:  ev,
	}); err == nil {
		t.Fatal("expected error on invalid config in Execute")
	}

	// Execute with non-numeric chat_id
	if _, err := executor.Execute(context.Background(), workflow.ActionRequest{
		Config: json.RawMessage(`{"chat_id":"not-a-number","text":"hi"}`),
		Event:  ev,
	}); err == nil {
		t.Fatal("expected error on invalid chat_id format")
	}
}

func TestDiscordActionExecutor(t *testing.T) {
	executor := NewDiscordActionExecutor()
	if executor.Name() != "Discord" || executor.Type() != "discord" {
		t.Fatalf("unexpected name or type: %s %s", executor.Name(), executor.Type())
	}

	// Validate
	if err := executor.Validate(json.RawMessage(`{"webhook_url":"https://8.8.8.8/hook"}`)); err != nil {
		t.Fatalf("expected valid config, got %v", err)
	}
	if err := executor.Validate(json.RawMessage(`{malformed`)); err == nil {
		t.Fatal("expected validation error on malformed json")
	}

	ev, _ := event.New("src-1", "ext-1", "test.event", nil, nil)

	// Execute with invalid json
	if _, err := executor.Execute(context.Background(), workflow.ActionRequest{
		Config: json.RawMessage(`{malformed`),
		Event:  ev,
	}); err == nil {
		t.Fatal("expected error for malformed json")
	}

	// Execute with missing webhook url and no credentials
	if _, err := executor.Execute(context.Background(), workflow.ActionRequest{
		Config: json.RawMessage(`{"content":"hello"}`),
		Event:  ev,
	}); err == nil {
		t.Fatal("expected error for missing webhook url")
	}

	// Execute with SSRF-blocked URL
	if _, err := executor.Execute(context.Background(), workflow.ActionRequest{
		Config: json.RawMessage(`{"webhook_url":"http://127.0.0.1:8080/webhook","content":"hello"}`),
		Event:  ev,
	}); err == nil {
		t.Fatal("expected SSRF error")
	}

	// Execute with mock client - HTTP 500 error
	mockClient := mockHTTPClient(func(req *http.Request) (*http.Response, error) {
		return &http.Response{
			StatusCode: http.StatusInternalServerError,
			Body:       io.NopCloser(strings.NewReader("internal server error")),
			Header:     make(http.Header),
		}, nil
	})
	clientExec := NewDiscordActionExecutorWithClient(mockClient)
	res, err := clientExec.Execute(context.Background(), workflow.ActionRequest{
		Config: json.RawMessage(`{"webhook_url":"https://8.8.8.8/webhook","content":"hello"}`),
		Event:  ev,
	})
	if err == nil {
		t.Fatal("expected error on 500 response")
	}
	if res.Success {
		t.Fatal("expected res.Success to be false")
	}
}

func TestGitHubActionExecutor(t *testing.T) {
	executor := NewGitHubActionExecutor()
	if executor.Name() != "GitHub" || executor.Type() != "github" {
		t.Fatalf("unexpected name or type: %s %s", executor.Name(), executor.Type())
	}

	// Validate
	if err := executor.Validate(json.RawMessage(`{"owner":"owner","repo":"repo","title":"Issue"}`)); err != nil {
		t.Fatalf("expected valid config, got %v", err)
	}
	if err := executor.Validate(json.RawMessage(`{"owner":""}`)); err == nil {
		t.Fatal("expected error when owner or repo missing")
	}
	if err := executor.Validate(json.RawMessage(`{malformed`)); err == nil {
		t.Fatal("expected validation error on malformed json")
	}

	ev, _ := event.New("src-1", "ext-1", "test.event", nil, nil)

	// Execute with invalid json
	if _, err := executor.Execute(context.Background(), workflow.ActionRequest{
		Config: json.RawMessage(`{malformed`),
		Event:  ev,
	}); err == nil {
		t.Fatal("expected error for malformed json")
	}

	// Execute without token
	if _, err := executor.Execute(context.Background(), workflow.ActionRequest{
		Config: json.RawMessage(`{"owner":"org","repo":"repo","title":"Bug"}`),
		Event:  ev,
	}); err == nil {
		t.Fatal("expected error when token is missing")
	}

	// Execute without title
	if _, err := executor.Execute(context.Background(), workflow.ActionRequest{
		Config: json.RawMessage(`{"owner":"org","repo":"repo","title":""}`),
		Event:  ev,
		Credentials: map[string]string{
			"token": "ghp_mock_token_123",
		},
	}); err == nil {
		t.Fatal("expected error when title is empty")
	}

	// Execute with HTTP 403 error
	mockClient := mockHTTPClient(func(req *http.Request) (*http.Response, error) {
		return &http.Response{
			StatusCode: http.StatusForbidden,
			Body:       io.NopCloser(strings.NewReader(`{"message":"Forbidden"}`)),
			Header:     make(http.Header),
		}, nil
	})
	clientExec := NewGitHubActionExecutorWithClient(mockClient)
	res, err := clientExec.Execute(context.Background(), workflow.ActionRequest{
		Config: json.RawMessage(`{"owner":"org","repo":"repo","title":"Bug"}`),
		Event:  ev,
		Credentials: map[string]string{
			"personal_access_token": "ghp_mock_token_123",
		},
	})
	if err == nil {
		t.Fatal("expected error on 403 response")
	}
	if res.Success {
		t.Fatal("expected res.Success to be false")
	}
}
