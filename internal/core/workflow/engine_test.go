package workflow

import (
	"context"
	"encoding/json"
	"errors"
	"testing"
	"time"

	"kerob/internal/core/conditions"
	"kerob/internal/core/credentials"
	"kerob/internal/core/event"
	"kerob/internal/core/transformations"
)

// MockActionExecutor implements ActionExecutor for unit testing.
type MockActionExecutor struct {
	TypeStr   string
	Executed  int
	LastReq   ActionRequest
	FailUntil int // fail while Executed < FailUntil
}

func (m *MockActionExecutor) Name() string { return "mock" }
func (m *MockActionExecutor) Type() string { return m.TypeStr }
func (m *MockActionExecutor) Validate(config json.RawMessage) error { return nil }
func (m *MockActionExecutor) Execute(ctx context.Context, req ActionRequest) (ActionResult, error) {
	m.Executed++
	m.LastReq = req
	if m.Executed <= m.FailUntil {
		return ActionResult{Success: false}, errors.New("simulated mock action failure")
	}
	return ActionResult{
		Success:    true,
		ExternalID: "mock-out-123",
		Output:     json.RawMessage(`{"status":"delivered"}`),
	}, nil
}

func TestWorkflowEngineSuccessFlow(t *testing.T) {
	ctx := context.Background()

	wfStore := NewMemoryStore()
	execStore := NewMemoryExecutionStore()
	dlStore := NewMemoryDeadLetterStore()
	credStore := credentials.NewMemoryStore(credentials.DeriveKey("test-key"))
	registry := NewActionRegistry()

	mockExecutor := &MockActionExecutor{TypeStr: "discord"}
	registry.Register(mockExecutor)

	// Create test workflow: WHEN github issue IF title contains 'urgent' THEN send discord message
	_, err := wfStore.Create(ctx, &Workflow{
		Name:    "Urgent GitHub Issues to Discord",
		Enabled: true,
		Trigger: TriggerConfig{
			Type:       "github",
			EventTypes: []string{"github.issue.opened"},
		},
		Conditions: &conditions.Condition{
			Field:    "payload.title",
			Operator: conditions.OpContains,
			Value:    "urgent",
		},
		Transformations: &transformations.Transformation{
			Type: "mapping",
			FieldMaps: map[string]string{
				"alert_text": "URGENT ISSUE: {{payload.title}} by {{payload.author}}",
			},
		},
		Actions: []ActionStep{
			{
				Integration: "discord",
				ActionType:  "send_message",
				Config:      json.RawMessage(`{"content":"{{payload.alert_text}}"}`),
			},
		},
	})
	if err != nil {
		t.Fatalf("failed to create workflow: %v", err)
	}

	engine := NewEngine(wfStore, execStore, dlStore, credStore, registry)

	// Ingest matching event
	ev, err := event.New("src-gh", "gh-issue-1", "github.issue.opened", map[string]any{
		"title":  "urgent bug in production",
		"author": "developer_dave",
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

	exec := execs[0]
	if exec.Status != ExecutionSuccess {
		t.Fatalf("expected ExecutionSuccess, got %s", exec.Status)
	}
	if mockExecutor.Executed != 1 {
		t.Fatalf("expected mock action executed once, got %d", mockExecutor.Executed)
	}

	// Verify execution step audit trail
	fetchedExec, err := execStore.GetByID(ctx, exec.ID)
	if err != nil {
		t.Fatalf("failed to fetch execution: %v", err)
	}
	if len(fetchedExec.Steps) != 4 {
		t.Fatalf("expected 4 steps (trigger, condition, transform, action), got %d", len(fetchedExec.Steps))
	}

	// Verify Idempotency: re-processing identical event skips re-execution
	execs2, err := engine.ProcessEvent(ctx, ev)
	if err != nil {
		t.Fatalf("second ProcessEvent failed: %v", err)
	}
	if len(execs2) != 1 || execs2[0].ID != exec.ID {
		t.Fatal("expected idempotent return of existing execution")
	}
	if mockExecutor.Executed != 1 {
		t.Fatalf("expected action NOT re-executed on duplicate event, still got %d", mockExecutor.Executed)
	}
}

func TestWorkflowEngineConditionSkipped(t *testing.T) {
	ctx := context.Background()

	wfStore := NewMemoryStore()
	execStore := NewMemoryExecutionStore()
	dlStore := NewMemoryDeadLetterStore()
	credStore := credentials.NewMemoryStore(credentials.DeriveKey("test-key"))
	registry := NewActionRegistry()

	mockExecutor := &MockActionExecutor{TypeStr: "telegram"}
	registry.Register(mockExecutor)

	// WHEN telegram.message IF text contains 'job' THEN action
	_, err := wfStore.Create(ctx, &Workflow{
		Name:    "Filter Job Postings",
		Enabled: true,
		Trigger: TriggerConfig{
			Type: "telegram",
		},
		Conditions: &conditions.Condition{
			Field:    "payload.text",
			Operator: conditions.OpContains,
			Value:    "job",
		},
		Actions: []ActionStep{
			{
				Integration: "telegram",
				ActionType:  "copy_message",
				Config:      json.RawMessage(`{}`),
			},
		},
	})
	if err != nil {
		t.Fatalf("failed to create workflow: %v", err)
	}

	engine := NewEngine(wfStore, execStore, dlStore, credStore, registry)

	// Ingest non-matching event
	ev, err := event.New("src-tele", "msg-nonmatching", "telegram.message", map[string]any{
		"text": "Hello world, what is the weather today?",
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

	exec := execs[0]
	if exec.Status != ExecutionSuccess {
		t.Fatalf("expected ExecutionSuccess for cleanly skipped workflow, got %s", exec.Status)
	}
	if mockExecutor.Executed != 0 {
		t.Fatalf("expected action NOT executed when condition fails, got %d", mockExecutor.Executed)
	}
}

func TestWorkflowEngineFailureAndDeadLetter(t *testing.T) {
	ctx := context.Background()

	wfStore := NewMemoryStore()
	execStore := NewMemoryExecutionStore()
	dlStore := NewMemoryDeadLetterStore()
	credStore := credentials.NewMemoryStore(credentials.DeriveKey("test-key"))
	registry := NewActionRegistry()

	// An executor that always fails
	failingExecutor := &MockActionExecutor{TypeStr: "http", FailUntil: 999}
	registry.Register(failingExecutor)

	wf, err := wfStore.Create(ctx, &Workflow{
		Name:       "Failing HTTP Request",
		Enabled:    true,
		MaxRetries: 1, // 1 max attempt -> fail immediately into dead-letter
		Trigger: TriggerConfig{
			Type: "webhook",
		},
		Actions: []ActionStep{
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

	engine := NewEngine(wfStore, execStore, dlStore, credStore, registry, EngineConfig{
		BaseBackoff: 10 * time.Millisecond,
		MaxBackoff:  50 * time.Millisecond,
	})

	ev, _ := event.New("wh-source", "wh-fail-1", "webhook.received", map[string]any{"data": "test"}, nil)

	_, _ = engine.ProcessEvent(ctx, ev)

	// Verify dead-letter was recorded
	deadLetters, err := dlStore.List(ctx, false)
	if err != nil {
		t.Fatalf("failed to list dead letters: %v", err)
	}
	if len(deadLetters) != 1 {
		t.Fatalf("expected 1 dead letter record, got %d", len(deadLetters))
	}

	rec := deadLetters[0]
	if rec.WorkflowID != wf.ID {
		t.Errorf("expected workflow ID %s, got %s", wf.ID, rec.WorkflowID)
	}
	if rec.Error == "" {
		t.Error("expected recorded error in dead letter")
	}
}
