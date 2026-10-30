package triggers

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"sync"
	"time"

	"kerob/internal/adapters/ssrf"
	"kerob/internal/core/event"
)

// HTTPPollerConfig configures a recurring HTTP poll trigger.
type HTTPPollerConfig struct {
	URL          string            `json:"url"`
	PollInterval string            `json:"poll_interval,omitempty"`
	ItemKey      string            `json:"item_key,omitempty"` // JSON field to track seen items, e.g. "id"
	Headers      map[string]string `json:"headers,omitempty"`
	Method       string            `json:"method,omitempty"`
}

// HTTPPoller polls external JSON endpoints with ETag / Last-Modified and deduplication.
type HTTPPoller struct {
	mu           sync.Mutex
	cfg          HTTPPollerConfig
	client       *http.Client
	lastETag     string
	lastModified string
	seenKeys     map[string]bool
}

// NewHTTPPoller creates an HTTPPoller with default SSRF-safe HTTP client.
func NewHTTPPoller(cfg HTTPPollerConfig) *HTTPPoller {
	return NewHTTPPollerWithClient(cfg, ssrf.NewSafeHTTPClient(30*time.Second))
}

// NewHTTPPollerWithClient creates an HTTPPoller with a custom client for testing.
func NewHTTPPollerWithClient(cfg HTTPPollerConfig, client *http.Client) *HTTPPoller {
	if cfg.Method == "" {
		cfg.Method = http.MethodGet
	}
	return &HTTPPoller{
		cfg:      cfg,
		client:   client,
		seenKeys: make(map[string]bool),
	}
}

// PollItem represents an extracted item from the polled endpoint.
type PollItem struct {
	Key     string         `json:"key"`
	Payload map[string]any `json:"payload"`
}

// Poll executes one polling cycle and returns newly discovered or changed items.
func (p *HTTPPoller) Poll(ctx context.Context) ([]PollItem, error) {
	p.mu.Lock()
	defer p.mu.Unlock()

	req, err := http.NewRequestWithContext(ctx, p.cfg.Method, p.cfg.URL, nil)
	if err != nil {
		return nil, fmt.Errorf("invalid poll request: %w", err)
	}

	for k, v := range p.cfg.Headers {
		req.Header.Set(k, v)
	}
	req.Header.Set("Accept", "application/json")

	if p.lastETag != "" {
		req.Header.Set("If-None-Match", p.lastETag)
	}
	if p.lastModified != "" {
		req.Header.Set("If-Modified-Since", p.lastModified)
	}

	resp, err := p.client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("http poll request failed: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode == http.StatusNotModified {
		return nil, nil // Nothing changed
	}

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 512))
		return nil, fmt.Errorf("http poll returned status %d: %s", resp.StatusCode, string(body))
	}

	// Capture caching headers
	if etag := resp.Header.Get("ETag"); etag != "" {
		p.lastETag = etag
	}
	if lm := resp.Header.Get("Last-Modified"); lm != "" {
		p.lastModified = lm
	}

	bodyBytes, err := io.ReadAll(io.LimitReader(resp.Body, 10*1024*1024))
	if err != nil {
		return nil, fmt.Errorf("failed to read response body: %w", err)
	}

	return p.extractItems(bodyBytes)
}

func (p *HTTPPoller) extractItems(data []byte) ([]PollItem, error) {
	var raw any
	if err := json.Unmarshal(data, &raw); err != nil {
		return nil, fmt.Errorf("failed to parse json response: %w", err)
	}

	var items []map[string]any
	switch v := raw.(type) {
	case []any:
		for _, el := range v {
			if m, ok := el.(map[string]any); ok {
				items = append(items, m)
			}
		}
	case map[string]any:
		// Check if there is an array field named "items", "data", "results"
		for _, key := range []string{"items", "data", "results", "records"} {
			if arr, ok := v[key].([]any); ok {
				for _, el := range arr {
					if m, ok := el.(map[string]any); ok {
						items = append(items, m)
					}
				}
				break
			}
		}
		if len(items) == 0 {
			items = append(items, v)
		}
	default:
		return nil, fmt.Errorf("unsupported json payload structure")
	}

	var newItems []PollItem
	for _, item := range items {
		key := p.computeItemKey(item)
		if p.seenKeys[key] {
			continue
		}
		p.seenKeys[key] = true
		newItems = append(newItems, PollItem{
			Key:     key,
			Payload: item,
		})
	}

	return newItems, nil
}

func (p *HTTPPoller) computeItemKey(item map[string]any) string {
	if p.cfg.ItemKey != "" {
		if val, ok := item[p.cfg.ItemKey]; ok && val != nil {
			return fmt.Sprintf("%v", val)
		}
	}
	for _, k := range []string{"id", "guid", "uuid", "key"} {
		if val, ok := item[k]; ok && val != nil {
			return fmt.Sprintf("%v", val)
		}
	}

	// Fallback to SHA256 of item
	serialized, _ := json.Marshal(item)
	h := sha256.Sum256(serialized)
	return hex.EncodeToString(h[:])
}

// ConvertToEvents converts new poll items into standard normalized Events.
func (p *HTTPPoller) ConvertToEvents(sourceID string, items []PollItem) []*event.Event {
	events := make([]*event.Event, 0, len(items))
	for _, item := range items {
		payloadBytes, _ := json.Marshal(item.Payload)
		metaBytes, _ := json.Marshal(map[string]string{
			"url":      p.cfg.URL,
			"item_key": item.Key,
		})
		ev := &event.Event{
			SourceID:        sourceID,
			ExternalEventID: fmt.Sprintf("http:%s:%s", sourceID, item.Key),
			EventType:       "http.poll",
			Payload:         payloadBytes,
			Metadata:        metaBytes,
			CreatedAt:       time.Now(),
		}
		events = append(events, ev)
	}
	return events
}
