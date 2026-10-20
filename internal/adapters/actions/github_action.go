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

type GitHubActionConfig struct {
	Owner  string   `json:"owner"`
	Repo   string   `json:"repo"`
	Title  string   `json:"title"`
	Body   string   `json:"body,omitempty"`
	Labels []string `json:"labels,omitempty"`
}

type GitHubActionExecutor struct{}

func NewGitHubActionExecutor() *GitHubActionExecutor {
	return &GitHubActionExecutor{}
}

func (g *GitHubActionExecutor) Name() string { return "GitHub" }
func (g *GitHubActionExecutor) Type() string { return "github" }

func (g *GitHubActionExecutor) Validate(config json.RawMessage) error {
	var cfg GitHubActionConfig
	if err := json.Unmarshal(config, &cfg); err != nil {
		return fmt.Errorf("invalid github action config: %w", err)
	}
	if cfg.Owner == "" || cfg.Repo == "" {
		return fmt.Errorf("github action requires 'owner' and 'repo'")
	}
	return nil
}

func (g *GitHubActionExecutor) Execute(ctx context.Context, req workflow.ActionRequest) (workflow.ActionResult, error) {
	var cfg GitHubActionConfig
	if err := json.Unmarshal(req.Config, &cfg); err != nil {
		return workflow.ActionResult{Success: false}, fmt.Errorf("invalid github config: %w", err)
	}

	token := ""
	if req.Credentials != nil {
		token = req.Credentials["personal_access_token"]
		if token == "" {
			token = req.Credentials["token"]
		}
	}

	if token == "" {
		return workflow.ActionResult{Success: false}, fmt.Errorf("github token is required in action credentials")
	}

	title := cfg.Title
	if title == "" {
		if t, ok := req.Payload["title"].(string); ok && t != "" {
			title = t
		} else {
			title = fmt.Sprintf("Automated issue from %s", req.Event.EventType)
		}
	}

	body := cfg.Body
	if body == "" {
		if b, ok := req.Payload["body"].(string); ok && b != "" {
			body = b
		} else if text, ok := req.Payload["text"].(string); ok && text != "" {
			body = text
		}
	}

	reqPayload := map[string]any{
		"title": title,
		"body":  body,
	}
	if len(cfg.Labels) > 0 {
		reqPayload["labels"] = cfg.Labels
	}

	bodyBytes, err := json.Marshal(reqPayload)
	if err != nil {
		return workflow.ActionResult{Success: false}, err
	}

	apiURL := fmt.Sprintf("https://api.github.com/repos/%s/%s/issues", cfg.Owner, cfg.Repo)
	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, apiURL, bytes.NewReader(bodyBytes))
	if err != nil {
		return workflow.ActionResult{Success: false}, err
	}

	httpReq.Header.Set("Authorization", "token "+token)
	httpReq.Header.Set("Accept", "application/vnd.github.v3+json")
	httpReq.Header.Set("User-Agent", "kerob/1.0 (automation-engine)")
	httpReq.Header.Set("Content-Type", "application/json")

	client := ssrf.NewSafeHTTPClient(15 * time.Second)
	resp, err := client.Do(httpReq)
	if err != nil {
		return workflow.ActionResult{Success: false}, fmt.Errorf("github api request failed: %w", err)
	}
	defer resp.Body.Close()

	respBytes, _ := io.ReadAll(io.LimitReader(resp.Body, 16*1024))

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return workflow.ActionResult{
			Success: false,
			Output:  json.RawMessage(fmt.Sprintf(`{"status_code":%d,"error":%q}`, resp.StatusCode, string(respBytes))),
		}, fmt.Errorf("github api returned status %d: %s", resp.StatusCode, string(respBytes))
	}

	var issueResp struct {
		ID      int64  `json:"id"`
		Number  int    `json:"number"`
		HTMLURL string `json:"html_url"`
	}
	_ = json.Unmarshal(respBytes, &issueResp)

	return workflow.ActionResult{
		Success:    true,
		ExternalID: fmt.Sprintf("issue#%d", issueResp.Number),
		Output:     json.RawMessage(fmt.Sprintf(`{"issue_number":%d,"html_url":%q}`, issueResp.Number, issueResp.HTMLURL)),
		Metadata: map[string]any{
			"issue_number": issueResp.Number,
			"html_url":     issueResp.HTMLURL,
		},
	}, nil
}
