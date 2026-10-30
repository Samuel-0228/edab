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
	"kerob/internal/telegram"
)

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
		_, _ = w.Write([]byte(htmlContent))
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
		if msg, ok := mockAPI.SentChattables[0].(tgbotapi.MessageConfig); ok {
			sentMsg = msg
		} else {
			t.Fatalf("expected tgbotapi.MessageConfig, got %T", mockAPI.SentChattables[0])
		}
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
