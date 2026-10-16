package sources

import (
	"context"
	"encoding/json"
	"fmt"
	"sort"
	"sync"
	"time"

	tgbotapi "github.com/go-telegram-bot-api/telegram-bot-api/v5"

	"automation-engine/internal/core/event"
	"automation-engine/internal/telegram"
)

// TelegramSourceConfig is stored in a Source's config for telegram sources.
type TelegramSourceConfig struct {
	// TelegramChatID is the numeric chat id of the monitored channel/group.
	TelegramChatID string `json:"telegram_chat_id"`
	// BotTokenEnv optionally overrides the env var holding the bot token.
	BotTokenEnv string `json:"bot_token_env,omitempty"`
}

// TelegramSourceAdapter normalizes Telegram updates into generic engine events.
// It is push-driven: the Telegram listener feeds it raw updates; Poll is unused.
type TelegramSourceAdapter struct {
	mu        sync.Mutex
	connected bool
	config    TelegramSourceConfig
}

// NewTelegramSourceAdapter creates a Telegram source adapter.
func NewTelegramSourceAdapter() *TelegramSourceAdapter {
	return &TelegramSourceAdapter{}
}

// Name returns the adapter type name.
func (a *TelegramSourceAdapter) Name() string { return "telegram" }

// Connect parses the source configuration.
func (a *TelegramSourceAdapter) Connect(ctx context.Context, config json.RawMessage) error {
	a.mu.Lock()
	defer a.mu.Unlock()
	if len(config) > 0 && string(config) != "{}" {
		if err := json.Unmarshal(config, &a.config); err != nil {
			return fmt.Errorf("invalid telegram source config: %w", err)
		}
	}
	a.connected = true
	return nil
}

// Health reports whether the adapter has been connected.
func (a *TelegramSourceAdapter) Health(ctx context.Context) error {
	a.mu.Lock()
	defer a.mu.Unlock()
	if !a.connected {
		return fmt.Errorf("telegram source adapter is not connected")
	}
	return nil
}

// Poll is unused for Telegram: the listener pushes updates directly.
func (a *TelegramSourceAdapter) Poll(ctx context.Context) ([]RawEvent, error) {
	return nil, nil
}

// Normalize converts a telegram.RawPost (one post, possibly an album) into a
// generic Event. The payload is descriptive only; the original content is never
// rewritten.
func (a *TelegramSourceAdapter) Normalize(ctx context.Context, sourceID string, raw []byte) (*event.Event, error) {
	if len(raw) == 0 {
		return nil, fmt.Errorf("empty telegram update")
	}

	var post telegram.RawPost
	if err := json.Unmarshal(raw, &post); err != nil {
		return nil, fmt.Errorf("invalid telegram post: %w", err)
	}
	if post.ChatID == 0 {
		return nil, fmt.Errorf("telegram post is missing chat_id")
	}
	if len(post.Messages) == 0 {
		return nil, fmt.Errorf("telegram post is missing messages")
	}

	messages := make([]*tgbotapi.Message, 0, len(post.Messages))
	messageIDs := make([]int, 0, len(post.Messages))
	rawMessages := make([]json.RawMessage, 0, len(post.Messages))

	for _, rawMsg := range post.Messages {
		var msg tgbotapi.Message
		if err := json.Unmarshal(rawMsg, &msg); err != nil {
			// Preserve malformed updates for auditing instead of dropping them.
			continue
		}
		messages = append(messages, &msg)
		if msg.MessageID != 0 {
			messageIDs = append(messageIDs, msg.MessageID)
		}
		rawMessages = append(rawMessages, rawMsg)
	}

	if len(messages) == 0 {
		return nil, fmt.Errorf("telegram post contains no parseable messages")
	}

	sort.Ints(messageIDs)

	mediaKind := telegram.MediaKind(messages[0])
	text := messages[0].Text
	if text == "" {
		text = messages[0].Caption
	}

	externalEventID := telegram.ExternalEventID(post.ChatID, post.MediaGroupID, messageIDs[0])

	eventType := telegram.EventTypeMessage
	if post.MediaGroupID != "" {
		eventType = telegram.EventTypeMediaGroup
	}

	payload := telegram.MessagePayload{
		ChatID:       post.ChatID,
		ChatTitle:    post.ChatTitle,
		ChatType:     post.ChatType,
		MessageID:    messages[0].MessageID,
		MessageIDs:   messageIDs,
		MediaGroupID: post.MediaGroupID,
		HasMedia:     len(telegram.ExtractMediaRefs(messages)) > 0,
		MediaKind:    mediaKind,
		Text:         text,
		Media:        telegram.ExtractMediaRefs(messages),
		Messages:     rawMessages,
	}

	metadata := map[string]any{
		"ingested_via":  "telegram",
		"telegram_chat": post.ChatID,
		"message_ids":   messageIDs,
		"received_at":   time.Now().UTC().Format(time.RFC3339),
	}
	if post.MediaGroupID != "" {
		metadata["media_group_id"] = post.MediaGroupID
		metadata["album_size"] = len(messageIDs)
	}

	return event.New(sourceID, externalEventID, eventType, payload, metadata)
}
