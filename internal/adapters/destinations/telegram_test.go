package destinations

import (
	"context"
	"encoding/json"
	"testing"

	"kerob/internal/core/event"
	"kerob/internal/telegram"
)

func TestTelegramDestinationAdapterLifecycle(t *testing.T) {
	ctx := context.Background()
	mockAPI := telegram.NewMockAPI()
	adapter := NewTelegramDestinationAdapterWithAPI(mockAPI)

	if adapter.Name() != "telegram" {
		t.Fatalf("expected name 'telegram', got %s", adapter.Name())
	}

	if err := adapter.Health(ctx); err == nil {
		t.Fatal("expected health failure before connect")
	}

	// Missing chat id
	if err := adapter.Connect(ctx, []byte(`{}`)); err == nil {
		t.Fatal("expected connect failure with missing chat_id")
	}

	// Valid connect
	if err := adapter.Connect(ctx, []byte(`{"telegram_chat_id":"-100888999"}`)); err != nil {
		t.Fatalf("connect failed: %v", err)
	}

	if err := adapter.Health(ctx); err != nil {
		t.Fatalf("health check failed after connect: %v", err)
	}
}

func TestTelegramDestinationDeliverSingle(t *testing.T) {
	ctx := context.Background()
	mockAPI := telegram.NewMockAPI()
	adapter := NewTelegramDestinationAdapterWithAPI(mockAPI)

	if err := adapter.Connect(ctx, []byte(`{"telegram_chat_id":"-100999888"}`)); err != nil {
		t.Fatalf("connect failed: %v", err)
	}

	payload := telegram.MessagePayload{
		ChatID:     -100111,
		MessageID:  42,
		MessageIDs: []int{42},
		Text:       "Mirror me",
	}
	payloadBytes, _ := json.Marshal(payload)

	ev, err := event.New("src-1", "ext-1", "telegram.message", payloadBytes, nil)
	if err != nil {
		t.Fatalf("failed to create event: %v", err)
	}

	rawResult, err := adapter.DeliverWithResult(ctx, ev)
	if err != nil {
		t.Fatalf("delivery failed: %v", err)
	}

	if len(mockAPI.CopiedMessages) != 1 {
		t.Fatalf("expected 1 CopiedMessage call, got %d", len(mockAPI.CopiedMessages))
	}
	call := mockAPI.CopiedMessages[0]
	if call.ChatID != -100999888 {
		t.Errorf("expected destination chat -100999888, got %d", call.ChatID)
	}
	if call.FromChatID != -100111 {
		t.Errorf("expected source chat -100111, got %d", call.FromChatID)
	}
	if call.MessageID != 42 {
		t.Errorf("expected message id 42, got %d", call.MessageID)
	}

	var res telegramDeliveryResult
	if err := json.Unmarshal(rawResult, &res); err != nil {
		t.Fatalf("failed to unmarshal delivery result: %v", err)
	}
	if res.ChatID != -100999888 {
		t.Errorf("expected result chat -100999888, got %d", res.ChatID)
	}
	if len(res.MessageIDs) != 1 || res.MessageIDs[0] == 0 {
		t.Errorf("expected non-zero delivered message id, got %v", res.MessageIDs)
	}
}

func TestTelegramDestinationDeliverMediaGroup(t *testing.T) {
	ctx := context.Background()
	mockAPI := telegram.NewMockAPI()
	adapter := NewTelegramDestinationAdapterWithAPI(mockAPI)

	if err := adapter.Connect(ctx, []byte(`{"telegram_chat_id":"-100999888"}`)); err != nil {
		t.Fatalf("connect failed: %v", err)
	}

	payload := telegram.MessagePayload{
		ChatID:       -100111,
		MediaGroupID: "mg_123",
		MessageID:    10,
		MessageIDs:   []int{10, 11, 12},
		Text:         "Album title",
	}
	payloadBytes, _ := json.Marshal(payload)

	ev, err := event.New("src-1", "ext-album", "telegram.media_group", payloadBytes, nil)
	if err != nil {
		t.Fatalf("failed to create event: %v", err)
	}

	rawResult, err := adapter.DeliverWithResult(ctx, ev)
	if err != nil {
		t.Fatalf("delivery failed: %v", err)
	}

	if len(mockAPI.CopiedAlbums) != 1 {
		t.Fatalf("expected 1 CopiedAlbums call, got %d", len(mockAPI.CopiedAlbums))
	}
	call := mockAPI.CopiedAlbums[0]
	if call.ToChatID != -100999888 {
		t.Errorf("expected to chat -100999888, got %d", call.ToChatID)
	}
	if call.FromChatID != -100111 {
		t.Errorf("expected from chat -100111, got %d", call.FromChatID)
	}
	if len(call.MessageIDs) != 3 {
		t.Errorf("expected 3 message IDs in album, got %v", call.MessageIDs)
	}

	var res telegramDeliveryResult
	if err := json.Unmarshal(rawResult, &res); err != nil {
		t.Fatalf("failed to unmarshal delivery result: %v", err)
	}
	if len(res.MessageIDs) != 3 {
		t.Errorf("expected 3 delivered message IDs, got %v", res.MessageIDs)
	}
}

func TestTelegramDestinationDeliverDirect(t *testing.T) {
	ctx := context.Background()
	mockAPI := telegram.NewMockAPI()
	adapter := NewTelegramDestinationAdapterWithAPI(mockAPI)

	if err := adapter.Connect(ctx, []byte(`{"telegram_chat_id":"-100999888"}`)); err != nil {
		t.Fatalf("connect failed: %v", err)
	}

	payload := telegram.MessagePayload{
		ChannelUsername: "publicchannel",
		MessageID:       77,
		Text:            "Plain text",
		HTMLText:        "<b>Bold title</b>\nPlain text",
	}
	payloadBytes, _ := json.Marshal(payload)

	ev, err := event.New("src-public", "ext-public-77", "telegram.public_post", payloadBytes, nil)
	if err != nil {
		t.Fatalf("failed to create event: %v", err)
	}

	rawResult, err := adapter.DeliverWithResult(ctx, ev)
	if err != nil {
		t.Fatalf("delivery failed: %v", err)
	}

	if len(mockAPI.SentChattables) != 1 {
		t.Fatalf("expected 1 Send call, got %d", len(mockAPI.SentChattables))
	}

	var res telegramDeliveryResult
	if err := json.Unmarshal(rawResult, &res); err != nil {
		t.Fatalf("failed to unmarshal delivery result: %v", err)
	}
	if len(res.MessageIDs) != 1 || res.MessageIDs[0] == 0 {
		t.Errorf("expected delivered message id, got %v", res.MessageIDs)
	}

	// Also test Deliver (discards result)
	if err := adapter.Deliver(ctx, ev); err != nil {
		t.Fatalf("Deliver failed: %v", err)
	}
}

func TestTelegramDestinationAdapterHelpers(t *testing.T) {
	adapter := NewTelegramDestinationAdapter()
	if adapter == nil {
		t.Fatal("expected non-nil adapter")
	}

	mockAPI := telegram.NewMockAPI()
	adapter.SetAPI(mockAPI)

	// Test truncateCaption
	short := truncateCaption("hello world", 20)
	if short != "hello world" {
		t.Errorf("expected 'hello world', got %q", short)
	}
	long := truncateCaption("abcdefghijklmnopqrstuvwxyz", 10)
	if len([]rune(long)) != 10 || long != "abcdefghij" {
		t.Errorf("expected 10 rune truncated string, got %q", long)
	}

	// Test deliverDirect with no content (neither media nor text)
	ctx := context.Background()
	if err := adapter.Connect(ctx, []byte(`{"telegram_chat_id":"-100123"}`)); err != nil {
		t.Fatalf("connect failed: %v", err)
	}

	emptyPayload := telegram.MessagePayload{
		ChannelUsername: "empty_channel",
	}
	emptyBytes, _ := json.Marshal(emptyPayload)
	ev, _ := event.New("src-empty", "ext-empty", "telegram.public_post", emptyBytes, nil)

	_, err := adapter.DeliverWithResult(ctx, ev)
	if err == nil {
		t.Fatal("expected error delivering empty public post")
	}
}
