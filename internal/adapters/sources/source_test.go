package sources

import (
	"context"
	"errors"
	"testing"
)

func TestSourceRegistry(t *testing.T) {
	reg := NewRegistry()

	// Initially empty
	if len(reg.RegisteredTypes()) != 0 {
		t.Fatalf("expected empty registry, got %v", reg.RegisteredTypes())
	}

	// Register factory
	reg.Register("mock", func() SourceAdapter {
		return NewMockSourceAdapter()
	})

	types := reg.RegisteredTypes()
	if len(types) != 1 || types[0] != "mock" {
		t.Fatalf("expected [mock], got %v", types)
	}

	// Create registered
	adapter, err := reg.Create("mock")
	if err != nil {
		t.Fatalf("unexpected error creating registered adapter: %v", err)
	}
	if adapter.Name() != "mock" {
		t.Errorf("expected adapter name mock, got %s", adapter.Name())
	}

	// Create unregistered
	_, err = reg.Create("unknown")
	if err == nil || !errors.Is(err, ErrAdapterNotFound) {
		t.Fatalf("expected ErrAdapterNotFound, got %v", err)
	}
}

func TestMockSourceAdapterLifecycle(t *testing.T) {
	ctx := context.Background()
	adapter := NewMockSourceAdapter()

	// Health before connect should fail
	if err := adapter.Health(ctx); err == nil {
		t.Error("expected error checking health before connect")
	}

	// Connect
	if err := adapter.Connect(ctx, []byte(`{"mode":"test"}`)); err != nil {
		t.Fatalf("unexpected error on connect: %v", err)
	}

	// Health after connect should succeed
	if err := adapter.Health(ctx); err != nil {
		t.Errorf("unexpected health error: %v", err)
	}

	// Polling empty queue
	events, err := adapter.Poll(ctx)
	if err != nil || len(events) != 0 {
		t.Errorf("expected empty poll result, got %v (err: %v)", events, err)
	}

	// Queue and poll
	adapter.QueueRawEvent(RawEvent{
		ExternalID: "ext-1",
		Type:       "test.event",
		Data:       []byte(`{"hello":"world"}`),
	})
	events, err = adapter.Poll(ctx)
	if err != nil || len(events) != 1 {
		t.Fatalf("expected 1 polled event, got %d", len(events))
	}
	if events[0].ExternalID != "ext-1" {
		t.Errorf("expected external ID ext-1, got %s", events[0].ExternalID)
	}
}

func TestMockSourceAdapterNormalization(t *testing.T) {
	ctx := context.Background()
	adapter := NewMockSourceAdapter()

	// Empty data returns error
	_, err := adapter.Normalize(ctx, "src-1", nil)
	if err == nil {
		t.Error("expected error normalizing empty data")
	}

	// Structured envelope
	envelopeJSON := []byte(`{
		"external_id": "msg-999",
		"type": "chat.message",
		"payload": {"text": "hello"},
		"metadata": {"user": "alice"}
	}`)
	ev1, err := adapter.Normalize(ctx, "src-1", envelopeJSON)
	if err != nil {
		t.Fatalf("unexpected error normalizing envelope: %v", err)
	}
	if ev1.ExternalEventID != "msg-999" || ev1.EventType != "chat.message" {
		t.Errorf("unexpected normalized fields: %+v", ev1)
	}

	// Raw arbitrary data
	rawJSON := []byte(`{"unstructured": true}`)
	ev2, err := adapter.Normalize(ctx, "src-2", rawJSON)
	if err != nil {
		t.Fatalf("unexpected error normalizing raw: %v", err)
	}
	if ev2.ExternalEventID == "" {
		t.Error("expected deterministic external ID generated for raw event")
	}
	if ev2.EventType != "raw.event" {
		t.Errorf("expected event_type 'raw.event', got %s", ev2.EventType)
	}
}

func TestWebhookSourceAdapter(t *testing.T) {
	ctx := context.Background()
	adapter := NewWebhookSourceAdapter()

	if adapter.Name() != "webhook" {
		t.Errorf("expected adapter name webhook, got %s", adapter.Name())
	}

	if err := adapter.Connect(ctx, nil); err != nil {
		t.Fatalf("failed to connect webhook adapter: %v", err)
	}
	if err := adapter.Health(ctx); err != nil {
		t.Fatalf("webhook adapter unhealthy: %v", err)
	}

	// Normalizing webhook payload
	ev, err := adapter.Normalize(ctx, "src-wh", []byte(`{"action":"opened","issue":{"number":42}}`))
	if err != nil {
		t.Fatalf("failed to normalize webhook: %v", err)
	}
	if ev.SourceID != "src-wh" {
		t.Errorf("expected source_id src-wh, got %s", ev.SourceID)
	}
}
