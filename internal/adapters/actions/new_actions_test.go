package actions

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/smtp"
	"strings"
	"testing"

	"kerob/internal/core/event"
	"kerob/internal/core/workflow"
)

func TestSlackActionExecutorWebhookAndAPI(t *testing.T) {
	var receivedURL string
	var receivedAuth string
	var receivedBody []byte

	client := mockHTTPClient(func(req *http.Request) (*http.Response, error) {
		receivedURL = req.URL.String()
		receivedAuth = req.Header.Get("Authorization")
		receivedBody, _ = io.ReadAll(req.Body)

		if strings.Contains(receivedURL, "chat.postMessage") {
			return jsonResponse(http.StatusOK, map[string]any{"ok": true, "ts": "1234567890.123456"}), nil
		}
		return jsonResponse(http.StatusOK, map[string]any{"status": "ok"}), nil
	})

	exec := NewSlackActionExecutorWithClient(client)

	// 1. Validation test
	if err := exec.Validate(json.RawMessage(`{}`)); err == nil {
		t.Fatal("expected validation error for empty text/blocks")
	}

	// 2. Incoming Webhook mode
	cfgWebhook, _ := json.Marshal(SlackActionConfig{
		WebhookURL: "https://8.8.8.8/slack/webhook/123",
		Text:       "Hello from Kerob webhook",
	})
	res1, err := exec.Execute(context.Background(), workflow.ActionRequest{
		Config: cfgWebhook,
	})
	if err != nil {
		t.Fatalf("webhook execution failed: %v", err)
	}
	if !res1.Success {
		t.Errorf("expected success")
	}
	if !strings.Contains(string(receivedBody), "Hello from Kerob webhook") {
		t.Errorf("unexpected body: %s", string(receivedBody))
	}

	// 3. Web API chat.postMessage mode (with bot_token in credentials)
	cfgAPI, _ := json.Marshal(SlackActionConfig{
		Channel: "C12345",
		Text:    "Hello Web API",
	})
	resAPI, errAPI := exec.Execute(context.Background(), workflow.ActionRequest{
		Config: cfgAPI,
		Credentials: map[string]string{
			"bot_token": "xoxb-mock-slack-token",
		},
	})
	if errAPI != nil {
		t.Fatalf("API execution failed: %v", errAPI)
	}
	if !resAPI.Success {
		t.Errorf("expected API execution success")
	}
	if receivedAuth != "Bearer xoxb-mock-slack-token" {
		t.Errorf("expected Bearer token, got: %s", receivedAuth)
	}

	// 4. Missing both webhook_url and channel
	resNoCred, errNoCred := exec.Execute(context.Background(), workflow.ActionRequest{
		Config: cfgAPI,
	})
	if errNoCred == nil || resNoCred.Success {
		t.Fatalf("expected error without token or webhook_url")
	}
}

func TestTeamsActionExecutor(t *testing.T) {
	var receivedBody []byte
	client := mockHTTPClient(func(req *http.Request) (*http.Response, error) {
		receivedBody, _ = io.ReadAll(req.Body)
		return jsonResponse(http.StatusOK, 1), nil
	})

	exec := NewTeamsActionExecutorWithClient(client)

	// Validation
	if err := exec.Validate(json.RawMessage(`{}`)); err == nil {
		t.Fatal("expected validation error on empty teams config")
	}

	cfg, _ := json.Marshal(TeamsActionConfig{
		WebhookURL: "https://8.8.8.8/teams/webhook/456",
		Title:      "Alert Triggered",
		Text:       "Production memory high",
		ThemeColor: "FF0000",
	})

	res, err := exec.Execute(context.Background(), workflow.ActionRequest{
		Config: cfg,
	})
	if err != nil {
		t.Fatalf("teams execution failed: %v", err)
	}
	if !res.Success {
		t.Errorf("expected success")
	}
	if !strings.Contains(string(receivedBody), "Alert Triggered") {
		t.Errorf("expected title in body, got: %s", string(receivedBody))
	}
}

func TestSMTPActionExecutor(t *testing.T) {
	var sentAddr string
	var sentFrom string
	var sentTo []string
	var sentMsg []byte

	mockSender := func(addr string, a smtp.Auth, from string, to []string, msg []byte) error {
		sentAddr = addr
		sentFrom = from
		sentTo = to
		sentMsg = msg
		return nil
	}

	exec := NewSMTPActionExecutorWithSender(mockSender)

	// Validation
	if err := exec.Validate(json.RawMessage(`{}`)); err == nil {
		t.Fatal("expected validation error on empty smtp config")
	}

	cfg, _ := json.Marshal(SMTPActionConfig{
		Host:    "smtp.example.com",
		Port:    587,
		From:    "alerts@example.com",
		To:      []string{"admin@example.com", "ops@example.com"},
		Subject: "Urgent Notification",
		Body:    "All systems nominal.",
	})

	res, err := exec.Execute(context.Background(), workflow.ActionRequest{
		Config: cfg,
	})
	if err != nil {
		t.Fatalf("smtp execute failed: %v", err)
	}
	if !res.Success {
		t.Errorf("expected success")
	}
	if sentAddr != "smtp.example.com:587" {
		t.Errorf("unexpected addr: %s", sentAddr)
	}
	if sentFrom != "alerts@example.com" || len(sentTo) != 2 {
		t.Errorf("unexpected from/to: %s, %v", sentFrom, sentTo)
	}
	if !strings.Contains(string(sentMsg), "Subject: Urgent Notification") {
		t.Errorf("message missing subject: %s", string(sentMsg))
	}
}

type mockInvoker struct {
	calledTarget string
	calledMeta   map[string]string
	returnExecID string
	returnErr    error
}

func (m *mockInvoker) InvokeWorkflow(ctx context.Context, targetWorkflowID string, payload map[string]any, metadata map[string]string) (string, error) {
	m.calledTarget = targetWorkflowID
	m.calledMeta = metadata
	return m.returnExecID, m.returnErr
}

func TestCallWorkflowActionExecutor(t *testing.T) {
	invoker := &mockInvoker{
		returnExecID: "exec_child_789",
	}
	exec := NewCallWorkflowActionExecutor(invoker)

	// Validation
	if err := exec.Validate(json.RawMessage(`{}`)); err == nil {
		t.Fatal("expected validation error on empty workflow_id")
	}

	// 1. Success execution
	cfg, _ := json.Marshal(CallWorkflowActionConfig{
		WorkflowID: "wf_target_b",
		Payload:    map[string]any{"data": "forwarded"},
	})
	metaBytes, _ := json.Marshal(map[string]string{
		"workflow_call_stack": "wf_parent_a",
		"workflow_depth":      "1",
	})
	res, err := exec.Execute(context.Background(), workflow.ActionRequest{
		Config:     cfg,
		ActionID:   "exec_parent_1",
		WorkflowID: "wf_parent_a",
		Event:      &event.Event{Metadata: metaBytes},
	})
	if err != nil {
		t.Fatalf("execute failed: %v", err)
	}
	if !res.Success {
		t.Errorf("expected success")
	}
	if invoker.calledTarget != "wf_target_b" {
		t.Errorf("expected target wf_target_b, got %s", invoker.calledTarget)
	}
	if invoker.calledMeta["workflow_depth"] != "2" {
		t.Errorf("expected depth 2, got %s", invoker.calledMeta["workflow_depth"])
	}

	// 2. Recursion loop detection
	cfgLoop, _ := json.Marshal(CallWorkflowActionConfig{
		WorkflowID: "wf_parent_a", // already in call stack!
	})
	loopMetaBytes, _ := json.Marshal(map[string]string{
		"workflow_call_stack": "wf_parent_a,wf_target_b",
		"workflow_depth":      "2",
	})
	_, errLoop := exec.Execute(context.Background(), workflow.ActionRequest{
		Config: cfgLoop,
		Event:  &event.Event{Metadata: loopMetaBytes},
	})
	if errLoop == nil || !strings.Contains(errLoop.Error(), "recursion loop detected") {
		t.Fatalf("expected recursion loop error, got: %v", errLoop)
	}

	// 3. Max depth guard
	cfgDepth, _ := json.Marshal(CallWorkflowActionConfig{
		WorkflowID: "wf_depth_test",
		MaxDepth:   3,
	})
	depthMetaBytes, _ := json.Marshal(map[string]string{
		"workflow_depth": "3",
	})
	_, errDepth := exec.Execute(context.Background(), workflow.ActionRequest{
		Config: cfgDepth,
		Event:  &event.Event{Metadata: depthMetaBytes},
	})
	if errDepth == nil || !strings.Contains(errDepth.Error(), "max call depth reached") {
		t.Fatalf("expected max depth reached error, got: %v", errDepth)
	}
}
