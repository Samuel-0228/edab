package actions

import (
	"context"
	"encoding/json"
	"fmt"
	"strconv"
	"strings"

	tgbotapi "github.com/go-telegram-bot-api/telegram-bot-api/v5"
	"kerob/internal/core/workflow"
	"kerob/internal/telegram"
)

type TelegramActionConfig struct {
	ChatID      string `json:"chat_id"`                 // Destination chat ID e.g. "-1001234567890"
	Text        string `json:"text,omitempty"`          // Formatted text
	ParseMode   string `json:"parse_mode,omitempty"`    // "HTML" or "MarkdownV2"
	ActionType  string `json:"action_type,omitempty"`   // "send_message" or "copy_message"
	SourceChat  int64  `json:"source_chat,omitempty"`   // For copy_message
	SourceMsgID int    `json:"source_msg_id,omitempty"` // For copy_message
}

type TelegramActionExecutor struct {
	defaultAPI telegram.API
}

func NewTelegramActionExecutor(defaultAPI telegram.API) *TelegramActionExecutor {
	return &TelegramActionExecutor{defaultAPI: defaultAPI}
}

func (t *TelegramActionExecutor) Name() string { return "Telegram" }
func (t *TelegramActionExecutor) Type() string { return "telegram" }

func (t *TelegramActionExecutor) Validate(config json.RawMessage) error {
	var cfg TelegramActionConfig
	if err := json.Unmarshal(config, &cfg); err != nil {
		return fmt.Errorf("invalid telegram action config: %w", err)
	}
	if cfg.ChatID == "" {
		return fmt.Errorf("telegram action requires 'chat_id'")
	}
	return nil
}

func (t *TelegramActionExecutor) Execute(ctx context.Context, req workflow.ActionRequest) (workflow.ActionResult, error) {
	var cfg TelegramActionConfig
	if err := json.Unmarshal(req.Config, &cfg); err != nil {
		return workflow.ActionResult{Success: false}, fmt.Errorf("invalid telegram config: %w", err)
	}

	// Resolve Telegram API Client
	api := t.defaultAPI
	if req.Credentials != nil {
		if token, ok := req.Credentials["bot_token"]; ok && token != "" {
			client, err := telegram.NewClient(token, telegram.ClientOptions{})
			if err == nil {
				api = client
			}
		}
	}

	if api == nil {
		return workflow.ActionResult{Success: false}, fmt.Errorf("telegram client is not initialized (no bot token provided)")
	}

	destChatID, err := telegram.ParseChatID(cfg.ChatID)
	if err != nil {
		return workflow.ActionResult{Success: false}, fmt.Errorf("invalid chat_id %s: %w", cfg.ChatID, err)
	}

	// 1. Check if copy_message is requested or event is a telegram message
	if cfg.ActionType == "copy_message" || (cfg.ActionType == "" && req.Event.EventType == "telegram.message" && cfg.Text == "") {
		var srcChat int64
		var srcMsg int

		if cfg.SourceChat != 0 && cfg.SourceMsgID != 0 {
			srcChat = cfg.SourceChat
			srcMsg = cfg.SourceMsgID
		} else {
			// Extract from normalized event payload
			if sc, ok := req.Payload["chat_id"].(float64); ok {
				srcChat = int64(sc)
			}
			if sm, ok := req.Payload["message_id"].(float64); ok {
				srcMsg = int(sm)
			}
		}

		if srcChat != 0 && srcMsg != 0 {
			copyCfg := tgbotapi.CopyMessageConfig{
				BaseChat:   tgbotapi.BaseChat{ChatID: destChatID},
				FromChatID: srcChat,
				MessageID:  srcMsg,
			}
			msgID, err := api.CopyMessage(copyCfg)
			if err != nil {
				return workflow.ActionResult{Success: false}, fmt.Errorf("telegram copyMessage failed: %w", err)
			}
			return workflow.ActionResult{
				Success:    true,
				ExternalID: strconv.Itoa(msgID.MessageID),
				Output:     json.RawMessage(fmt.Sprintf(`{"message_id":%d}`, msgID.MessageID)),
			}, nil
		}
	}

	// 2. Otherwise send new formatted text message
	text := cfg.Text
	if text == "" {
		if t, ok := req.Payload["text"].(string); ok && t != "" {
			text = t
		} else if title, ok := req.Payload["title"].(string); ok && title != "" {
			text = title
		} else {
			text = fmt.Sprintf("Event: %s (id: %s)", req.Event.EventType, req.Event.ID)
		}
	}

	parseMode := cfg.ParseMode
	if parseMode == "" {
		parseMode = "HTML"
	}

	msg := tgbotapi.NewMessage(destChatID, text)
	msg.ParseMode = parseMode
	sentMsg, err := api.Send(msg)
	if err != nil {
		// Fallback to plain text if Markdown/HTML parsing failed
		if strings.Contains(err.Error(), "can't parse entities") {
			msg.ParseMode = ""
			sentMsg, err = api.Send(msg)
		}
		if err != nil {
			return workflow.ActionResult{Success: false}, fmt.Errorf("telegram sendMessage failed: %w", err)
		}
	}

	return workflow.ActionResult{
		Success:    true,
		ExternalID: strconv.Itoa(sentMsg.MessageID),
		Output:     json.RawMessage(fmt.Sprintf(`{"message_id":%d}`, sentMsg.MessageID)),
	}, nil
}
