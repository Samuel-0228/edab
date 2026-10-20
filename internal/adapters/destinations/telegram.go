package destinations

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"strings"
	"sync"

	tgbotapi "github.com/go-telegram-bot-api/telegram-bot-api/v5"

	"kerob/internal/core/event"
	"kerob/internal/telegram"
)

// TelegramDestinationConfig is stored in a Destination's config for telegram targets.
type TelegramDestinationConfig struct {
	// TelegramChatID is the numeric chat id of the destination channel/group.
	TelegramChatID string `json:"telegram_chat_id"`
	// BotTokenEnv optionally overrides the env var holding the bot token.
	BotTokenEnv string `json:"bot_token_env,omitempty"`
	// DisableNotification mirrors silently when true.
	DisableNotification bool `json:"disable_notification,omitempty"`
}

// telegramDeliveryResult is persisted as the delivery job result so that retries
// can detect an already-completed delivery.
type telegramDeliveryResult struct {
	ChatID     int64 `json:"chat_id"`
	MessageIDs []int `json:"message_ids"`
}

// TelegramDestinationAdapter mirrors events into a Telegram chat using the
// native copy APIs. It never rewrites content and never adds attribution.
type TelegramDestinationAdapter struct {
	mu        sync.Mutex
	connected bool
	config    TelegramDestinationConfig
	chatID    int64
	api       telegram.API
}

// NewTelegramDestinationAdapter creates a Telegram destination adapter. The API
// client is built lazily from the environment during Connect.
func NewTelegramDestinationAdapter() *TelegramDestinationAdapter {
	return &TelegramDestinationAdapter{}
}

// NewTelegramDestinationAdapterWithAPI creates an adapter with an injected API
// client (used by tests and custom deployments).
func NewTelegramDestinationAdapterWithAPI(api telegram.API) *TelegramDestinationAdapter {
	return &TelegramDestinationAdapter{api: api}
}

// SetAPI injects the Telegram API client.
func (d *TelegramDestinationAdapter) SetAPI(api telegram.API) {
	d.mu.Lock()
	defer d.mu.Unlock()
	d.api = api
}

// Name returns the adapter type name.
func (d *TelegramDestinationAdapter) Name() string { return "telegram" }

// Connect parses the destination configuration and resolves the target chat.
func (d *TelegramDestinationAdapter) Connect(ctx context.Context, config json.RawMessage) error {
	d.mu.Lock()
	defer d.mu.Unlock()

	if len(config) > 0 && string(config) != "{}" {
		if err := json.Unmarshal(config, &d.config); err != nil {
			return fmt.Errorf("invalid telegram destination config: %w", err)
		}
	}

	chatID, err := telegram.ParseChatID(d.config.TelegramChatID)
	if err != nil {
		return fmt.Errorf("telegram destination requires a valid telegram_chat_id: %w", err)
	}
	d.chatID = chatID

	if d.api == nil {
		client, err := telegram.NewClientFromEnv(d.config.BotTokenEnv)
		if err != nil {
			return fmt.Errorf("failed to build telegram client: %w", err)
		}
		d.api = client
	}

	d.connected = true
	return nil
}

// Health reports whether the adapter has been connected.
func (d *TelegramDestinationAdapter) Health(ctx context.Context) error {
	d.mu.Lock()
	defer d.mu.Unlock()
	if !d.connected {
		return fmt.Errorf("telegram destination adapter is not connected")
	}
	return nil
}

// Deliver mirrors the event and discards the result.
func (d *TelegramDestinationAdapter) Deliver(ctx context.Context, ev *event.Event) error {
	_, err := d.DeliverWithResult(ctx, ev)
	return err
}

// DeliverWithResult mirrors the event into the destination chat and returns the
// created destination message id(s). Albums are reproduced as albums.
func (d *TelegramDestinationAdapter) DeliverWithResult(ctx context.Context, ev *event.Event) (json.RawMessage, error) {
	d.mu.Lock()
	api := d.api
	chatID := d.chatID
	disableNotification := d.config.DisableNotification
	d.mu.Unlock()

	if api == nil {
		return nil, fmt.Errorf("telegram destination adapter is not connected")
	}
	if chatID == 0 {
		return nil, fmt.Errorf("telegram destination chat id is not configured")
	}

	var payload telegram.MessagePayload
	if err := json.Unmarshal(ev.Payload, &payload); err != nil {
		return nil, fmt.Errorf("unsupported telegram event payload: %w", err)
	}

	var deliveredIDs []int
	var err error

	// If the post is from a public channel or has direct media URLs, deliver directly using Bot API
	if payload.ChannelUsername != "" || len(payload.MediaURLs) > 0 || payload.ChatID == 0 {
		deliveredIDs, err = d.deliverDirect(ctx, api, chatID, payload, disableNotification)
	} else {
		messageIDs := payload.MessageIDs
		if len(messageIDs) == 0 && payload.MessageID != 0 {
			messageIDs = []int{payload.MessageID}
		}
		if len(messageIDs) == 0 {
			return nil, fmt.Errorf("telegram event has no message ids")
		}

		if payload.MediaGroupID != "" && len(messageIDs) > 1 {
			deliveredIDs, err = d.deliverAlbum(ctx, api, payload.ChatID, chatID, messageIDs)
		} else {
			deliveredIDs, err = d.deliverSingle(ctx, api, payload.ChatID, chatID, messageIDs[0], disableNotification)
		}

		// Fallback to deliverDirect if CopyMessage failed due to bot not being in the source channel
		if err != nil && (payload.Text != "" || payload.HTMLText != "" || len(payload.MediaURLs) > 0) && strings.Contains(err.Error(), "message to copy not found") {
			slog.Info("copyMessage not allowed for source channel, falling back to direct delivery", slog.String("error", err.Error()))
			deliveredIDs, err = d.deliverDirect(ctx, api, chatID, payload, disableNotification)
		}
	}

	telegram.LogDeliveryResult(payload.ChatID, payload.MessageID, chatID, deliveredIDs, err)
	if err != nil {
		if wait, ok := telegram.RetryAfter(err); ok {
			return nil, fmt.Errorf("telegram flood control (retry_after=%ds): %w", wait, err)
		}
		return nil, err
	}

	result, _ := json.Marshal(telegramDeliveryResult{ChatID: chatID, MessageIDs: deliveredIDs})
	return result, nil
}

// deliverDirect uploads/sends content directly using Telegram Bot API methods (sendPhoto, sendVideo, sendMediaGroup, sendMessage).
func (d *TelegramDestinationAdapter) deliverDirect(ctx context.Context, api telegram.API, toChatID int64, payload telegram.MessagePayload, disableNotification bool) ([]int, error) {
	text := payload.HTMLText
	parseMode := tgbotapi.ModeHTML
	if text == "" {
		text = payload.Text
		parseMode = ""
	}

	// 1. Multiple media items (album / media group)
	if len(payload.MediaURLs) > 1 || payload.MediaKind == "album" {
		var inputMedia []interface{}
		for i, url := range payload.MediaURLs {
			bytes, err := telegram.DownloadMedia(ctx, nil, url, 50*1024*1024)
			if err != nil {
				return nil, fmt.Errorf("failed to download album media #%d: %w", i, err)
			}
			isVid := false
			if i < len(payload.Media) && payload.Media[i].Kind == "video" {
				isVid = true
			} else if strings.HasSuffix(strings.ToLower(url), ".mp4") {
				isVid = true
			}

			if isVid {
				item := tgbotapi.NewInputMediaVideo(tgbotapi.FileBytes{
					Name:  fmt.Sprintf("video_%d.mp4", i),
					Bytes: bytes,
				})
				if i == 0 && text != "" {
					item.Caption = truncateCaption(text, 1024)
					item.ParseMode = parseMode
				}
				inputMedia = append(inputMedia, item)
			} else {
				item := tgbotapi.NewInputMediaPhoto(tgbotapi.FileBytes{
					Name:  fmt.Sprintf("photo_%d.jpg", i),
					Bytes: bytes,
				})
				if i == 0 && text != "" {
					item.Caption = truncateCaption(text, 1024)
					item.ParseMode = parseMode
				}
				inputMedia = append(inputMedia, item)
			}
		}

		if len(inputMedia) > 0 {
			mg := tgbotapi.NewMediaGroup(toChatID, inputMedia)
			mg.DisableNotification = disableNotification
			msgs, err := api.SendMediaGroup(mg)
			if err != nil && parseMode == tgbotapi.ModeHTML {
				slog.Warn("album send with HTML failed, retrying plain text", slog.String("error", err.Error()))
				for i := range inputMedia {
					if i == 0 {
						if photo, ok := inputMedia[i].(tgbotapi.InputMediaPhoto); ok {
							photo.Caption = truncateCaption(payload.Text, 1024)
							photo.ParseMode = ""
							inputMedia[i] = photo
						} else if video, ok := inputMedia[i].(tgbotapi.InputMediaVideo); ok {
							video.Caption = truncateCaption(payload.Text, 1024)
							video.ParseMode = ""
							inputMedia[i] = video
						}
					}
				}
				mg = tgbotapi.NewMediaGroup(toChatID, inputMedia)
				mg.DisableNotification = disableNotification
				msgs, err = api.SendMediaGroup(mg)
			}
			if err != nil {
				return nil, fmt.Errorf("failed to send media group: %w", err)
			}
			var ids []int
			for _, m := range msgs {
				ids = append(ids, m.MessageID)
			}
			return ids, nil
		}
	}

	// 2. Single media item (photo or video)
	if len(payload.MediaURLs) == 1 {
		url := payload.MediaURLs[0]
		bytes, err := telegram.DownloadMedia(ctx, nil, url, 50*1024*1024)
		if err != nil {
			return nil, fmt.Errorf("failed to download media: %w", err)
		}

		isVid := payload.MediaKind == "video" || strings.HasSuffix(strings.ToLower(url), ".mp4")
		if isVid {
			vid := tgbotapi.NewVideo(toChatID, tgbotapi.FileBytes{Name: "video.mp4", Bytes: bytes})
			vid.Caption = truncateCaption(text, 1024)
			vid.ParseMode = parseMode
			vid.DisableNotification = disableNotification
			msg, err := api.Send(vid)
			if err != nil && parseMode == tgbotapi.ModeHTML {
				slog.Warn("video send with HTML failed, retrying plain text", slog.String("error", err.Error()))
				vid.Caption = truncateCaption(payload.Text, 1024)
				vid.ParseMode = ""
				msg, err = api.Send(vid)
			}
			if err != nil {
				return nil, fmt.Errorf("failed to send video: %w", err)
			}
			return []int{msg.MessageID}, nil
		}

		photo := tgbotapi.NewPhoto(toChatID, tgbotapi.FileBytes{Name: "photo.jpg", Bytes: bytes})
		photo.Caption = truncateCaption(text, 1024)
		photo.ParseMode = parseMode
		photo.DisableNotification = disableNotification
		msg, err := api.Send(photo)
		if err != nil && parseMode == tgbotapi.ModeHTML {
			slog.Warn("photo send with HTML failed, retrying plain text", slog.String("error", err.Error()))
			photo.Caption = truncateCaption(payload.Text, 1024)
			photo.ParseMode = ""
			msg, err = api.Send(photo)
		}
		if err != nil {
			return nil, fmt.Errorf("failed to send photo: %w", err)
		}
		return []int{msg.MessageID}, nil
	}

	// 3. Text-only message
	if text == "" {
		text = payload.Text
	}
	if text == "" {
		return nil, fmt.Errorf("public post has no content (no media and no text)")
	}

	msg := tgbotapi.NewMessage(toChatID, truncateCaption(text, 4096))
	msg.ParseMode = parseMode
	msg.DisableNotification = disableNotification
	res, err := api.Send(msg)
	if err != nil && parseMode == tgbotapi.ModeHTML {
		slog.Warn("message send with HTML failed, retrying plain text", slog.String("error", err.Error()))
		msg.Text = truncateCaption(payload.Text, 4096)
		msg.ParseMode = ""
		res, err = api.Send(msg)
	}
	if err != nil {
		return nil, fmt.Errorf("failed to send text message: %w", err)
	}
	return []int{res.MessageID}, nil
}

func truncateCaption(s string, limit int) string {
	if len(s) <= limit {
		return s
	}
	runes := []rune(s)
	if len(runes) <= limit {
		return s
	}
	return string(runes[:limit])
}

// deliverAlbum mirrors a media group as a single album via copyMessages,
// degrading gracefully to individual copies if album copy is unavailable.
func (d *TelegramDestinationAdapter) deliverAlbum(ctx context.Context, api telegram.API, fromChatID, toChatID int64, messageIDs []int) ([]int, error) {
	ids, err := api.CopyMessages(fromChatID, toChatID, messageIDs)
	if err == nil {
		return ids, nil
	}

	slog.Warn("telegram album copy failed, falling back to individual copies",
		slog.Int64("source_chat", fromChatID),
		slog.Int64("destination_chat", toChatID),
		slog.Int("album_size", len(messageIDs)),
		slog.String("error", err.Error()),
	)

	var delivered []int
	for _, id := range messageIDs {
		msg, copyErr := d.deliverSingle(ctx, api, fromChatID, toChatID, id, d.config.DisableNotification)
		if copyErr != nil {
			return delivered, fmt.Errorf("album fallback copy of message %d failed: %w", id, copyErr)
		}
		delivered = append(delivered, msg...)
	}
	return delivered, nil
}

func (d *TelegramDestinationAdapter) deliverSingle(ctx context.Context, api telegram.API, fromChatID, toChatID int64, messageID int, disableNotification bool) ([]int, error) {
	cfg := tgbotapi.NewCopyMessage(toChatID, fromChatID, messageID)
	cfg.DisableNotification = disableNotification
	res, err := api.CopyMessage(cfg)
	if err != nil {
		return nil, fmt.Errorf("telegram copyMessage failed: %w", err)
	}
	return []int{res.MessageID}, nil
}
