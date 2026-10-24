package destinations

import (
	"context"
	"errors"
	"io"
	"net/http"
	"strings"
	"testing"

	"kerob/internal/core/event"
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

type mockRoundTripper func(req *http.Request) (*http.Response, error)

func (m mockRoundTripper) RoundTrip(req *http.Request) (*http.Response, error) {
	return m(req)
}

func TestWebhookDestinationAdapter(t *testing.T) {
	ctx := context.Background()
	adapter := NewWebhookDestinationAdapter()

	if adapter.Name() != "webhook" {
		t.Fatalf("expected name webhook, got %s", adapter.Name())
	}

	// Health before connect should fail
	if err := adapter.Health(ctx); err == nil {
		t.Fatal("expected error before connect")
	}

	// Connect with invalid SSRF URL (private IP)
	err := adapter.Connect(ctx, []byte(`{"target_url":"http://127.0.0.1:8080/hook"}`))
	if err == nil {
		t.Fatal("expected SSRF error blocking 127.0.0.1")
	}

	// Connect with valid public IP URL (skips DNS)
	err = adapter.Connect(ctx, []byte(`{"target_url":"https://8.8.8.8/hook","headers":{"X-Secret":"token123"}}`))
	if err != nil {
		t.Fatalf("unexpected connect error: %v", err)
	}
	if err := adapter.Health(ctx); err != nil {
		t.Fatalf("expected healthy after connect: %v", err)
	}

	// Injected mock HTTP transport to test delivery
	adapter.client.Transport = mockRoundTripper(func(req *http.Request) (*http.Response, error) {
		if req.Header.Get("X-Secret") != "token123" {
			t.Errorf("missing configured header")
		}
		return &http.Response{
			StatusCode: http.StatusOK,
			Body:       io.NopCloser(strings.NewReader(`{"status":"received"}`)),
			Header:     make(http.Header),
		}, nil
	})

	ev, _ := event.New("src-1", "ext-1", "user.created", `{"user":"alice"}`, nil)
	if err := adapter.Deliver(ctx, ev); err != nil {
		t.Fatalf("failed to deliver event: %v", err)
	}

	// Deliver with result
	res, err := adapter.DeliverWithResult(ctx, ev)
	if err != nil || string(res) != `{"status":"received"}` {
		t.Fatalf("unexpected deliver with result: %s (err: %v)", string(res), err)
	}
}

func TestWebhookDestinationErrorsAndCancellation(t *testing.T) {
	ctx := context.Background()
	adapter := NewWebhookDestinationAdapter()
	if err := adapter.Connect(ctx, []byte(`{"target_url":"https://8.8.8.8/hook"}`)); err != nil {
		t.Fatalf("connect failed: %v", err)
	}
	adapter.client.Transport = mockRoundTripper(func(req *http.Request) (*http.Response, error) {
		return &http.Response{
			StatusCode: http.StatusBadGateway,
			Body:       io.NopCloser(strings.NewReader("upstream failed")),
			Header:     make(http.Header),
		}, nil
	})
	ev, _ := event.New("src", "external", "event", map[string]any{"value": "x"}, nil)
	if err := adapter.Deliver(ctx, ev); err == nil || !strings.Contains(err.Error(), "502") {
		t.Fatalf("expected non-2xx delivery error, got %v", err)
	}

	cancelled, cancel := context.WithCancel(context.Background())
	cancel()
	adapter.client.Transport = mockRoundTripper(func(req *http.Request) (*http.Response, error) {
		return nil, req.Context().Err()
	})
	if err := adapter.Deliver(cancelled, ev); err == nil || !strings.Contains(err.Error(), "context canceled") {
		t.Fatalf("expected cancellation error, got %v", err)
	}
}

func TestMemoryDestinationStore(t *testing.T) {
	ctx := context.Background()
	store := NewMemoryStore()

	// 1. Create Destination
	d1, err := store.Create(ctx, &Destination{
		Name:        "Webhook Target",
		AdapterType: "webhook",
		Config:      []byte(`{"target_url":"https://8.8.8.8/hook"}`),
		Enabled:     true,
	})
	if err != nil || d1.ID == "" {
		t.Fatalf("failed to create destination: %v", err)
	}

	// 2. GetByID
	fetched, err := store.GetByID(ctx, d1.ID)
	if err != nil || fetched.Name != "Webhook Target" {
		t.Fatalf("failed to get destination: %v", err)
	}
	if _, err := store.GetByID(ctx, "nonexistent"); err == nil {
		t.Fatal("expected error on nonexistent destination")
	}

	// 3. Update
	d1.Name = "Updated Target"
	updated, err := store.Update(ctx, d1)
	if err != nil || updated.Name != "Updated Target" {
		t.Fatalf("failed to update destination: %v", err)
	}

	// 4. List
	list, err := store.List(ctx)
	if err != nil || len(list) != 1 {
		t.Fatalf("expected 1 destination, got %d", len(list))
	}

	// 5. Delete
	if err := store.Delete(ctx, d1.ID); err != nil {
		t.Fatalf("failed to delete destination: %v", err)
	}
	if err := store.Delete(ctx, "nonexistent"); err == nil {
		t.Fatal("expected error deleting nonexistent destination")
	}
}
