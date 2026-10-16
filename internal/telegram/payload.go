// Package telegram contains the Telegram integration primitives used by the
// Damtie adapters and ingestion listener: a thin Bot API client, a rate limiter,
// the normalized event payload schema, and listener state persistence.
//
// The package deliberately has no dependency on the core engine so that the
// engine remains reusable for other sources and destinations.
package telegram

import (
	"encoding/json"
	"fmt"
	"strconv"
	"strings"

	tgbotapi "github.com/go-telegram-bot-api/telegram-bot-api/v5"
)

// Event type names emitted by the Telegram source adapter.
const (
	EventTypeMessage    = "telegram.message"
	EventTypeMediaGroup = "telegram.media_group"
)

// RawPost is the normalized external input handed to the Telegram source
// adapter's Normalize method. It carries one or more raw Telegram Message
// objects that belong to the same post (an album shares a media_group_id).
type RawPost struct {
	ChatID       int64             `json:"chat_id"`
	ChatTitle    string            `json:"chat_title,omitempty"`
	ChatType     string            `json:"chat_type,omitempty"`
	MediaGroupID string            `json:"media_group_id,omitempty"`
	Messages     []json.RawMessage `json:"messages"`
}

// MediaRef is a reference to a Telegram-hosted media file. It preserves the
// original file_id so media can be re-referenced without re-uploading.
type MediaRef struct {
	Kind   string `json:"kind"`
	FileID string `json:"file_id,omitempty"`
	URL    string `json:"url,omitempty"`
}

// MessagePayload is the generic Event payload emitted for Telegram posts.
// It never modifies the original content; it only describes where the post
// lives so the destination adapter can copy it verbatim.
type MessagePayload struct {
	ChatID          int64             `json:"chat_id,omitempty"`
	ChannelUsername string            `json:"channel_username,omitempty"`
	ChatTitle       string            `json:"chat_title,omitempty"`
	ChatType        string            `json:"chat_type,omitempty"`
	MessageID       int               `json:"message_id,omitempty"`
	MessageIDs      []int             `json:"message_ids,omitempty"`
	MediaGroupID    string            `json:"media_group_id,omitempty"`
	HasMedia        bool              `json:"has_media"`
	MediaKind       string            `json:"media_kind,omitempty"`
	Text            string            `json:"text,omitempty"`
	HTMLText        string            `json:"html_text,omitempty"`
	Media           []MediaRef        `json:"media,omitempty"`
	MediaURLs       []string          `json:"media_urls,omitempty"`
	Messages        []json.RawMessage `json:"messages,omitempty"`
}

// ExternalEventID builds the stable idempotency key for a Telegram post.
// Single messages use "<chat_id>:<message_id>"; albums use the media_group_id
// so that every item of the album collapses into a single event.
func ExternalEventID(chatID int64, mediaGroupID string, messageID int) string {
	if mediaGroupID != "" {
		return fmt.Sprintf("telegram:%d:mg:%s", chatID, mediaGroupID)
	}
	return fmt.Sprintf("telegram:%d:%d", chatID, messageID)
}

// ExternalPublicEventID builds the stable idempotency key for a public Telegram channel post.
func ExternalPublicEventID(channel string, messageID int) string {
	return fmt.Sprintf("telegram:public:%s:%d", strings.ToLower(channel), messageID)
}

// ParseChatID parses a Telegram chat id from its string form.
func ParseChatID(s string) (int64, error) {
	if s == "" {
		return 0, fmt.Errorf("empty telegram chat id")
	}
	id, err := strconv.ParseInt(s, 10, 64)
	if err != nil {
		return 0, fmt.Errorf("invalid telegram chat id %q: %w", s, err)
	}
	return id, nil
}

// MediaKind returns the media type of a Telegram message, or "" for text-only.
func MediaKind(msg *tgbotapi.Message) string {
	if msg == nil {
		return ""
	}
	switch {
	case msg.Photo != nil:
		return "photo"
	case msg.Video != nil:
		return "video"
	case msg.Document != nil:
		return "document"
	case msg.Audio != nil:
		return "audio"
	case msg.Voice != nil:
		return "voice"
	case msg.Sticker != nil:
		return "sticker"
	case msg.Animation != nil:
		return "animation"
	case msg.VideoNote != nil:
		return "video_note"
	case msg.Poll != nil:
		return "poll"
	default:
		return ""
	}
}

// ExtractMediaRefs collects the media references from a set of messages while
// preserving their original order.
func ExtractMediaRefs(msgs []*tgbotapi.Message) []MediaRef {
	var refs []MediaRef
	for _, m := range msgs {
		if m == nil {
			continue
		}
		switch {
		case len(m.Photo) > 0:
			// Telegram returns photos ordered by ascending size; the last is the largest.
			refs = append(refs, MediaRef{Kind: "photo", FileID: m.Photo[len(m.Photo)-1].FileID})
		case m.Video != nil:
			refs = append(refs, MediaRef{Kind: "video", FileID: m.Video.FileID})
		case m.Document != nil:
			refs = append(refs, MediaRef{Kind: "document", FileID: m.Document.FileID})
		case m.Audio != nil:
			refs = append(refs, MediaRef{Kind: "audio", FileID: m.Audio.FileID})
		case m.Voice != nil:
			refs = append(refs, MediaRef{Kind: "voice", FileID: m.Voice.FileID})
		case m.Animation != nil:
			refs = append(refs, MediaRef{Kind: "animation", FileID: m.Animation.FileID})
		case m.Sticker != nil:
			refs = append(refs, MediaRef{Kind: "sticker", FileID: m.Sticker.FileID})
		case m.VideoNote != nil:
			refs = append(refs, MediaRef{Kind: "video_note", FileID: m.VideoNote.FileID})
		}
	}
	return refs
}
