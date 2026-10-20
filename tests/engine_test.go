package tests

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"kerob/internal/adapters/destinations"
	"kerob/internal/adapters/sources"
	"kerob/internal/core/delivery"
	"kerob/internal/core/event"
	"kerob/internal/core/routing"
	"kerob/internal/core/workflow"
)

func setupTestEnvironment() (
	*MemoryEventStore,
	*MemorySourceStore,
	*MemoryDestinationStore,
	*MemoryRouteStore,
	*MemoryDeliveryStore,
	*sources.Registry,
	*destinations.Registry,
	*destinations.MockDestinationAdapter,
	*workflow.Pipeline,
) {
	evStore := NewMemoryEventStore()
	srcStore := NewMemorySourceStore()
	destStore := NewMemoryDestinationStore()
	routeStore := NewMemoryRouteStore()
	jobStore := NewMemoryDeliveryStore()

	srcReg := sources.NewRegistry()
	srcReg.Register("mock", func() sources.SourceAdapter { return sources.NewMockSourceAdapter() })
	srcReg.Register("webhook", func() sources.SourceAdapter { return sources.NewWebhookSourceAdapter() })

	destAdapter := destinations.NewMockDestinationAdapter()
	destReg := destinations.NewRegistry()
	destReg.Register("mock", func() destinations.DestinationAdapter { return destAdapter })
	destReg.Register("stdout", func() destinations.DestinationAdapter { return destinations.NewStdoutDestinationAdapter() })

	routingEngine := routing.NewRoutingEngine(routeStore)
	pipe := workflow.NewPipeline(srcStore, srcReg, evStore, routingEngine, jobStore, destStore, 3)

	return evStore, srcStore, destStore, routeStore, jobStore, srcReg, destReg, destAdapter, pipe
}

// 1. Test Event Normalization
func TestEventNormalization(t *testing.T) {
	ctx := context.Background()
	mockSrc := sources.NewMockSourceAdapter()
	_ = mockSrc.Connect(ctx, json.RawMessage("{}"))

	rawJSON := []byte(`{
		"external_id": "ext-norm-001",
		"type": "order.completed",
		"payload": {"amount": 100, "currency": "USD"},
		"metadata": {"origin": "web"}
	}`)

	ev, err := mockSrc.Normalize(ctx, "src-1", rawJSON)
	if err != nil {
		t.Fatalf("unexpected error normalizing: %v", err)
	}

	if ev.SourceID != "src-1" {
		t.Errorf("expected source_id src-1, got %s", ev.SourceID)
	}
	if ev.ExternalEventID != "ext-norm-001" {
		t.Errorf("expected external_id ext-norm-001, got %s", ev.ExternalEventID)
	}
	if ev.EventType != "order.completed" {
		t.Errorf("expected event_type order.completed, got %s", ev.EventType)
	}
	if string(ev.Payload) != `{"amount": 100, "currency": "USD"}` {
		t.Errorf("unexpected payload: %s", string(ev.Payload))
	}

	// Test webhook source normalization
	webhookSrc := sources.NewWebhookSourceAdapter()
	_ = webhookSrc.Connect(ctx, json.RawMessage("{}"))

	hookJSON := []byte(`{"id": "hook-123", "event": "commit", "author": "dev"}`)
	hookEv, err := webhookSrc.Normalize(ctx, "src-hook", hookJSON)
	if err != nil {
		t.Fatalf("webhook normalization failed: %v", err)
	}
	if hookEv.ExternalEventID != "hook-123" {
		t.Errorf("expected external_id hook-123, got %s", hookEv.ExternalEventID)
	}
	if hookEv.EventType != "commit" {
		t.Errorf("expected event commit, got %s", hookEv.EventType)
	}
}

// 2. Test Event Idempotency
func TestEventIdempotency(t *testing.T) {
	ctx := context.Background()
	evStore := NewMemoryEventStore()

	ev1, err := event.New("src-idem", "ext-100", "test.event", map[string]string{"foo": "bar"}, nil)
	if err != nil {
		t.Fatalf("failed to create event: %v", err)
	}

	created1, inserted1, err := evStore.Create(ctx, ev1)
	if err != nil || !inserted1 {
		t.Fatalf("first insert should succeed: inserted=%v, err=%v", inserted1, err)
	}

	// Ingest duplicate external event
	ev2, _ := event.New("src-idem", "ext-100", "test.event", map[string]string{"foo": "bar2"}, nil)
	created2, inserted2, err := evStore.Create(ctx, ev2)
	if err != nil {
		t.Fatalf("second insert must not error: %v", err)
	}
	if inserted2 {
		t.Errorf("duplicate event should not be marked as inserted")
	}
	if created2.ID != created1.ID {
		t.Errorf("expected existing event ID %s, got %s", created1.ID, created2.ID)
	}

	// Verify only 1 event exists
	list, _ := evStore.List(ctx, event.ListFilter{SourceID: "src-idem"})
	if len(list) != 1 {
		t.Errorf("expected 1 event, got %d", len(list))
	}
}

// 3. Test Route Resolution
func TestRouteResolution(t *testing.T) {
	ctx := context.Background()
	routeStore := NewMemoryRouteStore()
	routingEngine := routing.NewRoutingEngine(routeStore)

	// Route 1: matches source-A, all event types
	_, _ = routeStore.Create(ctx, &routing.Route{
		SourceID:      "source-A",
		DestinationID: "dest-1",
		Enabled:       true,
		Filters:       json.RawMessage("{}"),
	})

	// Route 2: matches source-A, only "alert" event type
	_, _ = routeStore.Create(ctx, &routing.Route{
		SourceID:      "source-A",
		DestinationID: "dest-2",
		Enabled:       true,
		Filters:       json.RawMessage(`{"event_types": ["alert"]}`),
	})

	// Route 3: disabled route
	_, _ = routeStore.Create(ctx, &routing.Route{
		SourceID:      "source-A",
		DestinationID: "dest-3",
		Enabled:       false,
	})

	// Test with general message
	evMsg, _ := event.New("source-A", "m-1", "chat.message", nil, nil)
	matched, err := routingEngine.ResolveRoutes(ctx, evMsg)
	if err != nil {
		t.Fatalf("route resolution error: %v", err)
	}
	if len(matched) != 1 || matched[0].DestinationID != "dest-1" {
		t.Errorf("expected 1 matched route (dest-1), got %d", len(matched))
	}

	// Test with alert event
	evAlert, _ := event.New("source-A", "m-2", "alert", nil, nil)
	matchedAlert, err := routingEngine.ResolveRoutes(ctx, evAlert)
	if err != nil {
		t.Fatalf("route resolution error: %v", err)
	}
	if len(matchedAlert) != 2 {
		t.Errorf("expected 2 matched routes for alert event, got %d", len(matchedAlert))
	}
}

// 4. Test Delivery Creation
func TestDeliveryCreation(t *testing.T) {
	ctx := context.Background()
	_, srcStore, destStore, routeStore, jobStore, _, _, _, pipe := setupTestEnvironment()

	_, _ = srcStore.Create(ctx, &sources.Source{ID: "s-1", Name: "Src1", AdapterType: "mock", Enabled: true})
	_, _ = destStore.Create(ctx, &destinations.Destination{ID: "d-1", Name: "Dest1", AdapterType: "mock", Enabled: true})
	_, _ = routeStore.Create(ctx, &routing.Route{SourceID: "s-1", DestinationID: "d-1", Enabled: true})

	ev, err := event.New("s-1", "ext-dc-1", "type.a", map[string]string{"msg": "hi"}, nil)
	if err != nil {
		t.Fatalf("failed creating event: %v", err)
	}

	persisted, err := pipe.ProcessEvent(ctx, ev)
	if err != nil {
		t.Fatalf("failed processing event: %v", err)
	}

	jobs, err := jobStore.List(ctx, delivery.ListFilter{EventID: persisted.ID})
	if err != nil {
		t.Fatalf("failed listing delivery jobs: %v", err)
	}
	if len(jobs) != 1 {
		t.Fatalf("expected 1 delivery job, got %d", len(jobs))
	}
	if jobs[0].Status != delivery.StatusPending {
		t.Errorf("expected job status pending, got %s", jobs[0].Status)
	}
	if jobs[0].DestinationID != "d-1" {
		t.Errorf("expected destination d-1, got %s", jobs[0].DestinationID)
	}
}

// 5. Test Successful Delivery
func TestSuccessfulDelivery(t *testing.T) {
	ctx := context.Background()
	evStore, srcStore, destStore, routeStore, jobStore, _, destReg, mockDest, pipe := setupTestEnvironment()

	_, _ = srcStore.Create(ctx, &sources.Source{ID: "s-1", Name: "Src1", AdapterType: "mock", Enabled: true})
	_, _ = destStore.Create(ctx, &destinations.Destination{ID: "d-1", Name: "Dest1", AdapterType: "mock", Enabled: true})
	_, _ = routeStore.Create(ctx, &routing.Route{SourceID: "s-1", DestinationID: "d-1", Enabled: true})

	ev, _ := event.New("s-1", "ext-succ-1", "type.a", "payload data", nil)
	persisted, err := pipe.ProcessEvent(ctx, ev)
	if err != nil {
		t.Fatalf("pipe failed: %v", err)
	}

	worker := delivery.NewWorker(jobStore, evStore, destStore, destReg, delivery.WorkerConfig{})
	claimed, err := worker.ProcessNextJob(ctx)
	if err != nil || !claimed {
		t.Fatalf("expected job claimed and executed successfully: claimed=%v, err=%v", claimed, err)
	}

	job, _ := jobStore.GetByID(ctx, (func() string {
		jobs, _ := jobStore.List(ctx, delivery.ListFilter{EventID: persisted.ID})
		return jobs[0].ID
	})())

	if job.Status != delivery.StatusDelivered {
		t.Errorf("expected job status delivered, got %s", job.Status)
	}
	if job.Attempts != 1 {
		t.Errorf("expected attempts 1, got %d", job.Attempts)
	}

	delivered := mockDest.GetDeliveredEvents()
	if len(delivered) != 1 || delivered[0].ID != persisted.ID {
		t.Errorf("mock destination did not receive event correctly")
	}
}

// 6. Test Failed Delivery
func TestFailedDelivery(t *testing.T) {
	ctx := context.Background()
	evStore, srcStore, destStore, routeStore, jobStore, _, destReg, mockDest, pipe := setupTestEnvironment()

	mockDest.SetFailBehavior(true, 0, "network timeout")

	_, _ = srcStore.Create(ctx, &sources.Source{ID: "s-fail", Name: "SrcFail", AdapterType: "mock", Enabled: true})
	_, _ = destStore.Create(ctx, &destinations.Destination{ID: "d-fail", Name: "DestFail", AdapterType: "mock", Enabled: true})
	_, _ = routeStore.Create(ctx, &routing.Route{SourceID: "s-fail", DestinationID: "d-fail", Enabled: true})

	ev, _ := event.New("s-fail", "ext-fail-1", "type.err", nil, nil)
	persisted, _ := pipe.ProcessEvent(ctx, ev)

	worker := delivery.NewWorker(jobStore, evStore, destStore, destReg, delivery.WorkerConfig{
		InitialBackoff: 50 * time.Millisecond,
	})

	claimed, err := worker.ProcessNextJob(ctx)
	if err != nil || !claimed {
		t.Fatalf("processNextJob error: claimed=%v, err=%v", claimed, err)
	}

	jobs, _ := jobStore.List(ctx, delivery.ListFilter{EventID: persisted.ID})
	job := jobs[0]

	if job.Status != delivery.StatusFailed {
		t.Errorf("expected status failed, got %s", job.Status)
	}
	if job.Attempts != 1 {
		t.Errorf("expected attempts 1, got %d", job.Attempts)
	}
	if job.LastError == nil || *job.LastError != "network timeout" {
		t.Errorf("expected last_error 'network timeout', got %v", job.LastError)
	}
	if !job.NextAttemptAt.After(time.Now().UTC().Add(-time.Second)) {
		t.Errorf("expected next_attempt_at in future")
	}
}

// 7. Test Retry Behavior & Dead Letter
func TestRetryBehaviorAndDeadLetter(t *testing.T) {
	ctx := context.Background()
	evStore, srcStore, destStore, routeStore, jobStore, _, destReg, mockDest, pipe := setupTestEnvironment()

	mockDest.SetFailBehavior(true, 0, "destination down")

	_, _ = srcStore.Create(ctx, &sources.Source{ID: "s-r", Name: "SrcR", AdapterType: "mock", Enabled: true})
	_, _ = destStore.Create(ctx, &destinations.Destination{ID: "d-r", Name: "DestR", AdapterType: "mock", Enabled: true})
	_, _ = routeStore.Create(ctx, &routing.Route{SourceID: "s-r", DestinationID: "d-r", Enabled: true})

	ev, _ := event.New("s-r", "ext-r-1", "type.retry", nil, nil)
	persisted, _ := pipe.ProcessEvent(ctx, ev)

	worker := delivery.NewWorker(jobStore, evStore, destStore, destReg, delivery.WorkerConfig{
		InitialBackoff: 1 * time.Millisecond,
	})

	jobs, _ := jobStore.List(ctx, delivery.ListFilter{EventID: persisted.ID})
	jobID := jobs[0].ID

	// Process Attempt 1
	worker.ProcessNextJob(ctx)
	j1, _ := jobStore.GetByID(ctx, jobID)
	if j1.Status != delivery.StatusFailed || j1.Attempts != 1 {
		t.Fatalf("attempt 1 failed: status=%s attempts=%d", j1.Status, j1.Attempts)
	}

	// Advance time for attempt 2
	j1.NextAttemptAt = time.Now().UTC().Add(-time.Second)
	// Process Attempt 2
	worker.ProcessNextJob(ctx)
	j2, _ := jobStore.GetByID(ctx, jobID)
	if j2.Status != delivery.StatusFailed || j2.Attempts != 2 {
		t.Fatalf("attempt 2 failed: status=%s attempts=%d", j2.Status, j2.Attempts)
	}

	// Advance time for attempt 3 (max_attempts = 3)
	j2.NextAttemptAt = time.Now().UTC().Add(-time.Second)
	// Process Attempt 3 -> should become dead_letter!
	worker.ProcessNextJob(ctx)
	j3, _ := jobStore.GetByID(ctx, jobID)
	if j3.Status != delivery.StatusDeadLetter {
		t.Fatalf("attempt 3 should transition to dead_letter, got status: %s", j3.Status)
	}
	if j3.Attempts != 3 {
		t.Errorf("expected 3 attempts, got %d", j3.Attempts)
	}

	// Test manual reset for retry
	resetJob, err := jobStore.ResetForRetry(ctx, jobID)
	if err != nil {
		t.Fatalf("reset error: %v", err)
	}
	if resetJob.Status != delivery.StatusPending || resetJob.Attempts != 0 {
		t.Errorf("expected reset to pending with 0 attempts, got status=%s attempts=%d", resetJob.Status, resetJob.Attempts)
	}
}

// 8. Test Multiple Destinations
func TestMultipleDestinations(t *testing.T) {
	ctx := context.Background()
	_, srcStore, destStore, routeStore, jobStore, _, _, _, pipe := setupTestEnvironment()

	_, _ = srcStore.Create(ctx, &sources.Source{ID: "src-multi", Name: "SrcMulti", AdapterType: "mock", Enabled: true})
	_, _ = destStore.Create(ctx, &destinations.Destination{ID: "dest-a", Name: "Target A", AdapterType: "mock", Enabled: true})
	_, _ = destStore.Create(ctx, &destinations.Destination{ID: "dest-b", Name: "Target B", AdapterType: "mock", Enabled: true})
	_, _ = destStore.Create(ctx, &destinations.Destination{ID: "dest-c", Name: "Target C", AdapterType: "mock", Enabled: true})

	_, _ = routeStore.Create(ctx, &routing.Route{SourceID: "src-multi", DestinationID: "dest-a", Enabled: true})
	_, _ = routeStore.Create(ctx, &routing.Route{SourceID: "src-multi", DestinationID: "dest-b", Enabled: true})
	_, _ = routeStore.Create(ctx, &routing.Route{SourceID: "src-multi", DestinationID: "dest-c", Enabled: true})

	ev, _ := event.New("src-multi", "ext-multi-1", "news.update", "hello all", nil)
	persisted, err := pipe.ProcessEvent(ctx, ev)
	if err != nil {
		t.Fatalf("process error: %v", err)
	}

	jobs, err := jobStore.List(ctx, delivery.ListFilter{EventID: persisted.ID})
	if err != nil {
		t.Fatalf("list jobs error: %v", err)
	}
	if len(jobs) != 3 {
		t.Fatalf("expected 3 delivery jobs for 3 destinations, got %d", len(jobs))
	}

	destMap := make(map[string]bool)
	for _, j := range jobs {
		destMap[j.DestinationID] = true
	}
	if !destMap["dest-a"] || !destMap["dest-b"] || !destMap["dest-c"] {
		t.Errorf("missing destination in jobs: %+v", destMap)
	}
}

// 9. Test Failure of One Destination Does Not Affect Another
func TestDestinationIsolation(t *testing.T) {
	ctx := context.Background()
	evStore, srcStore, destStore, routeStore, jobStore, _, destReg, _, pipe := setupTestEnvironment()

	// Register two separate destination adapters: one healthy, one faulty
	healthyMock := destinations.NewMockDestinationAdapter()
	faultyMock := destinations.NewMockDestinationAdapter()
	faultyMock.SetFailBehavior(true, 0, "connection refused")

	destReg.Register("healthy-type", func() destinations.DestinationAdapter { return healthyMock })
	destReg.Register("faulty-type", func() destinations.DestinationAdapter { return faultyMock })

	_, _ = srcStore.Create(ctx, &sources.Source{ID: "src-iso", Name: "SrcIso", AdapterType: "mock", Enabled: true})
	_, _ = destStore.Create(ctx, &destinations.Destination{ID: "dest-ok", Name: "Healthy", AdapterType: "healthy-type", Enabled: true})
	_, _ = destStore.Create(ctx, &destinations.Destination{ID: "dest-bad", Name: "Faulty", AdapterType: "faulty-type", Enabled: true})

	_, _ = routeStore.Create(ctx, &routing.Route{SourceID: "src-iso", DestinationID: "dest-ok", Enabled: true})
	_, _ = routeStore.Create(ctx, &routing.Route{SourceID: "src-iso", DestinationID: "dest-bad", Enabled: true})

	ev, _ := event.New("src-iso", "ext-iso-1", "broadcast", "payload", nil)
	persisted, _ := pipe.ProcessEvent(ctx, ev)

	worker := delivery.NewWorker(jobStore, evStore, destStore, destReg, delivery.WorkerConfig{
		InitialBackoff: 10 * time.Millisecond,
	})

	// Process first job
	worker.ProcessNextJob(ctx)
	// Process second job
	worker.ProcessNextJob(ctx)

	// Check jobs status
	jobs, _ := jobStore.List(ctx, delivery.ListFilter{EventID: persisted.ID})
	var okJob, badJob *delivery.DeliveryJob
	for _, j := range jobs {
		if j.DestinationID == "dest-ok" {
			okJob = j
		} else if j.DestinationID == "dest-bad" {
			badJob = j
		}
	}

	if okJob == nil || badJob == nil {
		t.Fatalf("expected both jobs to exist")
	}

	if okJob.Status != delivery.StatusDelivered {
		t.Errorf("expected healthy destination job to be delivered, got %s", okJob.Status)
	}
	if badJob.Status != delivery.StatusFailed {
		t.Errorf("expected faulty destination job to be failed, got %s", badJob.Status)
	}
	if len(healthyMock.GetDeliveredEvents()) != 1 {
		t.Errorf("healthy adapter should have received 1 event")
	}
	if len(faultyMock.GetDeliveredEvents()) != 0 {
		t.Errorf("faulty adapter should have 0 delivered events")
	}
}

// 10. Test Duplicate Events Do Not Create Duplicate Deliveries
func TestDuplicateEventsDoNotCreateDuplicateDeliveries(t *testing.T) {
	ctx := context.Background()
	_, srcStore, destStore, routeStore, jobStore, _, _, _, pipe := setupTestEnvironment()

	_, _ = srcStore.Create(ctx, &sources.Source{ID: "src-dup", Name: "SrcDup", AdapterType: "mock", Enabled: true})
	_, _ = destStore.Create(ctx, &destinations.Destination{ID: "dest-dup", Name: "DestDup", AdapterType: "mock", Enabled: true})
	_, _ = routeStore.Create(ctx, &routing.Route{SourceID: "src-dup", DestinationID: "dest-dup", Enabled: true})

	// Send external event first time
	rawPayload := []byte(`{"external_id": "unique-msg-999", "type": "chat.message", "payload": "hello"}`)
	ev1, err := pipe.IngestRaw(ctx, "src-dup", rawPayload)
	if err != nil {
		t.Fatalf("first ingest failed: %v", err)
	}

	jobs1, _ := jobStore.List(ctx, delivery.ListFilter{EventID: ev1.ID})
	if len(jobs1) != 1 {
		t.Fatalf("expected 1 job after first ingest, got %d", len(jobs1))
	}

	// Send duplicate external event second time
	ev2, err := pipe.IngestRaw(ctx, "src-dup", rawPayload)
	if err != nil {
		t.Fatalf("second ingest failed: %v", err)
	}

	if ev2.ID != ev1.ID {
		t.Errorf("expected returned event to be existing event ID %s, got %s", ev1.ID, ev2.ID)
	}

	jobs2, _ := jobStore.List(ctx, delivery.ListFilter{EventID: ev1.ID})
	if len(jobs2) != 1 {
		t.Errorf("expected still only 1 delivery job, got %d", len(jobs2))
	}
}
