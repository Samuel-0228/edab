package tests

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	tgbotapi "github.com/go-telegram-bot-api/telegram-bot-api/v5"

	"kerob/internal/adapters/destinations"
	"kerob/internal/adapters/sources"
	"kerob/internal/core/delivery"
	"kerob/internal/core/event"
	"kerob/internal/core/routing"
	"kerob/internal/telegram"
)

// TestDefinitionOfDoneEndToEnd verifies the complete workflow from USER_REQUEST:
// Step 1: One Telegram bot.
// Step 2-3: Register Source A (-100101), Source B (-100102), Source C (-100103).
// Step 4: Add Backup A (-100201), Backup B (-100202) as destinations.
// Step 5: Routes:
//
//	Source A -> Backup A
//	Source B -> Backup B
//	Source C -> Backup A
//
// Step 6: Post to Source A -> automatically published to Backup A.
// Step 7: Post to Source B -> appears in Backup B.
// Step 8: Post to Source C -> appears in Backup A.
// Step 9: Process messages without duplicates.
func TestDefinitionOfDoneEndToEnd(t *testing.T) {
	ctx := context.Background()
	mockAPI := telegram.NewMockAPI()

	srcStore, destStore, routeStore, evStore, jobStore, pipe, worker := setupTelegramEngineTest(mockAPI)

	// Step 2-3: Register Sources
	srcA, _ := srcStore.Create(ctx, &sources.Source{
		Name:        "Source A",
		AdapterType: "telegram",
		Config:      json.RawMessage(`{"telegram_chat_id":"-100101"}`),
		Enabled:     true,
	})
	srcB, _ := srcStore.Create(ctx, &sources.Source{
		Name:        "Source B",
		AdapterType: "telegram",
		Config:      json.RawMessage(`{"telegram_chat_id":"-100102"}`),
		Enabled:     true,
	})
	srcC, _ := srcStore.Create(ctx, &sources.Source{
		Name:        "Source C",
		AdapterType: "telegram",
		Config:      json.RawMessage(`{"telegram_chat_id":"-100103"}`),
		Enabled:     true,
	})

	// Step 4: Register Destinations
	backupA, _ := destStore.Create(ctx, &destinations.Destination{
		Name:        "Backup A",
		AdapterType: "telegram",
		Config:      json.RawMessage(`{"telegram_chat_id":"-100201"}`),
		Enabled:     true,
	})
	backupB, _ := destStore.Create(ctx, &destinations.Destination{
		Name:        "Backup B",
		AdapterType: "telegram",
		Config:      json.RawMessage(`{"telegram_chat_id":"-100202"}`),
		Enabled:     true,
	})

	// Step 5: Create Routes
	// Source A -> Backup A
	_, _ = routeStore.Create(ctx, &routing.Route{SourceID: srcA.ID, DestinationID: backupA.ID, Enabled: true})
	// Source B -> Backup B
	_, _ = routeStore.Create(ctx, &routing.Route{SourceID: srcB.ID, DestinationID: backupB.ID, Enabled: true})
	// Source C -> Backup A
	_, _ = routeStore.Create(ctx, &routing.Route{SourceID: srcC.ID, DestinationID: backupA.ID, Enabled: true})

	stateStore := telegram.NewMemoryStateStore()
	sourceResolver := sources.NewSourceResolver(srcStore)
	ingester := telegram.IngesterFunc(func(ctx context.Context, sourceID string, rawData []byte) error {
		_, err := pipe.IngestRaw(ctx, sourceID, rawData)
		return err
	})

	listener := telegram.NewListener(mockAPI, ingester, sourceResolver, stateStore, telegram.ListenerOptions{
		PollTimeout:    1,
		MediaGroupWait: 20 * time.Millisecond,
	})

	// Simulate Step 6: Post something to Source A (msg 1001)
	msgA := tgbotapi.Message{
		MessageID: 1001,
		Text:      "Post from Source A",
		Chat:      &tgbotapi.Chat{ID: -100101, Title: "Source A", Type: "channel"},
	}
	rawMsgA, _ := json.Marshal(msgA)
	rawPostA, _ := json.Marshal(telegram.RawPost{
		ChatID:    -100101,
		ChatTitle: "Source A",
		ChatType:  "channel",
		Messages:  []json.RawMessage{rawMsgA},
	})
	_, err := pipe.IngestRaw(ctx, srcA.ID, rawPostA)
	if err != nil {
		t.Fatalf("ingest Source A failed: %v", err)
	}

	// Process jobs with worker
	claimed, err := worker.ProcessNextJob(ctx)
	if err != nil || !claimed {
		t.Fatalf("worker failed to claim job for Source A: claimed=%v err=%v", claimed, err)
	}

	// Verify Step 6: Appears in Backup A (-100201)
	if len(mockAPI.CopiedMessages) != 1 {
		t.Fatalf("expected 1 delivery call, got %d", len(mockAPI.CopiedMessages))
	}
	deliv1 := mockAPI.CopiedMessages[0]
	if deliv1.ChatID != -100201 || deliv1.FromChatID != -100101 || deliv1.MessageID != 1001 {
		t.Errorf("expected post to Backup A from Source A, got %+v", deliv1)
	}

	// Simulate Step 7: Post something to Source B (msg 2001)
	msgB := tgbotapi.Message{
		MessageID: 2001,
		Text:      "Post from Source B",
		Chat:      &tgbotapi.Chat{ID: -100102, Title: "Source B", Type: "channel"},
	}
	rawMsgB, _ := json.Marshal(msgB)
	rawPostB, _ := json.Marshal(telegram.RawPost{
		ChatID:    -100102,
		ChatTitle: "Source B",
		ChatType:  "channel",
		Messages:  []json.RawMessage{rawMsgB},
	})
	_, err = pipe.IngestRaw(ctx, srcB.ID, rawPostB)
	if err != nil {
		t.Fatalf("ingest Source B failed: %v", err)
	}

	claimed, err = worker.ProcessNextJob(ctx)
	if err != nil || !claimed {
		t.Fatalf("worker failed to claim job for Source B: claimed=%v err=%v", claimed, err)
	}

	// Verify Step 7: Appears in Backup B (-100202)
	if len(mockAPI.CopiedMessages) != 2 {
		t.Fatalf("expected 2 delivery calls, got %d", len(mockAPI.CopiedMessages))
	}
	deliv2 := mockAPI.CopiedMessages[1]
	if deliv2.ChatID != -100202 || deliv2.FromChatID != -100102 || deliv2.MessageID != 2001 {
		t.Errorf("expected post to Backup B from Source B, got %+v", deliv2)
	}

	// Simulate Step 8: Post something to Source C (msg 3001)
	msgC := tgbotapi.Message{
		MessageID: 3001,
		Text:      "Post from Source C",
		Chat:      &tgbotapi.Chat{ID: -100103, Title: "Source C", Type: "channel"},
	}
	rawMsgC, _ := json.Marshal(msgC)
	rawPostC, _ := json.Marshal(telegram.RawPost{
		ChatID:    -100103,
		ChatTitle: "Source C",
		ChatType:  "channel",
		Messages:  []json.RawMessage{rawMsgC},
	})
	_, err = pipe.IngestRaw(ctx, srcC.ID, rawPostC)
	if err != nil {
		t.Fatalf("ingest Source C failed: %v", err)
	}

	claimed, err = worker.ProcessNextJob(ctx)
	if err != nil || !claimed {
		t.Fatalf("worker failed to claim job for Source C: claimed=%v err=%v", claimed, err)
	}

	// Verify Step 8: Appears in Backup A (-100201)
	if len(mockAPI.CopiedMessages) != 3 {
		t.Fatalf("expected 3 delivery calls, got %d", len(mockAPI.CopiedMessages))
	}
	deliv3 := mockAPI.CopiedMessages[2]
	if deliv3.ChatID != -100201 || deliv3.FromChatID != -100103 || deliv3.MessageID != 3001 {
		t.Errorf("expected post to Backup A from Source C, got %+v", deliv3)
	}

	// Ensure Source A never accidentally posted to Backup B
	for _, call := range mockAPI.CopiedMessages {
		if call.FromChatID == -100101 && call.ChatID == -100202 {
			t.Fatal("Source A was incorrectly delivered to Backup B!")
		}
		if call.FromChatID == -100102 && call.ChatID == -100201 {
			t.Fatal("Source B was incorrectly delivered to Backup A!")
		}
	}

	_ = listener
	_ = evStore
	_ = jobStore
}

// TestOneSourceToManyDestinations verifies that an event from one source configured
// to route to N destinations is ingested once and delivered to all N destinations.
func TestOneSourceToManyDestinations(t *testing.T) {
	ctx := context.Background()
	mockAPI := telegram.NewMockAPI()

	srcStore, destStore, routeStore, evStore, jobStore, pipe, worker := setupTelegramEngineTest(mockAPI)

	srcA, _ := srcStore.Create(ctx, &sources.Source{
		Name:        "Source A",
		AdapterType: "telegram",
		Config:      json.RawMessage(`{"telegram_chat_id":"-100100"}`),
		Enabled:     true,
	})

	dest1, _ := destStore.Create(ctx, &destinations.Destination{
		Name:        "Dest 1",
		AdapterType: "telegram",
		Config:      json.RawMessage(`{"telegram_chat_id":"-100201"}`),
		Enabled:     true,
	})
	dest2, _ := destStore.Create(ctx, &destinations.Destination{
		Name:        "Dest 2",
		AdapterType: "telegram",
		Config:      json.RawMessage(`{"telegram_chat_id":"-100202"}`),
		Enabled:     true,
	})
	dest3, _ := destStore.Create(ctx, &destinations.Destination{
		Name:        "Dest 3",
		AdapterType: "telegram",
		Config:      json.RawMessage(`{"telegram_chat_id":"-100203"}`),
		Enabled:     true,
	})

	_, _ = routeStore.Create(ctx, &routing.Route{SourceID: srcA.ID, DestinationID: dest1.ID, Enabled: true})
	_, _ = routeStore.Create(ctx, &routing.Route{SourceID: srcA.ID, DestinationID: dest2.ID, Enabled: true})
	_, _ = routeStore.Create(ctx, &routing.Route{SourceID: srcA.ID, DestinationID: dest3.ID, Enabled: true})

	msg := tgbotapi.Message{
		MessageID: 8888,
		Text:      "Multicast broadcast",
		Chat:      &tgbotapi.Chat{ID: -100100, Title: "Source A", Type: "channel"},
	}
	rawMsg, _ := json.Marshal(msg)
	rawPost, _ := json.Marshal(telegram.RawPost{
		ChatID:   -100100,
		Messages: []json.RawMessage{rawMsg},
	})

	ev, err := pipe.IngestRaw(ctx, srcA.ID, rawPost)
	if err != nil {
		t.Fatalf("ingest failed: %v", err)
	}

	// Verify event was created once
	events, _ := evStore.List(ctx, event.ListFilter{SourceID: srcA.ID})
	if len(events) != 1 {
		t.Fatalf("expected exactly 1 event created, got %d", len(events))
	}

	// Verify 3 distinct delivery jobs were queued
	jobs, _ := jobStore.List(ctx, delivery.ListFilter{EventID: ev.ID})
	if len(jobs) != 3 {
		t.Fatalf("expected 3 delivery jobs queued, got %d", len(jobs))
	}

	// Worker processes all 3 jobs
	for i := 0; i < 3; i++ {
		claimed, err := worker.ProcessNextJob(ctx)
		if err != nil || !claimed {
			t.Fatalf("worker failed to claim job %d: claimed=%v err=%v", i, claimed, err)
		}
	}

	// Verify Telegram API received 3 copyMessage calls targeting the 3 different destinations
	if len(mockAPI.CopiedMessages) != 3 {
		t.Fatalf("expected 3 Telegram CopyMessage calls, got %d", len(mockAPI.CopiedMessages))
	}

	destSet := make(map[int64]bool)
	for _, call := range mockAPI.CopiedMessages {
		if call.FromChatID != -100100 || call.MessageID != 8888 {
			t.Errorf("unexpected call source info: %+v", call)
		}
		destSet[call.ChatID] = true
	}

	if !destSet[-100201] || !destSet[-100202] || !destSet[-100203] {
		t.Errorf("expected delivery to all 3 destination chats, got %v", destSet)
	}
}
