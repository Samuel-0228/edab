package sources

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"errors"
	"fmt"
	"net/http"
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

func TestValidateWebhookRequestSecretAndSignature(t *testing.T) {
	body := []byte(`{"event":"created"}`)
	secret := "unit-test-secret"
	mac := hmac.New(sha256.New, []byte(secret))
	_, _ = mac.Write(body)
	signature := "sha256=" + fmt.Sprintf("%x", mac.Sum(nil))

	headers := http.Header{
		"X-Webhook-Secret":        []string{secret},
		"X-Webhook-Signature-256": []string{signature},
	}
	config := []byte(`{"secret":"unit-test-secret"}`)
	if err := ValidateRequest(config, headers, body); err != nil {
		t.Fatalf("expected valid webhook request, got %v", err)
	}
	headers.Set("X-Webhook-Secret", "wrong")
	if err := ValidateRequest(config, headers, body); err == nil {
		t.Fatal("expected invalid webhook secret to fail")
	}
}

func TestMemorySourceStoreAndResolvers(t *testing.T) {
	ctx := context.Background()
	store := NewMemoryStore()

	// 1. Create Source
	s1, err := store.Create(ctx, &Source{
		Name:        "Telegram Channel Source",
		AdapterType: "telegram",
		Config:      []byte(`{"telegram_chat_id":"-10012345"}`),
		Enabled:     true,
	})
	if err != nil || s1.ID == "" {
		t.Fatalf("failed to create source: %v", err)
	}

	// 2. Create Public Telegram Source
	s2, err := store.Create(ctx, &Source{
		Name:        "Telegram Public Channel",
		AdapterType: "telegram_public",
		Config:      []byte(`{"channel_username":"technews"}`),
		Enabled:     true,
	})
	if err != nil {
		t.Fatalf("failed to create public source: %v", err)
	}

	// 3. GetByID
	fetched, err := store.GetByID(ctx, s1.ID)
	if err != nil || fetched.Name != "Telegram Channel Source" {
		t.Fatalf("failed to get source: %v", err)
	}
	if _, err := store.GetByID(ctx, "nonexistent"); err == nil {
		t.Fatal("expected error on nonexistent source")
	}

	// 4. Update
	s1.Name = "Updated Name"
	updated, err := store.Update(ctx, s1)
	if err != nil || updated.Name != "Updated Name" {
		t.Fatalf("failed to update source: %v", err)
	}

	// 5. List
	all, err := store.List(ctx)
	if err != nil || len(all) != 2 {
		t.Fatalf("expected 2 sources, got %d", len(all))
	}

	// 6. Test SourceResolver
	resolver := NewSourceResolver(store)
	matchedID, found, err := resolver.ResolveSource(ctx, -10012345)
	if err != nil || !found || matchedID != s1.ID {
		t.Fatalf("expected source resolver to find s1, got id=%s, found=%v, err=%v", matchedID, found, err)
	}
	_, foundMissing, _ := resolver.ResolveSource(ctx, -999999)
	if foundMissing {
		t.Fatal("expected found=false for missing chat ID")
	}

	// 7. Test PublicSourceResolver
	publicResolver := NewPublicSourceResolver(store)
	pubList, err := publicResolver(ctx)
	if err != nil || len(pubList) != 1 || pubList[0].Channel != "technews" {
		t.Fatalf("unexpected public source list: %+v", pubList)
	}

	// 8. Delete
	if err := store.Delete(ctx, s2.ID); err != nil {
		t.Fatalf("failed to delete source: %v", err)
	}
	if err := store.Delete(ctx, "nonexistent"); err == nil {
		t.Fatal("expected error deleting nonexistent source")
	}
}

func TestSourceAdapterPollAndEdgeCases(t *testing.T) {
	ctx := context.Background()

	// Webhook source Poll and empty Normalize
	whAdapter := NewWebhookSourceAdapter()
	events, err := whAdapter.Poll(ctx)
	if err != nil || events != nil {
		t.Fatalf("expected nil poll events, got %v, err=%v", events, err)
	}
	_, err = whAdapter.Normalize(ctx, "src-wh", nil)
	if err == nil {
		t.Fatal("expected error normalizing empty webhook body")
	}

	// Telegram source Poll
	tgAdapter := NewTelegramSourceAdapter()
	tgEvents, err := tgAdapter.Poll(ctx)
	if err != nil || tgEvents != nil {
		t.Fatalf("expected nil poll events, got %v, err=%v", tgEvents, err)
	}
}
