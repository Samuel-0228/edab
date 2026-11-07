package streaming

import (
	"context"
	"testing"
)

func TestMemoryOffsetAndDeadLetterStores(t *testing.T) {
	ctx := context.Background()

	// MemoryOffsetStore
	offStore := NewMemoryOffsetStore()
	if err := offStore.CommitOffset(ctx, ConsumerOffset{
		GroupID:   "grp-1",
		Topic:     "events",
		Partition: 0,
		Offset:    42,
	}); err != nil {
		t.Fatalf("CommitOffset failed: %v", err)
	}

	off, err := offStore.GetOffset(ctx, "grp-1", "events", 0)
	if err != nil || off != 42 {
		t.Fatalf("GetOffset failed: %v, off=%d", err, off)
	}

	off0, err := offStore.GetOffset(ctx, "grp-unknown", "events", 0)
	if err != nil || off0 != 0 {
		t.Fatalf("GetOffset unknown failed: %v, off=%d", err, off0)
	}

	list, err := offStore.ListOffsets(ctx, "grp-1")
	if err != nil || len(list) != 1 {
		t.Fatalf("ListOffsets failed: %v", err)
	}

	// MemoryDeadLetterStore
	dlqStore := NewMemoryDeadLetterStore()
	dlq, err := dlqStore.Store(ctx, &DeadLetterEvent{
		TenantID:    "tenant-a",
		Topic:       "events",
		ErrorReason: "validation failed",
	})
	if err != nil {
		t.Fatalf("Store DLQ failed: %v", err)
	}

	gotDLQ, err := dlqStore.GetByID(ctx, dlq.ID)
	if err != nil || gotDLQ.ID != dlq.ID {
		t.Fatalf("GetByID DLQ failed: %v", err)
	}

	if _, err := dlqStore.GetByID(ctx, "non-existent"); err != ErrDLQNotFound {
		t.Errorf("expected ErrDLQNotFound")
	}

	dlqList, err := dlqStore.List(ctx, DLQFilter{TenantID: "tenant-a", Topic: "events"})
	if err != nil || len(dlqList) != 1 {
		t.Fatalf("List DLQ failed: %v", err)
	}

	if err := dlqStore.MarkResolved(ctx, dlq.ID); err != nil {
		t.Fatalf("MarkResolved failed: %v", err)
	}
	if err := dlqStore.MarkResolved(ctx, "non-existent"); err != ErrDLQNotFound {
		t.Errorf("expected ErrDLQNotFound on MarkResolved")
	}

	if err := dlqStore.Delete(ctx, dlq.ID); err != nil {
		t.Fatalf("Delete DLQ failed: %v", err)
	}
	if err := dlqStore.Delete(ctx, "non-existent"); err != ErrDLQNotFound {
		t.Errorf("expected ErrDLQNotFound on Delete")
	}
}

func TestStreamingAdapterAccessors(t *testing.T) {
	ctx := context.Background()
	mem := NewMemoryStreamAdapter(2)
	if mem.Name() != "memory" {
		t.Errorf("unexpected name: %s", mem.Name())
	}

	// Publish some records
	_, _ = mem.Publish(ctx, &Record{Topic: "test.topic", Partition: 0, OrderingKey: "k1"})
	_, _ = mem.Publish(ctx, &Record{Topic: "test.topic", Partition: 0, OrderingKey: "k2"})

	records := mem.GetRecords("test.topic", 0, 0, 10)
	if len(records) != 2 {
		t.Errorf("expected 2 records, got %d", len(records))
	}

	_ = mem.CommitOffset(ctx, "grp-1", "test.topic", 0, 1)
	off, err := mem.GetCommittedOffset(ctx, "grp-1", "test.topic", 0)
	if err != nil || off != 1 {
		t.Errorf("GetCommittedOffset failed: %v, off=%d", err, off)
	}

	if err := mem.Seek(ctx, "test.topic", 0, 0); err != nil {
		t.Errorf("Seek failed: %v", err)
	}

	// DurableStreamAdapter accessors
	dur, err := NewDurableStreamAdapter(DurableStreamConfig{
		DataDir:    t.TempDir(),
		Partitions: 2,
	}, NewMemoryOffsetStore())
	if err != nil {
		t.Fatalf("NewDurableStreamAdapter failed: %v", err)
	}
	defer func() { _ = dur.Close() }()

	if dur.Name() != "durable-wal" {
		t.Errorf("unexpected name: %s", dur.Name())
	}
	p, err := dur.Producer()
	if err != nil || p == nil {
		t.Error("expected non-nil producer")
	}
	c, err := dur.Consumer()
	if err != nil || c == nil {
		t.Error("expected non-nil consumer")
	}
}
