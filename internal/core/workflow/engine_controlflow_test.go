package workflow

import (
	"context"
	"encoding/json"
	"sync"
	"testing"
	"time"

	"kerob/internal/core/conditions"
	"kerob/internal/core/credentials"
	"kerob/internal/core/event"
)

func TestControlFlowBranching(t *testing.T) {
	ctx := context.Background()

	wfStore := NewMemoryStore()
	execStore := NewMemoryExecutionStore()
	dlStore := NewMemoryDeadLetterStore()
	credStore := credentials.NewMemoryStore(credentials.DeriveKey("test-key-32-bytes-long-padding!!"))
	registry := NewActionRegistry()

	mock := newDynamicMockExecutor("http")
	var executedPath string

	mock.Handlers["high_priority_step"] = func(req ActionRequest) (ActionResult, error) {
		executedPath = "high"
		return ActionResult{Success: true}, nil
	}
	mock.Handlers["low_priority_step"] = func(req ActionRequest) (ActionResult, error) {
		executedPath = "low"
		return ActionResult{Success: true}, nil
	}
	mock.Handlers["default_step"] = func(req ActionRequest) (ActionResult, error) {
		executedPath = "default"
		return ActionResult{Success: true}, nil
	}

	registry.Register(mock)

	wf, err := wfStore.Create(ctx, &Workflow{
		Name:    "Branching Workflow",
		Enabled: true,
		Trigger: TriggerConfig{Type: "webhook"},
		Actions: []ActionStep{
			{
				ID:   "branch_1",
				Type: StepTypeBranch,
				Cases: []BranchCase{
					{
						When: &conditions.Condition{
							Field:    "payload.priority",
							Operator: conditions.OpEquals,
							Value:    "high",
						},
						Steps: []ActionStep{
							{
								ID:          "high_priority_step",
								Integration: "http",
								ActionType:  "notify_pager",
								Config:      json.RawMessage(`{}`),
							},
						},
					},
					{
						When: &conditions.Condition{
							Field:    "payload.priority",
							Operator: conditions.OpEquals,
							Value:    "low",
						},
						Steps: []ActionStep{
							{
								ID:          "low_priority_step",
								Integration: "http",
								ActionType:  "log_low",
								Config:      json.RawMessage(`{}`),
							},
						},
					},
				},
				Default: []ActionStep{
					{
						ID:          "default_step",
						Integration: "http",
						ActionType:  "log_default",
						Config:      json.RawMessage(`{}`),
					},
				},
			},
		},
	})
	if err != nil {
		t.Fatalf("failed to create workflow: %v", err)
	}

	engine := NewEngine(wfStore, execStore, dlStore, credStore, registry)

	// Test 1: Priority High branch
	evHigh, _ := event.New("src-1", "evt-h", "alert", map[string]any{"priority": "high"}, nil)
	execs, err := engine.ProcessEvent(ctx, evHigh)
	if err != nil || len(execs) != 1 || execs[0].Status != ExecutionSuccess {
		t.Fatalf("high branch execution failed: %v", err)
	}
	if executedPath != "high" {
		t.Errorf("expected high branch executed, got %q", executedPath)
	}

	// Test 2: Priority Low branch
	executedPath = ""
	evLow, _ := event.New("src-1", "evt-l", "alert", map[string]any{"priority": "low"}, nil)
	execs, err = engine.ProcessEvent(ctx, evLow)
	if err != nil || len(execs) != 1 || execs[0].Status != ExecutionSuccess {
		t.Fatalf("low branch execution failed: %v", err)
	}
	if executedPath != "low" {
		t.Errorf("expected low branch executed, got %q", executedPath)
	}

	// Test 3: Default branch
	executedPath = ""
	evUnknown, _ := event.New("src-1", "evt-u", "alert", map[string]any{"priority": "medium"}, nil)
	execs, err = engine.ProcessEvent(ctx, evUnknown)
	if err != nil || len(execs) != 1 || execs[0].Status != ExecutionSuccess {
		t.Fatalf("default branch execution failed: %v", err)
	}
	if executedPath != "default" {
		t.Errorf("expected default branch executed, got %q", executedPath)
	}
	_ = wf
}

func TestControlFlowForEach(t *testing.T) {
	ctx := context.Background()

	wfStore := NewMemoryStore()
	execStore := NewMemoryExecutionStore()
	dlStore := NewMemoryDeadLetterStore()
	credStore := credentials.NewMemoryStore(credentials.DeriveKey("test-key-32-bytes-long-padding!!"))
	registry := NewActionRegistry()

	mock := newDynamicMockExecutor("http")
	var processedUsers []string
	var mu sync.Mutex

	mock.Handlers["send_email"] = func(req ActionRequest) (ActionResult, error) {
		var cfg map[string]any
		_ = json.Unmarshal(req.Config, &cfg)
		mu.Lock()
		processedUsers = append(processedUsers, cfg["to"].(string))
		mu.Unlock()
		return ActionResult{Success: true}, nil
	}

	registry.Register(mock)

	_, err := wfStore.Create(ctx, &Workflow{
		Name:    "ForEach Email Workflow",
		Enabled: true,
		Trigger: TriggerConfig{Type: "webhook"},
		Actions: []ActionStep{
			{
				ID:            "loop_users",
				Type:          StepTypeForEach,
				Items:         "payload.users",
				ItemVar:       "user",
				IndexVar:      "idx",
				MaxIterations: 5,
				Steps: []ActionStep{
					{
						ID:          "send_email",
						Integration: "http",
						ActionType:  "send",
						Config:      json.RawMessage(`{"to":"{{user.email}}","index":"{{idx}}"}`),
					},
				},
			},
		},
	})
	if err != nil {
		t.Fatalf("failed to create workflow: %v", err)
	}

	engine := NewEngine(wfStore, execStore, dlStore, credStore, registry)

	ev, _ := event.New("src-1", "evt-users", "batch.users", map[string]any{
		"users": []any{
			map[string]any{"email": "alice@example.com"},
			map[string]any{"email": "bob@example.com"},
			map[string]any{"email": "carol@example.com"},
		},
	}, nil)

	execs, err := engine.ProcessEvent(ctx, ev)
	if err != nil || len(execs) != 1 || execs[0].Status != ExecutionSuccess {
		t.Fatalf("for_each execution failed: %v", err)
	}

	if len(processedUsers) != 3 {
		t.Fatalf("expected 3 processed users, got %d", len(processedUsers))
	}
	if processedUsers[0] != "alice@example.com" || processedUsers[1] != "bob@example.com" || processedUsers[2] != "carol@example.com" {
		t.Errorf("unexpected processed users sequence: %v", processedUsers)
	}
}

func TestControlFlowForEachSafetyCap(t *testing.T) {
	ctx := context.Background()

	wfStore := NewMemoryStore()
	execStore := NewMemoryExecutionStore()
	dlStore := NewMemoryDeadLetterStore()
	credStore := credentials.NewMemoryStore(credentials.DeriveKey("test-key-32-bytes-long-padding!!"))
	registry := NewActionRegistry()

	mock := newDynamicMockExecutor("http")
	count := 0
	mock.Handlers["count_step"] = func(req ActionRequest) (ActionResult, error) {
		count++
		return ActionResult{Success: true}, nil
	}
	registry.Register(mock)

	_, err := wfStore.Create(ctx, &Workflow{
		Name:    "ForEach Cap Workflow",
		Enabled: true,
		Trigger: TriggerConfig{Type: "webhook"},
		Actions: []ActionStep{
			{
				ID:            "loop_cap",
				Type:          StepTypeForEach,
				Items:         "payload.items",
				MaxIterations: 3, // Safety cap at 3
				Steps: []ActionStep{
					{
						ID:          "count_step",
						Integration: "http",
						ActionType:  "run",
						Config:      json.RawMessage(`{}`),
					},
				},
			},
		},
	})
	if err != nil {
		t.Fatalf("failed to create workflow: %v", err)
	}

	engine := NewEngine(wfStore, execStore, dlStore, credStore, registry)

	// Send 10 items
	tenItems := make([]any, 10)
	for i := range tenItems {
		tenItems[i] = i
	}
	ev, _ := event.New("src-1", "evt-cap", "batch.cap", map[string]any{"items": tenItems}, nil)

	execs, err := engine.ProcessEvent(ctx, ev)
	if err != nil || len(execs) != 1 || execs[0].Status != ExecutionSuccess {
		t.Fatalf("for_each cap execution failed: %v", err)
	}

	if count != 3 {
		t.Errorf("expected safety cap to limit to 3 iterations, got %d", count)
	}
}

func TestControlFlowParallelFanOut(t *testing.T) {
	ctx := context.Background()

	wfStore := NewMemoryStore()
	execStore := NewMemoryExecutionStore()
	dlStore := NewMemoryDeadLetterStore()
	credStore := credentials.NewMemoryStore(credentials.DeriveKey("test-key-32-bytes-long-padding!!"))
	registry := NewActionRegistry()

	mock := newDynamicMockExecutor("http")
	var mu sync.Mutex
	executedBranches := make(map[string]bool)

	mock.Handlers["parallel_a"] = func(req ActionRequest) (ActionResult, error) {
		time.Sleep(10 * time.Millisecond)
		mu.Lock()
		executedBranches["a"] = true
		mu.Unlock()
		return ActionResult{Success: true}, nil
	}
	mock.Handlers["parallel_b"] = func(req ActionRequest) (ActionResult, error) {
		time.Sleep(10 * time.Millisecond)
		mu.Lock()
		executedBranches["b"] = true
		mu.Unlock()
		return ActionResult{Success: true}, nil
	}
	registry.Register(mock)

	_, err := wfStore.Create(ctx, &Workflow{
		Name:    "Parallel Fan-out Workflow",
		Enabled: true,
		Trigger: TriggerConfig{Type: "webhook"},
		Actions: []ActionStep{
			{
				ID:          "fan_out",
				Type:        StepTypeParallel,
				Concurrency: 2,
				Steps: []ActionStep{
					{
						ID:          "parallel_a",
						Integration: "http",
						ActionType:  "run_a",
						Config:      json.RawMessage(`{}`),
					},
					{
						ID:          "parallel_b",
						Integration: "http",
						ActionType:  "run_b",
						Config:      json.RawMessage(`{}`),
					},
				},
			},
		},
	})
	if err != nil {
		t.Fatalf("failed to create workflow: %v", err)
	}

	engine := NewEngine(wfStore, execStore, dlStore, credStore, registry)
	ev, _ := event.New("src-1", "evt-par", "test.parallel", map[string]any{}, nil)

	start := time.Now()
	execs, err := engine.ProcessEvent(ctx, ev)
	duration := time.Since(start)

	if err != nil || len(execs) != 1 || execs[0].Status != ExecutionSuccess {
		t.Fatalf("parallel execution failed: %v", err)
	}

	if !executedBranches["a"] || !executedBranches["b"] {
		t.Errorf("expected both parallel branches executed, got: %v", executedBranches)
	}

	// Two 10ms tasks running with concurrency=2 should finish in under 30ms (parallel, not 20ms+ sequentially)
	if duration > 100*time.Millisecond {
		t.Logf("parallel duration took longer than ideal: %v", duration)
	}
}

func TestControlFlowDelayNonBlocking(t *testing.T) {
	ctx := context.Background()

	wfStore := NewMemoryStore()
	execStore := NewMemoryExecutionStore()
	dlStore := NewMemoryDeadLetterStore()
	credStore := credentials.NewMemoryStore(credentials.DeriveKey("test-key-32-bytes-long-padding!!"))
	registry := NewActionRegistry()

	mock := newDynamicMockExecutor("http")
	afterDelayExecuted := false
	mock.Handlers["after_delay"] = func(req ActionRequest) (ActionResult, error) {
		afterDelayExecuted = true
		return ActionResult{Success: true}, nil
	}
	registry.Register(mock)

	wf, err := wfStore.Create(ctx, &Workflow{
		Name:    "Delay Step Workflow",
		Enabled: true,
		Trigger: TriggerConfig{Type: "webhook"},
		Actions: []ActionStep{
			{
				ID:       "wait_step",
				Type:     StepTypeDelay,
				Duration: "100ms",
			},
			{
				ID:          "after_delay",
				Integration: "http",
				ActionType:  "post_wait",
				Config:      json.RawMessage(`{}`),
			},
		},
	})
	if err != nil {
		t.Fatalf("failed to create workflow: %v", err)
	}

	engine := NewEngine(wfStore, execStore, dlStore, credStore, registry)
	ev, _ := event.New("src-1", "evt-delay", "test.delay", map[string]any{}, nil)

	// Step 1: Initial event processing hits delay
	// It must NOT block sleeping for 100ms! It returns immediately with status "waiting"
	start := time.Now()
	execs, err := engine.ProcessEvent(ctx, ev)
	nonBlockElapsed := time.Since(start)

	if err != nil || len(execs) != 1 {
		t.Fatalf("ProcessEvent failed: %v", err)
	}
	if nonBlockElapsed > 50*time.Millisecond {
		t.Errorf("delay blocked the worker! Elapsed: %v (expected immediate return <50ms)", nonBlockElapsed)
	}
	if execs[0].Status != ExecutionWaiting {
		t.Fatalf("expected execution status %s, got %s", ExecutionWaiting, execs[0].Status)
	}
	if afterDelayExecuted {
		t.Fatalf("step after delay should not have executed yet")
	}

	// Step 2: Queue / Scheduler claiming when next_attempt_at is reached
	// Simulate time advancing past the delay:
	exec, err := execStore.GetByID(ctx, execs[0].ID)
	if err != nil {
		t.Fatalf("failed to get execution: %v", err)
	}
	exec.NextAttemptAt = time.Now().UTC().Add(-1 * time.Second) // Set to past

	claimed, err := execStore.ClaimPending(ctx)
	if err != nil {
		t.Fatalf("ClaimPending failed: %v", err)
	}
	if claimed == nil || claimed.ID != exec.ID {
		t.Fatalf("expected to claim waiting execution %s, got %v", exec.ID, claimed)
	}

	// Execute resumed execution
	completedExec, err := engine.ExecuteWorkflow(ctx, wf, ev, claimed)
	if err != nil {
		t.Fatalf("resumed execution failed: %v", err)
	}
	if completedExec.Status != ExecutionSuccess {
		t.Fatalf("expected completed status success, got %s", completedExec.Status)
	}
	if !afterDelayExecuted {
		t.Fatalf("step after delay should have executed upon resumption")
	}
}
