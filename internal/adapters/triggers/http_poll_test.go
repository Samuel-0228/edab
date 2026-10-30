package triggers

import (
	"context"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
)

func TestHTTPPollerArrayAndDeduplication(t *testing.T) {
	var pollCount int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		count := atomic.AddInt32(&pollCount, 1)

		if r.Header.Get("If-None-Match") == `"etag-v1"` && count == 2 {
			w.WriteHeader(http.StatusNotModified)
			return
		}

		w.Header().Set("ETag", `"etag-v1"`)
		w.Header().Set("Last-Modified", "Wed, 21 Oct 2026 07:28:00 GMT")
		w.Header().Set("Content-Type", "application/json")

		if count == 1 {
			_, _ = w.Write([]byte(`[{"id": "item1", "val": "A"}, {"id": "item2", "val": "B"}]`))
		} else {
			// Poll 3: new item added
			w.Header().Set("ETag", `"etag-v2"`)
			_, _ = w.Write([]byte(`[{"id": "item1", "val": "A"}, {"id": "item2", "val": "B"}, {"id": "item3", "val": "C"}]`))
		}
	}))
	defer server.Close()

	poller := NewHTTPPollerWithClient(HTTPPollerConfig{
		URL:     server.URL,
		ItemKey: "id",
	}, server.Client())

	ctx := context.Background()

	// Poll 1: initial discovery
	items1, err := poller.Poll(ctx)
	if err != nil {
		t.Fatalf("poll 1 failed: %v", err)
	}
	if len(items1) != 2 {
		t.Fatalf("expected 2 items, got %d", len(items1))
	}
	if items1[0].Key != "item1" || items1[1].Key != "item2" {
		t.Errorf("unexpected keys: %v, %v", items1[0].Key, items1[1].Key)
	}

	// Poll 2: 304 Not Modified
	items2, err := poller.Poll(ctx)
	if err != nil {
		t.Fatalf("poll 2 failed: %v", err)
	}
	if len(items2) != 0 {
		t.Fatalf("expected 0 items on 304, got %d", len(items2))
	}

	// Poll 3: etag-v2 with new item
	items3, err := poller.Poll(ctx)
	if err != nil {
		t.Fatalf("poll 3 failed: %v", err)
	}
	if len(items3) != 1 {
		t.Fatalf("expected only 1 new item, got %d", len(items3))
	}
	if items3[0].Key != "item3" {
		t.Errorf("expected item3, got %s", items3[0].Key)
	}

	// Convert to events
	events := poller.ConvertToEvents("source_123", items3)
	if len(events) != 1 {
		t.Fatalf("expected 1 event, got %d", len(events))
	}
	if events[0].EventType != "http.poll" || events[0].SourceID != "source_123" {
		t.Errorf("unexpected event: %+v", events[0])
	}
}

func TestHTTPPollerSingleObjectAndError(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/error" {
			w.WriteHeader(http.StatusInternalServerError)
			_, _ = w.Write([]byte("server crashed"))
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"results": [{"guid": "rec-99", "status": "active"}]}`))
	}))
	defer server.Close()

	poller := NewHTTPPollerWithClient(HTTPPollerConfig{
		URL: server.URL + "/results",
	}, server.Client())

	items, err := poller.Poll(context.Background())
	if err != nil {
		t.Fatalf("expected success, got: %v", err)
	}
	if len(items) != 1 || items[0].Key != "rec-99" {
		t.Fatalf("unexpected items: %+v", items)
	}

	// Error endpoint
	errPoller := NewHTTPPollerWithClient(HTTPPollerConfig{
		URL: server.URL + "/error",
	}, server.Client())
	_, err500 := errPoller.Poll(context.Background())
	if err500 == nil {
		t.Fatal("expected error on 500")
	}
}
