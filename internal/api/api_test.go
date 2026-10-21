package api

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"kerob/internal/adapters/destinations"
	"kerob/internal/adapters/sources"
	"kerob/internal/core/credentials"
	"kerob/internal/core/webhooks"
	"kerob/internal/core/workflow"
)

func TestHealthReadyAndMetricsEndpoints(t *testing.T) {
	handlers := &Handlers{
		sourceReg: sources.NewRegistry(),
		destReg:   destinations.NewRegistry(),
	}
	server := NewServer("0", handlers)
	handler := server.Handler()

	// 1. Health check
	reqHealth := httptest.NewRequest(http.MethodGet, "/health", nil)
	wHealth := httptest.NewRecorder()
	handler.ServeHTTP(wHealth, reqHealth)

	if wHealth.Code != http.StatusOK {
		t.Errorf("expected /health status 200, got %d", wHealth.Code)
	}

	// 2. Ready check
	reqReady := httptest.NewRequest(http.MethodGet, "/ready", nil)
	wReady := httptest.NewRecorder()
	handler.ServeHTTP(wReady, reqReady)

	if wReady.Code != http.StatusOK {
		t.Errorf("expected /ready status 200, got %d", wReady.Code)
	}

	// 3. Metrics endpoint
	reqMetrics := httptest.NewRequest(http.MethodGet, "/metrics", nil)
	wMetrics := httptest.NewRecorder()
	handler.ServeHTTP(wMetrics, reqMetrics)

	if wMetrics.Code != http.StatusOK {
		t.Errorf("expected /metrics status 200, got %d", wMetrics.Code)
	}
	if !strings.Contains(wMetrics.Body.String(), "kerob_workflow_executions_total") {
		t.Errorf("expected prometheus metrics format, got: %s", wMetrics.Body.String())
	}
}

func TestWorkflowsAPI(t *testing.T) {
	wfStore := workflow.NewMemoryStore()
	handlers := &Handlers{
		workflows: wfStore,
	}
	server := NewServer("0", handlers)
	handler := server.Handler()

	// 1. Create Workflow
	createBody := `{
		"name": "GitHub to Discord Alert",
		"enabled": true,
		"trigger": {
			"type": "github",
			"event_types": ["github.issue.opened"]
		},
		"actions": [{
			"integration": "discord",
			"action_type": "send_message",
			"config": {"content": "New issue"}
		}]
	}`
	reqCreate := httptest.NewRequest(http.MethodPost, "/api/v1/workflows", bytes.NewBufferString(createBody))
	wCreate := httptest.NewRecorder()
	handler.ServeHTTP(wCreate, reqCreate)

	if wCreate.Code != http.StatusCreated {
		t.Fatalf("expected 201 Created, got %d: %s", wCreate.Code, wCreate.Body.String())
	}

	var created workflow.Workflow
	if err := json.Unmarshal(wCreate.Body.Bytes(), &created); err != nil {
		t.Fatalf("failed to unmarshal created workflow: %v", err)
	}
	if created.ID == "" || created.Name != "GitHub to Discord Alert" {
		t.Errorf("unexpected created workflow: %+v", created)
	}

	// 2. List Workflows
	reqList := httptest.NewRequest(http.MethodGet, "/api/v1/workflows", nil)
	wList := httptest.NewRecorder()
	handler.ServeHTTP(wList, reqList)

	if wList.Code != http.StatusOK {
		t.Fatalf("expected 200 OK, got %d", wList.Code)
	}
	var list []*workflow.Workflow
	_ = json.Unmarshal(wList.Body.Bytes(), &list)
	if len(list) != 1 {
		t.Fatalf("expected 1 workflow in list, got %d", len(list))
	}

	// 3. Patch Workflow (Toggle disable)
	patchBody := `{"enabled": false}`
	reqPatch := httptest.NewRequest(http.MethodPatch, "/api/v1/workflows/"+created.ID, bytes.NewBufferString(patchBody))
	wPatch := httptest.NewRecorder()
	handler.ServeHTTP(wPatch, reqPatch)

	if wPatch.Code != http.StatusOK {
		t.Fatalf("expected 200 OK for patch, got %d", wPatch.Code)
	}
	var patched workflow.Workflow
	_ = json.Unmarshal(wPatch.Body.Bytes(), &patched)
	if patched.Enabled != false {
		t.Errorf("expected workflow enabled=false after patch, got %v", patched.Enabled)
	}

	// 4. Delete Workflow
	reqDel := httptest.NewRequest(http.MethodDelete, "/api/v1/workflows/"+created.ID, nil)
	wDel := httptest.NewRecorder()
	handler.ServeHTTP(wDel, reqDel)

	if wDel.Code != http.StatusNoContent {
		t.Fatalf("expected 204 No Content for delete, got %d", wDel.Code)
	}
}

func TestCredentialsAPI(t *testing.T) {
	key := credentials.DeriveKey("api-test-key")
	credStore := credentials.NewMemoryStore(key)
	handlers := &Handlers{
		creds: credStore,
	}
	server := NewServer("0", handlers)
	handler := server.Handler()

	// 1. Create Credential
	createBody := `{
		"name": "Discord Production Webhook",
		"integration": "discord",
		"values": {
			"webhook_url": "https://discord.com/api/webhooks/123/xyzSecretToken456"
		}
	}`
	reqCreate := httptest.NewRequest(http.MethodPost, "/api/v1/credentials", bytes.NewBufferString(createBody))
	wCreate := httptest.NewRecorder()
	handler.ServeHTTP(wCreate, reqCreate)

	if wCreate.Code != http.StatusCreated {
		t.Fatalf("expected 201 Created, got %d: %s", wCreate.Code, wCreate.Body.String())
	}

	// Raw secret must NEVER be exposed in response body
	if strings.Contains(wCreate.Body.String(), "xyzSecretToken456") {
		t.Fatal("SECURITY VIOLATION: raw credential was leaked in API response")
	}

	var cred credentials.Credential
	_ = json.Unmarshal(wCreate.Body.Bytes(), &cred)
	if cred.MaskedFields["webhook_url"] == "" || !strings.Contains(cred.MaskedFields["webhook_url"], "••••") {
		t.Errorf("expected masked field in response, got %s", cred.MaskedFields["webhook_url"])
	}
}

func TestWebhooksInboundAPI(t *testing.T) {
	whStore := webhooks.NewMemoryStore()
	wh, _ := whStore.Create(context.Background(), &webhooks.Webhook{
		Name:        "GitHub Alert Inbound",
		SecretToken: "valid-secret-999",
		Enabled:     true,
	})

	handlers := &Handlers{
		webhookStore: whStore,
	}
	server := NewServer("0", handlers)
	handler := server.Handler()

	// 1. Authorized Webhook POST
	reqAuth := httptest.NewRequest(http.MethodPost, "/webhooks/"+wh.ID, bytes.NewBufferString(`{"status":"ok"}`))
	reqAuth.Header.Set("X-Webhook-Token", "valid-secret-999")
	wAuth := httptest.NewRecorder()
	handler.ServeHTTP(wAuth, reqAuth)

	if wAuth.Code != http.StatusAccepted {
		t.Fatalf("expected 202 Accepted, got %d: %s", wAuth.Code, wAuth.Body.String())
	}

	// 2. Unauthorized Webhook POST (bad secret)
	reqBad := httptest.NewRequest(http.MethodPost, "/webhooks/"+wh.ID, bytes.NewBufferString(`{"status":"ok"}`))
	reqBad.Header.Set("X-Webhook-Token", "wrong-secret")
	wBad := httptest.NewRecorder()
	handler.ServeHTTP(wBad, reqBad)

	if wBad.Code != http.StatusUnauthorized {
		t.Fatalf("expected 401 Unauthorized for bad webhook token, got %d", wBad.Code)
	}
}

func TestOverviewAPI(t *testing.T) {
	handlers := &Handlers{}
	server := NewServer("0", handlers)
	handler := server.Handler()

	req := httptest.NewRequest(http.MethodGet, "/api/v1/overview", nil)
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200 OK for /api/v1/overview, got %d", w.Code)
	}

	var resp OverviewResponse
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("failed to unmarshal overview response: %v", err)
	}
	if resp.Status != "operational" {
		t.Errorf("expected operational status, got %s", resp.Status)
	}
	if len(resp.Catalog) == 0 {
		t.Errorf("expected non-empty integrations catalog")
	}
}

func TestAuthMiddlewareProtection(t *testing.T) {
	handlers := &Handlers{}
	// Require API_TOKEN for management endpoints
	server := NewServerWithOptions("0", handlers, ServerOptions{
		APIToken: "super-secret-admin-token",
	})
	handler := server.Handler()

	// 1. Request without token -> 401 Unauthorized
	reqNoAuth := httptest.NewRequest(http.MethodGet, "/api/v1/workflows", nil)
	wNoAuth := httptest.NewRecorder()
	handler.ServeHTTP(wNoAuth, reqNoAuth)

	if wNoAuth.Code != http.StatusUnauthorized {
		t.Fatalf("expected 401 Unauthorized when API token is required, got %d", wNoAuth.Code)
	}

	// 2. Request with valid Bearer token -> passes auth middleware
	reqValid := httptest.NewRequest(http.MethodGet, "/api/v1/workflows", nil)
	reqValid.Header.Set("Authorization", "Bearer super-secret-admin-token")
	wValid := httptest.NewRecorder()
	handler.ServeHTTP(wValid, reqValid)

	if wValid.Code == http.StatusUnauthorized {
		t.Fatalf("expected valid token to pass auth middleware, got 401")
	}

	// 3. Health & Dashboard UI should be accessible without token
	reqHealth := httptest.NewRequest(http.MethodGet, "/health", nil)
	wHealth := httptest.NewRecorder()
	handler.ServeHTTP(wHealth, reqHealth)

	if wHealth.Code != http.StatusOK {
		t.Fatalf("expected /health to be public, got %d", wHealth.Code)
	}
}

func TestAuthMiddlewareDevelopmentWithoutToken(t *testing.T) {
	wfStore := workflow.NewMemoryStore()
	handlers := &Handlers{
		workflows: wfStore,
	}
	// In development, API_TOKEN is empty/optional
	server := NewServerWithOptions("0", handlers, ServerOptions{
		APIToken: "",
	})
	handler := server.Handler()

	// 1. Health endpoint remains public
	reqHealth := httptest.NewRequest(http.MethodGet, "/health", nil)
	wHealth := httptest.NewRecorder()
	handler.ServeHTTP(wHealth, reqHealth)
	if wHealth.Code != http.StatusOK {
		t.Fatalf("expected /health to be public, got %d", wHealth.Code)
	}

	// 2. Management endpoints must be accessible without Authorization header in dev
	reqMgmt := httptest.NewRequest(http.MethodGet, "/api/v1/workflows", nil)
	wMgmt := httptest.NewRecorder()
	handler.ServeHTTP(wMgmt, reqMgmt)
	if wMgmt.Code == http.StatusUnauthorized {
		t.Fatalf("expected management endpoint to allow access without token in development, got 401 Unauthorized")
	}
	if wMgmt.Code != http.StatusOK {
		t.Fatalf("expected management endpoint to return 200 OK in development, got %d", wMgmt.Code)
	}
}

func TestWorkflowTestEndpoint(t *testing.T) {
	wfStore := workflow.NewMemoryStore()
	execStore := workflow.NewMemoryExecutionStore()
	dlStore := workflow.NewMemoryDeadLetterStore()
	credStore := credentials.NewMemoryStore(credentials.DeriveKey("test-secret-key-32-bytes-long!"))
	actionReg := workflow.NewActionRegistry()
	engine := workflow.NewEngine(wfStore, execStore, dlStore, credStore, actionReg)

	handlers := NewHandlersFromConfig(Config{
		Workflows:      wfStore,
		Executions:     execStore,
		DeadLetters:    dlStore,
		WorkflowEngine: engine,
		ActionReg:      actionReg,
		Creds:          credStore,
	})

	server := NewServer("0", handlers)
	handler := server.Handler()

	// 1. Create a workflow
	wf, err := wfStore.Create(context.Background(), &workflow.Workflow{
		Name:    "Testable Workflow",
		Enabled: true,
		Trigger: workflow.TriggerConfig{
			Type: "webhook",
		},
		Actions: []workflow.ActionStep{
			{
				Integration: "http",
				ActionType:  "http_request",
				Config:      json.RawMessage(`{"url":"https://api.example.com"}`),
			},
		},
	})
	if err != nil {
		t.Fatalf("failed to create workflow: %v", err)
	}

	// 2. Invoke /api/v1/workflows/{id}/test
	testBody := `{"payload":{"text":"automated trigger test"}}`
	req := httptest.NewRequest(http.MethodPost, "/api/v1/workflows/"+wf.ID+"/test", strings.NewReader(testBody))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200 OK, got %d: %s", w.Code, w.Body.String())
	}

	var res map[string]any
	if err := json.Unmarshal(w.Body.Bytes(), &res); err != nil {
		t.Fatalf("failed to parse test response: %v", err)
	}
	if res["status"] == "" {
		t.Fatalf("expected non-empty status in test response")
	}
}
