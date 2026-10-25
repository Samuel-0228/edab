package workflow

import (
	"context"
	"encoding/json"
	"errors"
	"testing"
	"time"

	"kerob/internal/core/conditions"
	"kerob/internal/core/credentials"
	"kerob/internal/core/errtracker"
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

func (m *MockActionExecutor) Name() string                          { return "mock" }
func (m *MockActionExecutor) Type() string                          { return m.TypeStr }
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

type SlowActionExecutor struct {
	Delay time.Duration
}

func (s *SlowActionExecutor) Name() string                          { return "slow" }
func (s *SlowActionExecutor) Type() string                          { return "slow" }
func (s *SlowActionExecutor) Validate(config json.RawMessage) error { return nil }
func (s *SlowActionExecutor) Execute(ctx context.Context, req ActionRequest) (ActionResult, error) {
	select {
	case <-ctx.Done():
		return ActionResult{Success: false}, ctx.Err()
	case <-time.After(s.Delay):
		return ActionResult{Success: true}, nil
	}
}

func TestWorkflowEngineConditionEvaluationError(t *testing.T) {
	ctx := context.Background()
	wfStore := NewMemoryStore()
	execStore := NewMemoryExecutionStore()
	dlStore := NewMemoryDeadLetterStore()
	credStore := credentials.NewMemoryStore(credentials.DeriveKey("test-key"))
	registry := NewActionRegistry()

	// Condition with invalid operator or regex
	wf, err := wfStore.Create(ctx, &Workflow{
		Name:    "Broken Regex Condition",
		Enabled: true,
		Trigger: TriggerConfig{Type: "webhook"},
		Conditions: &conditions.Condition{
			Field:    "payload.val",
			Operator: conditions.OpMatches,
			Value:    "[unclosed regex group",
		},
		Actions: []ActionStep{
			{Integration: "mock", ActionType: "test"},
		},
	})
	if err != nil {
		t.Fatalf("failed to create workflow: %v", err)
	}

	engine := NewEngine(wfStore, execStore, dlStore, credStore, registry)
	ev, _ := event.New("src-1", "ext-1", "webhook.received", map[string]any{"val": "hello"}, nil)

	execs, err := engine.ProcessEvent(ctx, ev)
	if err != nil {
		t.Logf("ProcessEvent error: %v", err)
	}
	if len(execs) != 1 || execs[0].Status != ExecutionFailed {
		t.Fatalf("expected ExecutionFailed, got: %+v", execs)
	}

	// Verify dead letter
	dls, _ := dlStore.List(ctx, false)
	if len(dls) != 1 || dls[0].WorkflowID != wf.ID {
		t.Fatalf("expected dead letter for condition error, got: %+v", dls)
	}
}

func TestWorkflowEngineTransformationError(t *testing.T) {
	ctx := context.Background()
	wfStore := NewMemoryStore()
	execStore := NewMemoryExecutionStore()
	dlStore := NewMemoryDeadLetterStore()
	credStore := credentials.NewMemoryStore(credentials.DeriveKey("test-key"))
	registry := NewActionRegistry()

	// Transformation with invalid json template
	wf, err := wfStore.Create(ctx, &Workflow{
		Name:    "Invalid Transformation",
		Enabled: true,
		Trigger: TriggerConfig{Type: "webhook"},
		Transformations: &transformations.Transformation{
			Type:     "json",
			Template: "{invalid json string",
		},
		Actions: []ActionStep{
			{Integration: "mock", ActionType: "test"},
		},
	})
	if err != nil {
		t.Fatalf("failed to create workflow: %v", err)
	}

	engine := NewEngine(wfStore, execStore, dlStore, credStore, registry)
	ev, _ := event.New("src-1", "ext-1", "webhook.received", map[string]any{"val": "hello"}, nil)

	execs, err := engine.ProcessEvent(ctx, ev)
	if err != nil {
		t.Logf("ProcessEvent error: %v", err)
	}
	if len(execs) != 1 || execs[0].Status != ExecutionFailed {
		t.Fatalf("expected ExecutionFailed, got: %+v", execs)
	}

	dls, _ := dlStore.List(ctx, false)
	if len(dls) != 1 || dls[0].WorkflowID != wf.ID {
		t.Fatalf("expected dead letter for transform error, got: %+v", dls)
	}
}

func TestWorkflowEngineActionErrorsAndRetries(t *testing.T) {
	ctx := context.Background()
	wfStore := NewMemoryStore()
	execStore := NewMemoryExecutionStore()
	dlStore := NewMemoryDeadLetterStore()
	credStore := credentials.NewMemoryStore(credentials.DeriveKey("test-key"))
	registry := NewActionRegistry()

	memTracker := errtracker.NewMemoryProvider()

	// 1. Missing executor
	wfNoExecutor, _ := wfStore.Create(ctx, &Workflow{
		Name:       "No Registered Executor",
		Enabled:    true,
		MaxRetries: 1,
		Trigger:    TriggerConfig{Type: "webhook", EventTypes: []string{"test.no_exec"}},
		Actions: []ActionStep{
			{Integration: "unregistered_integration", ActionType: "send"},
		},
	})
	engine := NewEngine(wfStore, execStore, dlStore, credStore, registry)
	engine.SetErrorTracker(memTracker)

	ev1, _ := event.New("src-1", "ext-no-exec", "test.no_exec", map[string]any{}, nil)
	execs, _ := engine.ProcessEvent(ctx, ev1)
	if len(execs) != 1 || execs[0].Status != ExecutionFailed {
		t.Fatalf("expected ExecutionFailed for missing executor, got: %+v", execs)
	}

	// 2. Action Credential Error
	wfBadCred, _ := wfStore.Create(ctx, &Workflow{
		Name:       "Missing Credential",
		Enabled:    true,
		MaxRetries: 1,
		Trigger:    TriggerConfig{Type: "webhook", EventTypes: []string{"test.bad_cred"}},
		Actions: []ActionStep{
			{
				Integration:  "mock",
				ActionType:   "send",
				CredentialID: "non-existent-cred-id",
			},
		},
	})
	ev2, _ := event.New("src-1", "ext-bad-cred", "test.bad_cred", map[string]any{}, nil)
	execs2, _ := engine.ProcessEvent(ctx, ev2)
	if len(execs2) != 1 || execs2[0].Status != ExecutionFailed {
		t.Fatalf("expected ExecutionFailed for bad cred, got: %+v", execs2)
	}

	// 3. Action Timeout
	slowExec := &SlowActionExecutor{Delay: 100 * time.Millisecond}
	registry.Register(slowExec)

	wfTimeout, _ := wfStore.Create(ctx, &Workflow{
		Name:           "Slow Action",
		Enabled:        true,
		MaxRetries:     1,
		TimeoutSeconds: 1,
		Trigger:        TriggerConfig{Type: "webhook", EventTypes: []string{"test.timeout"}},
		Actions: []ActionStep{
			{
				Integration: "slow",
				ActionType:  "run",
				TimeoutSec:  1, // 1 second timeout
			},
		},
	})
	ev3, _ := event.New("src-1", "ext-slow", "test.timeout", map[string]any{}, nil)
	execs3, _ := engine.ProcessEvent(ctx, ev3)
	if len(execs3) != 1 || execs3[0].Status != ExecutionSuccess {
		// 100ms finishes within 1s timeout
		t.Fatalf("expected ExecutionSuccess for 100ms in 1s timeout, got: %+v", execs3)
	}

	// 4. Action Retrying Status when attempts < MaxRetries
	failingExec := &MockActionExecutor{TypeStr: "failing", FailUntil: 10}
	registry.Register(failingExec)

	wfRetry, _ := wfStore.Create(ctx, &Workflow{
		Name:       "Retrying Workflow",
		Enabled:    true,
		MaxRetries: 3, // 3 attempts allowed
		Trigger:    TriggerConfig{Type: "webhook", EventTypes: []string{"test.retry"}},
		Actions: []ActionStep{
			{Integration: "failing", ActionType: "fail_once"},
		},
	})
	ev4, _ := event.New("src-1", "ext-retry", "test.retry", map[string]any{}, nil)
	execs4, _ := engine.ProcessEvent(ctx, ev4)
	if len(execs4) != 1 || execs4[0].Status != ExecutionRetrying {
		t.Fatalf("expected ExecutionRetrying on 1st failure with 3 retries, got: %+v", execs4)
	}
	if execs4[0].NextAttemptAt.IsZero() {
		t.Fatal("expected NextAttemptAt to be set for retrying execution")
	}

	// Verify error tracking recorded captures
	if len(memTracker.Records()) == 0 {
		t.Fatal("expected errtracker captures to be recorded")
	}
	_ = wfNoExecutor
	_ = wfBadCred
	_ = wfTimeout
	_ = wfRetry
}

func TestWorkflowEngineMultiActionSequentialSuccess(t *testing.T) {
	ctx := context.Background()
	wfStore := NewMemoryStore()
	execStore := NewMemoryExecutionStore()
	dlStore := NewMemoryDeadLetterStore()
	credStore := credentials.NewMemoryStore(credentials.DeriveKey("test-key"))
	registry := NewActionRegistry()

	action1 := &MockActionExecutor{TypeStr: "step1"}
	action2 := &MockActionExecutor{TypeStr: "step2"}
	registry.Register(action1)
	registry.Register(action2)

	wf, err := wfStore.Create(ctx, &Workflow{
		Name:    "Sequential Pipeline",
		Enabled: true,
		Trigger: TriggerConfig{Type: "webhook", EventTypes: []string{"pipeline.start"}},
		Actions: []ActionStep{
			{Integration: "step1", ActionType: "first"},
			{Integration: "step2", ActionType: "second"},
		},
	})
	if err != nil {
		t.Fatalf("failed to create workflow: %v", err)
	}

	engine := NewEngine(wfStore, execStore, dlStore, credStore, registry)
	ev, _ := event.New("src-pipe", "evt-pipe-1", "pipeline.start", map[string]any{"data": "start"}, nil)

	execs, err := engine.ProcessEvent(ctx, ev)
	if err != nil {
		t.Fatalf("ProcessEvent failed: %v", err)
	}
	if len(execs) != 1 || execs[0].Status != ExecutionSuccess {
		t.Fatalf("expected 1 execution with success status, got: %+v", execs)
	}

	if action1.Executed != 1 || action2.Executed != 1 {
		t.Fatalf("expected both actions executed once, got action1=%d action2=%d", action1.Executed, action2.Executed)
	}

	// Verify step history in execution store
	storedExec, err := execStore.GetByID(ctx, execs[0].ID)
	if err != nil {
		t.Fatalf("failed to get execution: %v", err)
	}
	// trigger, condition, transform, action1, action2 = 5 steps
	if len(storedExec.Steps) != 5 {
		t.Fatalf("expected 5 steps recorded, got %d", len(storedExec.Steps))
	}
	_ = wf
}

func TestWorkflowEngineMultiActionMidFailure(t *testing.T) {
	ctx := context.Background()
	wfStore := NewMemoryStore()
	execStore := NewMemoryExecutionStore()
	dlStore := NewMemoryDeadLetterStore()
	credStore := credentials.NewMemoryStore(credentials.DeriveKey("test-key"))
	registry := NewActionRegistry()

	action1 := &MockActionExecutor{TypeStr: "step1"}
	action2Fails := &MockActionExecutor{TypeStr: "step2_fail", FailUntil: 999}
	action3Never := &MockActionExecutor{TypeStr: "step3_never"}

	registry.Register(action1)
	registry.Register(action2Fails)
	registry.Register(action3Never)

	wf, err := wfStore.Create(ctx, &Workflow{
		Name:       "Partial Fail Pipeline",
		Enabled:    true,
		MaxRetries: 1,
		Trigger:    TriggerConfig{Type: "webhook", EventTypes: []string{"pipeline.run"}},
		Actions: []ActionStep{
			{Integration: "step1", ActionType: "run1"},
			{Integration: "step2_fail", ActionType: "run2"},
			{Integration: "step3_never", ActionType: "run3"},
		},
	})
	if err != nil {
		t.Fatalf("failed to create workflow: %v", err)
	}

	engine := NewEngine(wfStore, execStore, dlStore, credStore, registry)
	ev, _ := event.New("src-pipe", "evt-pipe-2", "pipeline.run", map[string]any{}, nil)

	execs, _ := engine.ProcessEvent(ctx, ev)
	if len(execs) != 1 || execs[0].Status != ExecutionFailed {
		t.Fatalf("expected ExecutionFailed, got: %+v", execs)
	}

	if action1.Executed != 1 {
		t.Errorf("expected action1 to have executed, got %d", action1.Executed)
	}
	if action2Fails.Executed != 1 {
		t.Errorf("expected action2 to have executed and failed, got %d", action2Fails.Executed)
	}
	if action3Never.Executed != 0 {
		t.Errorf("expected action3 NEVER to have executed, got %d", action3Never.Executed)
	}

	// Verify dead letter recorded specifically for action2
	dls, _ := dlStore.List(ctx, false)
	if len(dls) != 1 {
		t.Fatalf("expected 1 dead letter record, got %d", len(dls))
	}
	if dls[0].ActionID != wf.Actions[1].ID {
		t.Errorf("expected dead letter action ID %s, got %s", wf.Actions[1].ID, dls[0].ActionID)
	}
}

func TestWorkflowEngineDisabledWorkflowIgnored(t *testing.T) {
	ctx := context.Background()
	wfStore := NewMemoryStore()
	execStore := NewMemoryExecutionStore()
	dlStore := NewMemoryDeadLetterStore()
	credStore := credentials.NewMemoryStore(credentials.DeriveKey("test-key"))
	registry := NewActionRegistry()

	spy := &MockActionExecutor{TypeStr: "test_action"}
	registry.Register(spy)

	// Disabled workflow
	_, err := wfStore.Create(ctx, &Workflow{
		Name:    "Disabled Workflow",
		Enabled: false,
		Trigger: TriggerConfig{Type: "webhook", EventTypes: []string{"active.event"}},
		Actions: []ActionStep{{Integration: "test_action", ActionType: "run"}},
	})
	if err != nil {
		t.Fatalf("failed to create workflow: %v", err)
	}

	engine := NewEngine(wfStore, execStore, dlStore, credStore, registry)
	ev, _ := event.New("src-1", "evt-1", "active.event", map[string]any{}, nil)

	execs, err := engine.ProcessEvent(ctx, ev)
	if err != nil {
		t.Fatalf("ProcessEvent failed: %v", err)
	}
	if len(execs) != 0 {
		t.Fatalf("expected 0 executions for disabled workflow, got %d", len(execs))
	}
	if spy.Executed != 0 {
		t.Errorf("expected action not executed for disabled workflow, got %d", spy.Executed)
	}
}

func TestWorkflowEngineTriggerSourceFiltering(t *testing.T) {
	ctx := context.Background()
	wfStore := NewMemoryStore()
	execStore := NewMemoryExecutionStore()
	dlStore := NewMemoryDeadLetterStore()
	credStore := credentials.NewMemoryStore(credentials.DeriveKey("test-key"))
	registry := NewActionRegistry()

	spy := &MockActionExecutor{TypeStr: "action_spy"}
	registry.Register(spy)

	_, _ = wfStore.Create(ctx, &Workflow{
		Name:    "Restricted to Telegram Source",
		Enabled: true,
		Trigger: TriggerConfig{
			Type:       "telegram",
			SourceID:   "tele-source-123",
			EventTypes: []string{"telegram.message"},
		},
		Actions: []ActionStep{{Integration: "action_spy", ActionType: "run"}},
	})

	engine := NewEngine(wfStore, execStore, dlStore, credStore, registry)

	// Event from wrong source -> does not trigger
	evWrongSource, _ := event.New("other-source-999", "msg-1", "telegram.message", map[string]any{}, nil)
	execs1, _ := engine.ProcessEvent(ctx, evWrongSource)
	if len(execs1) != 0 {
		t.Errorf("expected 0 executions from mismatched source, got %d", len(execs1))
	}

	// Event from correct source -> triggers
	evRightSource, _ := event.New("tele-source-123", "msg-2", "telegram.message", map[string]any{}, nil)
	execs2, _ := engine.ProcessEvent(ctx, evRightSource)
	if len(execs2) != 1 || execs2[0].Status != ExecutionSuccess {
		t.Errorf("expected 1 execution with success from matching source, got: %+v", execs2)
	}
}

func TestWorkflowEngineContextCancellation(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel() // cancel immediately

	wfStore := NewMemoryStore()
	execStore := NewMemoryExecutionStore()
	dlStore := NewMemoryDeadLetterStore()
	credStore := credentials.NewMemoryStore(credentials.DeriveKey("test-key"))
	registry := NewActionRegistry()

	wf, _ := wfStore.Create(context.Background(), &Workflow{
		Name:    "Cancelled Workflow",
		Enabled: true,
		Trigger: TriggerConfig{Type: "webhook"},
		Actions: []ActionStep{{Integration: "mock", ActionType: "test"}},
	})

	engine := NewEngine(wfStore, execStore, dlStore, credStore, registry)
	ev, _ := event.New("src-1", "evt-canc", "webhook.test", map[string]any{}, nil)

	exec := &Execution{
		ID:          "exec-canc-1",
		WorkflowID:  wf.ID,
		EventID:     ev.ID,
		Status:      ExecutionPending,
		MaxAttempts: 1,
	}

	_, err := engine.ExecuteWorkflow(ctx, wf, ev, exec)
	if err == nil {
		t.Fatal("expected error on cancelled context, got nil")
	}
}
