package workflow

import (
	"context"
	"testing"
	"time"
)

func TestTrafficControllerRateLimiting(t *testing.T) {
	tc := NewTrafficController()
	wf := &Workflow{
		ID:        "wf_rate_limit",
		RateLimit: 2, // 2 per minute
	}

	ctx := context.Background()

	// 1st acquire allowed
	rel1, err1 := tc.TryAcquire(ctx, wf, "")
	if err1 != nil {
		t.Fatalf("expected 1st acquire to succeed, got %v", err1)
	}

	// 2nd acquire allowed
	rel2, err2 := tc.TryAcquire(ctx, wf, "")
	if err2 != nil {
		t.Fatalf("expected 2nd acquire to succeed, got %v", err2)
	}

	// 3rd acquire rejected (rate limit exceeded)
	_, err3 := tc.TryAcquire(ctx, wf, "")
	if err3 != ErrRateLimited {
		t.Fatalf("expected ErrRateLimited, got err=%v", err3)
	}

	rel1()
	rel2()
}

func TestTrafficControllerMaxConcurrency(t *testing.T) {
	tc := NewTrafficController()
	wf := &Workflow{
		ID:             "wf_concurrency",
		MaxConcurrency: 2,
	}

	ctx := context.Background()

	rel1, err1 := tc.TryAcquire(ctx, wf, "")
	if err1 != nil {
		t.Fatalf("acquire 1 failed: %v", err1)
	}
	if tc.ActiveCount(wf.ID) != 1 {
		t.Errorf("expected 1 active, got %d", tc.ActiveCount(wf.ID))
	}

	rel2, err2 := tc.TryAcquire(ctx, wf, "")
	if err2 != nil {
		t.Fatalf("acquire 2 failed: %v", err2)
	}
	if tc.ActiveCount(wf.ID) != 2 {
		t.Errorf("expected 2 active, got %d", tc.ActiveCount(wf.ID))
	}

	// 3rd should fail max concurrency
	_, err3 := tc.TryAcquire(ctx, wf, "")
	if err3 != ErrMaxConcurrencyReached {
		t.Fatalf("expected ErrMaxConcurrencyReached, got %v", err3)
	}

	// Release 1 and try again
	rel1()
	if tc.ActiveCount(wf.ID) != 1 {
		t.Errorf("expected 1 active after release, got %d", tc.ActiveCount(wf.ID))
	}

	rel3, err3Post := tc.TryAcquire(ctx, wf, "")
	if err3Post != nil {
		t.Fatalf("expected acquire to succeed after release, got %v", err3Post)
	}

	rel2()
	rel3()
	if tc.ActiveCount(wf.ID) != 0 {
		t.Errorf("expected 0 active, got %d", tc.ActiveCount(wf.ID))
	}
}

func TestTrafficControllerConcurrencyKey(t *testing.T) {
	tc := NewTrafficController()
	wf := &Workflow{
		ID: "wf_key",
	}

	ctx := context.Background()

	// Acquire for chat_123
	rel1, err1 := tc.TryAcquire(ctx, wf, "chat_123")
	if err1 != nil {
		t.Fatalf("expected acquire for chat_123 to succeed, got %v", err1)
	}

	// Another chat_456 can run concurrently
	rel2, err2 := tc.TryAcquire(ctx, wf, "chat_456")
	if err2 != nil {
		t.Fatalf("expected acquire for chat_456 to succeed, got %v", err2)
	}

	// Same chat_123 rejected because it is currently executing
	_, errBusy := tc.TryAcquire(ctx, wf, "chat_123")
	if errBusy != ErrConcurrencyKeyBusy {
		t.Fatalf("expected ErrConcurrencyKeyBusy for same key, got %v", errBusy)
	}

	rel1()

	// After release, chat_123 can acquire again
	rel1New, err1New := tc.TryAcquire(ctx, wf, "chat_123")
	if err1New != nil {
		t.Fatalf("expected chat_123 to succeed after release, got %v", err1New)
	}

	rel2()
	rel1New()
}

func TestCircuitBreakerLifecycle(t *testing.T) {
	reg := NewCircuitBreakerRegistry(CircuitBreakerConfig{
		FailureThreshold: 3,
		OpenTimeout:      50 * time.Millisecond,
	})

	integ := "test_api"

	// 1. Initial State: Closed, allowed
	if err := reg.Allow(integ); err != nil {
		t.Fatalf("expected closed circuit to allow calls, got %v", err)
	}

	// 2. 2 Failures: still closed
	reg.RecordFailure(integ)
	reg.RecordFailure(integ)
	if err := reg.Allow(integ); err != nil {
		t.Fatalf("expected circuit to stay closed below threshold, got %v", err)
	}

	// 3. 3rd Failure: trips to Open
	reg.RecordFailure(integ)
	errOpen := reg.Allow(integ)
	if errOpen == nil {
		t.Fatal("expected circuit to be open after reaching failure threshold")
	}

	status := reg.Status()
	if status[integ] != string(StateOpen) {
		t.Errorf("expected open state, got %s", status[integ])
	}

	// 4. Wait for openTimeout to expire -> StateHalfOpen
	time.Sleep(60 * time.Millisecond)

	// Half-open allows 1 probe request
	if err := reg.Allow(integ); err != nil {
		t.Fatalf("expected half-open probe to be allowed, got %v", err)
	}

	// 5. Successful probe resets to Closed
	reg.RecordSuccess(integ)
	if err := reg.Allow(integ); err != nil {
		t.Fatalf("expected circuit to be closed after success, got %v", err)
	}

	finalStatus := reg.Status()
	if finalStatus[integ] != string(StateClosed) {
		t.Errorf("expected closed state, got %s", finalStatus[integ])
	}
}

func TestExecutionPriorityClaiming(t *testing.T) {
	store := NewMemoryExecutionStore()
	ctx := context.Background()

	// Create low priority execution
	execLow, _ := store.Create(ctx, &Execution{
		WorkflowID: "wf_1",
		EventID:    "evt_low",
		Priority:   0,
		Status:     ExecutionPending,
	})

	// Create high priority execution created later
	execHigh, _ := store.Create(ctx, &Execution{
		WorkflowID: "wf_1",
		EventID:    "evt_high",
		Priority:   100,
		Status:     ExecutionPending,
	})

	// ClaimPending should pick the high priority execution first
	claimed1, err := store.ClaimPending(ctx)
	if err != nil {
		t.Fatalf("ClaimPending 1 failed: %v", err)
	}
	if claimed1.ID != execHigh.ID {
		t.Errorf("expected high priority execution %s, got %s", execHigh.ID, claimed1.ID)
	}

	// ClaimPending should pick the low priority execution next
	claimed2, err := store.ClaimPending(ctx)
	if err != nil {
		t.Fatalf("ClaimPending 2 failed: %v", err)
	}
	if claimed2.ID != execLow.ID {
		t.Errorf("expected low priority execution %s, got %s", execLow.ID, claimed2.ID)
	}
}
