package tests

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"kerob/internal/adapters/destinations"
	"kerob/internal/adapters/sources"
	"kerob/internal/api"
	"kerob/internal/core/auth"
	"kerob/internal/core/credentials"
	"kerob/internal/core/delivery"
	"kerob/internal/core/event"
	"kerob/internal/core/routing"
	"kerob/internal/core/webhooks"
	"kerob/internal/core/workflow"
)

func setupRealtimeTestServer(t *testing.T) (*httptest.Server, *workflow.Engine, workflow.ExecutionStore, workflow.Store, event.Store, *api.LiveHub, auth.Store) {
	wfStore := workflow.NewMemoryStore()
	execStore := workflow.NewMemoryExecutionStore()
	dlStore := workflow.NewMemoryDeadLetterStore()
	credStore := credentials.NewMemoryStore([]byte("test-secret-key-32-chars-length!!"))
	eventStore := event.NewMemoryStore()
	actions := workflow.NewActionRegistry()

	// Register a slow action for testing cancellation
	actions.Register(&slowTestExecutor{})
	// Register standard fast action
	actions.Register(&fastTestExecutor{})

	engine := workflow.NewEngine(wfStore, execStore, dlStore, credStore, actions)
	liveHub := api.NewLiveHub()
	engine.SetEventListener(liveHub)

	authStore := auth.NewMemoryStore()
	_, _ = authStore.CreateTenant(context.Background(), &auth.Tenant{
		ID:   "tenant-alpha",
		Name: "Tenant Alpha",
		Slug: "tenant-alpha",
	})
	_, _ = authStore.CreateTenant(context.Background(), &auth.Tenant{
		ID:   "tenant-beta",
		Name: "Tenant Beta",
		Slug: "tenant-beta",
	})

	cfg := api.Config{
		Workflows:      wfStore,
		Executions:     execStore,
		DeadLetters:    dlStore,
		WorkflowEngine: engine,
		ActionReg:      actions,
		Creds:          credStore,
		WebhookStore:   webhooks.NewMemoryStore(),
		VersionStore:   workflow.NewMemoryVersionStore(),
		AuthStore:      authStore,
		MultiTenancy:   true,
		Sources:        sources.NewMemoryStore(),
		Destinations:   destinations.NewMemoryStore(),
		Routes:         routing.NewMemoryStore(),
		Events:         eventStore,
		Deliveries:     delivery.NewMemoryStore(),
		LiveHub:        liveHub,
	}

	handlers := api.NewHandlersFromConfig(cfg)
	server := api.NewServerWithOptions("0", handlers, api.ServerOptions{
		MultiTenancyEnabled: true,
		AuthStore:           authStore,
		DefaultTenantID:     "tenant-alpha",
	})

	ts := httptest.NewServer(server.Handler())
	t.Cleanup(func() {
		ts.Close()
	})

	return ts, engine, execStore, wfStore, eventStore, liveHub, authStore
}

type slowTestExecutor struct {
	started atomic.Bool
}

func (e *slowTestExecutor) Name() string                          { return "slow_action" }
func (e *slowTestExecutor) Type() string                          { return "slow_action" }
func (e *slowTestExecutor) Validate(config json.RawMessage) error { return nil }
func (e *slowTestExecutor) Execute(ctx context.Context, req workflow.ActionRequest) (workflow.ActionResult, error) {
	e.started.Store(true)
	select {
	case <-time.After(3 * time.Second):
		return workflow.ActionResult{Success: true, Output: []byte(`{"status":"slow_done"}`)}, nil
	case <-ctx.Done():
		return workflow.ActionResult{Success: false}, ctx.Err()
	}
}

type fastTestExecutor struct {
	typeName string
	executed atomic.Int32
}

func (e *fastTestExecutor) Name() string {
	if e.typeName != "" {
		return e.typeName
	}
	return "fast_action"
}
func (e *fastTestExecutor) Type() string {
	if e.typeName != "" {
		return e.typeName
	}
	return "fast_action"
}
func (e *fastTestExecutor) Validate(config json.RawMessage) error { return nil }
func (e *fastTestExecutor) Execute(ctx context.Context, req workflow.ActionRequest) (workflow.ActionResult, error) {
	e.executed.Add(1)
	return workflow.ActionResult{Success: true, Output: []byte(`{"ok":true}`)}, nil
}

// 1. Test LiveHub Pub/Sub and Tenant Isolation
func TestLiveHubTenantIsolation(t *testing.T) {
	hub := api.NewLiveHub()

	subAlpha, cancelAlpha := hub.Subscribe("tenant-alpha")
	defer cancelAlpha()

	subBeta, cancelBeta := hub.Subscribe("tenant-beta")
	defer cancelBeta()

	if hub.SubscriberCount() != 2 {
		t.Fatalf("expected 2 subscribers, got %d", hub.SubscriberCount())
	}

	// Broadcast event for tenant-alpha
	hub.Broadcast(&api.OperationalEvent{
		Type:        "execution.started",
		TenantID:    "tenant-alpha",
		ExecutionID: "exec-alpha-1",
		WorkflowID:  "wf-1",
	})

	// Broadcast event for tenant-beta
	hub.Broadcast(&api.OperationalEvent{
		Type:        "execution.started",
		TenantID:    "tenant-beta",
		ExecutionID: "exec-beta-1",
		WorkflowID:  "wf-2",
	})

	// SubAlpha should only receive tenant-alpha
	select {
	case evt := <-subAlpha.C():
		if evt.TenantID != "tenant-alpha" || evt.ExecutionID != "exec-alpha-1" {
			t.Errorf("subAlpha got unexpected event: %+v", evt)
		}
	case <-time.After(500 * time.Millisecond):
		t.Fatal("subAlpha timed out waiting for event")
	}

	select {
	case evt := <-subAlpha.C():
		t.Fatalf("subAlpha received cross-tenant event: %+v", evt)
	default:
		// OK
	}

	// SubBeta should only receive tenant-beta
	select {
	case evt := <-subBeta.C():
		if evt.TenantID != "tenant-beta" || evt.ExecutionID != "exec-beta-1" {
			t.Errorf("subBeta got unexpected event: %+v", evt)
		}
	case <-time.After(500 * time.Millisecond):
		t.Fatal("subBeta timed out waiting for event")
	}

	// Test unsubscribe cleanup
	cancelAlpha()
	if hub.SubscriberCount() != 1 {
		t.Fatalf("expected 1 subscriber after cleanup, got %d", hub.SubscriberCount())
	}
}

// 2. Test Execution Cancellation and Propagation to Context
func TestExecutionCancellationPropagation(t *testing.T) {
	ts, engine, execStore, wfStore, eventStore, _, authStore := setupRealtimeTestServer(t)

	// Create operator token
	rawTok := "krb_test_operator_token_cancel"
	tokHash := auth.HashToken(rawTok)
	_, err := authStore.CreateAPIToken(context.Background(), &auth.APIToken{
		TenantID:  "tenant-alpha",
		Name:      "Operator",
		TokenHash: tokHash,
		Role:      auth.RoleOperator,
		CreatedAt: time.Now().UTC(),
	})
	if err != nil {
		t.Fatalf("failed to create token: %v", err)
	}

	// Create workflow with slow action
	wf, err := wfStore.Create(context.Background(), &workflow.Workflow{
		TenantID: "tenant-alpha",
		Name:     "Slow Cancel Test",
		Enabled:  true,
		Trigger:  workflow.TriggerConfig{Type: "webhook"},
		Actions: []workflow.ActionStep{
			{
				ID:          "step1",
				Integration: "slow_action",
				ActionType:  "run",
				TimeoutSec:  10,
			},
			{
				ID:          "step2",
				Integration: "fast_action",
				ActionType:  "run",
			},
		},
	})
	if err != nil {
		t.Fatalf("failed to create workflow: %v", err)
	}

	exec, err := execStore.Create(context.Background(), &workflow.Execution{
		TenantID:   "tenant-alpha",
		WorkflowID: wf.ID,
		EventID:    "ev-slow-1",
		Status:     workflow.ExecutionPending,
	})
	if err != nil {
		t.Fatalf("failed to create execution: %v", err)
	}

	ev := &event.Event{
		ID:        "ev-slow-1",
		EventType: "test",
		Payload:   []byte(`{}`),
	}
	_, _, _ = eventStore.Create(context.Background(), ev)

	// Start execution in background
	execDone := make(chan struct{})
	go func() {
		defer close(execDone)
		_, _ = engine.ExecuteWorkflow(context.Background(), wf, ev, exec)
	}()

	// Wait briefly for step1 to begin
	time.Sleep(50 * time.Millisecond)

	// Issue Cancel via API endpoint
	req, _ := http.NewRequest(http.MethodPost, fmt.Sprintf("%s/api/v1/executions/%s/cancel", ts.URL, exec.ID), nil)
	req.Header.Set("Authorization", "Bearer "+rawTok)
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("cancel request failed: %v", err)
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected 200 OK from cancel, got %d", resp.StatusCode)
	}

	var cancelledExec workflow.Execution
	_ = json.NewDecoder(resp.Body).Decode(&cancelledExec)
	if cancelledExec.Status != workflow.ExecutionCancelled {
		t.Fatalf("expected status cancelled, got %s", cancelledExec.Status)
	}

	// Execution should complete quickly due to context cancellation
	select {
	case <-execDone:
		// Succeeded in terminating promptly
	case <-time.After(1 * time.Second):
		t.Fatal("execution failed to abort promptly on cancellation")
	}

	// Verify in store
	finalExec, _ := execStore.GetByID(context.Background(), exec.ID)
	if finalExec.Status != workflow.ExecutionCancelled {
		t.Errorf("expected final status cancelled, got %s", finalExec.Status)
	}
	if finalExec.CompletedAt == nil {
		t.Errorf("expected CompletedAt to be set on cancelled execution")
	}
}

// 3. Test Execution Pause and Resume with Step Idempotency
func TestExecutionPauseAndResume(t *testing.T) {
	ts, engine, execStore, wfStore, eventStore, _, authStore := setupRealtimeTestServer(t)

	rawTok := "krb_test_operator_token_pause"
	tokHash := auth.HashToken(rawTok)
	_, _ = authStore.CreateAPIToken(context.Background(), &auth.APIToken{
		TenantID:  "tenant-alpha",
		Name:      "Operator",
		TokenHash: tokHash,
		Role:      auth.RoleOperator,
		CreatedAt: time.Now().UTC(),
	})

	fastExec := &fastTestExecutor{typeName: "fast_action_count"}
	engine.Actions().Register(fastExec)

	wf, err := wfStore.Create(context.Background(), &workflow.Workflow{
		TenantID: "tenant-alpha",
		Name:     "Pause Resume Test",
		Enabled:  true,
		Trigger:  workflow.TriggerConfig{Type: "webhook"},
		Actions: []workflow.ActionStep{
			{
				ID:          "step1",
				Integration: "fast_action_count",
				ActionType:  "run",
				Config:      json.RawMessage(`{"key":"first"}`),
			},
			{
				ID:          "step2",
				Integration: "slow_action",
				ActionType:  "run",
				TimeoutSec:  10,
			},
			{
				ID:          "step3",
				Integration: "fast_action_count",
				ActionType:  "run",
				Config:      json.RawMessage(`{"key":"third"}`),
			},
		},
	})
	if err != nil {
		t.Fatalf("failed to create workflow: %v", err)
	}

	exec, err := execStore.Create(context.Background(), &workflow.Execution{
		TenantID:   "tenant-alpha",
		WorkflowID: wf.ID,
		EventID:    "ev-pause-1",
		Status:     workflow.ExecutionPending,
	})
	if err != nil {
		t.Fatalf("failed to create execution: %v", err)
	}

	ev := &event.Event{
		ID:        "ev-pause-1",
		EventType: "test",
		Payload:   []byte(`{}`),
	}
	_, _, _ = eventStore.Create(context.Background(), ev)

	// Start workflow
	go func() {
		_, _ = engine.ExecuteWorkflow(context.Background(), wf, ev, exec)
	}()

	// Wait for step1 to complete and step2 (slow) to start
	time.Sleep(60 * time.Millisecond)

	// Pause execution via API
	pauseReq, _ := http.NewRequest(http.MethodPost, fmt.Sprintf("%s/api/v1/executions/%s/pause", ts.URL, exec.ID), nil)
	pauseReq.Header.Set("Authorization", "Bearer "+rawTok)
	pauseResp, err := http.DefaultClient.Do(pauseReq)
	if err != nil {
		t.Fatalf("pause request failed: %v", err)
	}
	defer func() { _ = pauseResp.Body.Close() }()

	if pauseResp.StatusCode != http.StatusOK {
		t.Fatalf("expected 200 from pause, got %d", pauseResp.StatusCode)
	}

	pausedExec, _ := execStore.GetByID(context.Background(), exec.ID)
	if pausedExec.Status != workflow.ExecutionPaused {
		t.Fatalf("expected status paused, got %s", pausedExec.Status)
	}

	// Initial step1 should have run once
	if fastExec.executed.Load() != 1 {
		t.Fatalf("expected step1 to run once, got %d", fastExec.executed.Load())
	}

	// Verify worker claiming doesn't pick up paused execution
	claimed, err := execStore.ClaimPendingWithLease(context.Background(), "worker-test", 10*time.Second)
	if err != nil {
		t.Fatalf("claim failed: %v", err)
	}
	if claimed != nil {
		t.Fatalf("paused execution was claimed by worker: %+v", claimed)
	}

	// Replace slow_action with fast_action so resume completes immediately
	engine.Actions().Register(&fastTestExecutor{typeName: "slow_action"})

	// Resume execution via API
	resumeReq, _ := http.NewRequest(http.MethodPost, fmt.Sprintf("%s/api/v1/executions/%s/resume", ts.URL, exec.ID), nil)
	resumeReq.Header.Set("Authorization", "Bearer "+rawTok)
	resumeResp, err := http.DefaultClient.Do(resumeReq)
	if err != nil {
		t.Fatalf("resume request failed: %v", err)
	}
	defer func() { _ = resumeResp.Body.Close() }()

	if resumeResp.StatusCode != http.StatusOK {
		t.Fatalf("expected 200 from resume, got %d", resumeResp.StatusCode)
	}

	// Wait for resumed workflow to complete
	time.Sleep(100 * time.Millisecond)

	finalExec, _ := execStore.GetByID(context.Background(), exec.ID)
	if finalExec.Status != workflow.ExecutionSuccess {
		t.Fatalf("expected final status success after resume, got %s (err: %v)", finalExec.Status, finalExec.Error)
	}

	// Step1 should still have run only once (not re-executed on resume!)
	// Step3 was also executed, total fast count = 2
	if fastExec.executed.Load() != 2 {
		t.Fatalf("expected step1 to NOT re-run (idempotency), total fast count should be 2, got %d", fastExec.executed.Load())
	}
}

// 4. Test Server-Sent Events (SSE) Live Operations Stream
func TestServerSentEventsOperationsStream(t *testing.T) {
	ts, engine, execStore, wfStore, eventStore, _, authStore := setupRealtimeTestServer(t)

	rawTok := "krb_test_stream_token"
	tokHash := auth.HashToken(rawTok)
	_, _ = authStore.CreateAPIToken(context.Background(), &auth.APIToken{
		TenantID:  "tenant-alpha",
		Name:      "Operator",
		TokenHash: tokHash,
		Role:      auth.RoleOperator,
		CreatedAt: time.Now().UTC(),
	})

	// Connect to SSE stream using query parameter authentication
	streamURL := fmt.Sprintf("%s/api/v1/operations/stream?token=%s", ts.URL, rawTok)
	ctx, cancelStream := context.WithCancel(context.Background())
	defer cancelStream()

	req, _ := http.NewRequestWithContext(ctx, http.MethodGet, streamURL, nil)
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("failed to connect to SSE stream: %v", err)
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected 200 from stream, got %d", resp.StatusCode)
	}
	contentType := resp.Header.Get("Content-Type")
	if !strings.HasPrefix(contentType, "text/event-stream") {
		t.Fatalf("expected Content-Type text/event-stream, got %s", contentType)
	}

	reader := bufio.NewReader(resp.Body)

	// Read initial connected event
	var eventType, eventData string
	for {
		line, err := reader.ReadString('\n')
		if err != nil {
			t.Fatalf("failed to read initial SSE line: %v", err)
		}
		line = strings.TrimSpace(line)
		if strings.HasPrefix(line, "event:") {
			eventType = strings.TrimSpace(strings.TrimPrefix(line, "event:"))
		} else if strings.HasPrefix(line, "data:") {
			eventData = strings.TrimSpace(strings.TrimPrefix(line, "data:"))
		}
		if line == "" && eventType != "" {
			break
		}
	}

	if eventType != "connected" {
		t.Fatalf("expected initial event connected, got %s (data: %s)", eventType, eventData)
	}

	// Trigger a workflow execution and verify real-time events are streamed
	wf, _ := wfStore.Create(context.Background(), &workflow.Workflow{
		TenantID: "tenant-alpha",
		Name:     "Stream Test",
		Enabled:  true,
		Trigger:  workflow.TriggerConfig{Type: "webhook"},
		Actions: []workflow.ActionStep{
			{
				ID:          "step1",
				Integration: "fast_action",
				ActionType:  "run",
			},
		},
	})
	exec, _ := execStore.Create(context.Background(), &workflow.Execution{
		TenantID:   "tenant-alpha",
		WorkflowID: wf.ID,
		EventID:    "ev-stream-1",
		Status:     workflow.ExecutionPending,
	})
	ev := &event.Event{ID: "ev-stream-1", EventType: "test", Payload: []byte(`{}`)}
	_, _, _ = eventStore.Create(context.Background(), ev)

	go func() {
		_, _ = engine.ExecuteWorkflow(context.Background(), wf, ev, exec)
	}()

	// Collect streamed execution events
	receivedEvents := make(map[string]bool)
	var mu sync.Mutex

	readDone := make(chan struct{})
	go func() {
		defer close(readDone)
		currentEvent := ""
		for {
			line, err := reader.ReadString('\n')
			if err != nil {
				return
			}
			line = strings.TrimSpace(line)
			if strings.HasPrefix(line, "event:") {
				currentEvent = strings.TrimSpace(strings.TrimPrefix(line, "event:"))
			} else if line == "" && currentEvent != "" {
				mu.Lock()
				receivedEvents[currentEvent] = true
				hasStarted := receivedEvents["execution.started"]
				hasStep := receivedEvents["execution.step"]
				hasCompleted := receivedEvents["execution.completed"]
				mu.Unlock()
				currentEvent = ""

				if hasStarted && hasStep && hasCompleted {
					return
				}
			}
		}
	}()

	select {
	case <-readDone:
		// Successfully received started, step, and completed!
	case <-time.After(3 * time.Second):
		mu.Lock()
		t.Fatalf("timed out waiting for execution stream events, received: %+v", receivedEvents)
		mu.Unlock()
	}
}

// 5. Test Cross-Tenant Access Denial and RBAC for Control Endpoints
func TestRealtimeOperationsRBACAndCrossTenantDenial(t *testing.T) {
	ts, _, execStore, _, _, _, authStore := setupRealtimeTestServer(t)

	// Viewer token (read only)
	rawViewer := "krb_test_viewer_token"
	_, _ = authStore.CreateAPIToken(context.Background(), &auth.APIToken{
		TenantID:  "tenant-alpha",
		Name:      "Viewer",
		TokenHash: auth.HashToken(rawViewer),
		Role:      auth.RoleViewer,
		CreatedAt: time.Now().UTC(),
	})

	// Operator in Tenant Beta
	rawBeta := "krb_test_beta_operator"
	_, _ = authStore.CreateAPIToken(context.Background(), &auth.APIToken{
		TenantID:  "tenant-beta",
		Name:      "Beta Operator",
		TokenHash: auth.HashToken(rawBeta),
		Role:      auth.RoleOperator,
		CreatedAt: time.Now().UTC(),
	})

	// Execution belonging to Tenant Alpha
	exec, _ := execStore.Create(context.Background(), &workflow.Execution{
		TenantID:   "tenant-alpha",
		WorkflowID: "wf-alpha",
		EventID:    "ev-alpha",
		Status:     workflow.ExecutionPending,
	})

	// 1. Viewer trying to Cancel -> 403 Forbidden
	req, _ := http.NewRequest(http.MethodPost, fmt.Sprintf("%s/api/v1/executions/%s/cancel", ts.URL, exec.ID), nil)
	req.Header.Set("Authorization", "Bearer "+rawViewer)
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("request failed: %v", err)
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusForbidden {
		t.Fatalf("expected 403 for viewer cancelling, got %d", resp.StatusCode)
	}

	// 2. Tenant Beta operator trying to Cancel Tenant Alpha execution -> 403 CROSS_TENANT_DENIED
	reqBeta, _ := http.NewRequest(http.MethodPost, fmt.Sprintf("%s/api/v1/executions/%s/cancel", ts.URL, exec.ID), nil)
	reqBeta.Header.Set("Authorization", "Bearer "+rawBeta)
	respBeta, err := http.DefaultClient.Do(reqBeta)
	if err != nil {
		t.Fatalf("request failed: %v", err)
	}
	defer func() { _ = respBeta.Body.Close() }()
	if respBeta.StatusCode != http.StatusForbidden {
		t.Fatalf("expected 403 for cross-tenant cancel, got %d", respBeta.StatusCode)
	}

	var errBody map[string]any
	_ = json.NewDecoder(respBeta.Body).Decode(&errBody)
	if errBody["code"] != "CROSS_TENANT_DENIED" {
		t.Errorf("expected CROSS_TENANT_DENIED code, got %v", errBody["code"])
	}

	// 3. Tenant Beta operator trying to Pause Tenant Alpha execution -> 403 CROSS_TENANT_DENIED
	reqPause, _ := http.NewRequest(http.MethodPost, fmt.Sprintf("%s/api/v1/executions/%s/pause", ts.URL, exec.ID), nil)
	reqPause.Header.Set("Authorization", "Bearer "+rawBeta)
	respPause, err := http.DefaultClient.Do(reqPause)
	if err != nil {
		t.Fatalf("request failed: %v", err)
	}
	defer func() { _ = respPause.Body.Close() }()
	if respPause.StatusCode != http.StatusForbidden {
		t.Fatalf("expected 403 for cross-tenant pause, got %d", respPause.StatusCode)
	}

	// 4. Tenant Beta operator trying to Resume Tenant Alpha execution -> 403 CROSS_TENANT_DENIED
	reqResume, _ := http.NewRequest(http.MethodPost, fmt.Sprintf("%s/api/v1/executions/%s/resume", ts.URL, exec.ID), nil)
	reqResume.Header.Set("Authorization", "Bearer "+rawBeta)
	respResume, err := http.DefaultClient.Do(reqResume)
	if err != nil {
		t.Fatalf("request failed: %v", err)
	}
	defer func() { _ = respResume.Body.Close() }()
	if respResume.StatusCode != http.StatusForbidden {
		t.Fatalf("expected 403 for cross-tenant resume, got %d", respResume.StatusCode)
	}
}
