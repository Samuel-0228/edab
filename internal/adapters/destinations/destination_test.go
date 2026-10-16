package destinations

import (
	"context"
	"errors"
	"testing"

	"automation-engine/internal/core/event"
)

func TestDestinationRegistry(t *testing.T) {
	reg := NewRegistry()

	if len(reg.RegisteredTypes()) != 0 {
		t.Fatalf("expected empty registry, got %v", reg.RegisteredTypes())
	}

	reg.Register("mock", func() DestinationAdapter {
		return NewMockDestinationAdapter()
	})

	types := reg.RegisteredTypes()
	if len(types) != 1 || types[0] != "mock" {
		t.Fatalf("expected [mock], got %v", types)
	}

	adapter, err := reg.Create("mock")
	if err != nil {
		t.Fatalf("unexpected error creating mock adapter: %v", err)
	}
	if adapter.Name() != "mock" {
		t.Errorf("expected adapter name mock, got %s", adapter.Name())
	}

	_, err = reg.Create("nonexistent")
	if err == nil || !errors.Is(err, ErrAdapterNotFound) {
		t.Fatalf("expected ErrAdapterNotFound, got %v", err)
	}
}

func TestMockDestinationAdapter(t *testing.T) {
	ctx := context.Background()
	adapter := NewMockDestinationAdapter()

	// Health before connect should fail
	if err := adapter.Health(ctx); err == nil {
		t.Error("expected health check to fail before connect")
	}

	// Connect
	if err := adapter.Connect(ctx, nil); err != nil {
		t.Fatalf("connect failed: %v", err)
	}
	if err := adapter.Health(ctx); err != nil {
		t.Fatalf("health check failed after connect: %v", err)
	}

	ev, err := event.New("src-1", "ext-1", "test.event", `{"msg":"hi"}`, nil)
	if err != nil {
		t.Fatalf("failed to create event: %v", err)
	}

	// Successful delivery
	if err := adapter.Deliver(ctx, ev); err != nil {
		t.Fatalf("delivery failed: %v", err)
	}
	if adapter.GetDeliveryCount() != 1 {
		t.Errorf("expected delivery count 1, got %d", adapter.GetDeliveryCount())
	}
	delivered := adapter.GetDeliveredEvents()
	if len(delivered) != 1 || delivered[0].ID != ev.ID {
		t.Errorf("expected delivered event %s, got %v", ev.ID, delivered)
	}

	// Permanent failure simulation
	adapter.SetFailBehavior(true, 0, "target endpoint unavailable")
	if err := adapter.Deliver(ctx, ev); err == nil || err.Error() != "target endpoint unavailable" {
		t.Errorf("expected simulated failure, got %v", err)
	}

	// Transient failure simulation
	adapter.Reset()
	adapter.SetFailBehavior(false, 2, "rate limit exceeded")
	// Attempt 1 fails
	if err := adapter.Deliver(ctx, ev); err == nil {
		t.Error("expected attempt 1 to fail")
	}
	// Attempt 2 fails
	if err := adapter.Deliver(ctx, ev); err == nil {
		t.Error("expected attempt 2 to fail")
	}
	// Attempt 3 succeeds
	if err := adapter.Deliver(ctx, ev); err != nil {
		t.Errorf("expected attempt 3 to succeed, got %v", err)
	}
}

func TestStdoutDestinationAdapter(t *testing.T) {
	ctx := context.Background()
	adapter := NewStdoutDestinationAdapter()

	if adapter.Name() != "stdout" {
		t.Errorf("expected name stdout, got %s", adapter.Name())
	}

	if err := adapter.Connect(ctx, nil); err != nil {
		t.Fatalf("failed to connect: %v", err)
	}
	if err := adapter.Health(ctx); err != nil {
		t.Fatalf("health check failed: %v", err)
	}

	ev, _ := event.New("src-std", "ext-std", "log.info", `{"text":"test"}`, nil)
	if err := adapter.Deliver(ctx, ev); err != nil {
		t.Errorf("stdout deliver failed: %v", err)
	}
}
