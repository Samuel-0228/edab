package delivery

import (
	"context"
	"encoding/json"
	"errors"
	"sync"
	"testing"
	"time"

	"kerob/internal/adapters/destinations"
	"kerob/internal/core/event"
)

type mockDestAdapter struct {
	mu        sync.Mutex
	delivered []*event.Event
	failCount int
	calls     int
}

func (m *mockDestAdapter) Name() string                                           { return "mock" }
func (m *mockDestAdapter) Connect(ctx context.Context, cfg json.RawMessage) error { return nil }
func (m *mockDestAdapter) Health(ctx context.Context) error                       { return nil }
func (m *mockDestAdapter) Deliver(ctx context.Context, ev *event.Event) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.calls++
	if m.calls <= m.failCount {
		return errors.New("simulated delivery error")
	}
	m.delivered = append(m.delivered, ev)
	return nil
}

type mockEventStore struct {
	events map[string]*event.Event
}

func (m *mockEventStore) Create(ctx context.Context, e *event.Event) (*event.Event, bool, error) {
	m.events[e.ID] = e
	return e, true, nil
}
func (m *mockEventStore) GetByID(ctx context.Context, id string) (*event.Event, error) {
	if ev, ok := m.events[id]; ok {
		return ev, nil
	}
	return nil, errors.New("event not found")
}
func (m *mockEventStore) GetByExternalID(ctx context.Context, s, ext string) (*event.Event, error) {
	return nil, nil
}
func (m *mockEventStore) List(ctx context.Context, f event.ListFilter) ([]*event.Event, error) {
	return nil, nil
}

type mockDestStore struct {
	dests map[string]*destinations.Destination
}

func (m *mockDestStore) Create(ctx context.Context, d *destinations.Destination) (*destinations.Destination, error) {
	m.dests[d.ID] = d
	return d, nil
}
func (m *mockDestStore) GetByID(ctx context.Context, id string) (*destinations.Destination, error) {
	if d, ok := m.dests[id]; ok {
		return d, nil
	}
	return nil, errors.New("dest not found")
}
func (m *mockDestStore) Update(ctx context.Context, d *destinations.Destination) (*destinations.Destination, error) {
	m.dests[d.ID] = d
	return d, nil
}
func (m *mockDestStore) Delete(ctx context.Context, id string) error {
	delete(m.dests, id)
	return nil
}
func (m *mockDestStore) List(ctx context.Context) ([]*destinations.Destination, error) {
	var l []*destinations.Destination
	for _, d := range m.dests {
		l = append(l, d)
	}
	return l, nil
}

func TestNewJobDefaults(t *testing.T) {
	j1 := NewJob("ev-123", "dest-456", 5)
	if j1.ID == "" {
		t.Error("expected job ID to be generated")
	}
	if j1.Status != StatusPending {
		t.Errorf("expected StatusPending, got %s", j1.Status)
	}
	if j1.Attempts != 0 {
		t.Errorf("expected 0 attempts, got %d", j1.Attempts)
	}
	if j1.MaxAttempts != 5 {
		t.Errorf("expected 5 max attempts, got %d", j1.MaxAttempts)
	}
	if j1.CreatedAt.IsZero() {
		t.Error("expected CreatedAt timestamp to be set")
	}

	j2 := NewJob("ev-123", "dest-456", 0)
	if j2.MaxAttempts != 3 {
		t.Errorf("expected default max attempts 3, got %d", j2.MaxAttempts)
	}
}

func TestWorkerCalculateBackoff(t *testing.T) {
	cfg := WorkerConfig{
		InitialBackoff: 100 * time.Millisecond,
		MaxBackoff:     1 * time.Second,
	}
	worker := NewWorker(nil, nil, nil, nil, cfg)

	tests := []struct {
		attempt  int
		expected time.Duration
	}{
		{attempt: 0, expected: 100 * time.Millisecond},
		{attempt: 1, expected: 100 * time.Millisecond},
		{attempt: 2, expected: 200 * time.Millisecond},
		{attempt: 3, expected: 400 * time.Millisecond},
		{attempt: 4, expected: 800 * time.Millisecond},
		{attempt: 5, expected: 1000 * time.Millisecond},
		{attempt: 10, expected: 1000 * time.Millisecond},
	}

	for _, tt := range tests {
		got := worker.CalculateBackoff(tt.attempt)
		if got != tt.expected {
			t.Errorf("CalculateBackoff(%d) = %v, expected %v", tt.attempt, got, tt.expected)
		}
	}
}

func TestWorkerDefaults(t *testing.T) {
	emptyCfg := WorkerConfig{}
	worker := NewWorker(nil, nil, nil, nil, emptyCfg)

	if worker.cfg.PollInterval != 1*time.Second {
		t.Errorf("expected default PollInterval 1s, got %v", worker.cfg.PollInterval)
	}
	if worker.cfg.Concurrency != 2 {
		t.Errorf("expected default Concurrency 2, got %d", worker.cfg.Concurrency)
	}
	if worker.cfg.InitialBackoff != 2*time.Second {
		t.Errorf("expected default InitialBackoff 2s, got %v", worker.cfg.InitialBackoff)
	}
	if worker.cfg.MaxBackoff != 1*time.Minute {
		t.Errorf("expected default MaxBackoff 1m, got %v", worker.cfg.MaxBackoff)
	}
}

func TestMemoryStoreOperations(t *testing.T) {
	ctx := context.Background()
	store := NewMemoryStore()

	// 1. Create Jobs
	j1 := NewJob("ev-1", "dest-1", 3)
	j2 := NewJob("ev-2", "dest-2", 3)
	if err := store.CreateJobs(ctx, []*DeliveryJob{j1, j2}); err != nil {
		t.Fatalf("failed to create jobs: %v", err)
	}

	// 2. GetByID
	fetched, err := store.GetByID(ctx, j1.ID)
	if err != nil || fetched.EventID != "ev-1" {
		t.Fatalf("failed to fetch job: %v", err)
	}
	if _, err := store.GetByID(ctx, "nonexistent"); err == nil {
		t.Fatal("expected error on nonexistent job")
	}

	// 3. List with filters & pagination
	list, err := store.List(ctx, ListFilter{EventID: "ev-1"})
	if err != nil || len(list) != 1 {
		t.Fatalf("expected 1 job for ev-1, got %d", len(list))
	}

	listPaging, err := store.List(ctx, ListFilter{Limit: 1, Offset: 1})
	if err != nil || len(listPaging) != 1 {
		t.Fatalf("expected 1 job with paging, got %d", len(listPaging))
	}

	// 4. ClaimNextJob
	claimed, err := store.ClaimNextJob(ctx)
	if err != nil || claimed == nil {
		t.Fatalf("failed to claim job: %v", err)
	}
	if claimed.Status != StatusProcessing || claimed.Attempts != 1 {
		t.Errorf("unexpected claimed job status: %+v", claimed)
	}

	// 5. MarkDeliveredWithResult
	res := json.RawMessage(`{"message_id":12345}`)
	if err := store.MarkDeliveredWithResult(ctx, claimed.ID, res); err != nil {
		t.Fatalf("failed to mark delivered: %v", err)
	}

	// 6. Stats check
	stats, err := store.Stats(ctx)
	if err != nil {
		t.Fatalf("failed to get stats: %v", err)
	}
	if stats.Delivered != 1 || stats.Pending != 1 || stats.Total != 2 {
		t.Errorf("unexpected stats: %+v", stats)
	}

	// 7. MarkFailed and Retry
	claimed2, _ := store.ClaimNextJob(ctx)
	if err := store.MarkFailed(ctx, claimed2.ID, "connection timeout", time.Now().Add(-time.Hour)); err != nil {
		t.Fatalf("failed to mark failed: %v", err)
	}

	claimedRetry, _ := store.ClaimNextJob(ctx)
	if claimedRetry == nil || claimedRetry.Attempts != 2 {
		t.Fatalf("expected failed job to be reclaimable")
	}

	// 8. MarkDeadLetter
	if err := store.MarkDeadLetter(ctx, claimedRetry.ID, "permanent fatal failure"); err != nil {
		t.Fatalf("failed to mark dead letter: %v", err)
	}

	// 9. ResetForRetry
	resetJob, err := store.ResetForRetry(ctx, claimedRetry.ID)
	if err != nil || resetJob.Status != StatusPending || resetJob.Attempts != 0 {
		t.Fatalf("failed to reset for retry: %v", err)
	}
}

func TestWorkerEndToEndDelivery(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	deliveryStore := NewMemoryStore()
	evStore := &mockEventStore{events: make(map[string]*event.Event)}
	destStore := &mockDestStore{dests: make(map[string]*destinations.Destination)}
	destAdapter := &mockDestAdapter{failCount: 1} // Fail 1st attempt, succeed 2nd attempt

	destReg := destinations.NewRegistry()
	destReg.Register("mock_adapter", func() destinations.DestinationAdapter {
		return destAdapter
	})

	// Setup destination
	dest := &destinations.Destination{
		ID:          "dest-mock-1",
		Name:        "Mock Target",
		AdapterType: "mock_adapter",
		Enabled:     true,
	}
	_, _ = destStore.Create(ctx, dest)

	// Setup event
	ev, _ := event.New("src-1", "ext-1", "order.created", map[string]string{"id": "ord-1"}, nil)
	_, _, _ = evStore.Create(ctx, ev)

	// Setup delivery job with max 3 attempts
	job := NewJob(ev.ID, dest.ID, 3)
	_ = deliveryStore.CreateJob(ctx, job)

	worker := NewWorker(deliveryStore, evStore, destStore, destReg, WorkerConfig{
		PollInterval:   10 * time.Millisecond,
		Concurrency:    1,
		InitialBackoff: 10 * time.Millisecond,
		MaxBackoff:     50 * time.Millisecond,
	})

	go worker.Start(ctx)

	// Wait for worker to retry and succeed
	deadline := time.Now().Add(1 * time.Second)
	var delivered bool
	for time.Now().Before(deadline) {
		j, _ := deliveryStore.GetByID(ctx, job.ID)
		if j.Status == StatusDelivered {
			delivered = true
			break
		}
		time.Sleep(15 * time.Millisecond)
	}

	worker.Stop()

	if !delivered {
		t.Fatalf("expected delivery job to succeed after retry")
	}
}

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
