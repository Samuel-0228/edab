package triggers

import (
	"context"
	"encoding/xml"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"sync"
	"time"

	"kerob/internal/adapters/ssrf"
	"kerob/internal/core/event"
)

// RSS XML structs supporting RSS 2.0 and Atom feeds
type rssRoot struct {
	XMLName xml.Name   `xml:"rss"`
	Channel rssChannel `xml:"channel"`
}

type rssChannel struct {
	Title       string    `xml:"title"`
	Link        string    `xml:"link"`
	Description string    `xml:"description"`
	Items       []rssItem `xml:"item"`
}

type rssItem struct {
	Title       string `xml:"title"`
	Link        string `xml:"link"`
	Description string `xml:"description"`
	GUID        string `xml:"guid"`
	PubDate     string `xml:"pubDate"`
	Author      string `xml:"author"`
}

// Atom XML structs
type atomFeed struct {
	XMLName xml.Name    `xml:"feed"`
	Title   string      `xml:"title"`
	Entries []atomEntry `xml:"entry"`
}

type atomEntry struct {
	Title     string    `xml:"title"`
	Link      atomLink  `xml:"link"`
	Summary   string    `xml:"summary"`
	Content   string    `xml:"content"`
	ID        string    `xml:"id"`
	Updated   string    `xml:"updated"`
	Published string    `xml:"published"`
	Author    atomAuthor `xml:"author"`
}

type atomLink struct {
	Href string `xml:"href,attr"`
}

type atomAuthor struct {
	Name string `xml:"name"`
}

// Ingester is the callback to send normalized events to the core engine.
type Ingester interface {
	ProcessEvent(ctx context.Context, ev *event.Event) error
}

// IngesterFunc allows plain functions to act as Ingester.
type IngesterFunc func(ctx context.Context, ev *event.Event) error

func (f IngesterFunc) ProcessEvent(ctx context.Context, ev *event.Event) error {
	return f(ctx, ev)
}

// RSSPoller periodically fetches and normalizes RSS/Atom feeds.
type RSSPoller struct {
	sourceID  string
	feedURL   string
	interval  time.Duration
	ingester  Ingester
	client    *http.Client
	seenMu    sync.RWMutex
	seenGUIDs map[string]bool
	stopChan  chan struct{}
}

func NewRSSPoller(sourceID, feedURL string, interval time.Duration, ingester Ingester) *RSSPoller {
	if interval <= 0 {
		interval = 5 * time.Minute
	}
	return &RSSPoller{
		sourceID:  sourceID,
		feedURL:   feedURL,
		interval:  interval,
		ingester:  ingester,
		client:    ssrf.NewSafeHTTPClient(15 * time.Second),
		seenGUIDs: make(map[string]bool),
		stopChan:  make(chan struct{}),
	}
}

func (p *RSSPoller) Start(ctx context.Context) {
	go func() {
		// Run initial poll immediately
		if err := p.PollOnce(ctx); err != nil {
			slog.Warn("initial rss poll error", slog.String("feed", p.feedURL), slog.String("error", err.Error()))
		}

		ticker := time.NewTicker(p.interval)
		defer ticker.Stop()

		for {
			select {
			case <-ticker.C:
				if err := p.PollOnce(ctx); err != nil {
					slog.Warn("rss poll error", slog.String("feed", p.feedURL), slog.String("error", err.Error()))
				}
			case <-p.stopChan:
				return
			case <-ctx.Done():
				return
			}
		}
	}()
}

func (p *RSSPoller) Stop() {
	close(p.stopChan)
}

// PollOnce executes a single fetch and parses items.
func (p *RSSPoller) PollOnce(ctx context.Context) error {
	if err := ssrf.ValidateURL(ctx, p.feedURL); err != nil {
		return fmt.Errorf("feed url blocked by SSRF: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, p.feedURL, nil)
	if err != nil {
		return err
	}
	req.Header.Set("User-Agent", "kerob/1.0 (RSS Reader)")

	resp, err := p.client.Do(req)
	if err != nil {
		return fmt.Errorf("failed to fetch feed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return fmt.Errorf("feed returned HTTP status %d", resp.StatusCode)
	}

	body, err := io.ReadAll(io.LimitReader(resp.Body, 5*1024*1024)) // 5 MB max
	if err != nil {
		return fmt.Errorf("failed to read feed body: %w", err)
	}

	// Try RSS 2.0 first
	var rss rssRoot
	if err := xml.Unmarshal(body, &rss); err == nil && len(rss.Channel.Items) > 0 {
		return p.processRSSItems(ctx, rss.Channel.Items)
	}

	// Try Atom
	var atom atomFeed
	if err := xml.Unmarshal(body, &atom); err == nil && len(atom.Entries) > 0 {
		return p.processAtomEntries(ctx, atom.Entries)
	}

	return nil
}

func (p *RSSPoller) processRSSItems(ctx context.Context, items []rssItem) error {
	for _, it := range items {
		guid := it.GUID
		if guid == "" {
			guid = it.Link
		}
		if guid == "" {
			guid = it.Title
		}

		p.seenMu.Lock()
		if p.seenGUIDs[guid] {
			p.seenMu.Unlock()
			continue
		}
		p.seenGUIDs[guid] = true
		p.seenMu.Unlock()

		payload := map[string]any{
			"title":        it.Title,
			"link":         it.Link,
			"description":  it.Description,
			"guid":         guid,
			"published_at": it.PubDate,
			"author":       it.Author,
		}

		externalID := fmt.Sprintf("rss:%s", guid)
		ev, err := event.New(p.sourceID, externalID, "rss.item", payload, map[string]any{
			"feed_url": p.feedURL,
		})
		if err != nil {
			continue
		}

		_ = p.ingester.ProcessEvent(ctx, ev)
	}
	return nil
}

func (p *RSSPoller) processAtomEntries(ctx context.Context, entries []atomEntry) error {
	for _, e := range entries {
		guid := e.ID
		if guid == "" {
			guid = e.Link.Href
		}
		if guid == "" {
			guid = e.Title
		}

		p.seenMu.Lock()
		if p.seenGUIDs[guid] {
			p.seenMu.Unlock()
			continue
		}
		p.seenGUIDs[guid] = true
		p.seenMu.Unlock()

		summary := e.Summary
		if summary == "" {
			summary = e.Content
		}

		pub := e.Published
		if pub == "" {
			pub = e.Updated
		}

		payload := map[string]any{
			"title":        e.Title,
			"link":         e.Link.Href,
			"description":  summary,
			"guid":         guid,
			"published_at": pub,
			"author":       e.Author.Name,
		}

		externalID := fmt.Sprintf("atom:%s", guid)
		ev, err := event.New(p.sourceID, externalID, "rss.item", payload, map[string]any{
			"feed_url": p.feedURL,
		})
		if err != nil {
			continue
		}

		_ = p.ingester.ProcessEvent(ctx, ev)
	}
	return nil
}
