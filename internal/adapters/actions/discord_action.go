package actions

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"

	"kerob/internal/adapters/ssrf"
	"kerob/internal/core/workflow"
)

type DiscordActionConfig struct {
	WebhookURL string        `json:"webhook_url,omitempty"`
	Content    string        `json:"content,omitempty"`
	Username   string        `json:"username,omitempty"`
	AvatarURL  string        `json:"avatar_url,omitempty"`
	Embed      *DiscordEmbed `json:"embed,omitempty"`
}

type DiscordEmbed struct {
	Title       string         `json:"title,omitempty"`
	Description string         `json:"description,omitempty"`
	URL         string         `json:"url,omitempty"`
	Color       int            `json:"color,omitempty"` // Hex integer e.g. 0x58a6ff
	Fields      []DiscordField `json:"fields,omitempty"`
	Footer      *DiscordFooter `json:"footer,omitempty"`
	Timestamp   string         `json:"timestamp,omitempty"`
}

type DiscordField struct {
	Name   string `json:"name"`
	Value  string `json:"value"`
	Inline bool   `json:"inline,omitempty"`
}

type DiscordFooter struct {
	Text    string `json:"text"`
	IconURL string `json:"icon_url,omitempty"`
}

type DiscordActionExecutor struct {
	client *http.Client
}

func NewDiscordActionExecutor() *DiscordActionExecutor {
	return &DiscordActionExecutor{}
}

func NewDiscordActionExecutorWithClient(client *http.Client) *DiscordActionExecutor {
	return &DiscordActionExecutor{client: client}
}

func (d *DiscordActionExecutor) Name() string { return "Discord" }
func (d *DiscordActionExecutor) Type() string { return "discord" }

func (d *DiscordActionExecutor) Validate(config json.RawMessage) error {
	var cfg DiscordActionConfig
	if err := json.Unmarshal(config, &cfg); err != nil {
		return fmt.Errorf("invalid discord action config: %w", err)
	}
	return nil
}

func (d *DiscordActionExecutor) Execute(ctx context.Context, req workflow.ActionRequest) (workflow.ActionResult, error) {
	var cfg DiscordActionConfig
	if err := json.Unmarshal(req.Config, &cfg); err != nil {
		return workflow.ActionResult{Success: false}, fmt.Errorf("invalid discord config: %w", err)
	}

	webhookURL := cfg.WebhookURL
	if webhookURL == "" && req.Credentials != nil {
		webhookURL = req.Credentials["webhook_url"]
	}

	if webhookURL == "" {
		return workflow.ActionResult{Success: false}, fmt.Errorf("discord webhook_url is required (either in action config or credentials)")
	}

	// Validate webhook URL for SSRF protection
	if err := ssrf.ValidateURL(ctx, webhookURL); err != nil {
		return workflow.ActionResult{Success: false}, fmt.Errorf("SSRF protection blocked discord webhook URL: %w", err)
	}

	content := cfg.Content
	if content == "" && cfg.Embed == nil {
		// Fallback to transformed text or title
		if text, ok := req.Payload["text"].(string); ok && text != "" {
			content = text
		} else if title, ok := req.Payload["title"].(string); ok && title != "" {
			content = title
		} else {
			content = fmt.Sprintf("Event received: %s (id: %s)", req.Event.EventType, req.Event.ID)
		}
	}

	payload := map[string]any{
		"content": content,
	}
	if cfg.Username != "" {
		payload["username"] = cfg.Username
	}
	if cfg.AvatarURL != "" {
		payload["avatar_url"] = cfg.AvatarURL
	}
	if cfg.Embed != nil {
		payload["embeds"] = []any{cfg.Embed}
	}

	bodyBytes, err := json.Marshal(payload)
	if err != nil {
		return workflow.ActionResult{Success: false}, fmt.Errorf("failed to marshal discord payload: %w", err)
	}

	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, webhookURL, bytes.NewReader(bodyBytes))
	if err != nil {
		return workflow.ActionResult{Success: false}, fmt.Errorf("failed to create http request: %w", err)
	}

	httpReq.Header.Set("Content-Type", "application/json")
	httpReq.Header.Set("User-Agent", "kerob/1.0 (automation-engine)")

	client := d.client
	if client == nil {
		client = ssrf.NewSafeHTTPClient(15 * time.Second)
	}
	resp, err := client.Do(httpReq)
	if err != nil {
		return workflow.ActionResult{Success: false}, fmt.Errorf("discord webhook request failed: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()

	respBytes, _ := io.ReadAll(io.LimitReader(resp.Body, 4096))

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return workflow.ActionResult{
			Success: false,
			Output:  json.RawMessage(fmt.Sprintf(`{"status_code":%d,"error":%q}`, resp.StatusCode, string(respBytes))),
		}, fmt.Errorf("discord returned status %d: %s", resp.StatusCode, string(respBytes))
	}

	return workflow.ActionResult{
		Success: true,
		Output:  json.RawMessage(`{"status":"delivered"}`),
	}, nil
}
