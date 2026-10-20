package sources

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"net/http"
	"sync"
	"time"

	"kerob/internal/core/event"
)

// WebhookConfig defines configuration options for the generic webhook source adapter.
type WebhookConfig struct {
	HeaderSecret string `json:"header_secret,omitempty"`
	Secret       string `json:"secret,omitempty"`
}

// ValidateRequest authenticates a webhook before its payload reaches the
// normalization pipeline. Secret comparison is constant-time.
func ValidateRequest(config json.RawMessage, headers http.Header, body []byte) error {
	var cfg WebhookConfig
	if len(config) > 0 && string(config) != "{}" {
		if err := json.Unmarshal(config, &cfg); err != nil {
			return fmt.Errorf("invalid webhook config: %w", err)
		}
	}
	expectedSecret := cfg.Secret
	if expectedSecret == "" {
		expectedSecret = cfg.HeaderSecret
	}
	if expectedSecret == "" {
		return nil
	}

	provided := headers.Get("X-Webhook-Secret")
	if subtle.ConstantTimeCompare([]byte(provided), []byte(expectedSecret)) != 1 {
		return fmt.Errorf("invalid webhook secret")
	}

	signature := headers.Get("X-Webhook-Signature-256")
	if signature != "" {
		mac := hmac.New(sha256.New, []byte(expectedSecret))
		_, _ = mac.Write(body)
		expected := "sha256=" + hex.EncodeToString(mac.Sum(nil))
		if subtle.ConstantTimeCompare([]byte(signature), []byte(expected)) != 1 {
			return fmt.Errorf("invalid webhook signature")
		}
	}
	return nil
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
