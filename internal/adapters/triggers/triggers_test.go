package triggers

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/xml"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"kerob/internal/core/event"
)

func TestValidateGitHubSignature(t *testing.T) {
	secret := "my-secret-webhook-key"
	body := []byte(`{"action":"opened","issue":{"title":"Test Issue"}}`)

	// Generate valid signature
	mac := hmac.New(sha256.New, []byte(secret))
	mac.Write(body)
	validSig := "sha256=" + hex.EncodeToString(mac.Sum(nil))

	if !ValidateGitHubSignature(secret, body, validSig) {
		t.Fatal("expected valid signature to pass verification")
	}

	if ValidateGitHubSignature(secret, body, "sha256=invalidhexsignature1234567890") {
		t.Fatal("expected invalid signature to fail verification")
	}

	if ValidateGitHubSignature(secret, []byte(`tampered body`), validSig) {
		t.Fatal("expected tampered body to fail verification")
	}
}

func TestNormalizeGitHubEvent(t *testing.T) {
	body := []byte(`{
		"action": "opened",
		"issue": {
			"number": 42,
			"title": "Fix memory leak",
			"body": "Detailed report here...",
			"html_url": "https://github.com/org/repo/issues/42",
			"user": {"login": "gopher"}
		},
		"repository": {
			"full_name": "org/repo"
		}
	}`)

	req := httptest.NewRequest(http.MethodPost, "/webhooks/github", nil)
	req.Header.Set("X-GitHub-Event", "issues")
	req.Header.Set("X-GitHub-Delivery", "delivery-uuid-99")

	ev, err := NormalizeGitHubEvent("src-gh", req, body)
	if err != nil {
		t.Fatalf("NormalizeGitHubEvent failed: %v", err)
	}

	if ev.EventType != "github.issue.opened" {
		t.Errorf("expected EventType 'github.issue.opened', got %s", ev.EventType)
	}
	if ev.ExternalEventID != "github:issues:delivery-uuid-99" {
		t.Errorf("expected external ID 'github:issues:delivery-uuid-99', got %s", ev.ExternalEventID)
	}
}

func TestRSSPollerParse(t *testing.T) {
	sampleRSS := `<?xml version="1.0" encoding="UTF-8"?>
	<rss version="2.0">
	  <channel>
	    <title>Tech News</title>
	    <link>https://example.com</link>
	    <description>Daily tech news</description>
	    <item>
	      <title>Go 1.24 is out</title>
	      <link>https://example.com/go-124</link>
	      <description>Great new features</description>
	      <guid>https://example.com/go-124</guid>
	      <pubDate>Mon, 20 Oct 2026 10:00:00 GMT</pubDate>
	    </item>
	  </channel>
	</rss>`

	var receivedEvents []*event.Event
	ingester := IngesterFunc(func(ctx context.Context, ev *event.Event) error {
		receivedEvents = append(receivedEvents, ev)
		return nil
	})

	poller := NewRSSPoller("src-rss", "https://example.com/rss.xml", 1*time.Minute, ingester)

	// Test processing items directly
	var rss rssRoot
	_ = xmlUnmarshal([]byte(sampleRSS), &rss)

	err := poller.processRSSItems(context.Background(), rss.Channel.Items)
	if err != nil {
		t.Fatalf("processRSSItems failed: %v", err)
	}

	if len(receivedEvents) != 1 {
		t.Fatalf("expected 1 event, got %d", len(receivedEvents))
	}
	ev := receivedEvents[0]
	if ev.EventType != "rss.item" {
		t.Errorf("expected EventType 'rss.item', got %s", ev.EventType)
	}
}

func xmlUnmarshal(data []byte, v any) error {
	return xml.Unmarshal(data, v)
}
