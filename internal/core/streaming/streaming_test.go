package streaming_test

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"kerob/internal/core/event"
	"kerob/internal/core/schema"
	"kerob/internal/core/streaming"
	"kerob/internal/core/workflow"
)

func TestPartitionStrategyOrderingKeys(t *testing.T) {
	strategy := streaming.DefaultPartitionStrategy
	numPartitions := 4

	// Events with the same ordering key MUST land on the exact same partition every time
	key1 := "tenant-alpha"
	key2 := "tenant-beta"

	part1A := strategy.AssignPartition(key1, numPartitions)
	part1B := strategy.AssignPartition(key1, numPartitions)
	if part1A != part1B {
		t.Fatalf("partition assignment is not deterministic: %d vs %d", part1A, part1B)
	}

	part2A := strategy.AssignPartition(key2, numPartitions)
	part2B := strategy.AssignPartition(key2, numPartitions)
	if part2A != part2B {
		t.Fatalf("partition assignment is not deterministic: %d vs %d", part2A, part2B)
	}

	if part1A < 0 || part1A >= int32(numPartitions) {
		t.Fatalf("partition %d out of bounds [0, %d)", part1A, numPartitions)
	}
}

func TestMemoryStreamAdapterPublishAndConsume(t *testing.T) {
	adapter := streaming.NewMemoryStreamAdapter(4)
	defer adapter.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	producer, _ := adapter.Producer()
	consumer, _ := adapter.Consumer()

	var receivedCount int32
	var mu sync.Mutex
	receivedOrder := make(map[string][]int64)

	handler := func(msgCtx context.Context, record *streaming.Record) error {
		atomic.AddInt32(&receivedCount, 1)
		mu.Lock()
		receivedOrder[record.OrderingKey] = append(receivedOrder[record.OrderingKey], record.Offset)
		mu.Unlock()
		return nil
	}

	err := consumer.Subscribe(ctx, "test-group-1", []string{"orders"}, handler)
	if err != nil {
		t.Fatalf("Subscribe failed: %v", err)
	}

	// Publish 10 messages for tenant-1 and 10 messages for tenant-2
	for i := 0; i < 10; i++ {
		ev, _ := event.New("source-1", fmt.Sprintf("ext-%d", i), "order.created", map[string]any{"i": i}, nil)
		_, err := producer.Publish(ctx, &streaming.Record{
			Topic:       "orders",
			OrderingKey: "tenant-1",
			Event:       ev,
		})
		if err != nil {
			t.Fatalf("Publish failed: %v", err)
		}
	}

	for i := 10; i < 20; i++ {
		ev, _ := event.New("source-1", fmt.Sprintf("ext-%d", i), "order.created", map[string]any{"i": i}, nil)
		_, err := producer.Publish(ctx, &streaming.Record{
			Topic:       "orders",
			OrderingKey: "tenant-2",
			Event:       ev,
		})
		if err != nil {
			t.Fatalf("Publish failed: %v", err)
		}
	}

	// Wait for consumer to process all 20 records
	deadline := time.Now().Add(3 * time.Second)
	for time.Now().Before(deadline) {
		if atomic.LoadInt32(&receivedCount) >= 20 {
			break
		}
		time.Sleep(20 * time.Millisecond)
	}

	if atomic.LoadInt32(&receivedCount) < 20 {
		t.Fatalf("expected 20 received records, got %d", receivedCount)
	}

	// Verify monotonic offsets per ordering key partition
	mu.Lock()
	tenant1Offsets := receivedOrder["tenant-1"]
	mu.Unlock()

	if len(tenant1Offsets) != 10 {
		t.Fatalf("expected 10 offsets for tenant-1, got %d", len(tenant1Offsets))
	}
	for i := 1; i < len(tenant1Offsets); i++ {
		if tenant1Offsets[i] <= tenant1Offsets[i-1] {
			t.Fatalf("offsets not strictly increasing: %v", tenant1Offsets)
		}
	}
}

func TestStreamWorkerSchemaValidationAndDLQ(t *testing.T) {
	adapter := streaming.NewMemoryStreamAdapter(2)
	defer adapter.Close()

	schemaStore := schema.NewMemoryStore()
	schemaReg := schema.NewRegistry(schemaStore)
	dlqStore := streaming.NewMemoryDeadLetterStore()
	eventStore := event.NewMemoryStore()

	// Register strict schema for "user.signup"
	_, err := schemaReg.RegisterSchema(context.Background(), &schema.Schema{
		Name: "user.signup",
		Definition: `{
			"type": "object",
			"required": ["username", "email"],
			"properties": {
				"username": { "type": "string" },
				"email": { "type": "string" }
			}
		}`,
	})
	if err != nil {
		t.Fatalf("failed to register schema: %v", err)
	}

	worker := streaming.NewStreamWorker(
		adapter,
		schemaReg,
		nil, // No workflow engine needed for schema check
		dlqStore,
		eventStore,
		streaming.WorkerConfig{
			GroupID: "test-schema-group",
			Topics:  []string{"users"},
		},
	)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	if err := worker.Start(ctx); err != nil {
		t.Fatalf("failed to start worker: %v", err)
	}

	producer, _ := adapter.Producer()

	// 1. Publish valid record
	evValid, _ := event.New("auth0", "valid-1", "user.signup", map[string]any{
		"username": "johndoe",
		"email":    "john@example.com",
	}, nil)
	_, _ = producer.Publish(ctx, &streaming.Record{
		Topic: "users",
		Event: evValid,
	})

	// 2. Publish invalid record (missing email)
	evInvalid, _ := event.New("auth0", "invalid-1", "user.signup", map[string]any{
		"username": "baduser",
	}, nil)
	_, _ = producer.Publish(ctx, &streaming.Record{
		Topic: "users",
		Event: evInvalid,
	})

	// Wait for processing
	time.Sleep(300 * time.Millisecond)

	// Valid record was persisted in event store
	storedEv, err := eventStore.GetByExternalID(ctx, "auth0", "valid-1")
	if err != nil || storedEv == nil {
		t.Fatalf("expected valid event in event store")
	}

	// Invalid record was routed to DLQ store
	dlqList, err := dlqStore.List(ctx, streaming.DLQFilter{Topic: "users"})
	if err != nil {
		t.Fatalf("failed to list DLQ: %v", err)
	}
	if len(dlqList) != 1 {
		t.Fatalf("expected 1 dead-letter event, got %d", len(dlqList))
	}
	if dlqList[0].EventID != evInvalid.ID {
		t.Fatalf("expected DLQ event to be %s, got %s", evInvalid.ID, dlqList[0].EventID)
	}
}

func TestDurableFileStreamAdapterPersistenceAndRecovery(t *testing.T) {
	tempDir, err := os.MkdirTemp("", "kerob_stream_test_*")
	if err != nil {
		t.Fatalf("failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tempDir)

	offsetStore := streaming.NewMemoryOffsetStore()

	// 1. Start adapter and write records
	adapter1, err := streaming.NewDurableStreamAdapter(streaming.DurableStreamConfig{
		DataDir:     tempDir,
		Partitions:  2,
		SyncOnWrite: true,
	}, offsetStore)
	if err != nil {
		t.Fatalf("failed to create durable adapter: %v", err)
	}

	ctx := context.Background()
	for i := 0; i < 5; i++ {
		ev, _ := event.New("sensor", fmt.Sprintf("m-%d", i), "metric.temperature", map[string]any{"val": 25 + i}, nil)
		_, err := adapter1.Publish(ctx, &streaming.Record{
			Topic:       "telemetry",
			Partition:   0,
			OrderingKey: "device-1",
			Event:       ev,
		})
		if err != nil {
			t.Fatalf("publish failed: %v", err)
		}
	}
	_ = adapter1.Close()

	// Verify log file exists on disk
	logPath := filepath.Join(tempDir, "telemetry", "partition-000.log")
	if _, err := os.Stat(logPath); err != nil {
		t.Fatalf("log file not found on disk: %v", err)
	}

	// 2. Re-open adapter on same directory (simulating restart)
	adapter2, err := streaming.NewDurableStreamAdapter(streaming.DurableStreamConfig{
		DataDir:     tempDir,
		Partitions:  2,
		SyncOnWrite: true,
	}, offsetStore)
	if err != nil {
		t.Fatalf("failed to re-open durable adapter: %v", err)
	}
	defer adapter2.Close()

	// Write 6th record and ensure offset continues from 5
	ev6, _ := event.New("sensor", "m-5", "metric.temperature", map[string]any{"val": 31}, nil)
	res, err := adapter2.Publish(ctx, &streaming.Record{
		Topic:       "telemetry",
		Partition:   0,
		OrderingKey: "device-1",
		Event:       ev6,
	})
	if err != nil {
		t.Fatalf("failed to publish after restart: %v", err)
	}
	if res.Offset != 5 {
		t.Fatalf("expected offset 5 after restart recovery, got %d", res.Offset)
	}
}

func TestReplayEngine(t *testing.T) {
	eventStore := event.NewMemoryStore()
	dlqStore := streaming.NewMemoryDeadLetterStore()

	// Insert historical events
	now := time.Now().UTC()
	ev1, _ := event.New("github", "gh-1", "github.push", map[string]any{"ref": "main"}, nil)
	ev1.CreatedAt = now.Add(-1 * time.Hour)
	_, _, _ = eventStore.Create(context.Background(), ev1)

	ev2, _ := event.New("github", "gh-2", "github.push", map[string]any{"ref": "feature"}, nil)
	ev2.CreatedAt = now.Add(-30 * time.Minute)
	_, _, _ = eventStore.Create(context.Background(), ev2)

	// Workflow engine setup
	wfStore := workflow.NewMemoryStore()
	execStore := workflow.NewMemoryExecutionStore()
	dlStore := workflow.NewMemoryDeadLetterStore()
	actionReg := workflow.NewActionRegistry()
	engine := workflow.NewEngine(wfStore, execStore, dlStore, nil, actionReg)

	// Create workflow that matches github.push
	_, err := wfStore.Create(context.Background(), &workflow.Workflow{
		Name:    "On Push Workflow",
		Enabled: true,
		Trigger: workflow.TriggerConfig{
			Type:       "github.push",
			EventTypes: []string{"github.push"},
		},
		Actions: []workflow.ActionStep{
			{ID: "s1", Name: "Action Step", Type: "action", Integration: "http", ActionType: "post"},
		},
	})
	if err != nil {
		t.Fatalf("failed to create workflow: %v", err)
	}

	replayEng := streaming.NewReplayEngine(eventStore, engine, dlqStore, nil)

	// 1. Replay by EventType
	res, err := replayEng.ReplayEvents(context.Background(), streaming.ReplayFilter{
		EventType:         "github.push",
		BypassIdempotency: true,
	})
	if err != nil {
		t.Fatalf("ReplayEvents failed: %v", err)
	}
	if res.TotalMatched != 2 {
		t.Fatalf("expected 2 matched events, got %d", res.TotalMatched)
	}
	if res.Replayed != 2 {
		t.Fatalf("expected 2 replayed events, got %d", res.Replayed)
	}

	// 2. Replay Dead-Letter Event
	dlqItem, _ := dlqStore.Store(context.Background(), &streaming.DeadLetterEvent{
		Topic:       "events",
		EventType:   "github.push",
		Payload:     json.RawMessage(`{"ref": "repaired-branch"}`),
		ErrorReason: "temporary downstream outage",
	})

	replayedEv, execs, err := replayEng.ReplayDeadLetterEvent(context.Background(), dlqItem.ID)
	if err != nil {
		t.Fatalf("ReplayDeadLetterEvent failed: %v", err)
	}
	if replayedEv == nil {
		t.Fatal("expected reconstituted event")
	}
	if len(execs) != 1 {
		t.Fatalf("expected 1 execution triggered from replayed DLQ event, got %d", len(execs))
	}

	// Verify DLQ marked resolved
	resolvedItem, _ := dlqStore.GetByID(context.Background(), dlqItem.ID)
	if !resolvedItem.Resolved {
		t.Fatal("expected DLQ record to be marked resolved")
	}
}
