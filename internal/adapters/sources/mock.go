package sources

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"sync"
	"time"

	"automation-engine/internal/core/event"
)

// MockSourceAdapter is a flexible source adapter used for testing and synthetic event generation.
type MockSourceAdapter struct {
	mu        sync.Mutex
	connected bool
	config    json.RawMessage
	pollQueue []RawEvent
}

func NewMockSourceAdapter() *MockSourceAdapter {
	return &MockSourceAdapter{
		pollQueue: make([]RawEvent, 0),
	}
}

func (a *MockSourceAdapter) Name() string {
	return "mock"
}

func (a *MockSourceAdapter) Connect(ctx context.Context, config json.RawMessage) error {
	a.mu.Lock()
	defer a.mu.Unlock()
	a.connected = true
	a.config = config
	return nil
}

func (a *MockSourceAdapter) Health(ctx context.Context) error {
	a.mu.Lock()
	defer a.mu.Unlock()
	if !a.connected {
		return fmt.Errorf("mock source adapter is not connected")
	}
	return nil
}

// QueueRawEvent enqueues a raw event for testing Poll().
func (a *MockSourceAdapter) QueueRawEvent(ev RawEvent) {
	a.mu.Lock()
	defer a.mu.Unlock()
	a.pollQueue = append(a.pollQueue, ev)
}

func (a *MockSourceAdapter) Poll(ctx context.Context) ([]RawEvent, error) {
	a.mu.Lock()
	defer a.mu.Unlock()
	if len(a.pollQueue) == 0 {
		return nil, nil
	}
	events := a.pollQueue
	a.pollQueue = make([]RawEvent, 0)
	return events, nil
}

// Normalize converts arbitrary input data into a generic *event.Event.
func (a *MockSourceAdapter) Normalize(ctx context.Context, sourceID string, raw []byte) (*event.Event, error) {
	if len(raw) == 0 {
		return nil, fmt.Errorf("empty raw event data")
	}

	// Try to decode as structured envelope
	var envelope struct {
		ExternalID string          `json:"external_id"`
		Type       string          `json:"type"`
		Payload    json.RawMessage `json:"payload"`
		Metadata   json.RawMessage `json:"metadata"`
	}

	if err := json.Unmarshal(raw, &envelope); err == nil && (envelope.ExternalID != "" || envelope.Payload != nil) {
		extID := envelope.ExternalID
		if extID == "" {
			hash := sha256.Sum256(raw)
			extID = hex.EncodeToString(hash[:8])
		}
		eventType := envelope.Type
		if eventType == "" {
			eventType = "mock.event"
		}
		payload := envelope.Payload
		if len(payload) == 0 {
			payload = json.RawMessage(raw)
		}
		return event.New(sourceID, extID, eventType, payload, envelope.Metadata)
	}

	// If not structured envelope, treat raw as payload and compute deterministic external ID
	hash := sha256.Sum256(raw)
	extID := hex.EncodeToString(hash[:12])
	return event.New(sourceID, extID, "raw.event", raw, map[string]any{"normalized_at": time.Now().UTC()})
}
