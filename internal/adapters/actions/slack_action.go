package actions

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"kerob/internal/adapters/ssrf"
	"kerob/internal/core/workflow"
)

// SlackActionConfig defines options for dispatching a message to Slack.
type SlackActionConfig struct {
	WebhookURL string         `json:"webhook_url,omitempty"`
	Channel    string         `json:"channel,omitempty"`
	Text       string         `json:"text,omitempty"`
	Blocks     []any          `json:"blocks,omitempty"`
	Username   string         `json:"username,omitempty"`
	IconEmoji  string         `json:"icon_emoji,omitempty"`
	IconURL    string         `json:"icon_url,omitempty"`
	Metadata   map[string]any `json:"metadata,omitempty"`
}

// SlackActionExecutor delivers notifications to Slack channels via webhooks or Web API.
type SlackActionExecutor struct {
	client *http.Client
}

func NewSlackActionExecutor() *SlackActionExecutor {
	return NewSlackActionExecutorWithClient(ssrf.NewSafeHTTPClient(15 * time.Second))
}

func NewSlackActionExecutorWithClient(client *http.Client) *SlackActionExecutor {
	return &SlackActionExecutor{client: client}
}

func (s *SlackActionExecutor) Name() string { return "Slack" }
func (s *SlackActionExecutor) Type() string { return "slack" }

func (s *SlackActionExecutor) Validate(config json.RawMessage) error {
	var cfg SlackActionConfig
	if err := json.Unmarshal(config, &cfg); err != nil {
		return fmt.Errorf("invalid slack action config: %w", err)
	}
	if strings.TrimSpace(cfg.Text) == "" && len(cfg.Blocks) == 0 {
		return fmt.Errorf("slack action requires 'text' or 'blocks'")
	}
	return nil
}

func (s *SlackActionExecutor) Execute(ctx context.Context, req workflow.ActionRequest) (workflow.ActionResult, error) {
	var cfg SlackActionConfig
	if err := json.Unmarshal(req.Config, &cfg); err != nil {
		return workflow.ActionResult{Success: false}, fmt.Errorf("failed to parse slack config: %w", err)
	}

	webhookURL := cfg.WebhookURL
	botToken := ""
	if req.Credentials != nil {
		if u, ok := req.Credentials["webhook_url"]; ok && u != "" {
			webhookURL = u
		}
		if t, ok := req.Credentials["bot_token"]; ok && t != "" {
			botToken = t
		}
	}

	var targetURL string
	headers := make(http.Header)
	headers.Set("Content-Type", "application/json; charset=utf-8")

	if botToken != "" && cfg.Channel != "" {
		targetURL = "https://slack.com/api/chat.postMessage"
		headers.Set("Authorization", "Bearer "+botToken)
	} else if webhookURL != "" {
		targetURL = webhookURL
	} else {
		return workflow.ActionResult{Success: false}, fmt.Errorf("slack action requires 'webhook_url' or 'channel' with bot_token credential")
	}

	if err := ssrf.ValidateURL(ctx, targetURL); err != nil {
		return workflow.ActionResult{Success: false}, fmt.Errorf("SSRF protection blocked target URL: %w", err)
	}

	text := cfg.Text
	if text == "" && len(cfg.Blocks) == 0 && req.Payload != nil {
		if t, ok := req.Payload["text"].(string); ok {
			text = t
		}
	}

	payloadMap := map[string]any{
		"text": text,
	}
	if len(cfg.Blocks) > 0 {
		payloadMap["blocks"] = cfg.Blocks
	}
	if cfg.Channel != "" {
		payloadMap["channel"] = cfg.Channel
	}
	if cfg.Username != "" {
		payloadMap["username"] = cfg.Username
	}
	if cfg.IconEmoji != "" {
		payloadMap["icon_emoji"] = cfg.IconEmoji
	}
	if cfg.IconURL != "" {
		payloadMap["icon_url"] = cfg.IconURL
	}

	bodyBytes, err := json.Marshal(payloadMap)
	if err != nil {
		return workflow.ActionResult{Success: false}, fmt.Errorf("failed to serialize slack payload: %w", err)
	}

	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, targetURL, bytes.NewReader(bodyBytes))
	if err != nil {
		return workflow.ActionResult{Success: false}, fmt.Errorf("failed to construct request: %w", err)
	}
	httpReq.Header = headers

	resp, err := s.client.Do(httpReq)
	if err != nil {
		return workflow.ActionResult{Success: false}, fmt.Errorf("slack request failed: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()

	respBody, _ := io.ReadAll(io.LimitReader(resp.Body, 1024*1024))

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return workflow.ActionResult{Success: false}, fmt.Errorf("slack endpoint returned status %d: %s", resp.StatusCode, string(respBody))
	}

	if strings.Contains(targetURL, "slack.com/api/") {
		var apiResp struct {
			OK    bool   `json:"ok"`
			Error string `json:"error,omitempty"`
			TS    string `json:"ts,omitempty"`
		}
		if err := json.Unmarshal(respBody, &apiResp); err == nil && !apiResp.OK {
			return workflow.ActionResult{Success: false}, fmt.Errorf("slack api error: %s", apiResp.Error)
		}
	}

	return workflow.ActionResult{
		Success: true,
		Output:  respBody,
		Metadata: map[string]any{
			"status": resp.StatusCode,
		},
	}, nil
}
