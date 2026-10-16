package telegram

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"os"
	"strconv"
	"strings"
	"sync"

	tgbotapi "github.com/go-telegram-bot-api/telegram-bot-api/v5"
)

// API is the subset of the Telegram Bot API used by the engine. It is defined
// as an interface so tests (and local Bot API servers) can substitute
// implementations without touching the network.
type API interface {
	GetMe() (tgbotapi.User, error)
	GetUpdates(config tgbotapi.UpdateConfig) ([]tgbotapi.Update, error)
	GetChat(config tgbotapi.ChatInfoConfig) (tgbotapi.Chat, error)
	GetChatMember(config tgbotapi.GetChatMemberConfig) (tgbotapi.ChatMember, error)
	CopyMessage(config tgbotapi.CopyMessageConfig) (tgbotapi.MessageID, error)
	CopyMessages(fromChatID, toChatID int64, messageIDs []int) ([]int, error)
	Send(c tgbotapi.Chattable) (tgbotapi.Message, error)
	SendMediaGroup(config tgbotapi.MediaGroupConfig) ([]tgbotapi.Message, error)
}

// ClientOptions configures a Telegram API client.
type ClientOptions struct {
	// APIBaseURL overrides the default Bot API endpoint (e.g. a local bot server).
	APIBaseURL string
	// GlobalRate is the maximum number of requests per second across all chats.
	GlobalRate float64
	// PerChatRate is the maximum number of requests per second for a single chat.
	PerChatRate float64
}

// sharedLimiters ensures a single rate budget per bot token, even when several
// adapter instances are created for the same bot.
var sharedLimiters sync.Map // token -> *RateLimiter

// Client is a thin, rate-limited wrapper around the Telegram Bot API.
type Client struct {
	bot     *tgbotapi.BotAPI
	limiter *RateLimiter
	token   string
}

// NewClient builds a client for the given bot token.
func NewClient(token string, opts ClientOptions) (*Client, error) {
	if strings.TrimSpace(token) == "" {
		return nil, fmt.Errorf("telegram bot token is required")
	}

	var bot *tgbotapi.BotAPI
	var err error
	if opts.APIBaseURL != "" {
		bot, err = tgbotapi.NewBotAPIWithAPIEndpoint(token, opts.APIBaseURL)
	} else {
		bot, err = tgbotapi.NewBotAPI(token)
	}
	if err != nil {
		return nil, fmt.Errorf("failed to initialize telegram bot: %w", err)
	}
	bot.Debug = false

	limiter := limiterForToken(token, opts)
	return &Client{bot: bot, limiter: limiter, token: token}, nil
}

func limiterForToken(token string, opts ClientOptions) *RateLimiter {
	if existing, ok := sharedLimiters.Load(token); ok {
		return existing.(*RateLimiter)
	}
	limiter := NewRateLimiter(opts.GlobalRate, opts.PerChatRate)
	actual, _ := sharedLimiters.LoadOrStore(token, limiter)
	return actual.(*RateLimiter)
}

// ClientOptionsFromEnv reads client options from environment variables.
func ClientOptionsFromEnv() ClientOptions {
	return ClientOptions{
		APIBaseURL:  os.Getenv("TELEGRAM_API_BASE_URL"),
		GlobalRate:  envFloat("TELEGRAM_RATE_GLOBAL", 25),
		PerChatRate: envFloat("TELEGRAM_RATE_PER_CHAT", 1),
	}
}

// NewClientFromEnv builds a client using the token from the given environment
// variable (defaulting to TELEGRAM_BOT_TOKEN).
func NewClientFromEnv(envName string) (*Client, error) {
	if envName == "" {
		envName = "TELEGRAM_BOT_TOKEN"
	}
	return NewClient(os.Getenv(envName), ClientOptionsFromEnv())
}

func envFloat(key string, def float64) float64 {
	if v, err := strconv.ParseFloat(os.Getenv(key), 64); err == nil {
		return v
	}
	return def
}

// GetMe returns the bot's own account information.
func (c *Client) GetMe() (tgbotapi.User, error) {
	return c.bot.GetMe()
}

// GetUpdates performs a long-poll request for new updates.
func (c *Client) GetUpdates(config tgbotapi.UpdateConfig) ([]tgbotapi.Update, error) {
	return c.bot.GetUpdates(config)
}

// GetChat resolves chat information (used to verify the bot has access).
func (c *Client) GetChat(config tgbotapi.ChatInfoConfig) (tgbotapi.Chat, error) {
	return c.bot.GetChat(config)
}

// GetChatMember returns the bot's membership in a chat.
func (c *Client) GetChatMember(config tgbotapi.GetChatMemberConfig) (tgbotapi.ChatMember, error) {
	return c.bot.GetChatMember(config)
}

// CopyMessage mirrors a single message into toChatID without a forward header.
func (c *Client) CopyMessage(config tgbotapi.CopyMessageConfig) (tgbotapi.MessageID, error) {
	if err := c.limiter.Wait(context.Background(), config.ChatID); err != nil {
		return tgbotapi.MessageID{}, err
	}
	return c.bot.CopyMessage(config)
}

// CopyMessages mirrors a batch of messages (a Telegram media group / album)
// as a single album. It uses the Bot API 7.0 copyMessages method directly
// because the pinned client library does not expose it yet.
func (c *Client) CopyMessages(fromChatID, toChatID int64, messageIDs []int) ([]int, error) {
	if len(messageIDs) == 0 {
		return nil, fmt.Errorf("copyMessages requires at least one message id")
	}
	if err := c.limiter.Wait(context.Background(), toChatID); err != nil {
		return nil, err
	}

	params := make(tgbotapi.Params)
	params.AddNonZero64("chat_id", toChatID)
	params.AddNonZero64("from_chat_id", fromChatID)
	if err := params.AddInterface("message_ids", messageIDs); err != nil {
		return nil, fmt.Errorf("failed to encode message_ids: %w", err)
	}

	resp, err := c.bot.MakeRequest("copyMessages", params)
	if err != nil {
		return nil, err
	}

	var ids []tgbotapi.MessageID
	if err := json.Unmarshal(resp.Result, &ids); err != nil {
		return nil, fmt.Errorf("failed to decode copyMessages response: %w", err)
	}

	out := make([]int, 0, len(ids))
	for _, m := range ids {
		out = append(out, m.MessageID)
	}
	return out, nil
}

// Send sends a Chattable item (message, photo, video, etc.) to Telegram with rate limiting.
func (c *Client) Send(ch tgbotapi.Chattable) (tgbotapi.Message, error) {
	var chatID int64
	switch v := ch.(type) {
	case tgbotapi.MessageConfig:
		chatID = v.ChatID
	case tgbotapi.PhotoConfig:
		chatID = v.ChatID
	case tgbotapi.VideoConfig:
		chatID = v.ChatID
	case tgbotapi.DocumentConfig:
		chatID = v.ChatID
	}
	if chatID != 0 && c.limiter != nil {
		if err := c.limiter.Wait(context.Background(), chatID); err != nil {
			return tgbotapi.Message{}, err
		}
	}
	return c.bot.Send(ch)
}

// SendMediaGroup sends an album of media to Telegram with rate limiting.
func (c *Client) SendMediaGroup(config tgbotapi.MediaGroupConfig) ([]tgbotapi.Message, error) {
	if config.ChatID != 0 && c.limiter != nil {
		if err := c.limiter.Wait(context.Background(), config.ChatID); err != nil {
			return nil, err
		}
	}
	return c.bot.SendMediaGroup(config)
}


// RetryAfter extracts the flood-control wait from a Telegram API error.
func RetryAfter(err error) (int, bool) {
	var tgErr tgbotapi.Error
	if errors.As(err, &tgErr) && tgErr.RetryAfter > 0 {
		return tgErr.RetryAfter, true
	}
	return 0, false
}

// AccessInfo describes whether the bot can access a chat.
type AccessInfo struct {
	Accessible bool   `json:"accessible"`
	Title      string `json:"title,omitempty"`
	Type       string `json:"type,omitempty"`
	Status     string `json:"status,omitempty"`
	Error      string `json:"error,omitempty"`
}

// CheckAccess verifies that the bot can see the given chat and returns its
// membership status. It is used to let operators confirm a source/destination
// is reachable before relying on it.
func CheckAccess(api API, chatID int64) AccessInfo {
	info := AccessInfo{Accessible: false}

	chat, err := api.GetChat(tgbotapi.ChatInfoConfig{ChatConfig: tgbotapi.ChatConfig{ChatID: chatID}})
	if err != nil {
		info.Error = err.Error()
		return info
	}

	info.Accessible = true
	info.Title = chat.Title
	info.Type = chat.Type

	bot, err := api.GetMe()
	if err == nil {
		member, memberErr := api.GetChatMember(tgbotapi.GetChatMemberConfig{
			ChatConfigWithUser: tgbotapi.ChatConfigWithUser{ChatID: chatID, UserID: bot.ID},
		})
		if memberErr == nil {
			info.Status = member.Status
		}
	}

	return info
}

// LogDeliveryResult emits a structured delivery log line without leaking secrets.
func LogDeliveryResult(sourceChatID int64, sourceMessageID int, destChatID int64, destMessageIDs []int, err error) {
	if err != nil {
		slog.Warn("telegram delivery failed",
			slog.Int64("source_chat", sourceChatID),
			slog.Int("source_message", sourceMessageID),
			slog.Int64("destination_chat", destChatID),
			slog.String("error", err.Error()),
		)
		return
	}
	slog.Info("telegram delivery success",
		slog.Int64("source_chat", sourceChatID),
		slog.Int("source_message", sourceMessageID),
		slog.Int64("destination_chat", destChatID),
		slog.Any("destination_messages", destMessageIDs),
	)
}
