package telegram

import (
	"context"
	"encoding/json"
	"sync"
	"testing"
	"time"

	tgbotapi "github.com/go-telegram-bot-api/telegram-bot-api/v5"
)

type mockIngester struct {
	mu    sync.Mutex
	calls []struct {
		SourceID string
		RawPost  RawPost
	}
}

func (m *mockIngester) IngestRaw(ctx context.Context, sourceID string, rawData []byte) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	var post RawPost
	_ = json.Unmarshal(rawData, &post)
	m.calls = append(m.calls, struct {
		SourceID string
		RawPost  RawPost
	}{SourceID: sourceID, RawPost: post})
	return nil
}

func (m *mockIngester) CallCount() int {
	m.mu.Lock()
	defer m.mu.Unlock()
	return len(m.calls)
}

func (m *mockIngester) GetCall(i int) (string, RawPost) {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.calls[i].SourceID, m.calls[i].RawPost
}

func TestListenerSingleMessage(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	mockAPI := NewMockAPI()
	ingester := &mockIngester{}
	state := NewMemoryStateStore()

	resolver := SourceResolverFunc(func(ctx context.Context, chatID int64) (string, bool, error) {
		if chatID == -100123 {
			return "src-1", true, nil
		}
		return "", false, nil
	})

	listener := NewListener(mockAPI, ingester, resolver, state, ListenerOptions{
		PollTimeout:    1,
		MediaGroupWait: 50 * time.Millisecond,
	})

	updateReceived := make(chan struct{})

	mockAPI.GetUpdatesFunc = func(cfg tgbotapi.UpdateConfig) ([]tgbotapi.Update, error) {
		if cfg.Offset == -1 {
			return nil, nil
		}
		if cfg.Offset == 0 {
			return []tgbotapi.Update{
				{
					UpdateID: 101,
					ChannelPost: &tgbotapi.Message{
						MessageID: 55,
						Chat: &tgbotapi.Chat{
							ID:    -100123,
							Title: "Monitored Channel",
							Type:  "channel",
						},
						Text: "Hello world!",
					},
				},
			}, nil
		}
		select {
		case <-updateReceived:
		default:
			close(updateReceived)
		}
		select {
		case <-ctx.Done():
		case <-time.After(100 * time.Millisecond):
		}
		return nil, nil
	}

	listener.Start(ctx)

	select {
	case <-updateReceived:
	case <-time.After(2 * time.Second):
		t.Fatal("timed out waiting for update")
	}

	listener.Stop()

	if ingester.CallCount() != 1 {
		t.Fatalf("expected 1 ingestion call, got %d", ingester.CallCount())
	}
	srcID, post := ingester.GetCall(0)
	if srcID != "src-1" {
		t.Errorf("expected source src-1, got %s", srcID)
	}
	if post.ChatID != -100123 {
		t.Errorf("expected chat id -100123, got %d", post.ChatID)
	}
	if len(post.Messages) != 1 {
		t.Fatalf("expected 1 message in post, got %d", len(post.Messages))
	}

	lastID, err := state.GetLastUpdateID(context.Background())
	if err != nil || lastID != 101 {
		t.Errorf("expected last update id 101, got %d (err: %v)", lastID, err)
	}
}

func TestListenerUnmonitoredChatIgnored(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	mockAPI := NewMockAPI()
	ingester := &mockIngester{}
	state := NewMemoryStateStore()

	resolver := SourceResolverFunc(func(ctx context.Context, chatID int64) (string, bool, error) {
		if chatID == -100123 {
			return "src-1", true, nil
		}
		return "", false, nil
	})

	listener := NewListener(mockAPI, ingester, resolver, state, ListenerOptions{
		PollTimeout:    1,
		MediaGroupWait: 50 * time.Millisecond,
	})

	done := make(chan struct{})
	mockAPI.GetUpdatesFunc = func(cfg tgbotapi.UpdateConfig) ([]tgbotapi.Update, error) {
		if cfg.Offset == -1 {
			return nil, nil
		}
		if cfg.Offset == 0 {
			return []tgbotapi.Update{
				{
					UpdateID: 201,
					ChannelPost: &tgbotapi.Message{
						MessageID: 10,
						Chat: &tgbotapi.Chat{
							ID:    -100999, // Unmonitored!
							Title: "Random Channel",
							Type:  "channel",
						},
						Text: "Spam post",
					},
				},
			}, nil
		}
		select {
		case <-done:
		default:
			close(done)
		}
		select {
		case <-ctx.Done():
		case <-time.After(100 * time.Millisecond):
		}
		return nil, nil
	}

	listener.Start(ctx)
	<-done
	listener.Stop()

	if ingester.CallCount() != 0 {
		t.Fatalf("expected 0 ingestion calls for unmonitored chat, got %d", ingester.CallCount())
	}
}

func TestListenerMediaGroupBuffering(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	mockAPI := NewMockAPI()
	ingester := &mockIngester{}
	state := NewMemoryStateStore()

	resolver := SourceResolverFunc(func(ctx context.Context, chatID int64) (string, bool, error) {
		if chatID == -100555 {
			return "src-media", true, nil
		}
		return "", false, nil
	})

	listener := NewListener(mockAPI, ingester, resolver, state, ListenerOptions{
		PollTimeout:    1,
		MediaGroupWait: 60 * time.Millisecond,
	})

	done := make(chan struct{})
	mockAPI.GetUpdatesFunc = func(cfg tgbotapi.UpdateConfig) ([]tgbotapi.Update, error) {
		if cfg.Offset == -1 {
			return nil, nil
		}
		if cfg.Offset == 0 {
			return []tgbotapi.Update{
				{
					UpdateID: 301,
					ChannelPost: &tgbotapi.Message{
						MessageID:    1,
						MediaGroupID: "mg_999",
						Chat:         &tgbotapi.Chat{ID: -100555, Title: "Media Channel", Type: "channel"},
						Caption:      "Photo 1",
					},
				},
				{
					UpdateID: 302,
					ChannelPost: &tgbotapi.Message{
						MessageID:    2,
						MediaGroupID: "mg_999",
						Chat:         &tgbotapi.Chat{ID: -100555, Title: "Media Channel", Type: "channel"},
						Caption:      "Photo 2",
					},
				},
				{
					UpdateID: 303,
					ChannelPost: &tgbotapi.Message{
						MessageID:    3,
						MediaGroupID: "mg_999",
						Chat:         &tgbotapi.Chat{ID: -100555, Title: "Media Channel", Type: "channel"},
						Caption:      "Photo 3",
					},
				},
			}, nil
		}
		select {
		case <-done:
		default:
			close(done)
		}
		select {
		case <-ctx.Done():
		case <-time.After(100 * time.Millisecond):
		}
		return nil, nil
	}

	listener.Start(ctx)
	<-done
	time.Sleep(120 * time.Millisecond)
	listener.Stop()

	if ingester.CallCount() != 1 {
		t.Fatalf("expected 1 aggregated ingestion call for album, got %d", ingester.CallCount())
	}

	srcID, post := ingester.GetCall(0)
	if srcID != "src-media" {
		t.Errorf("expected source src-media, got %s", srcID)
	}
	if post.MediaGroupID != "mg_999" {
		t.Errorf("expected media_group_id mg_999, got %s", post.MediaGroupID)
	}
	if len(post.Messages) != 3 {
		t.Fatalf("expected 3 messages aggregated in album, got %d", len(post.Messages))
	}
}
