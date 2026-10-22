package sources

import (
	"context"
	"encoding/json"
	"testing"

	tgbotapi "github.com/go-telegram-bot-api/telegram-bot-api/v5"

	"kerob/internal/telegram"
)

func TestTelegramSourceAdapterLifecycle(t *testing.T) {
	ctx := context.Background()
	adapter := NewTelegramSourceAdapter()

	if adapter.Name() != "telegram" {
		t.Fatalf("expected name 'telegram', got %s", adapter.Name())
	}

	if err := adapter.Health(ctx); err == nil {
		t.Fatal("expected health check failure before connect")
	}

	if err := adapter.Connect(ctx, []byte(`{"telegram_chat_id":"-100123"}`)); err != nil {
		t.Fatalf("unexpected connect error: %v", err)
	}

	if err := adapter.Health(ctx); err != nil {
		t.Fatalf("unexpected health check error after connect: %v", err)
	}
}

func TestTelegramSourceAdapterNormalizeSingle(t *testing.T) {
	ctx := context.Background()
	adapter := NewTelegramSourceAdapter()
	_ = adapter.Connect(ctx, nil)

	msg := tgbotapi.Message{
		MessageID: 777,
		Text:      "Important broadcast message",
		Chat: &tgbotapi.Chat{
			ID:    -100111222,
			Title: "News Source",
			Type:  "channel",
		},
	}
	msgBytes, _ := json.Marshal(msg)

	rawPost := telegram.RawPost{
		ChatID:    -100111222,
		ChatTitle: "News Source",
		ChatType:  "channel",
		Messages:  []json.RawMessage{msgBytes},
	}
	rawBytes, _ := json.Marshal(rawPost)

	ev, err := adapter.Normalize(ctx, "src-news", rawBytes)
	if err != nil {
		t.Fatalf("normalization failed: %v", err)
	}

	if ev.SourceID != "src-news" {
		t.Errorf("expected source src-news, got %s", ev.SourceID)
	}
	expectedExtID := "telegram:-100111222:777"
	if ev.ExternalEventID != expectedExtID {
		t.Errorf("expected external id %s, got %s", expectedExtID, ev.ExternalEventID)
	}
	if ev.EventType != telegram.EventTypeMessage {
		t.Errorf("expected event type %s, got %s", telegram.EventTypeMessage, ev.EventType)
	}

	var payload telegram.MessagePayload
	if err := json.Unmarshal(ev.Payload, &payload); err != nil {
		t.Fatalf("failed to decode payload: %v", err)
	}
	if payload.Text != "Important broadcast message" {
		t.Errorf("expected text 'Important broadcast message', got %q", payload.Text)
	}
	if payload.MessageID != 777 {
		t.Errorf("expected message id 777, got %d", payload.MessageID)
	}
}

func TestTelegramSourceAdapterNormalizeMediaGroup(t *testing.T) {
	ctx := context.Background()
	adapter := NewTelegramSourceAdapter()
	_ = adapter.Connect(ctx, nil)

	msg1 := tgbotapi.Message{
		MessageID:    101,
		MediaGroupID: "album_42",
		Caption:      "First photo of album",
		Photo: []tgbotapi.PhotoSize{
			{FileID: "thumb_1"},
			{FileID: "large_1"},
		},
		Chat: &tgbotapi.Chat{ID: -100444, Title: "Photos", Type: "channel"},
	}
	msg2 := tgbotapi.Message{
		MessageID:    102,
		MediaGroupID: "album_42",
		Photo: []tgbotapi.PhotoSize{
			{FileID: "thumb_2"},
			{FileID: "large_2"},
		},
		Chat: &tgbotapi.Chat{ID: -100444, Title: "Photos", Type: "channel"},
	}

	b1, _ := json.Marshal(msg1)
	b2, _ := json.Marshal(msg2)

	rawPost := telegram.RawPost{
		ChatID:       -100444,
		ChatTitle:    "Photos",
		ChatType:     "channel",
		MediaGroupID: "album_42",
		Messages:     []json.RawMessage{b1, b2},
	}
	rawBytes, _ := json.Marshal(rawPost)

	ev, err := adapter.Normalize(ctx, "src-photos", rawBytes)
	if err != nil {
		t.Fatalf("normalization failed: %v", err)
	}

	expectedExtID := "telegram:-100444:mg:album_42"
	if ev.ExternalEventID != expectedExtID {
		t.Errorf("expected external id %s, got %s", expectedExtID, ev.ExternalEventID)
	}
	if ev.EventType != telegram.EventTypeMediaGroup {
		t.Errorf("expected event type %s, got %s", telegram.EventTypeMediaGroup, ev.EventType)
	}

	var payload telegram.MessagePayload
	_ = json.Unmarshal(ev.Payload, &payload)
	if len(payload.MessageIDs) != 2 || payload.MessageIDs[0] != 101 || payload.MessageIDs[1] != 102 {
		t.Errorf("expected message ids [101, 102], got %v", payload.MessageIDs)
	}
	if payload.Text != "First photo of album" {
		t.Errorf("expected caption preserved, got %q", payload.Text)
	}
	if len(payload.Media) != 2 {
		t.Fatalf("expected 2 media refs, got %d", len(payload.Media))
	}
	if payload.Media[0].FileID != "large_1" || payload.Media[1].FileID != "large_2" {
		t.Errorf("expected largest photo file IDs, got %v", payload.Media)
	}
}

func TestTelegramPublicSourceAdapter(t *testing.T) {
	ctx := context.Background()
	adapter := NewTelegramPublicSourceAdapter()

	if adapter.Name() != "telegram_public" {
		t.Errorf("expected telegram_public, got %s", adapter.Name())
	}

	// Health before connect
	if err := adapter.Health(ctx); err == nil {
		t.Fatal("expected error on health before connect")
	}

	// Connect invalid JSON
	if err := adapter.Connect(ctx, []byte(`{invalid json`)); err == nil {
		t.Fatal("expected error on invalid config JSON")
	}

	// Connect valid
	if err := adapter.Connect(ctx, []byte(`{"channel_username":"@golang_news"}`)); err != nil {
		t.Fatalf("connect failed: %v", err)
	}
	if err := adapter.Health(ctx); err != nil {
		t.Fatalf("health failed after connect: %v", err)
	}

	// Poll returns nil
	posts, err := adapter.Poll(ctx)
	if err != nil || posts != nil {
		t.Errorf("expected nil from Poll, got: %v, %v", posts, err)
	}

	// Normalize validation errors
	if _, err := adapter.Normalize(ctx, "src-1", nil); err == nil {
		t.Fatal("expected error on empty raw bytes")
	}
	if _, err := adapter.Normalize(ctx, "src-1", []byte(`{bad json`)); err == nil {
		t.Fatal("expected error on bad json")
	}
	if _, err := adapter.Normalize(ctx, "src-1", []byte(`{"message_id":1}`)); err == nil {
		t.Fatal("expected error on missing channel")
	}
	if _, err := adapter.Normalize(ctx, "src-1", []byte(`{"channel":"golang_news","message_id":0}`)); err == nil {
		t.Fatal("expected error on missing/invalid message_id")
	}

	// Normalize single text post
	singleRaw := []byte(`{
		"channel": "@golang_news",
		"message_id": 55,
		"text": "Go 1.22 released",
		"html_text": "<b>Go 1.22</b> released"
	}`)
	evSingle, err := adapter.Normalize(ctx, "src-pub", singleRaw)
	if err != nil {
		t.Fatalf("normalize single failed: %v", err)
	}
	if evSingle.EventType != "telegram.public_post" {
		t.Errorf("expected telegram.public_post, got %s", evSingle.EventType)
	}

	// Normalize album with media
	albumRaw := []byte(`{
		"channel": "golang_news",
		"message_id": 60,
		"text": "Conference photos",
		"is_album": true,
		"media": [
			{"kind":"photo","url":"https://8.8.8.8/p1.jpg"},
			{"kind":"photo","url":"https://8.8.8.8/p2.jpg"}
		]
	}`)
	evAlbum, err := adapter.Normalize(ctx, "src-pub", albumRaw)
	if err != nil {
		t.Fatalf("normalize album failed: %v", err)
	}
	if evAlbum.EventType != "telegram.media_group" {
		t.Errorf("expected telegram.media_group for album, got %s", evAlbum.EventType)
	}
}
