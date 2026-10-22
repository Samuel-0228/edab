package telegram

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
	"time"
)

func TestPublicPollerLifecycleAndPolling(t *testing.T) {
	ctx := context.Background()

	// 1. Defaults
	pollerDefault := NewPublicPoller(nil, NewMemoryStateStore(), nil, nil, 0)
	if pollerDefault.interval != 15*time.Second {
		t.Errorf("expected 15s default interval, got %v", pollerDefault.interval)
	}

	// 2. PollAll with nil resolver
	pollerDefault.pollAll(ctx)

	// 3. PollAll with resolver returning error
	errResolver := func(ctx context.Context) ([]PublicSourceInfo, error) {
		return nil, errors.New("database connection failed")
	}
	pollerErr := NewPublicPoller(nil, NewMemoryStateStore(), nil, errResolver, 10*time.Millisecond)
	pollerErr.pollAll(ctx)

	// 4. PollSource with empty channel
	stateStore := NewMemoryStateStore()
	pollerErr.pollSource(ctx, PublicSourceInfo{SourceID: "src-1", Channel: ""})

	// 5. Full End-to-End Poll with Mock HTTP Server
	var currentHTML atomic.Pointer[string]
	html1 := `
<!DOCTYPE html>
<html>
<body>
<div class="tgme_widget_message_wrap js-widget_message_wrap">
  <div class="tgme_widget_message text_not_supported_wrap js-widget_message" data-post="tech_news/101">
    <div class="tgme_widget_message_text js-message_text" dir="auto"><b>Post 101</b></div>
    <time datetime="2026-10-16T12:00:00Z"></time>
  </div>
</div>
</body>
</html>`
	currentHTML.Store(&html1)

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		_, _ = w.Write([]byte(*currentHTML.Load()))
	}))
	defer server.Close()

	scraper := NewPublicScraperWithClient(server.Client(), server.URL+"/")

	var ingestedCount atomic.Int32
	ingester := IngesterFunc(func(ctx context.Context, sourceID string, rawData []byte) error {
		ingestedCount.Add(1)
		return nil
	})

	resolver := func(ctx context.Context) ([]PublicSourceInfo, error) {
		return []PublicSourceInfo{
			{SourceID: "src-tech", Channel: "tech_news"},
		}, nil
	}

	poller := NewPublicPoller(scraper, stateStore, ingester, resolver, 20*time.Millisecond)

	// First poll: bookmarks initial offset to 101
	poller.pollSource(ctx, PublicSourceInfo{SourceID: "src-tech", Channel: "tech_news"})
	lastID, _ := stateStore.GetLastMessageID(ctx, "tech_news")
	if lastID != 101 {
		t.Errorf("expected initialized offset 101, got %d", lastID)
	}

	// New post 102 arrives
	html2 := `
<!DOCTYPE html>
<html>
<body>
<div class="tgme_widget_message_wrap js-widget_message_wrap">
  <div class="tgme_widget_message text_not_supported_wrap js-widget_message" data-post="tech_news/102">
    <div class="tgme_widget_message_text js-message_text" dir="auto"><b>Post 102 New!</b></div>
    <time datetime="2026-10-16T12:05:00Z"></time>
  </div>
</div>
</body>
</html>`
	currentHTML.Store(&html2)

	// Second poll: ingests post 102
	poller.pollSource(ctx, PublicSourceInfo{SourceID: "src-tech", Channel: "tech_news"})
	if ingestedCount.Load() != 1 {
		t.Errorf("expected 1 ingested post, got %d", ingestedCount.Load())
	}

	lastID, _ = stateStore.GetLastMessageID(ctx, "tech_news")
	if lastID != 102 {
		t.Errorf("expected updated message id 102, got %d", lastID)
	}

	// 6. Test Start and Stop background loop
	pollCtx, pollCancel := context.WithCancel(ctx)
	defer pollCancel()

	poller.Start(pollCtx)
	time.Sleep(50 * time.Millisecond)
	poller.Stop()
}
