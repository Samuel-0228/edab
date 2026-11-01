package delivery

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"kerob/internal/adapters/destinations"
	"kerob/internal/core/event"
)

func TestWorkerDeadLetterAndErrors(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	deliveryStore := NewMemoryStore()
	evStore := &mockEventStore{events: make(map[string]*event.Event)}
	destStore := &mockDestStore{dests: make(map[string]*destinations.Destination)}
	destAdapter := &mockDestAdapter{failCount: 10} // Always fail

	destReg := destinations.NewRegistry()
	destReg.Register("mock_adapter", func() destinations.DestinationAdapter {
		return destAdapter
	})

	dest := &destinations.Destination{
		ID:          "dest-dead",
		Name:        "Failing Target",
		AdapterType: "mock_adapter",
		Enabled:     true,
	}
	_, _ = destStore.Create(ctx, dest)

	ev, _ := event.New("src-1", "ext-dead", "alert", map[string]string{"msg": "fail"}, nil)
	_, _, _ = evStore.Create(ctx, ev)

	// Max 1 attempt so it goes straight to dead-letter on failure
	job := NewJob(ev.ID, dest.ID, 1)
	_ = deliveryStore.CreateJob(ctx, job)

	worker := NewWorker(deliveryStore, evStore, destStore, destReg, WorkerConfig{
		PollInterval: 10 * time.Millisecond,
		Concurrency:  1,
	})

	go worker.Start(ctx)

	deadline := time.Now().Add(1 * time.Second)
	var deadLettered bool
	for time.Now().Before(deadline) {
		j, _ := deliveryStore.GetByID(ctx, job.ID)
		if j.Status == StatusDeadLetter {
			deadLettered = true
			break
		}
		time.Sleep(15 * time.Millisecond)
	}

	worker.Stop()

	if !deadLettered {
		t.Fatalf("expected job to be moved to dead letter")
	}
}

func TestCalculateBackoffAndJobDefaults(t *testing.T) {
	// NewJob default
	j1 := NewJob("ev1", "dest1", 0)
	if j1.MaxAttempts != 3 {
		t.Errorf("expected default maxAttempts 3, got %d", j1.MaxAttempts)
	}

	worker := NewWorker(nil, nil, nil, nil, WorkerConfig{
		InitialBackoff: 2 * time.Second,
		MaxBackoff:     10 * time.Second,
	})

	if b := worker.CalculateBackoff(0); b != 2*time.Second {
		t.Errorf("expected 2s for attempt 0, got %v", b)
	}
	if b := worker.CalculateBackoff(1); b != 2*time.Second {
		t.Errorf("expected 2s for attempt 1, got %v", b)
	}
	if b := worker.CalculateBackoff(2); b != 4*time.Second {
		t.Errorf("expected 4s for attempt 2, got %v", b)
	}
	if b := worker.CalculateBackoff(3); b != 8*time.Second {
		t.Errorf("expected 8s for attempt 3, got %v", b)
	}
	if b := worker.CalculateBackoff(10); b != 10*time.Second {
		t.Errorf("expected capped 10s for attempt 10, got %v", b)
	}
}

func TestMemoryStoreExtended(t *testing.T) {
	ctx := context.Background()
	store := NewMemoryStore()

	// 1. CreateJobs batch
	j1 := NewJob("e1", "d1", 3)
	j2 := NewJob("e2", "d2", 3)
	if err := store.CreateJobs(ctx, []*DeliveryJob{j1, j2}); err != nil {
		t.Fatalf("CreateJobs failed: %v", err)
	}

	// 2. Stats
	stats, err := store.Stats(ctx)
	if err != nil || stats.Total != 2 || stats.Pending != 2 {
		t.Fatalf("unexpected stats: %+v", stats)
	}

	// 3. MarkDeliveredWithResult
	resPayload := json.RawMessage(`{"status":"ok"}`)
	if err := store.MarkDeliveredWithResult(ctx, j1.ID, resPayload); err != nil {
		t.Fatalf("MarkDeliveredWithResult failed: %v", err)
	}
	fetched1, _ := store.GetByID(ctx, j1.ID)
	if fetched1.Status != StatusDelivered || string(fetched1.Result) != `{"status":"ok"}` {
		t.Fatalf("unexpected delivered job: %+v", fetched1)
	}

	// 4. MarkFailed
	if err := store.MarkFailed(ctx, j2.ID, "temporary err", time.Now().Add(5*time.Minute)); err != nil {
		t.Fatalf("MarkFailed failed: %v", err)
	}
	fetched2, _ := store.GetByID(ctx, j2.ID)
	if fetched2.Status != StatusFailed || fetched2.LastError == nil || *fetched2.LastError != "temporary err" {
		t.Fatalf("unexpected failed job: %+v", fetched2)
	}

	// 5. MarkDeadLetter
	if err := store.MarkDeadLetter(ctx, j2.ID, "terminal err"); err != nil {
		t.Fatalf("MarkDeadLetter failed: %v", err)
	}

	// 6. ResetForRetry
	retried, err := store.ResetForRetry(ctx, j2.ID)
	if err != nil || retried.Status != StatusPending || retried.Attempts != 0 {
		t.Fatalf("ResetForRetry failed: %+v (err: %v)", retried, err)
	}

	// 7. Nonexistent error cases
	if _, err := store.GetByID(ctx, "nonexistent"); err == nil {
		t.Fatal("expected error on nonexistent GetByID")
	}
	if err := store.MarkDeliveredWithResult(ctx, "nonexistent", nil); err == nil {
		t.Fatal("expected error on nonexistent MarkDeliveredWithResult")
	}
	if err := store.MarkFailed(ctx, "nonexistent", "err", time.Now()); err == nil {
		t.Fatal("expected error on nonexistent MarkFailed")
	}
	if err := store.MarkDeadLetter(ctx, "nonexistent", "err"); err == nil {
		t.Fatal("expected error on nonexistent MarkDeadLetter")
	}
	if _, err := store.ResetForRetry(ctx, "nonexistent"); err == nil {
		t.Fatal("expected error on nonexistent ResetForRetry")
	}

	// 8. List with filters
	listAll, err := store.List(ctx, ListFilter{})
	if err != nil || len(listAll) != 2 {
		t.Fatalf("expected 2 jobs, got %d", len(listAll))
	}
	listD1, _ := store.List(ctx, ListFilter{DestinationID: "d1"})
	if len(listD1) != 1 || listD1[0].DestinationID != "d1" {
		t.Fatalf("expected 1 job for d1, got %d", len(listD1))
	}
	listStatus, _ := store.List(ctx, ListFilter{Status: StatusDelivered})
	if len(listStatus) != 1 || listStatus[0].Status != StatusDelivered {
		t.Fatalf("expected 1 delivered job, got %d", len(listStatus))
	}
	listOffset, _ := store.List(ctx, ListFilter{Offset: 1, Limit: 1})
	if len(listOffset) != 1 {
		t.Fatalf("expected 1 job with offset 1 limit 1, got %d", len(listOffset))
	}
	listOutRange, _ := store.List(ctx, ListFilter{Offset: 10})
	if len(listOutRange) != 0 {
		t.Fatalf("expected 0 jobs with out-of-range offset, got %d", len(listOutRange))
	}
}

func TestWorkerProcessingFailures(t *testing.T) {
	ctx := context.Background()
	deliveryStore := NewMemoryStore()
	evStore := &mockEventStore{events: make(map[string]*event.Event)}
	destStore := &mockDestStore{dests: make(map[string]*destinations.Destination)}
	destReg := destinations.NewRegistry()

	worker := NewWorker(deliveryStore, evStore, destStore, destReg, WorkerConfig{
		PollInterval: 10 * time.Millisecond,
		Concurrency:  1,
	})

	// 1. ProcessNextJob with no jobs available
	claimed, err := worker.ProcessNextJob(ctx)
	if err != nil || claimed {
		t.Fatalf("expected false, nil for empty store, got claimed=%v, err=%v", claimed, err)
	}

	// 2. Event not found failure
	jMissingEv := NewJob("missing-event-id", "dest-1", 3)
	_ = deliveryStore.CreateJob(ctx, jMissingEv)
	claimed, err = worker.ProcessNextJob(ctx)
	if !claimed || err == nil {
		t.Fatalf("expected claimed=true with error for missing event, got %v, %v", claimed, err)
	}

	// 3. Destination not found failure
	ev, _ := event.New("src-1", "ext-1", "test", map[string]string{"foo": "bar"}, nil)
	_, _, _ = evStore.Create(ctx, ev)
	jMissingDest := NewJob(ev.ID, "missing-dest-id", 3)
	_ = deliveryStore.CreateJob(ctx, jMissingDest)
	claimed, err = worker.ProcessNextJob(ctx)
	if !claimed || err == nil {
		t.Fatalf("expected claimed=true with error for missing dest, got %v, %v", claimed, err)
	}

	// 4. Destination disabled
	disabledDest := &destinations.Destination{
		ID:          "dest-disabled",
		Name:        "Disabled",
		AdapterType: "mock",
		Enabled:     false,
	}
	_, _ = destStore.Create(ctx, disabledDest)
	jDisabled := NewJob(ev.ID, disabledDest.ID, 3)
	_ = deliveryStore.CreateJob(ctx, jDisabled)
	claimed, err = worker.ProcessNextJob(ctx)
	if !claimed || err != nil {
		t.Fatalf("expected claimed=true with nil error for disabled dest, got %v, %v", claimed, err)
	}

	// 5. Unregistered adapter
	unregDest := &destinations.Destination{
		ID:          "dest-unreg",
		Name:        "Unregistered",
		AdapterType: "nonexistent_type",
		Enabled:     true,
	}
	_, _ = destStore.Create(ctx, unregDest)
	jUnreg := NewJob(ev.ID, unregDest.ID, 3)
	_ = deliveryStore.CreateJob(ctx, jUnreg)
	claimed, err = worker.ProcessNextJob(ctx)
	if !claimed || err == nil {
		t.Fatalf("expected error for unregistered adapter, got %v, %v", claimed, err)
	}
}
