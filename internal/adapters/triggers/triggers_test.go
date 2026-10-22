package triggers

import (
	"bytes"
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"kerob/internal/core/event"
)

type roundTripperFunc func(req *http.Request) (*http.Response, error)

func (f roundTripperFunc) RoundTrip(req *http.Request) (*http.Response, error) {
	return f(req)
}

func mockHTTPClient(handler func(req *http.Request) (*http.Response, error)) *http.Client {
	return &http.Client{
		Transport: roundTripperFunc(handler),
	}
}

func TestValidateGitHubSignature(t *testing.T) {
	secret := "my-secret-webhook-key"
	body := []byte(`{"action":"opened","issue":{"title":"Test Issue"}}`)

	// 1. Valid signature
	mac := hmac.New(sha256.New, []byte(secret))
	mac.Write(body)
	validSig := "sha256=" + hex.EncodeToString(mac.Sum(nil))

	if !ValidateGitHubSignature(secret, body, validSig) {
		t.Fatal("expected valid signature to pass verification")
	}

	// 2. Empty secret allows any
	if !ValidateGitHubSignature("", body, "any-sig") {
		t.Fatal("expected empty secret to allow validation")
	}

	// 3. Missing header
	if ValidateGitHubSignature(secret, body, "") {
		t.Fatal("expected empty signature header to fail")
	}

	// 4. Invalid hex signature
	if ValidateGitHubSignature(secret, body, "sha256=invalidhexsignature1234567890") {
		t.Fatal("expected invalid signature to fail verification")
	}

	// 5. Tampered body
	if ValidateGitHubSignature(secret, []byte(`tampered body`), validSig) {
		t.Fatal("expected tampered body to fail verification")
	}
}

func TestNormalizeGitHubEvent(t *testing.T) {
	// Issue opened
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

	// Pull request merged
	prBody := []byte(`{
		"action": "closed",
		"pull_request": {
			"number": 105,
			"title": "Add Svelte UI",
			"merged": true,
			"html_url": "https://github.com/org/repo/pull/105"
		},
		"repository": {"full_name": "org/repo"}
	}`)
	reqPR := httptest.NewRequest(http.MethodPost, "/webhooks/github", nil)
	reqPR.Header.Set("X-GitHub-Event", "pull_request")

	evPR, err := NormalizeGitHubEvent("src-gh", reqPR, prBody)
	if err != nil {
		t.Fatalf("Normalize pull_request failed: %v", err)
	}
	if evPR.EventType != "github.pull_request.closed" {
		t.Errorf("expected EventType 'github.pull_request.closed', got %s", evPR.EventType)
	}

	// Push event
	pushBody := []byte(`{
		"ref": "refs/heads/main",
		"commits": [{"id": "abc1234", "message": "feat: init"}],
		"repository": {"full_name": "org/repo"}
	}`)
	reqPush := httptest.NewRequest(http.MethodPost, "/webhooks/github", nil)
	reqPush.Header.Set("X-GitHub-Event", "push")

	evPush, err := NormalizeGitHubEvent("src-gh", reqPush, pushBody)
	if err != nil || evPush.EventType != "github.push" {
		t.Fatalf("Normalize push failed: %v (eventType: %s)", err, evPush.EventType)
	}

	// Malformed JSON
	reqBad := httptest.NewRequest(http.MethodPost, "/webhooks/github", nil)
	_, err = NormalizeGitHubEvent("src-gh", reqBad, []byte(`{bad-json`))
	if err == nil {
		t.Fatal("expected error on malformed JSON")
	}
}

func TestRSSPollerWithClient(t *testing.T) {
	sampleRSS := `<?xml version="1.0" encoding="UTF-8"?>
	<rss version="2.0">
	  <channel>
	    <title>Tech News</title>
	    <link>https://8.8.8.8</link>
	    <description>Daily tech news</description>
	    <item>
	      <title>Go 1.24 is out</title>
	      <link>https://8.8.8.8/go-124</link>
	      <description>Great new features</description>
	      <guid>guid-124</guid>
	      <pubDate>Mon, 20 Oct 2026 10:00:00 GMT</pubDate>
	    </item>
	  </channel>
	</rss>`

	sampleAtom := `<?xml version="1.0" encoding="utf-8"?>
	<feed xmlns="http://www.w3.org/2005/Atom">
	  <title>Atom Example</title>
	  <entry>
	    <title>Atom Post Title</title>
	    <link href="https://8.8.8.8/atom-post" />
	    <id>urn:uuid:12345</id>
	    <summary>Atom summary text</summary>
	  </entry>
	</feed>`

	var receivedEvents []*event.Event
	ingester := IngesterFunc(func(ctx context.Context, ev *event.Event) error {
		receivedEvents = append(receivedEvents, ev)
		return nil
	})

	// 1. RSS 2.0 Poll
	mockClient := mockHTTPClient(func(req *http.Request) (*http.Response, error) {
		return &http.Response{
			StatusCode: http.StatusOK,
			Body:       io.NopCloser(strings.NewReader(sampleRSS)),
			Header:     make(http.Header),
		}, nil
	})

	poller := NewRSSPollerWithClient("src-rss", "https://8.8.8.8/feed.xml", 1*time.Minute, ingester, mockClient)

	if err := poller.PollOnce(context.Background()); err != nil {
		t.Fatalf("PollOnce RSS failed: %v", err)
	}

	if len(receivedEvents) != 1 || receivedEvents[0].EventType != "rss.item" {
		t.Fatalf("expected 1 rss.item event, got %d", len(receivedEvents))
	}

	// Poll again with same GUID: deduplication should drop it
	if err := poller.PollOnce(context.Background()); err != nil {
		t.Fatalf("PollOnce duplicate check failed: %v", err)
	}
	if len(receivedEvents) != 1 {
		t.Fatalf("expected duplicate item to be ignored, got count=%d", len(receivedEvents))
	}

	// 2. Atom Feed Poll
	mockAtomClient := mockHTTPClient(func(req *http.Request) (*http.Response, error) {
		return &http.Response{
			StatusCode: http.StatusOK,
			Body:       io.NopCloser(strings.NewReader(sampleAtom)),
			Header:     make(http.Header),
		}, nil
	})

	atomPoller := NewRSSPollerWithClient("src-atom", "https://8.8.8.8/atom.xml", 1*time.Minute, ingester, mockAtomClient)
	if err := atomPoller.PollOnce(context.Background()); err != nil {
		t.Fatalf("PollOnce Atom failed: %v", err)
	}
	if len(receivedEvents) != 2 || receivedEvents[1].EventType != "rss.item" {
		t.Fatalf("expected rss.item event, got %+v", receivedEvents[len(receivedEvents)-1])
	}

	// 3. HTTP 500 error
	mockErrClient := mockHTTPClient(func(req *http.Request) (*http.Response, error) {
		return &http.Response{
			StatusCode: http.StatusInternalServerError,
			Body:       io.NopCloser(bytes.NewReader(nil)),
			Header:     make(http.Header),
		}, nil
	})
	errPoller := NewRSSPollerWithClient("src-err", "https://8.8.8.8/500.xml", 1*time.Minute, ingester, mockErrClient)
	if err := errPoller.PollOnce(context.Background()); err == nil {
		t.Fatal("expected error on HTTP 500 feed response")
	}

	// 4. SSRF blocked private IP URL
	ssrfPoller := NewRSSPoller("src-ssrf", "http://127.0.0.1:8080/feed.xml", 1*time.Minute, ingester)
	if err := ssrfPoller.PollOnce(context.Background()); err == nil {
		t.Fatal("expected SSRF error blocking 127.0.0.1 feed URL")
	}
}
