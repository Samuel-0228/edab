package workflow

import (
	"context"
	"fmt"
	"sync"
	"testing"
	"time"
)

func TestMemoryStore_DeepCopyAndIsolation(t *testing.T) {
	ctx := context.Background()
	store := NewMemoryStore()

	original := &Workflow{
		Name:    "Isolation Test",
		Enabled: true,
		Trigger: TriggerConfig{
			Type:       "webhook",
			EventTypes: []string{"test.event"},
		},
		Actions: []ActionStep{
			{Integration: "http", ActionType: "post"},
		},
	}

	created, err := store.Create(ctx, original)
	if err != nil {
		t.Fatalf("failed to create workflow: %v", err)
	}

	// 1. Mutate original after Create — should NOT affect store
	original.Name = "Mutated Original Name"
	original.Trigger.EventTypes[0] = "mutated.event"
	original.Actions[0].Integration = "mutated_integration"

	fetched, err := store.GetByID(ctx, created.ID)
	if err != nil {
		t.Fatalf("failed to fetch created workflow: %v", err)
	}
	if fetched.Name != "Isolation Test" {
		t.Errorf("expected store name 'Isolation Test', got: %s", fetched.Name)
	}
	if fetched.Trigger.EventTypes[0] != "test.event" {
		t.Errorf("expected trigger event type 'test.event', got: %s", fetched.Trigger.EventTypes[0])
	}
	if fetched.Actions[0].Integration != "http" {
		t.Errorf("expected action integration 'http', got: %s", fetched.Actions[0].Integration)
	}

	// 2. Mutate fetched object — should NOT affect store
	fetched.Name = "Mutated Fetched Name"
	fetchedAgain, err := store.GetByID(ctx, created.ID)
	if err != nil {
		t.Fatalf("failed to re-fetch workflow: %v", err)
	}
	if fetchedAgain.Name != "Isolation Test" {
		t.Errorf("expected store name to remain 'Isolation Test', got: %s", fetchedAgain.Name)
	}
}

func TestMemoryStore_ConcurrentAccessThreadSafety(t *testing.T) {
	ctx := context.Background()
	store := NewMemoryStore()

	// Seed with a few initial workflows
	for i := 0; i < 5; i++ {
		_, err := store.Create(ctx, &Workflow{
			Name:    fmt.Sprintf("Initial WF %d", i),
			Enabled: i%2 == 0,
			Trigger: TriggerConfig{Type: "webhook"},
			Actions: []ActionStep{{Integration: "http", ActionType: "post"}},
		})
		if err != nil {
			t.Fatalf("failed to seed initial workflow: %v", err)
		}
	}

	var wg sync.WaitGroup
	workers := 20
	iterations := 50

	// Concurrent Creators
	for w := 0; w < workers; w++ {
		wg.Add(1)
		go func(workerID int) {
			defer wg.Done()
			for i := 0; i < iterations; i++ {
				_, _ = store.Create(ctx, &Workflow{
					Name:    fmt.Sprintf("Worker-%d-WF-%d", workerID, i),
					Enabled: i%2 == 0,
					Trigger: TriggerConfig{Type: "webhook"},
					Actions: []ActionStep{{Integration: "discord", ActionType: "send"}},
				})
			}
		}(w)
	}

	// Concurrent Readers (List, ListActive, GetByID)
	for w := 0; w < workers; w++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for i := 0; i < iterations; i++ {
				all, _ := store.List(ctx)
				if len(all) > 0 {
					target := all[0]
					_, _ = store.GetByID(ctx, target.ID)
				}
				_, _ = store.ListActive(ctx)
				time.Sleep(100 * time.Microsecond)
			}
		}()
	}

	// Concurrent Updaters and Deleters
	for w := 0; w < 5; w++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for i := 0; i < iterations; i++ {
				all, _ := store.List(ctx)
				if len(all) > 5 {
					toUpdate := all[len(all)-1]
					toUpdate.Name = "Updated in Goroutine"
					_, _ = store.Update(ctx, toUpdate)
				}
			}
		}()
	}

	wg.Wait()

	finalList, err := store.List(ctx)
	if err != nil {
		t.Fatalf("failed to list workflows after concurrency test: %v", err)
	}
	if len(finalList) < 5 {
		t.Errorf("expected at least 5 workflows, got %d", len(finalList))
	}
}

func TestMemoryStore_CRUDAndNotFoundErrors(t *testing.T) {
	ctx := context.Background()
	store := NewMemoryStore()

	// 1. GetByID on non-existent
	if _, err := store.GetByID(ctx, "nonexistent-id"); err != ErrWorkflowNotFound {
		t.Errorf("expected ErrWorkflowNotFound, got: %v", err)
	}

	// 2. Delete on non-existent
	if err := store.Delete(ctx, "nonexistent-id"); err != ErrWorkflowNotFound {
		t.Errorf("expected ErrWorkflowNotFound, got: %v", err)
	}

	// 3. Update on non-existent
	badUpdate := &Workflow{
		ID:      "nonexistent-id",
		Name:    "Nonexistent WF",
		Trigger: TriggerConfig{Type: "webhook"},
		Actions: []ActionStep{{Integration: "http", ActionType: "get"}},
	}
	if _, err := store.Update(ctx, badUpdate); err != ErrWorkflowNotFound {
		t.Errorf("expected ErrWorkflowNotFound on update, got: %v", err)
	}

	// 4. Create validation failure (missing actions)
	invalidWf := &Workflow{
		Name:    "Invalid WF",
		Trigger: TriggerConfig{Type: "webhook"},
	}
	if _, err := store.Create(ctx, invalidWf); err == nil {
		t.Error("expected error creating workflow without actions, got nil")
	}

	// 5. Update validation failure (missing name)
	validWf, err := store.Create(ctx, &Workflow{
		Name:    "Valid WF",
		Trigger: TriggerConfig{Type: "webhook"},
		Actions: []ActionStep{{Integration: "http", ActionType: "post"}},
	})
	if err != nil {
		t.Fatalf("failed to create valid workflow: %v", err)
	}

	validWf.Name = ""
	if _, err := store.Update(ctx, validWf); err == nil {
		t.Error("expected validation error updating workflow with empty name, got nil")
	}

	// 6. Successful delete
	validWf.Name = "Valid WF"
	if err := store.Delete(ctx, validWf.ID); err != nil {
		t.Fatalf("failed to delete workflow: %v", err)
	}
	if _, err := store.GetByID(ctx, validWf.ID); err != ErrWorkflowNotFound {
		t.Errorf("expected ErrWorkflowNotFound after delete, got: %v", err)
	}
}
