package sources

import (
	"context"
	"encoding/json"
	"fmt"
	"sync"
	"time"

	"automation-engine/internal/core/event"
	"automation-engine/internal/telegram"
)

// TelegramPublicSourceConfig is stored in a Source's config for public telegram channel sources.
type TelegramPublicSourceConfig struct {
	ChannelUsername string `json:"channel_username"`
	PollInterval    string `json:"poll_interval,omitempty"`
}

// TelegramPublicSourceAdapter normalizes scraped public channel posts into generic engine events.
type TelegramPublicSourceAdapter struct {
	mu        sync.Mutex
	connected bool
	config    TelegramPublicSourceConfig
}

// NewTelegramPublicSourceAdapter creates an adapter for public Telegram channels.
func NewTelegramPublicSourceAdapter() *TelegramPublicSourceAdapter {
	return &TelegramPublicSourceAdapter{}
}

// Name returns the adapter type name.
func (a *TelegramPublicSourceAdapter) Name() string { return "telegram_public" }

// Connect parses the source configuration.
func (a *TelegramPublicSourceAdapter) Connect(ctx context.Context, config json.RawMessage) error {
	a.mu.Lock()
	defer a.mu.Unlock()
	if len(config) > 0 && string(config) != "{}" {
		if err := json.Unmarshal(config, &a.config); err != nil {
			return fmt.Errorf("invalid telegram public source config: %w", err)
		}
	}
	a.config.ChannelUsername = telegram.NormalizeChannelName(a.config.ChannelUsername)
	a.connected = true
	return nil
}

// Health reports whether the adapter is connected and configured.
func (a *TelegramPublicSourceAdapter) Health(ctx context.Context) error {
	a.mu.Lock()
	defer a.mu.Unlock()
	if !a.connected {
		return fmt.Errorf("telegram public source adapter is not connected")
	}
	if a.config.ChannelUsername == "" {
		return fmt.Errorf("channel_username is required")
	}
	return nil
}

// Poll is handled by the PublicPoller background service.
func (a *TelegramPublicSourceAdapter) Poll(ctx context.Context) ([]RawEvent, error) {
	return nil, nil
}

// Normalize converts a telegram.PublicPost into a generic Event.
func (a *TelegramPublicSourceAdapter) Normalize(ctx context.Context, sourceID string, raw []byte) (*event.Event, error) {
	if len(raw) == 0 {
		return nil, fmt.Errorf("empty public post payload")
	}

	var post telegram.PublicPost
	if err := json.Unmarshal(raw, &post); err != nil {
		return nil, fmt.Errorf("invalid public post: %w", err)
	}

	channel := telegram.NormalizeChannelName(post.Channel)
	if channel == "" {
		return nil, fmt.Errorf("public post missing channel name")
	}
	if post.MessageID <= 0 {
		return nil, fmt.Errorf("public post missing message_id")
	}

	mediaKind := ""
	var mediaURLs []string
	var mediaRefs []telegram.MediaRef
	for _, m := range post.Media {
		mediaURLs = append(mediaURLs, m.URL)
		mediaRefs = append(mediaRefs, telegram.MediaRef{
			Kind: m.Kind,
			URL:  m.URL,
		})
	}

	if post.IsAlbum || len(post.Media) > 1 {
		mediaKind = "album"
	} else if len(post.Media) == 1 {
		mediaKind = post.Media[0].Kind
	}

	externalID := telegram.ExternalPublicEventID(channel, post.MessageID)

	payload := telegram.MessagePayload{
		ChannelUsername: channel,
		MessageID:       post.MessageID,
		MessageIDs:      []int{post.MessageID},
		HasMedia:        len(post.Media) > 0,
		MediaKind:       mediaKind,
		Text:            post.Text,
		HTMLText:        post.HTMLText,
		MediaURLs:       mediaURLs,
		Media:           mediaRefs,
	}

	createdAt := post.Date
	if createdAt.IsZero() {
		createdAt = time.Now().UTC()
	}

	eventType := "telegram.public_post"
	if post.IsAlbum {
		eventType = "telegram.media_group"
	}

	metadata := map[string]any{
		"ingested_via":     "telegram_public",
		"channel_username": channel,
		"message_id":       post.MessageID,
		"received_at":      time.Now().UTC().Format(time.RFC3339),
	}
	if post.IsAlbum {
		metadata["is_album"] = true
		metadata["media_count"] = len(post.Media)
	}

	return event.New(sourceID, externalID, eventType, payload, metadata)
}
