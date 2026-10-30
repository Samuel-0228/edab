package workflow

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"sync"
	"testing"

	"kerob/internal/core/credentials"
	"kerob/internal/core/event"
)

// DynamicMockExecutor allows customized execution behavior per action ID.
type DynamicMockExecutor struct {
	mu       sync.Mutex
	TypeStr  string
	Executed map[string]int
	Handlers map[string]func(req ActionRequest) (ActionResult, error)
}

func newDynamicMockExecutor(typeStr string) *DynamicMockExecutor {
	return &DynamicMockExecutor{
		TypeStr:  typeStr,
		Executed: make(map[string]int),
		Handlers: make(map[string]func(req ActionRequest) (ActionResult, error)),
	}
}

func (m *DynamicMockExecutor) Name() string                          { return m.TypeStr }
func (m *DynamicMockExecutor) Type() string                          { return m.TypeStr }
func (m *DynamicMockExecutor) Validate(config json.RawMessage) error { return nil }
func (m *DynamicMockExecutor) Execute(ctx context.Context, req ActionRequest) (ActionResult, error) {
	m.mu.Lock()
	m.Executed[req.ActionID]++
	var matchedHandler func(req ActionRequest) (ActionResult, error)
	if handler, ok := m.Handlers[req.ActionID]; ok {
		matchedHandler = handler
	} else {
		for k, handler := range m.Handlers {
			if strings.HasSuffix(req.ActionID, k) {
				matchedHandler = handler
				break
			}
		}
	}
	m.mu.Unlock()

	if matchedHandler != nil {
		return matchedHandler(req)
	}
	return ActionResult{
		Success: true,
		Output:  json.RawMessage(`{"status":"ok"}`),
	}, nil
}

func TestMultiStepDataFlow(t *testing.T) {
	ctx := context.Background()

	wfStore := NewMemoryStore()
	execStore := NewMemoryExecutionStore()
	dlStore := NewMemoryDeadLetterStore()
	credStore := credentials.NewMemoryStore(credentials.DeriveKey("test-key-32-bytes-long-padding!!"))
	registry := NewActionRegistry()

	mock := newDynamicMockExecutor("http")
	var step2ReceivedConfig map[string]any

	mock.Handlers["step_1"] = func(req ActionRequest) (ActionResult, error) {
		return ActionResult{
			Success: true,
			Output:  json.RawMessage(`{"ticket_id":"TCK-1042","meta":{"priority":"critical"}}`),
		}, nil
	}

	mock.Handlers["step_2"] = func(req ActionRequest) (ActionResult, error) {
		_ = json.Unmarshal(req.Config, &step2ReceivedConfig)
		return ActionResult{
			Success: true,
			Output:  json.RawMessage(`{"notified":true}`),
		}, nil
	}

	registry.Register(mock)

	wf, err := wfStore.Create(ctx, &Workflow{
		Name:    "Multi-step Pipeline Flow",
		Enabled: true,
		Trigger: TriggerConfig{Type: "webhook"},
		Actions: []ActionStep{
			{
				ID:          "step_1",
				Integration: "http",
				ActionType:  "create_ticket",
				Config:      json.RawMessage(`{"title":"{{payload.summary}}"}`),
			},
			{
				ID:          "step_2",
				Integration: "http",
				ActionType:  "send_notification",
				Config:      json.RawMessage(`{"ticket":"{{steps.step_1.output.ticket_id}}","level":"{{steps.step_1.output.meta.priority}}","status":"{{steps.step_1.status}}"}`),
			},
		},
	})
	if err != nil {
		t.Fatalf("failed to create workflow: %v", err)
	}

	engine := NewEngine(wfStore, execStore, dlStore, credStore, registry)

	ev, err := event.New("src-1", "evt-1", "incident.created", map[string]any{
		"summary": "Database connectivity alert",
	}, nil)
	if err != nil {
		t.Fatalf("failed to create event: %v", err)
	}

	execs, err := engine.ProcessEvent(ctx, ev)
	if err != nil {
		t.Fatalf("ProcessEvent failed: %v", err)
	}
	if len(execs) != 1 {
		t.Fatalf("expected 1 execution, got %d", len(execs))
	}
	if execs[0].Status != ExecutionSuccess {
		t.Fatalf("expected execution status %s, got %s (err: %v)", ExecutionSuccess, execs[0].Status, execs[0].Error)
	}

	if mock.Executed["step_1"] != 1 || mock.Executed["step_2"] != 1 {
		t.Errorf("expected each step executed once, got step_1=%d, step_2=%d", mock.Executed["step_1"], mock.Executed["step_2"])
	}

	if step2ReceivedConfig["ticket"] != "TCK-1042" {
		t.Errorf("expected step_2 ticket 'TCK-1042', got %v", step2ReceivedConfig["ticket"])
	}
	if step2ReceivedConfig["level"] != "critical" {
		t.Errorf("expected step_2 level 'critical', got %v", step2ReceivedConfig["level"])
	}
	if step2ReceivedConfig["status"] != "success" {
		t.Errorf("expected step_2 status 'success', got %v", step2ReceivedConfig["status"])
	}
	_ = wf
}

func TestMultiStepOnErrorContinue(t *testing.T) {
	ctx := context.Background()

	wfStore := NewMemoryStore()
	execStore := NewMemoryExecutionStore()
	dlStore := NewMemoryDeadLetterStore()
	credStore := credentials.NewMemoryStore(credentials.DeriveKey("test-key-32-bytes-long-padding!!"))
	registry := NewActionRegistry()

	mock := newDynamicMockExecutor("http")
	var step2ReceivedConfig map[string]any

	mock.Handlers["step_1"] = func(req ActionRequest) (ActionResult, error) {
		return ActionResult{Success: false}, errors.New("temporary upstream 503")
	}
	mock.Handlers["step_2"] = func(req ActionRequest) (ActionResult, error) {
		_ = json.Unmarshal(req.Config, &step2ReceivedConfig)
		return ActionResult{Success: true, Output: json.RawMessage(`{"recovered":true}`)}, nil
	}
	registry.Register(mock)

	_, err := wfStore.Create(ctx, &Workflow{
		Name:    "Continue On Error Workflow",
		Enabled: true,
		Trigger: TriggerConfig{Type: "webhook"},
		Actions: []ActionStep{
			{
				ID:          "step_1",
				Integration: "http",
				ActionType:  "unreliable_call",
				OnError:     OnErrorContinue,
				Config:      json.RawMessage(`{}`),
			},
			{
				ID:          "step_2",
				Integration: "http",
				ActionType:  "fallback_call",
				Config:      json.RawMessage(`{"prev_status":"{{steps.step_1.status}}"}`),
			},
		},
	})
	if err != nil {
		t.Fatalf("failed to create workflow: %v", err)
	}

	engine := NewEngine(wfStore, execStore, dlStore, credStore, registry)
	ev, _ := event.New("src-1", "evt-2", "webhook.test", map[string]any{}, nil)

	execs, err := engine.ProcessEvent(ctx, ev)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if execs[0].Status != ExecutionSuccess {
		t.Fatalf("expected status success because of on_error=continue, got %s", execs[0].Status)
	}
	if mock.Executed["step_1"] != 1 || mock.Executed["step_2"] != 1 {
		t.Errorf("expected both steps executed, got step_1=%d, step_2=%d", mock.Executed["step_1"], mock.Executed["step_2"])
	}
	if step2ReceivedConfig["prev_status"] != "failed" {
		t.Errorf("expected step_2 prev_status 'failed', got %v", step2ReceivedConfig["prev_status"])
	}
}

func TestMultiStepOnErrorSkipRest(t *testing.T) {
	ctx := context.Background()

	wfStore := NewMemoryStore()
	execStore := NewMemoryExecutionStore()
	dlStore := NewMemoryDeadLetterStore()
	credStore := credentials.NewMemoryStore(credentials.DeriveKey("test-key-32-bytes-long-padding!!"))
	registry := NewActionRegistry()

	mock := newDynamicMockExecutor("http")
	mock.Handlers["step_1"] = func(req ActionRequest) (ActionResult, error) {
		return ActionResult{Success: false}, errors.New("upstream declined")
	}
	registry.Register(mock)

	_, err := wfStore.Create(ctx, &Workflow{
		Name:    "Skip Rest Workflow",
		Enabled: true,
		Trigger: TriggerConfig{Type: "webhook"},
		Actions: []ActionStep{
			{
				ID:          "step_1",
				Integration: "http",
				ActionType:  "check",
				OnError:     OnErrorSkipRest,
				Config:      json.RawMessage(`{}`),
			},
			{
				ID:          "step_2",
				Integration: "http",
				ActionType:  "unreachable_step",
				Config:      json.RawMessage(`{}`),
			},
		},
	})
	if err != nil {
		t.Fatalf("failed to create workflow: %v", err)
	}

	engine := NewEngine(wfStore, execStore, dlStore, credStore, registry)
	ev, _ := event.New("src-1", "evt-3", "webhook.test", map[string]any{}, nil)

	execs, err := engine.ProcessEvent(ctx, ev)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if execs[0].Status != ExecutionSuccess {
		t.Fatalf("expected status success when skipping rest, got %s", execs[0].Status)
	}
	if mock.Executed["step_1"] != 1 {
		t.Errorf("step_1 should execute once, got %d", mock.Executed["step_1"])
	}
	if mock.Executed["step_2"] != 0 {
		t.Errorf("step_2 should NOT execute, got %d", mock.Executed["step_2"])
	}
}

func TestMultiStepIdempotentResumption(t *testing.T) {
	ctx := context.Background()

	wfStore := NewMemoryStore()
	execStore := NewMemoryExecutionStore()
	dlStore := NewMemoryDeadLetterStore()
	credStore := credentials.NewMemoryStore(credentials.DeriveKey("test-key-32-bytes-long-padding!!"))
	registry := NewActionRegistry()

	mock := newDynamicMockExecutor("http")
	step2Attempts := 0

	mock.Handlers["step_1"] = func(req ActionRequest) (ActionResult, error) {
		return ActionResult{
			Success: true,
			Output:  json.RawMessage(`{"charge_id":"ch_123"}`),
		}, nil
	}

	mock.Handlers["step_2"] = func(req ActionRequest) (ActionResult, error) {
		step2Attempts++
		if step2Attempts == 1 {
			return ActionResult{Success: false}, errors.New("receipt service timeout")
		}
		var cfg map[string]any
		_ = json.Unmarshal(req.Config, &cfg)
		if cfg["charge"] != "ch_123" {
			return ActionResult{Success: false}, errors.New("missing charge from step_1 output")
		}
		return ActionResult{Success: true, Output: json.RawMessage(`{"receipt_id":"rec_999"}`)}, nil
	}

	registry.Register(mock)

	wf, err := wfStore.Create(ctx, &Workflow{
		Name:    "Payment and Receipt Pipeline",
		Enabled: true,
		Trigger: TriggerConfig{Type: "webhook"},
		Actions: []ActionStep{
			{
				ID:          "step_1",
				Integration: "http",
				ActionType:  "charge_card",
				Config:      json.RawMessage(`{}`),
			},
			{
				ID:          "step_2",
				Integration: "http",
				ActionType:  "send_receipt",
				Config:      json.RawMessage(`{"charge":"{{steps.step_1.output.charge_id}}"}`),
			},
		},
	})
	if err != nil {
		t.Fatalf("failed to create workflow: %v", err)
	}

	engine := NewEngine(wfStore, execStore, dlStore, credStore, registry)
	ev, _ := event.New("src-1", "evt-charge-1", "payment.created", map[string]any{}, nil)

	// First execution attempt: step_1 succeeds, step_2 fails -> execution retrying
	execs, err := engine.ProcessEvent(ctx, ev)
	if err != nil {
		t.Fatalf("unexpected dispatch error: %v", err)
	}
	if len(execs) != 1 {
		t.Fatalf("expected 1 execution, got %d", len(execs))
	}
	if execs[0].Status != ExecutionRetrying {
		t.Fatalf("expected status retrying, got %s", execs[0].Status)
	}
	if mock.Executed["step_1"] != 1 {
		t.Fatalf("step_1 executed count should be 1, got %d", mock.Executed["step_1"])
	}
	if mock.Executed["step_2"] != 1 {
		t.Fatalf("step_2 executed count should be 1, got %d", mock.Executed["step_2"])
	}

	// Retry execution: should resume from step_2 without re-running step_1!
	exec, err := execStore.GetByID(ctx, execs[0].ID)
	if err != nil {
		t.Fatalf("failed to get execution: %v", err)
	}

	completedExec, err := engine.ExecuteWorkflow(ctx, wf, ev, exec)
	if err != nil {
		t.Fatalf("retry failed: %v", err)
	}
	if completedExec.Status != ExecutionSuccess {
		t.Fatalf("expected status success on retry, got %s", completedExec.Status)
	}

	// Assert step_1 was NEVER re-executed!
	if mock.Executed["step_1"] != 1 {
		t.Errorf("CRITICAL IDEMPOTENCY VIOLATION: step_1 was re-executed on retry! Expected 1, got %d", mock.Executed["step_1"])
	}
	if mock.Executed["step_2"] != 2 {
		t.Errorf("step_2 should have executed 2 times (initial + retry), got %d", mock.Executed["step_2"])
	}
}

func TestMultiStepSecretResolution(t *testing.T) {
	ctx := context.Background()

	wfStore := NewMemoryStore()
	execStore := NewMemoryExecutionStore()
	dlStore := NewMemoryDeadLetterStore()
	credStore := credentials.NewMemoryStore(credentials.DeriveKey("test-key-32-bytes-long-padding!!"))
	registry := NewActionRegistry()

	// Store secret in vault
	cred, err := credStore.Create(ctx, &credentials.Credential{
		Name:        "prod_stripe_key",
		Integration: "http",
	}, map[string]string{
		"api_key": "mock_vault_secret_token_12345",
	})
	if err != nil {
		t.Fatalf("failed to store credential: %v", err)
	}

	mock := newDynamicMockExecutor("http")
	var receivedConfig map[string]any
	mock.Handlers["step_1"] = func(req ActionRequest) (ActionResult, error) {
		_ = json.Unmarshal(req.Config, &receivedConfig)
		return ActionResult{Success: true}, nil
	}
	registry.Register(mock)

	_, err = wfStore.Create(ctx, &Workflow{
		Name:    "Vault Secret Workflow",
		Enabled: true,
		Trigger: TriggerConfig{Type: "webhook"},
		Actions: []ActionStep{
			{
				ID:           "step_1",
				Integration:  "http",
				ActionType:   "api_call",
				CredentialID: cred.ID,
				Config:       json.RawMessage(`{"auth_header":"Bearer {{secrets.api_key}}"}`),
			},
		},
	})
	if err != nil {
		t.Fatalf("failed to create workflow: %v", err)
	}

	engine := NewEngine(wfStore, execStore, dlStore, credStore, registry)
	ev, _ := event.New("src-1", "evt-secret", "webhook.test", map[string]any{}, nil)

	execs, err := engine.ProcessEvent(ctx, ev)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if execs[0].Status != ExecutionSuccess {
		t.Fatalf("expected status success, got %s", execs[0].Status)
	}

	if receivedConfig["auth_header"] != "Bearer mock_vault_secret_token_12345" {
		t.Errorf("expected secret resolved in config, got %v", receivedConfig["auth_header"])
	}
}
