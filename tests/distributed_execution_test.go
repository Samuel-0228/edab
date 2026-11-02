package tests

import (
	"context"
	"encoding/json"
	"fmt"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"kerob/internal/core/credentials"
	"kerob/internal/core/delivery"
	"kerob/internal/core/event"
	"kerob/internal/core/workflow"
)

type distributedMockExecutor struct {
	mu        sync.Mutex
	executed  []string
	execCount atomic.Int32
	delay     time.Duration
}

func (m *distributedMockExecutor) Name() string                       { return "Distributed Mock" }
func (m *distributedMockExecutor) Type() string                       { return "dist_mock" }
func (m *distributedMockExecutor) Validate(cfg json.RawMessage) error { return nil }
func (m *distributedMockExecutor) Execute(ctx context.Context, req workflow.ActionRequest) (workflow.ActionResult, error) {
	if m.delay > 0 {
		time.Sleep(m.delay)
	}
	m.mu.Lock()
	m.executed = append(m.executed, req.ActionID)
	m.mu.Unlock()
	m.execCount.Add(1)

	return workflow.ActionResult{
		Success:    true,
		ExternalID: fmt.Sprintf("out-%d", m.execCount.Load()),
		Output:     json.RawMessage(`{"status":"ok"}`),
	}, nil
}

// 1. TestMultiWorkerNoDuplicateDeliveryJobs: Multiple concurrent delivery workers claim jobs without duplicates
func TestMultiWorkerNoDuplicateDeliveryJobs(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	store := NewMemoryDeliveryStore()
	const jobCount = 20

	for i := 0; i < jobCount; i++ {
		job := delivery.NewJob(fmt.Sprintf("ev-%d", i), fmt.Sprintf("dest-%d", i), 3)
		if err := store.CreateJob(ctx, job); err != nil {
			t.Fatalf("CreateJob failed: %v", err)
		}
	}

	var claimedIDs sync.Map
	var duplicateCount atomic.Int32
	var totalClaimed atomic.Int32

	// Launch 5 concurrent workers attempting to claim jobs
	var wg sync.WaitGroup
	for w := 0; w < 5; w++ {
		wg.Add(1)
		go func(workerID string) {
			defer wg.Done()
			for {
				job, err := store.ClaimNextJobWithLease(ctx, workerID, 2*time.Second)
				if err != nil {
					t.Errorf("claim error: %v", err)
					return
				}
				if job == nil {
					return
				}
				if _, exists := claimedIDs.LoadOrStore(job.ID, workerID); exists {
					duplicateCount.Add(1)
				}
				totalClaimed.Add(1)
				// Small processing delay
				time.Sleep(5 * time.Millisecond)
				_ = store.MarkDelivered(ctx, job.ID)
			}
		}(fmt.Sprintf("worker-%d", w))
	}

	wg.Wait()

	if dup := duplicateCount.Load(); dup > 0 {
		t.Fatalf("detected %d duplicate job claims across concurrent workers", dup)
	}
	if total := totalClaimed.Load(); total != jobCount {
		t.Fatalf("expected all %d jobs claimed, got %d", jobCount, total)
	}
}

// 2. TestMultiWorkerNoDuplicateWorkflowExecutions: Multiple concurrent workflow workers claim executions without duplicates
func TestMultiWorkerNoDuplicateWorkflowExecutions(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	wfStore := workflow.NewMemoryStore()
	execStore := workflow.NewMemoryExecutionStore()
	dlStore := workflow.NewMemoryDeadLetterStore()
	credStore := credentials.NewMemoryStore(make([]byte, 32))
	evStore := event.NewMemoryStore()

	actionReg := workflow.NewActionRegistry()
	mockExec := &distributedMockExecutor{delay: 10 * time.Millisecond}
	actionReg.Register(mockExec)

	engine := workflow.NewEngine(wfStore, execStore, dlStore, credStore, actionReg)

	const wfCount = 15
	for i := 0; i < wfCount; i++ {
		wf := &workflow.Workflow{
			ID:      fmt.Sprintf("wf-concurrent-%d", i),
			Name:    fmt.Sprintf("Concurrent WF %d", i),
			Enabled: true,
			Trigger: workflow.TriggerConfig{Type: "webhook"},
			Actions: []workflow.ActionStep{
				{
					ID:          fmt.Sprintf("step-%d", i),
					Integration: "dist_mock",
					ActionType:  "dist_mock",
				},
			},
		}
		_, _ = wfStore.Create(ctx, wf)

		ev, _ := event.New("src-1", fmt.Sprintf("ext-wf-%d", i), "webhook", map[string]any{"idx": i}, nil)
		_, _, _ = evStore.Create(ctx, ev)

		_, _ = execStore.Create(ctx, &workflow.Execution{
			WorkflowID:  wf.ID,
			EventID:     ev.ID,
			Status:      workflow.ExecutionPending,
			Attempts:    0,
			MaxAttempts: 3,
		})
	}

	// 3 separate workers with concurrency 2 each running simultaneously
	w1 := workflow.NewWorker(engine, execStore, wfStore, evStore, workflow.WorkerConfig{
		PollInterval: 5 * time.Millisecond,
		Concurrency:  2,
		WorkerID:     "cluster-w1",
	})
	w2 := workflow.NewWorker(engine, execStore, wfStore, evStore, workflow.WorkerConfig{
		PollInterval: 5 * time.Millisecond,
		Concurrency:  2,
		WorkerID:     "cluster-w2",
	})
	w3 := workflow.NewWorker(engine, execStore, wfStore, evStore, workflow.WorkerConfig{
		PollInterval: 5 * time.Millisecond,
		Concurrency:  2,
		WorkerID:     "cluster-w3",
	})

	w1.Start(ctx)
	w2.Start(ctx)
	w3.Start(ctx)

	deadline := time.Now().Add(3 * time.Second)
	allDone := false
	for time.Now().Before(deadline) {
		if int(mockExec.execCount.Load()) == wfCount {
			allDone = true
			break
		}
		time.Sleep(25 * time.Millisecond)
	}

	w1.Stop()
	w2.Stop()
	w3.Stop()

	if !allDone {
		t.Fatalf("expected all %d workflow executions to complete, got %d", wfCount, mockExec.execCount.Load())
	}
	if int(mockExec.execCount.Load()) != wfCount {
		t.Fatalf("detected duplicate workflow execution: expected %d executions, got %d", wfCount, mockExec.execCount.Load())
	}
}

// 3. TestLeaseHeartbeatAndSafeTakeover: Renewal extends lease; worker failure allows takeover after expiration
func TestLeaseHeartbeatAndSafeTakeover(t *testing.T) {
	ctx := context.Background()
	store := workflow.NewMemoryExecutionStore()

	exec, err := store.Create(ctx, &workflow.Execution{
		WorkflowID:  "wf-heartbeat",
		EventID:     "ev-heartbeat",
		Status:      workflow.ExecutionPending,
		Attempts:    0,
		MaxAttempts: 3,
	})
	if err != nil {
		t.Fatalf("Create execution failed: %v", err)
	}

	// Worker 1 claims with a 100ms lease
	claimed, err := store.ClaimPendingWithLease(ctx, "worker-1", 100*time.Millisecond)
	if err != nil || claimed == nil {
		t.Fatalf("Worker 1 failed to claim: %v", err)
	}
	if *claimed.LockedBy != "worker-1" {
		t.Fatalf("expected LockedBy 'worker-1', got %s", *claimed.LockedBy)
	}

	// Worker 2 attempts to claim immediately -> should get nil
	unclaimed, err := store.ClaimPendingWithLease(ctx, "worker-2", 100*time.Millisecond)
	if err != nil {
		t.Fatalf("claim error: %v", err)
	}
	if unclaimed != nil {
		t.Fatalf("Worker 2 should not claim active leased execution")
	}

	// Worker 1 heartbeats at 30ms -> extends expiry
	time.Sleep(30 * time.Millisecond)
	oldExpiry := *claimed.LeaseExpiresAt
	if err := store.RenewLease(ctx, exec.ID, "worker-1", 150*time.Millisecond); err != nil {
		t.Fatalf("Worker 1 RenewLease failed: %v", err)
	}

	updated, _ := store.GetByID(ctx, exec.ID)
	if !updated.LeaseExpiresAt.After(oldExpiry) {
		t.Fatalf("expected renewed expiry %v to be after old expiry %v", updated.LeaseExpiresAt, oldExpiry)
	}

	// Worker 1 crashes (stops heartbeating). Simulate lease expiring past current time:
	_ = store.UpdateStatus(ctx, exec.ID, workflow.ExecutionRunning, nil, 0)
	past := time.Now().UTC().Add(-1 * time.Second)
	store.ExpireLeaseForTest(exec.ID, past)

	// Worker 2 can now safely take over
	takeover, err := store.ClaimPendingWithLease(ctx, "worker-2", 200*time.Millisecond)
	if err != nil || takeover == nil {
		t.Fatalf("Worker 2 failed to take over expired lease: %v", err)
	}
	if takeover.ID != exec.ID || *takeover.LockedBy != "worker-2" {
		t.Fatalf("Worker 2 did not acquire expired execution: %+v", takeover)
	}
}

// 4. TestDelayedAndRetryingJobRecovery: Delayed and retrying jobs recover and execute after next_attempt_at is reached
func TestDelayedAndRetryingJobRecovery(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	wfStore := workflow.NewMemoryStore()
	execStore := workflow.NewMemoryExecutionStore()
	dlStore := workflow.NewMemoryDeadLetterStore()
	credStore := credentials.NewMemoryStore(make([]byte, 32))
	evStore := event.NewMemoryStore()

	actionReg := workflow.NewActionRegistry()
	mockExec := &distributedMockExecutor{}
	actionReg.Register(mockExec)

	engine := workflow.NewEngine(wfStore, execStore, dlStore, credStore, actionReg)

	// Setup delayed workflow
	wfDelay := &workflow.Workflow{
		ID:      "wf-delay-rec",
		Name:    "Delayed Recovery WF",
		Enabled: true,
		Trigger: workflow.TriggerConfig{Type: "webhook"},
		Actions: []workflow.ActionStep{
			{
				ID:          "step-delay",
				Integration: "dist_mock",
				ActionType:  "dist_mock",
			},
		},
	}
	_, _ = wfStore.Create(ctx, wfDelay)
	evDelay, _ := event.New("src-1", "ext-del-1", "webhook", map[string]any{"data": 1}, nil)
	_, _, _ = evStore.Create(ctx, evDelay)

	// Execution was delayed in waiting state, next_attempt_at in the past
	execDelay, _ := execStore.Create(ctx, &workflow.Execution{
		WorkflowID:    wfDelay.ID,
		EventID:       evDelay.ID,
		Status:        workflow.ExecutionWaiting,
		NextAttemptAt: time.Now().UTC().Add(-50 * time.Millisecond),
		Attempts:      1,
		MaxAttempts:   3,
	})

	// Setup retrying workflow
	wfRetry := &workflow.Workflow{
		ID:      "wf-retry-rec",
		Name:    "Retry Recovery WF",
		Enabled: true,
		Trigger: workflow.TriggerConfig{Type: "webhook"},
		Actions: []workflow.ActionStep{
			{
				ID:          "step-retry",
				Integration: "dist_mock",
				ActionType:  "dist_mock",
			},
		},
	}
	_, _ = wfStore.Create(ctx, wfRetry)
	evRetry, _ := event.New("src-1", "ext-ret-1", "webhook", map[string]any{"data": 2}, nil)
	_, _, _ = evStore.Create(ctx, evRetry)

	// Execution was retrying, next_attempt_at in the past
	execRetry, _ := execStore.Create(ctx, &workflow.Execution{
		WorkflowID:    wfRetry.ID,
		EventID:       evRetry.ID,
		Status:        workflow.ExecutionRetrying,
		NextAttemptAt: time.Now().UTC().Add(-50 * time.Millisecond),
		Attempts:      1,
		MaxAttempts:   3,
	})

	// Worker starts up (simulating server restart)
	worker := workflow.NewWorker(engine, execStore, wfStore, evStore, workflow.WorkerConfig{
		PollInterval: 10 * time.Millisecond,
		Concurrency:  2,
		WorkerID:     "restarted-worker",
	})
	worker.Start(ctx)

	deadline := time.Now().Add(2 * time.Second)
	recovered := false
	for time.Now().Before(deadline) {
		d, _ := execStore.GetByID(ctx, execDelay.ID)
		r, _ := execStore.GetByID(ctx, execRetry.ID)
		if d.Status == workflow.ExecutionSuccess && r.Status == workflow.ExecutionSuccess {
			recovered = true
			break
		}
		time.Sleep(20 * time.Millisecond)
	}

	worker.Stop()

	if !recovered {
		d, _ := execStore.GetByID(ctx, execDelay.ID)
		r, _ := execStore.GetByID(ctx, execRetry.ID)
		t.Fatalf("delayed or retrying executions failed to recover: delay=%s, retry=%s", d.Status, r.Status)
	}
	if mockExec.execCount.Load() != 2 {
		t.Errorf("expected both recovered workflows to execute, got %d", mockExec.execCount.Load())
	}
}

// 5. TestWorkerGracefulShutdown: Stopping worker finishes in-flight jobs without dropping them
func TestWorkerGracefulShutdown(t *testing.T) {
	ctx := context.Background()

	wfStore := workflow.NewMemoryStore()
	execStore := workflow.NewMemoryExecutionStore()
	dlStore := workflow.NewMemoryDeadLetterStore()
	credStore := credentials.NewMemoryStore(make([]byte, 32))
	evStore := event.NewMemoryStore()

	actionReg := workflow.NewActionRegistry()
	mockExec := &distributedMockExecutor{delay: 40 * time.Millisecond}
	actionReg.Register(mockExec)

	engine := workflow.NewEngine(wfStore, execStore, dlStore, credStore, actionReg)

	wf := &workflow.Workflow{
		ID:      "wf-graceful",
		Name:    "Graceful WF",
		Enabled: true,
		Trigger: workflow.TriggerConfig{Type: "webhook"},
		Actions: []workflow.ActionStep{
			{
				ID:          "step-1",
				Integration: "dist_mock",
				ActionType:  "dist_mock",
			},
		},
	}
	_, _ = wfStore.Create(ctx, wf)
	ev, _ := event.New("src-1", "ext-grace", "webhook", map[string]any{"data": "drain"}, nil)
	_, _, _ = evStore.Create(ctx, ev)

	exec, _ := execStore.Create(ctx, &workflow.Execution{
		WorkflowID:  wf.ID,
		EventID:     ev.ID,
		Status:      workflow.ExecutionPending,
		Attempts:    0,
		MaxAttempts: 3,
	})

	worker := workflow.NewWorker(engine, execStore, wfStore, evStore, workflow.WorkerConfig{
		PollInterval: 5 * time.Millisecond,
		Concurrency:  1,
	})

	worker.Start(ctx)

	// Wait briefly so worker claims the job and starts executing
	time.Sleep(10 * time.Millisecond)

	// Call Stop() while the 40ms job is actively in-flight
	stopDone := make(chan struct{})
	go func() {
		worker.Stop()
		close(stopDone)
	}()

	select {
	case <-stopDone:
		// Worker shut down cleanly
	case <-time.After(1 * time.Second):
		t.Fatal("worker graceful shutdown timed out")
	}

	// Verify the in-flight job completed rather than being aborted
	completedExec, err := execStore.GetByID(ctx, exec.ID)
	if err != nil {
		t.Fatalf("failed to fetch execution: %v", err)
	}
	if completedExec.Status != workflow.ExecutionSuccess {
		t.Fatalf("expected in-flight execution to complete with success during graceful shutdown, got %s", completedExec.Status)
	}
}
