package workflow

import (
	"context"
	"testing"
	"time"
)

func TestMemoryExecutionStoreFull(t *testing.T) {
	ctx := context.Background()
	store := NewMemoryExecutionStore()

	// 1. Create execution
	exec, err := store.Create(ctx, &Execution{
		WorkflowID:    "wf-1",
		EventID:       "ev-1",
		Status:        ExecutionPending,
		NextAttemptAt: time.Now().UTC().Add(-1 * time.Minute), // ready to claim
	})
	if err != nil || exec.ID == "" {
		t.Fatalf("failed to create execution: %v", err)
	}

	// 2. ClaimPending
	claimed, err := store.ClaimPending(ctx)
	if err != nil || claimed == nil || claimed.ID != exec.ID {
		t.Fatalf("expected to claim pending execution, got: %+v, err: %v", claimed, err)
	}
	if claimed.Status != ExecutionRunning {
		t.Errorf("expected status ExecutionRunning, got %s", claimed.Status)
	}

	// No more pending to claim
	none, _ := store.ClaimPending(ctx)
	if none != nil {
		t.Errorf("expected nil when no pending executions, got %+v", none)
	}

	// 3. GetByWorkflowAndEvent
	byWfEv, err := store.GetByWorkflowAndEvent(ctx, "wf-1", "ev-1")
	if err != nil || byWfEv.ID != exec.ID {
		t.Fatalf("expected to get by workflow and event, got: %+v, err: %v", byWfEv, err)
	}
	if _, err := store.GetByWorkflowAndEvent(ctx, "wf-none", "ev-none"); err != ErrExecutionNotFound {
		t.Errorf("expected ErrExecutionNotFound, got %v", err)
	}

	// 4. UpdateStatus
	errStr := "cancelled by operator"
	if err := store.UpdateStatus(ctx, exec.ID, ExecutionCancelled, &errStr, 150); err != nil {
		t.Fatalf("UpdateStatus failed: %v", err)
	}
	if err := store.UpdateStatus(ctx, "nonexistent", ExecutionCancelled, nil, 0); err != ErrExecutionNotFound {
		t.Errorf("expected ErrExecutionNotFound, got %v", err)
	}

	// 5. ResetForRetry
	reset, err := store.ResetForRetry(ctx, exec.ID)
	if err != nil || reset.Status != ExecutionPending {
		t.Fatalf("ResetForRetry failed: %v, status: %s", err, reset.Status)
	}
	if _, err := store.ResetForRetry(ctx, "nonexistent"); err != ErrExecutionNotFound {
		t.Errorf("expected ErrExecutionNotFound for ResetForRetry nonexistent, got %v", err)
	}

	// 6. List with filtering, offset and limit
	_, _ = store.Create(ctx, &Execution{
		WorkflowID: "wf-2",
		EventID:    "ev-2",
		Status:     ExecutionSuccess,
	})

	listWf1, _ := store.List(ctx, ExecutionFilter{WorkflowID: "wf-1"})
	if len(listWf1) != 1 {
		t.Errorf("expected 1 execution for wf-1, got %d", len(listWf1))
	}

	listOffsetExceeded, _ := store.List(ctx, ExecutionFilter{Offset: 100})
	if len(listOffsetExceeded) != 0 {
		t.Errorf("expected 0 for large offset, got %d", len(listOffsetExceeded))
	}

	listLimit, _ := store.List(ctx, ExecutionFilter{Limit: 1})
	if len(listLimit) != 1 {
		t.Errorf("expected 1 execution with limit=1, got %d", len(listLimit))
	}
}

func TestMemoryDeadLetterStoreFull(t *testing.T) {
	ctx := context.Background()
	store := NewMemoryDeadLetterStore()

	// 1. Record
	rec, err := store.Record(ctx, &DeadLetterRecord{
		ExecutionID: "exec-1",
		WorkflowID:  "wf-1",
		EventID:     "ev-1",
		Error:       "third retry exhausted",
	})
	if err != nil || rec.ID == "" {
		t.Fatalf("failed to record dead letter: %v", err)
	}

	// 2. GetByID
	fetched, err := store.GetByID(ctx, rec.ID)
	if err != nil || fetched.Error != "third retry exhausted" {
		t.Fatalf("GetByID failed: %v", err)
	}
	if _, err := store.GetByID(ctx, "nonexistent"); err != ErrDeadLetterNotFound {
		t.Errorf("expected ErrDeadLetterNotFound, got %v", err)
	}

	// 3. List
	activeList, _ := store.List(ctx, false)
	if len(activeList) != 1 {
		t.Fatalf("expected 1 active dead letter, got %d", len(activeList))
	}

	// 4. Discard
	if err := store.Discard(ctx, rec.ID); err != nil {
		t.Fatalf("Discard failed: %v", err)
	}
	if err := store.Discard(ctx, "nonexistent"); err != ErrDeadLetterNotFound {
		t.Errorf("expected ErrDeadLetterNotFound on discard nonexistent, got %v", err)
	}

	// Verify List without discarded vs with discarded
	activeAfterDiscard, _ := store.List(ctx, false)
	if len(activeAfterDiscard) != 0 {
		t.Errorf("expected 0 active dead letters after discard, got %d", len(activeAfterDiscard))
	}

	allList, _ := store.List(ctx, true)
	if len(allList) != 1 {
		t.Errorf("expected 1 dead letter when including discarded, got %d", len(allList))
	}
}

func TestMemoryWorkflowStoreExtended(t *testing.T) {
	ctx := context.Background()
	store := NewMemoryStore()

	// 1. Create workflows (one enabled, one disabled)
	wf1, err := store.Create(ctx, &Workflow{
		Name:    "Alpha Workflow",
		Enabled: true,
		Trigger: TriggerConfig{Type: "webhook"},
		Actions: []ActionStep{{Integration: "discord", ActionType: "send"}},
	})
	if err != nil || wf1.ID == "" {
		t.Fatalf("failed to create workflow: %v", err)
	}

	_, _ = store.Create(ctx, &Workflow{
		Name:    "Beta Disabled",
		Enabled: false,
		Trigger: TriggerConfig{Type: "webhook"},
		Actions: []ActionStep{{Integration: "discord", ActionType: "send"}},
	})

	// 2. Validate Workflow constraints
	validWf := &Workflow{
		Name:    "Alpha Workflow",
		Enabled: true,
		Trigger: TriggerConfig{Type: "webhook"},
		Actions: []ActionStep{{Integration: "discord", ActionType: "send_message"}},
	}
	if err := validWf.Validate(); err != nil {
		t.Fatalf("expected valid workflow, got: %v", err)
	}

	invalidNoName := &Workflow{Trigger: TriggerConfig{Type: "webhook"}, Actions: []ActionStep{{Integration: "discord", ActionType: "send"}}}
	if err := invalidNoName.Validate(); err == nil {
		t.Fatal("expected error on missing name")
	}

	invalidNoTrigger := &Workflow{Name: "No Trigger", Actions: []ActionStep{{Integration: "discord", ActionType: "send"}}}
	if err := invalidNoTrigger.Validate(); err == nil {
		t.Fatal("expected error on missing trigger")
	}

	invalidNoActions := &Workflow{Name: "No Actions", Trigger: TriggerConfig{Type: "webhook"}}
	if err := invalidNoActions.Validate(); err == nil {
		t.Fatal("expected error on missing actions")
	}

	invalidActionInteg := &Workflow{Name: "Bad Action", Trigger: TriggerConfig{Type: "webhook"}, Actions: []ActionStep{{ActionType: "send"}}}
	if err := invalidActionInteg.Validate(); err == nil {
		t.Fatal("expected error on missing action integration")
	}

	invalidActionType := &Workflow{Name: "Bad Action", Trigger: TriggerConfig{Type: "webhook"}, Actions: []ActionStep{{Integration: "discord"}}}
	if err := invalidActionType.Validate(); err == nil {
		t.Fatal("expected error on missing action action_type")
	}

	// 3. List and ListActive
	allList, err := store.List(ctx)
	if err != nil || len(allList) != 2 {
		t.Fatalf("expected 2 workflows in list, got %d", len(allList))
	}

	activeList, err := store.ListActive(ctx)
	if err != nil || len(activeList) != 1 || activeList[0].ID != wf1.ID {
		t.Fatalf("expected 1 active workflow (Alpha), got %d", len(activeList))
	}

	// 4. Update and Delete nonexistent
	wf1.Name = "Updated Alpha"
	updated, err := store.Update(ctx, wf1)
	if err != nil || updated.Name != "Updated Alpha" {
		t.Fatalf("expected successful update, got: %+v, err: %v", updated, err)
	}

	_, err = store.Update(ctx, &Workflow{
		ID:      "nonexistent",
		Name:    "Ghost",
		Trigger: TriggerConfig{Type: "webhook"},
		Actions: []ActionStep{{Integration: "discord", ActionType: "send"}},
	})
	if err != ErrWorkflowNotFound {
		t.Errorf("expected ErrWorkflowNotFound on update, got %v", err)
	}

	err = store.Delete(ctx, "nonexistent")
	if err != ErrWorkflowNotFound {
		t.Errorf("expected ErrWorkflowNotFound on delete, got %v", err)
	}
}
