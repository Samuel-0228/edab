package webhooks

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestWebhookMemoryStoreCRUD(t *testing.T) {
	ctx := context.Background()
	store := NewMemoryStore()

	wh, err := store.Create(ctx, &Webhook{
		Name:        "GitHub Push Notifications",
		SecretToken: "secret123",
		Enabled:     true,
	})
	if err != nil {
		t.Fatalf("failed to create webhook: %v", err)
	}

	if wh.ID == "" {
		t.Fatal("expected generated webhook ID")
	}

	fetched, err := store.GetByID(ctx, wh.ID)
	if err != nil {
		t.Fatalf("GetByID failed: %v", err)
	}
	if fetched.Name != "GitHub Push Notifications" {
		t.Errorf("expected name 'GitHub Push Notifications', got %s", fetched.Name)
	}

	// List
	list, err := store.List(ctx)
	if err != nil {
		t.Fatalf("List failed: %v", err)
	}
	if len(list) != 1 {
		t.Fatalf("expected 1 webhook, got %d", len(list))
	}

	// Update
	wh.Enabled = false
	updated, err := store.Update(ctx, wh)
	if err != nil {
		t.Fatalf("Update failed: %v", err)
	}
	if updated.Enabled {
		t.Fatal("expected Enabled to be false")
	}

	// Delete
	if err := store.Delete(ctx, wh.ID); err != nil {
		t.Fatalf("Delete failed: %v", err)
	}
	_, err = store.GetByID(ctx, wh.ID)
	if err != ErrWebhookNotFound {
		t.Fatalf("expected ErrWebhookNotFound, got %v", err)
	}
}

func TestWebhookAuthentication(t *testing.T) {
	wh := &Webhook{
		ID:          "wh-123",
		Name:        "Test Ingest",
		SecretToken: "valid-secret-token",
		Enabled:     true,
	}

	// 1. Authenticate with X-Webhook-Token header
	req1 := httptest.NewRequest(http.MethodPost, "/webhooks/wh-123", nil)
	req1.Header.Set("X-Webhook-Token", "valid-secret-token")
	if err := Authenticate(wh, req1); err != nil {
		t.Errorf("expected success with header, got %v", err)
	}

	// 2. Authenticate with Bearer token
	req2 := httptest.NewRequest(http.MethodPost, "/webhooks/wh-123", nil)
	req2.Header.Set("Authorization", "Bearer valid-secret-token")
	if err := Authenticate(wh, req2); err != nil {
		t.Errorf("expected success with bearer, got %v", err)
	}

	// 3. Authenticate with query param
	req3 := httptest.NewRequest(http.MethodPost, "/webhooks/wh-123?token=valid-secret-token", nil)
	if err := Authenticate(wh, req3); err != nil {
		t.Errorf("expected success with query param, got %v", err)
	}

	// 4. Invalid token
	reqInvalid := httptest.NewRequest(http.MethodPost, "/webhooks/wh-123", nil)
	reqInvalid.Header.Set("X-Webhook-Token", "wrong-secret")
	if err := Authenticate(wh, reqInvalid); err != ErrInvalidSecretToken {
		t.Errorf("expected ErrInvalidSecretToken, got %v", err)
	}

	// 5. Missing token
	reqMissing := httptest.NewRequest(http.MethodPost, "/webhooks/wh-123", nil)
	if err := Authenticate(wh, reqMissing); err != ErrInvalidSecretToken {
		t.Errorf("expected ErrInvalidSecretToken for missing token, got %v", err)
	}
}

func TestNormalizeWebhookEvent(t *testing.T) {
	wh := &Webhook{
		ID:   "wh-456",
		Name: "Monitoring Hook",
	}

	body := []byte(`{"alert":"cpu_high","usage":95.5}`)
	headers := http.Header{}
	headers.Set("User-Agent", "Prometheus-Alertmanager/0.25")
	headers.Set("Content-Type", "application/json")
	headers.Set("X-Event-Type", "infra.alert")

	ev, err := NormalizeWebhookEvent(wh, body, headers)
	if err != nil {
		t.Fatalf("NormalizeWebhookEvent failed: %v", err)
	}

	if ev.SourceID != "wh-456" {
		t.Errorf("expected SourceID wh-456, got %s", ev.SourceID)
	}
	if ev.EventType != "infra.alert" {
		t.Errorf("expected EventType infra.alert, got %s", ev.EventType)
	}
}
