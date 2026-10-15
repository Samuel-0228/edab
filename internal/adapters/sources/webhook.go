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

// WebhookConfig defines configuration options for the generic webhook source adapter.
type WebhookConfig struct {
	HeaderSecret string `json:"header_secret,omitempty"`
	Secret       string `json:"secret,omitempty"`
}

// WebhookSourceAdapter ingests raw HTTP push notifications/webhooks.
type WebhookSourceAdapter struct {
	mu        sync.Mutex
	connected bool
	config    WebhookConfig
}

func NewWebhookSourceAdapter() *WebhookSourceAdapter {
	return &WebhookSourceAdapter{}
}

func (w *WebhookSourceAdapter) Name() string {
	return "webhook"
}

func (w *WebhookSourceAdapter) Connect(ctx context.Context, config json.RawMessage) error {
	w.mu.Lock()
	defer w.mu.Unlock()
	if len(config) > 0 && string(config) != "{}" {
		_ = json.Unmarshal(config, &w.config)
	}
	w.connected = true
	return nil
}

func (w *WebhookSourceAdapter) Health(ctx context.Context) error {
	w.mu.Lock()
	defer w.mu.Unlock()
	if !w.connected {
		return fmt.Errorf("webhook source adapter is not connected")
	}
	return nil
}

func (w *WebhookSourceAdapter) Poll(ctx context.Context) ([]RawEvent, error) {
	// Webhooks are push-based, no polling needed
	return nil, nil
}

func (w *WebhookSourceAdapter) Normalize(ctx context.Context, sourceID string, raw []byte) (*event.Event, error) {
	if len(raw) == 0 {
		return nil, fmt.Errorf("empty webhook body")
	}

	var parsed map[string]any
	if err := json.Unmarshal(raw, &parsed); err == nil {
		extID := ""
		// Look for common ID field names
		for _, key := range []string{"id", "event_id", "external_id", "message_id", "delivery_id"} {
			if v, ok := parsed[key]; ok {
				extID = fmt.Sprintf("%v", v)
				break
			}
		}
		if extID == "" {
			hash := sha256.Sum256(raw)
			extID = hex.EncodeToString(hash[:10])
		}

		eventType := "webhook.generic"
		for _, key := range []string{"type", "event_type", "action", "event"} {
			if v, ok := parsed[key]; ok {
				eventType = fmt.Sprintf("%v", v)
				break
			}
		}

		metadata := map[string]any{
			"ingested_via": "webhook",
			"received_at":  time.Now().UTC().Format(time.RFC3339),
		}

		return event.New(sourceID, extID, eventType, raw, metadata)
	}

	// Plain text or non-JSON body
	hash := sha256.Sum256(raw)
	extID := hex.EncodeToString(hash[:10])
	metadata := map[string]any{
		"ingested_via": "webhook",
		"content_type": "text/plain",
	}
	return event.New(sourceID, extID, "webhook.text", raw, metadata)
}
