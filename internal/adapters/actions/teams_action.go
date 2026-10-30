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

// TeamsActionConfig defines parameters for posting to MS Teams incoming webhooks.
type TeamsActionConfig struct {
	WebhookURL string `json:"webhook_url,omitempty"`
	Title      string `json:"title,omitempty"`
	Text       string `json:"text,omitempty"`
	ThemeColor string `json:"theme_color,omitempty"`
	Summary    string `json:"summary,omitempty"`
}

// TeamsActionExecutor sends cards to Microsoft Teams via Incoming Webhooks.
type TeamsActionExecutor struct {
	client *http.Client
}

func NewTeamsActionExecutor() *TeamsActionExecutor {
	return NewTeamsActionExecutorWithClient(ssrf.NewSafeHTTPClient(15 * time.Second))
}

func NewTeamsActionExecutorWithClient(client *http.Client) *TeamsActionExecutor {
	return &TeamsActionExecutor{client: client}
}

func (t *TeamsActionExecutor) Name() string { return "Microsoft Teams" }
func (t *TeamsActionExecutor) Type() string { return "teams" }

func (t *TeamsActionExecutor) Validate(config json.RawMessage) error {
	var cfg TeamsActionConfig
	if err := json.Unmarshal(config, &cfg); err != nil {
		return fmt.Errorf("invalid teams action config: %w", err)
	}
	if strings.TrimSpace(cfg.Text) == "" && strings.TrimSpace(cfg.Title) == "" {
		return fmt.Errorf("teams action requires either 'text' or 'title'")
	}
	return nil
}

func (t *TeamsActionExecutor) Execute(ctx context.Context, req workflow.ActionRequest) (workflow.ActionResult, error) {
	var cfg TeamsActionConfig
	if err := json.Unmarshal(req.Config, &cfg); err != nil {
		return workflow.ActionResult{Success: false}, fmt.Errorf("failed to parse teams config: %w", err)
	}

	webhookURL := cfg.WebhookURL
	if req.Credentials != nil {
		if u, ok := req.Credentials["webhook_url"]; ok && u != "" {
			webhookURL = u
		}
	}

	if webhookURL == "" {
		return workflow.ActionResult{Success: false}, fmt.Errorf("teams action requires 'webhook_url'")
	}

	if err := ssrf.ValidateURL(ctx, webhookURL); err != nil {
		return workflow.ActionResult{Success: false}, fmt.Errorf("SSRF protection blocked webhook URL: %w", err)
	}

	text := cfg.Text
	if text == "" && req.Payload != nil {
		if t, ok := req.Payload["text"].(string); ok {
			text = t
		}
	}

	title := cfg.Title
	if title == "" && req.Payload != nil {
		if t, ok := req.Payload["title"].(string); ok {
			title = t
		}
	}

	summary := cfg.Summary
	if summary == "" {
		summary = title
		if summary == "" {
			summary = text
		}
	}

	themeColor := cfg.ThemeColor
	if themeColor == "" {
		themeColor = "0076D7"
	}

	cardPayload := map[string]any{
		"@type":      "MessageCard",
		"@context":   "http://schema.org/extensions",
		"themeColor": themeColor,
		"summary":    summary,
		"title":      title,
		"text":       text,
	}

	bodyBytes, err := json.Marshal(cardPayload)
	if err != nil {
		return workflow.ActionResult{Success: false}, fmt.Errorf("failed to serialize card payload: %w", err)
	}

	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, webhookURL, bytes.NewReader(bodyBytes))
	if err != nil {
		return workflow.ActionResult{Success: false}, fmt.Errorf("failed to build request: %w", err)
	}
	httpReq.Header.Set("Content-Type", "application/json")

	resp, err := t.client.Do(httpReq)
	if err != nil {
		return workflow.ActionResult{Success: false}, fmt.Errorf("teams webhook request failed: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()

	respBody, _ := io.ReadAll(io.LimitReader(resp.Body, 1024*1024))

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return workflow.ActionResult{Success: false}, fmt.Errorf("teams webhook returned status %d: %s", resp.StatusCode, string(respBody))
	}

	return workflow.ActionResult{
		Success: true,
		Output:  respBody,
		Metadata: map[string]any{
			"status": resp.StatusCode,
		},
	}, nil
}
