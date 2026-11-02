package workflow

import (
	"context"
	"encoding/json"
	"fmt"
	"sync/atomic"
	"testing"
	"time"

	"kerob/internal/core/credentials"
	"kerob/internal/core/event"
)

type mockActionExecutor struct {
	execCount atomic.Int32
	delay     time.Duration
}

func (m *mockActionExecutor) Name() string                       { return "Mock Action" }
func (m *mockActionExecutor) Type() string                       { return "mock_integration" }
func (m *mockActionExecutor) Validate(cfg json.RawMessage) error { return nil }
func (m *mockActionExecutor) Execute(ctx context.Context, req ActionRequest) (ActionResult, error) {
	if m.delay > 0 {
		time.Sleep(m.delay)
	}
	m.execCount.Add(1)
	return ActionResult{
		Success:    true,
		ExternalID: fmt.Sprintf("mock-res-%d", m.execCount.Load()),
		Output:     json.RawMessage(`{"status":"ok"}`),
	}, nil
}

func setupTestWorkflowEngine() (*Engine, *MemoryExecutionStore, *MemoryStore, *event.MemoryStore, *mockActionExecutor) {
	wfStore := NewMemoryStore()
	execStore := NewMemoryExecutionStore()
	dlStore := NewMemoryDeadLetterStore()
	credStore := credentials.NewMemoryStore(make([]byte, 32))
	actionReg := NewActionRegistry()
	mockExec := &mockActionExecutor{}
	actionReg.Register(mockExec)

	engine := NewEngine(wfStore, execStore, dlStore, credStore, actionReg)
	evStore := event.NewMemoryStore()

	return engine, execStore, wfStore, evStore, mockExec
}

func TestWorkflowWorkerClaimAndExecute(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	engine, execStore, wfStore, evStore, mockExec := setupTestWorkflowEngine()

	wf := &Workflow{
		ID:      "wf-worker-1",
		Name:    "Worker Test WF",
		Enabled: true,
		Trigger: TriggerConfig{Type: "webhook"},
		Actions: []ActionStep{
			{
				ID:          "step-1",
				Integration: "mock_integration",
				ActionType:  "mock_action",
			},
		},
	}
	_, _ = wfStore.Create(ctx, wf)

	ev, _ := event.New("src-1", "ext-1", "webhook", map[string]any{"data": "test"}, nil)
	_, _, _ = evStore.Create(ctx, ev)

	exec, err := execStore.Create(ctx, &Execution{
		WorkflowID:  wf.ID,
		EventID:     ev.ID,
		Status:      ExecutionPending,
		Attempts:    0,
		MaxAttempts: 3,
	})
	if err != nil {
		t.Fatalf("failed to create execution: %v", err)
	}

	worker := NewWorker(engine, execStore, wfStore, evStore, WorkerConfig{
		PollInterval:      10 * time.Millisecond,
		Concurrency:       2,
		LeaseDuration:     1 * time.Second,
		HeartbeatInterval: 200 * time.Millisecond,
		WorkerID:          "test-worker-1",
	})

	worker.Start(ctx)

	// Await completion
	deadline := time.Now().Add(1 * time.Second)
	var completed bool
	for time.Now().Before(deadline) {
		res, _ := execStore.GetByID(ctx, exec.ID)
		if res.Status == ExecutionSuccess {
			completed = true
			if res.LockedBy != nil || res.LeaseExpiresAt != nil {
				t.Errorf("expected lease info to be cleared on completion, got locked_by=%v", res.LockedBy)
			}
			break
		}
		time.Sleep(15 * time.Millisecond)
	}

	worker.Stop()

	if !completed {
		t.Fatalf("expected execution to complete with success via worker")
	}
	if mockExec.execCount.Load() != 1 {
		t.Errorf("expected 1 action execution, got %d", mockExec.execCount.Load())
	}
}

func TestWorkflowWorkerDelayedExecutionResumption(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	engine, execStore, wfStore, evStore, mockExec := setupTestWorkflowEngine()

	wf := &Workflow{
		ID:      "wf-delayed-1",
		Name:    "Delayed Resumption WF",
		Enabled: true,
		Trigger: TriggerConfig{Type: "webhook"},
		Actions: []ActionStep{
			{
				ID:          "step-1",
				Integration: "mock_integration",
				ActionType:  "mock_action",
			},
		},
	}
	_, _ = wfStore.Create(ctx, wf)

	ev, _ := event.New("src-1", "ext-delayed", "webhook", map[string]any{"key": "val"}, nil)
	_, _, _ = evStore.Create(ctx, ev)

	// Create execution in waiting state with next_attempt_at in past
	exec, _ := execStore.Create(ctx, &Execution{
		WorkflowID:    wf.ID,
		EventID:       ev.ID,
		Status:        ExecutionWaiting,
		NextAttemptAt: time.Now().UTC().Add(-100 * time.Millisecond),
		Attempts:      1,
		MaxAttempts:   3,
	})

	worker := NewWorker(engine, execStore, wfStore, evStore, WorkerConfig{
		PollInterval: 10 * time.Millisecond,
		Concurrency:  1,
	})

	claimed, err := worker.ProcessNextExecution(ctx)
	if err != nil {
		t.Fatalf("ProcessNextExecution failed: %v", err)
	}
	if !claimed {
		t.Fatalf("expected worker to claim waiting execution")
	}

	res, _ := execStore.GetByID(ctx, exec.ID)
	if res.Status != ExecutionSuccess {
		t.Fatalf("expected resumed execution to succeed, got %s", res.Status)
	}
	if mockExec.execCount.Load() != 1 {
		t.Errorf("expected action to execute once resumed, got %d", mockExec.execCount.Load())
	}
}

func TestWorkflowWorkerRetryExecutionResumption(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	engine, execStore, wfStore, evStore, mockExec := setupTestWorkflowEngine()

	wf := &Workflow{
		ID:      "wf-retry-1",
		Name:    "Retry Resumption WF",
		Enabled: true,
		Trigger: TriggerConfig{Type: "webhook"},
		Actions: []ActionStep{
			{
				ID:          "step-1",
				Integration: "mock_integration",
				ActionType:  "mock_action",
			},
		},
	}
	_, _ = wfStore.Create(ctx, wf)

	ev, _ := event.New("src-1", "ext-retry", "webhook", map[string]any{"key": "retry"}, nil)
	_, _, _ = evStore.Create(ctx, ev)

	// Create execution in retrying state with next_attempt_at in past
	exec, _ := execStore.Create(ctx, &Execution{
		WorkflowID:    wf.ID,
		EventID:       ev.ID,
		Status:        ExecutionRetrying,
		NextAttemptAt: time.Now().UTC().Add(-50 * time.Millisecond),
		Attempts:      1,
		MaxAttempts:   3,
	})

	worker := NewWorker(engine, execStore, wfStore, evStore, WorkerConfig{
		PollInterval: 10 * time.Millisecond,
		Concurrency:  1,
	})

	claimed, err := worker.ProcessNextExecution(ctx)
	if err != nil {
		t.Fatalf("ProcessNextExecution failed: %v", err)
	}
	if !claimed {
		t.Fatalf("expected worker to claim retrying execution")
	}

	res, _ := execStore.GetByID(ctx, exec.ID)
	if res.Status != ExecutionSuccess {
		t.Fatalf("expected retried execution to succeed, got %s", res.Status)
	}
	if mockExec.execCount.Load() != 1 {
		t.Errorf("expected action to execute on retry, got %d", mockExec.execCount.Load())
	}
}

func TestWorkflowWorkerLeaseExpirationAndSafeTakeover(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	engine, execStore, wfStore, evStore, mockExec := setupTestWorkflowEngine()

	wf := &Workflow{
		ID:      "wf-takeover-1",
		Name:    "Takeover WF",
		Enabled: true,
		Trigger: TriggerConfig{Type: "webhook"},
		Actions: []ActionStep{
			{
				ID:          "step-1",
				Integration: "mock_integration",
				ActionType:  "mock_action",
			},
		},
	}
	_, _ = wfStore.Create(ctx, wf)

	ev, _ := event.New("src-1", "ext-takeover", "webhook", map[string]any{"data": "crash"}, nil)
	_, _, _ = evStore.Create(ctx, ev)

	deadWorkerID := "crashed-worker-999"
	pastExpiry := time.Now().UTC().Add(-30 * time.Second)

	// Execution stuck in running state because dead worker crashed
	exec, _ := execStore.Create(ctx, &Execution{
		WorkflowID:     wf.ID,
		EventID:        ev.ID,
		Status:         ExecutionRunning,
		Attempts:       1,
		MaxAttempts:    3,
		LockedBy:       &deadWorkerID,
		LeaseExpiresAt: &pastExpiry,
	})

	worker := NewWorker(engine, execStore, wfStore, evStore, WorkerConfig{
		PollInterval: 10 * time.Millisecond,
		Concurrency:  1,
		WorkerID:     "healthy-worker-1",
	})

	claimed, err := worker.ProcessNextExecution(ctx)
	if err != nil {
		t.Fatalf("failed to take over execution: %v", err)
	}
	if !claimed {
		t.Fatalf("expected healthy worker to safely take over expired execution")
	}

	res, _ := execStore.GetByID(ctx, exec.ID)
	if res.Status != ExecutionSuccess {
		t.Fatalf("expected taken over execution to complete with success, got %s", res.Status)
	}
	if mockExec.execCount.Load() != 1 {
		t.Errorf("expected action to execute after takeover, got %d", mockExec.execCount.Load())
	}
}

func TestWorkflowWorkerConcurrencyAndZeroDuplicateExecution(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	engine, execStore, wfStore, evStore, mockExec := setupTestWorkflowEngine()
	mockExec.delay = 15 * time.Millisecond

	const numJobs = 10
	var execIDs []string

	for i := 0; i < numJobs; i++ {
		wf := &Workflow{
			ID:      fmt.Sprintf("wf-batch-%d", i),
			Name:    fmt.Sprintf("Batch WF %d", i),
			Enabled: true,
			Trigger: TriggerConfig{Type: "webhook"},
			Actions: []ActionStep{
				{
					ID:          "step-1",
					Integration: "mock_integration",
					ActionType:  "mock_action",
				},
			},
		}
		_, _ = wfStore.Create(ctx, wf)

		ev, _ := event.New("src-1", fmt.Sprintf("ext-batch-%d", i), "webhook", map[string]any{"i": i}, nil)
		_, _, _ = evStore.Create(ctx, ev)

		exec, _ := execStore.Create(ctx, &Execution{
			WorkflowID:  wf.ID,
			EventID:     ev.ID,
			Status:      ExecutionPending,
			Attempts:    0,
			MaxAttempts: 3,
		})
		execIDs = append(execIDs, exec.ID)
	}

	// Run two separate workers with concurrency 4 each, claiming against the shared store
	worker1 := NewWorker(engine, execStore, wfStore, evStore, WorkerConfig{
		PollInterval: 5 * time.Millisecond,
		Concurrency:  4,
		WorkerID:     "cluster-worker-1",
	})
	worker2 := NewWorker(engine, execStore, wfStore, evStore, WorkerConfig{
		PollInterval: 5 * time.Millisecond,
		Concurrency:  4,
		WorkerID:     "cluster-worker-2",
	})

	worker1.Start(ctx)
	worker2.Start(ctx)

	deadline := time.Now().Add(3 * time.Second)
	allDone := false
	for time.Now().Before(deadline) {
		completed := 0
		for _, id := range execIDs {
			e, _ := execStore.GetByID(ctx, id)
			if e.Status == ExecutionSuccess {
				completed++
			}
		}
		if completed == numJobs {
			allDone = true
			break
		}
		time.Sleep(20 * time.Millisecond)
	}

	worker1.Stop()
	worker2.Stop()

	if !allDone {
		t.Fatalf("not all concurrent executions completed")
	}
	if int(mockExec.execCount.Load()) != numJobs {
		t.Fatalf("expected exactly %d action executions, got %d (potential duplicate execution!)", numJobs, mockExec.execCount.Load())
	}
}
