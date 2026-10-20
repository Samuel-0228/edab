package tests

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"
	"time"

	tgbotapi "github.com/go-telegram-bot-api/telegram-bot-api/v5"

	"kerob/internal/adapters/destinations"
	"kerob/internal/adapters/sources"
	"kerob/internal/core/delivery"
	"kerob/internal/core/event"
	"kerob/internal/core/routing"
	"kerob/internal/core/workflow"
	"kerob/internal/telegram"
)

func setupTelegramEngineTest(mockAPI *telegram.MockAPI) (
	*MemorySourceStore,
	*MemoryDestinationStore,
	*MemoryRouteStore,
	*MemoryEventStore,
	*MemoryDeliveryStore,
	*workflow.Pipeline,
	*delivery.Worker,
) {
	evStore := NewMemoryEventStore()
	srcStore := NewMemorySourceStore()
	destStore := NewMemoryDestinationStore()
	routeStore := NewMemoryRouteStore()
	jobStore := NewMemoryDeliveryStore()

	srcReg := sources.NewRegistry()
	srcReg.Register("telegram", func() sources.SourceAdapter {
		return sources.NewTelegramSourceAdapter()
	})
	srcReg.Register("telegram_public", func() sources.SourceAdapter {
		return sources.NewTelegramPublicSourceAdapter()
	})

	destReg := destinations.NewRegistry()
	destReg.Register("telegram", func() destinations.DestinationAdapter {
		return destinations.NewTelegramDestinationAdapterWithAPI(mockAPI)
	})

	rEngine := routing.NewRoutingEngine(routeStore)
	pipe := workflow.NewPipeline(srcStore, srcReg, evStore, rEngine, jobStore, destStore, 3)

	worker := delivery.NewWorker(jobStore, evStore, destStore, destReg, delivery.WorkerConfig{
		PollInterval:   10 * time.Millisecond,
		Concurrency:    1,
		InitialBackoff: 10 * time.Millisecond,
		MaxBackoff:     100 * time.Millisecond,
	})

	return srcStore, destStore, routeStore, evStore, jobStore, pipe, worker
}

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

// TestDeduplicationSameMessage verifies that receiving the exact same Telegram
// message twice creates only 1 event and does not enqueue duplicate jobs.
func TestDeduplicationSameMessage(t *testing.T) {
	ctx := context.Background()
	mockAPI := telegram.NewMockAPI()

	srcStore, destStore, routeStore, evStore, jobStore, pipe, worker := setupTelegramEngineTest(mockAPI)

	srcA, _ := srcStore.Create(ctx, &sources.Source{
		Name:        "Dedupe Source",
		AdapterType: "telegram",
		Config:      json.RawMessage(`{"telegram_chat_id":"-100777"}`),
		Enabled:     true,
	})
	dest1, _ := destStore.Create(ctx, &destinations.Destination{
		Name:        "Dedupe Target",
		AdapterType: "telegram",
		Config:      json.RawMessage(`{"telegram_chat_id":"-100888"}`),
		Enabled:     true,
	})
	_, _ = routeStore.Create(ctx, &routing.Route{SourceID: srcA.ID, DestinationID: dest1.ID, Enabled: true})

	msg := tgbotapi.Message{
		MessageID: 444,
		Text:      "Duplicate Test",
		Chat:      &tgbotapi.Chat{ID: -100777, Title: "Dedupe Source", Type: "channel"},
	}
	rawMsg, _ := json.Marshal(msg)
	rawPost, _ := json.Marshal(telegram.RawPost{
		ChatID:   -100777,
		Messages: []json.RawMessage{rawMsg},
	})

	// First ingestion
	ev1, err := pipe.IngestRaw(ctx, srcA.ID, rawPost)
	if err != nil {
		t.Fatalf("first ingest failed: %v", err)
	}

	// Second ingestion of identical message
	ev2, err := pipe.IngestRaw(ctx, srcA.ID, rawPost)
	if err != nil {
		t.Fatalf("second ingest failed: %v", err)
	}

	if ev1.ID != ev2.ID {
		t.Fatalf("expected identical event returned on duplicate, got ev1=%s ev2=%s", ev1.ID, ev2.ID)
	}

	// Verify only 1 event stored
	events, _ := evStore.List(ctx, event.ListFilter{SourceID: srcA.ID})
	if len(events) != 1 {
		t.Fatalf("expected exactly 1 event stored, got %d", len(events))
	}

	// Verify only 1 delivery job exists
	jobs, _ := jobStore.List(ctx, delivery.ListFilter{EventID: ev1.ID})
	if len(jobs) != 1 {
		t.Fatalf("expected 1 delivery job, got %d", len(jobs))
	}

	// Worker processes job
	claimed, _ := worker.ProcessNextJob(ctx)
	if !claimed {
		t.Fatal("worker should claim job")
	}

	// No more jobs ready
	claimed2, _ := worker.ProcessNextJob(ctx)
	if claimed2 {
		t.Fatal("no second job should exist")
	}

	if len(mockAPI.CopiedMessages) != 1 {
		t.Fatalf("expected exactly 1 delivery call, got %d", len(mockAPI.CopiedMessages))
	}
}

// TestDeliveryIdempotencyOnRetry verifies that retrying an already completed job
// uses the recorded result and avoids creating duplicate posts at the destination.
func TestDeliveryIdempotencyOnRetry(t *testing.T) {
	ctx := context.Background()
	mockAPI := telegram.NewMockAPI()

	srcStore, destStore, routeStore, _, jobStore, pipe, worker := setupTelegramEngineTest(mockAPI)

	src, _ := srcStore.Create(ctx, &sources.Source{
		Name:        "Retry Src",
		AdapterType: "telegram",
		Config:      json.RawMessage(`{"telegram_chat_id":"-100500"}`),
		Enabled:     true,
	})
	dest, _ := destStore.Create(ctx, &destinations.Destination{
		Name:        "Retry Dst",
		AdapterType: "telegram",
		Config:      json.RawMessage(`{"telegram_chat_id":"-100600"}`),
		Enabled:     true,
	})
	_, _ = routeStore.Create(ctx, &routing.Route{SourceID: src.ID, DestinationID: dest.ID, Enabled: true})

	msg := tgbotapi.Message{
		MessageID: 1234,
		Text:      "Retry Idempotency Test",
		Chat:      &tgbotapi.Chat{ID: -100500, Title: "Retry Src", Type: "channel"},
	}
	rawMsg, _ := json.Marshal(msg)
	rawPost, _ := json.Marshal(telegram.RawPost{
		ChatID:   -100500,
		Messages: []json.RawMessage{rawMsg},
	})

	ev, _ := pipe.IngestRaw(ctx, src.ID, rawPost)

	// Deliver once
	claimed, err := worker.ProcessNextJob(ctx)
	if err != nil || !claimed {
		t.Fatalf("initial delivery failed: claimed=%v err=%v", claimed, err)
	}

	if len(mockAPI.CopiedMessages) != 1 {
		t.Fatalf("expected 1 delivery call, got %d", len(mockAPI.CopiedMessages))
	}

	// Inspect the job: Result must be recorded
	jobs, _ := jobStore.List(ctx, delivery.ListFilter{EventID: ev.ID})
	if len(jobs) != 1 {
		t.Fatalf("expected 1 job, got %d", len(jobs))
	}
	job := jobs[0]
	if len(job.Result) == 0 {
		t.Fatal("expected delivery result to be recorded on successful job")
	}

	// Reset for retry (as done by POST /deliveries/{id}/retry)
	retriedJob, err := jobStore.ResetForRetry(ctx, job.ID)
	if err != nil {
		t.Fatalf("reset for retry failed: %v", err)
	}
	if retriedJob.Status != delivery.StatusPending {
		t.Fatalf("expected status pending, got %s", retriedJob.Status)
	}

	// Worker processes retried job
	claimed2, err2 := worker.ProcessNextJob(ctx)
	if err2 != nil || !claimed2 {
		t.Fatalf("worker failed to claim retried job: %v", err2)
	}

	// Crucial check: mockAPI must NOT have received a second delivery request!
	if len(mockAPI.CopiedMessages) != 1 {
		t.Fatalf("duplicate post detected! Expected CopiedMessages count=1, got %d", len(mockAPI.CopiedMessages))
	}
}

// TestMediaGroupAlbumEndToEnd verifies album ingestion, buffering, and delivery
// using CopyMessages.
func TestMediaGroupAlbumEndToEnd(t *testing.T) {
	ctx := context.Background()
	mockAPI := telegram.NewMockAPI()

	srcStore, destStore, routeStore, _, _, pipe, worker := setupTelegramEngineTest(mockAPI)

	src, _ := srcStore.Create(ctx, &sources.Source{
		Name:        "Album Source",
		AdapterType: "telegram",
		Config:      json.RawMessage(`{"telegram_chat_id":"-100701"}`),
		Enabled:     true,
	})
	dest, _ := destStore.Create(ctx, &destinations.Destination{
		Name:        "Album Target",
		AdapterType: "telegram",
		Config:      json.RawMessage(`{"telegram_chat_id":"-100702"}`),
		Enabled:     true,
	})
	_, _ = routeStore.Create(ctx, &routing.Route{SourceID: src.ID, DestinationID: dest.ID, Enabled: true})

	// Create 3 photos in an album
	msg1 := tgbotapi.Message{
		MessageID:    501,
		MediaGroupID: "album_xyz",
		Caption:      "Scenic View",
		Photo:        []tgbotapi.PhotoSize{{FileID: "photo_large_1"}},
		Chat:         &tgbotapi.Chat{ID: -100701, Title: "Album Source", Type: "channel"},
	}
	msg2 := tgbotapi.Message{
		MessageID:    502,
		MediaGroupID: "album_xyz",
		Photo:        []tgbotapi.PhotoSize{{FileID: "photo_large_2"}},
		Chat:         &tgbotapi.Chat{ID: -100701, Title: "Album Source", Type: "channel"},
	}
	msg3 := tgbotapi.Message{
		MessageID:    503,
		MediaGroupID: "album_xyz",
		Photo:        []tgbotapi.PhotoSize{{FileID: "photo_large_3"}},
		Chat:         &tgbotapi.Chat{ID: -100701, Title: "Album Source", Type: "channel"},
	}

	b1, _ := json.Marshal(msg1)
	b2, _ := json.Marshal(msg2)
	b3, _ := json.Marshal(msg3)

	rawPost, _ := json.Marshal(telegram.RawPost{
		ChatID:       -100701,
		MediaGroupID: "album_xyz",
		Messages:     []json.RawMessage{b1, b2, b3},
	})

	ev, err := pipe.IngestRaw(ctx, src.ID, rawPost)
	if err != nil {
		t.Fatalf("album ingest failed: %v", err)
	}
	if ev.EventType != telegram.EventTypeMediaGroup {
		t.Fatalf("expected event type telegram.media_group, got %s", ev.EventType)
	}

	// Worker processes album
	claimed, err := worker.ProcessNextJob(ctx)
	if err != nil || !claimed {
		t.Fatalf("failed to process album job: %v", err)
	}

	// Verify CopyMessages was used
	if len(mockAPI.CopiedAlbums) != 1 {
		t.Fatalf("expected 1 CopiedAlbums call, got %d", len(mockAPI.CopiedAlbums))
	}
	album := mockAPI.CopiedAlbums[0]
	if album.FromChatID != -100701 || album.ToChatID != -100702 {
		t.Errorf("unexpected chats: from=%d to=%d", album.FromChatID, album.ToChatID)
	}
	if len(album.MessageIDs) != 3 || album.MessageIDs[0] != 501 || album.MessageIDs[1] != 502 || album.MessageIDs[2] != 503 {
		t.Errorf("unexpected message IDs in album: %v", album.MessageIDs)
	}
}

// TestRestartRecovery verifies that listener offset state survives restarts and
// resumes safely without missing or duplicating messages.
func TestRestartRecovery(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	mockAPI := telegram.NewMockAPI()
	stateStore := telegram.NewMemoryStateStore()

	srcStore := NewMemorySourceStore()
	_, _ = srcStore.Create(ctx, &sources.Source{
		ID:          "src-restart",
		Name:        "Restart Channel",
		AdapterType: "telegram",
		Config:      json.RawMessage(`{"telegram_chat_id":"-100999"}`),
		Enabled:     true,
	})

	var ingestedMessages []int
	var ingMu sync.Mutex
	ingester := telegram.IngesterFunc(func(ctx context.Context, sourceID string, rawData []byte) error {
		ingMu.Lock()
		defer ingMu.Unlock()
		var p telegram.RawPost
		_ = json.Unmarshal(rawData, &p)
		var m tgbotapi.Message
		if len(p.Messages) > 0 {
			_ = json.Unmarshal(p.Messages[0], &m)
			ingestedMessages = append(ingestedMessages, m.MessageID)
		}
		return nil
	})

	sourceResolver := sources.NewSourceResolver(srcStore)

	// Phase 1: Listener starts, receives update 50, updates state
	firstDone := make(chan struct{})
	mockAPI.GetUpdatesFunc = func(cfg tgbotapi.UpdateConfig) ([]tgbotapi.Update, error) {
		if cfg.Offset == -1 {
			return nil, nil
		}
		if cfg.Offset == 0 {
			return []tgbotapi.Update{
				{
					UpdateID: 50,
					ChannelPost: &tgbotapi.Message{
						MessageID: 1050,
						Chat:      &tgbotapi.Chat{ID: -100999, Title: "Restart Channel", Type: "channel"},
						Text:      "First message before crash",
					},
				},
			}, nil
		}
		select {
		case <-firstDone:
		default:
			close(firstDone)
		}
		return nil, nil
	}

	listener := telegram.NewListener(mockAPI, ingester, sourceResolver, stateStore, telegram.ListenerOptions{
		PollTimeout: 1,
	})
	listener.Start(ctx)
	<-firstDone
	listener.Stop()

	// Verify phase 1 ingested message 1050
	ingMu.Lock()
	if len(ingestedMessages) != 1 || ingestedMessages[0] != 1050 {
		t.Fatalf("expected message 1050 ingested in phase 1, got %v", ingestedMessages)
	}
	ingMu.Unlock()

	// Verify state saved update 50
	lastID, _ := stateStore.GetLastUpdateID(ctx)
	if lastID != 50 {
		t.Fatalf("expected last update id 50, got %d", lastID)
	}

	// Phase 2: Process restarts! New listener instance with same stateStore.
	secondDone := make(chan struct{})
	var initialResumedOffset int
	mockAPI.GetUpdatesFunc = func(cfg tgbotapi.UpdateConfig) ([]tgbotapi.Update, error) {
		if initialResumedOffset == 0 {
			initialResumedOffset = cfg.Offset
		}
		if cfg.Offset == 51 {
			return []tgbotapi.Update{
				{
					UpdateID: 51,
					ChannelPost: &tgbotapi.Message{
						MessageID: 1051,
						Chat:      &tgbotapi.Chat{ID: -100999, Title: "Restart Channel", Type: "channel"},
						Text:      "Second message after restart",
					},
				},
			}, nil
		}
		select {
		case <-secondDone:
		default:
			close(secondDone)
		}
		return nil, nil
	}

	restartedListener := telegram.NewListener(mockAPI, ingester, sourceResolver, stateStore, telegram.ListenerOptions{
		PollTimeout: 1,
	})
	restartedListener.Start(ctx)
	<-secondDone
	restartedListener.Stop()

	// Verify resumed from offset 51
	if initialResumedOffset != 51 {
		t.Errorf("expected resumed offset 51, got %d", initialResumedOffset)
	}

	// Verify both messages were ingested without duplicates
	ingMu.Lock()
	defer ingMu.Unlock()
	if len(ingestedMessages) != 2 || ingestedMessages[0] != 1050 || ingestedMessages[1] != 1051 {
		t.Fatalf("expected [1050, 1051] ingested across restart, got %v", ingestedMessages)
	}
}

func TestPublicTelegramChannelMirroring(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	mockAPI := telegram.NewMockAPI()
	srcStore, destStore, routeStore, evStore, jobStore, pipe, worker := setupTelegramEngineTest(mockAPI)

	// Mock Telegram Web Preview Server
	htmlContent := `
<div class="tgme_widget_message_wrap">
  <div class="tgme_widget_message" data-post="technews/101">
    <div class="tgme_widget_message_text js-message_text"><b>Breaking Tech</b><br/>Go 1.24 released!</div>
    <time datetime="2026-10-16T12:00:00Z"></time>
  </div>
</div>
`
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		w.Write([]byte(htmlContent))
	}))
	defer ts.Close()

	// 1. Create public channel source (no bot credentials needed in source)
	src, err := srcStore.Create(ctx, &sources.Source{
		Name:        "Public News",
		AdapterType: "telegram_public",
		Config:      json.RawMessage(`{"channel_username":"technews"}`),
		Enabled:     true,
	})
	if err != nil {
		t.Fatalf("failed to create source: %v", err)
	}

	// 2. Create destination channel (where your bot is admin)
	dest, err := destStore.Create(ctx, &destinations.Destination{
		Name:        "My Backup Channel",
		AdapterType: "telegram",
		Config:      json.RawMessage(`{"telegram_chat_id":"-100888999"}`),
		Enabled:     true,
	})
	if err != nil {
		t.Fatalf("failed to create destination: %v", err)
	}

	// 3. Create route linking public channel to backup
	_, err = routeStore.Create(ctx, &routing.Route{
		SourceID:      src.ID,
		DestinationID: dest.ID,
		Enabled:       true,
	})
	if err != nil {
		t.Fatalf("failed to create route: %v", err)
	}

	// 4. Start worker
	worker.Start(ctx)
	defer worker.Stop()

	// 5. Setup state store with offset 100 so message 101 is detected as new
	stateStore := telegram.NewMemoryStateStore()
	_ = stateStore.SetLastMessageID(ctx, "technews", 100)

	scraper := telegram.NewPublicScraperWithClient(ts.Client(), ts.URL+"/")
	publicResolver := sources.NewPublicSourceResolver(srcStore)

	poller := telegram.NewPublicPoller(
		scraper,
		stateStore,
		telegram.IngesterFunc(func(ctx context.Context, sourceID string, rawData []byte) error {
			_, err := pipe.IngestRaw(ctx, sourceID, rawData)
			return err
		}),
		publicResolver,
		20*time.Millisecond,
	)

	poller.Start(ctx)
	defer poller.Stop()

	// Wait for delivery to complete
	deadline := time.Now().Add(3 * time.Second)
	var delivered bool
	for time.Now().Before(deadline) {
		jobs, _ := jobStore.List(ctx, delivery.ListFilter{})
		if len(jobs) > 0 && jobs[0].Status == delivery.StatusDelivered {
			delivered = true
			break
		}
		time.Sleep(20 * time.Millisecond)
	}

	if !delivered {
		t.Fatal("expected delivery job to be completed and marked delivered")
	}

	// Verify the bot delivered the message to the destination channel
	sentCount := len(mockAPI.SentChattables)
	var sentMsg tgbotapi.MessageConfig
	if sentCount > 0 {
		sentMsg = mockAPI.SentChattables[0].(tgbotapi.MessageConfig)
	}

	if sentCount != 1 {
		t.Fatalf("expected 1 message sent by bot, got %d", sentCount)
	}
	if sentMsg.ChatID != -100888999 {
		t.Errorf("expected destination chat -100888999, got %d", sentMsg.ChatID)
	}
	if sentMsg.Text != "<b>Breaking Tech</b>\nGo 1.24 released!" {
		t.Errorf("unexpected text delivered: %q", sentMsg.Text)
	}
	if sentMsg.ParseMode != "HTML" {
		t.Errorf("expected HTML parse mode, got %q", sentMsg.ParseMode)
	}

	// Verify event stored in event store
	evs, _ := evStore.List(ctx, event.ListFilter{})
	if len(evs) != 1 {
		t.Fatalf("expected 1 stored event, got %d", len(evs))
	}
	if evs[0].ExternalEventID != "telegram:public:technews:101" {
		t.Errorf("expected external id 'telegram:public:technews:101', got %q", evs[0].ExternalEventID)
	}
}

