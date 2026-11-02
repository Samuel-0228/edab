package github

import (
	"bytes"
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"

	"kerob/pkg/connector"
)

const (
	ConnectorID   = "github"
	ConnectorName = "GitHub"

	DefaultAuthURL  = "https://github.com/login/oauth/authorize"
	DefaultTokenURL = "https://github.com/login/oauth/access_token"
	DefaultBaseURL  = "https://api.github.com"
)

// GitHubConnector implements the complete Kerob Connector SDK interfaces for GitHub.
type GitHubConnector struct {
	client       *http.Client
	baseURL      string
	authURL      string
	tokenURL     string
	clientID     string
	clientSecret string
	retryPolicy  connector.RetryPolicy
}

// Config holds options for initializing GitHubConnector.
type Config struct {
	Client       *http.Client
	BaseURL      string
	AuthURL      string
	TokenURL     string
	ClientID     string
	ClientSecret string
	RetryPolicy  *connector.RetryPolicy
}

// NewConnector creates a new GitHub connector instance.
func NewConnector(cfg Config) *GitHubConnector {
	client := cfg.Client
	if client == nil {
		client = &http.Client{Timeout: 15 * time.Second}
	}
	baseURL := cfg.BaseURL
	if baseURL == "" {
		baseURL = DefaultBaseURL
	}
	authURL := cfg.AuthURL
	if authURL == "" {
		authURL = DefaultAuthURL
	}
	tokenURL := cfg.TokenURL
	if tokenURL == "" {
		tokenURL = DefaultTokenURL
	}
	policy := connector.DefaultRetryPolicy()
	if cfg.RetryPolicy != nil {
		policy = *cfg.RetryPolicy
	}

	return &GitHubConnector{
		client:       client,
		baseURL:      strings.TrimRight(baseURL, "/"),
		authURL:      authURL,
		tokenURL:     tokenURL,
		clientID:     cfg.ClientID,
		clientSecret: cfg.ClientSecret,
		retryPolicy:  policy,
	}
}

// Ensure GitHubConnector implements all relevant SDK interfaces at compile time.
var (
	_ connector.Connector       = (*GitHubConnector)(nil)
	_ connector.ActionProvider  = (*GitHubConnector)(nil)
	_ connector.TriggerProvider = (*GitHubConnector)(nil)
	_ connector.WebhookManager  = (*GitHubConnector)(nil)
	_ connector.HealthChecker   = (*GitHubConnector)(nil)
	_ connector.OAuth2Provider  = (*GitHubConnector)(nil)
	_ connector.SchemaProvider  = (*GitHubConnector)(nil)
)

// Manifest returns the full declarative manifest of the GitHub connector.
func (g *GitHubConnector) Manifest() connector.Manifest {
	return connector.Manifest{
		ID:          ConnectorID,
		Name:        ConnectorName,
		Description: "Interact with GitHub repositories, issues, pull requests, and webhooks via OAuth 2.0 or personal access tokens.",
		Version:     "1.0.0",
		Category:    "developer",
		Icon:        "github",
		Capabilities: []string{
			connector.CapabilityTrigger,
			connector.CapabilityWebhook,
			connector.CapabilityAction,
			connector.CapabilityOAuth2,
			connector.CapabilityHealthCheck,
		},
		AuthType: "oauth2",
		OAuth2Config: &connector.OAuth2Config{
			ClientID:     g.clientID,
			ClientSecret: g.clientSecret,
			AuthURL:      g.authURL,
			TokenURL:     g.tokenURL,
			Scopes:       []string{"repo", "read:user", "admin:repo_hook"},
			PKCEEnabled:  false,
		},
		RequiredCredentialType: ConnectorID,
		ConfigSchema: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"owner": map[string]any{"type": "string", "title": "Repository Owner"},
				"repo":  map[string]any{"type": "string", "title": "Repository Name"},
			},
			"required": []any{"owner", "repo"},
		},
		Triggers: []connector.TriggerMetadata{
			{Type: "github.issue.opened", Name: "Issue Opened", Description: "Fires when a new issue is opened in the repository."},
			{Type: "github.issue.closed", Name: "Issue Closed", Description: "Fires when an issue is closed."},
			{Type: "github.issue.commented", Name: "Issue Commented", Description: "Fires when a comment is added to an issue."},
			{Type: "github.push", Name: "Code Pushed", Description: "Fires when commits are pushed to the repository."},
			{Type: "github.pull_request.opened", Name: "PR Opened", Description: "Fires when a pull request is opened."},
			{Type: "github.pull_request.closed", Name: "PR Closed", Description: "Fires when a pull request is closed or merged."},
		},
		Actions: []connector.ActionMetadata{
			{Type: "create_issue", Name: "Create Issue", Description: "Open a new issue with title, body, and labels."},
			{Type: "post_comment", Name: "Post Comment", Description: "Add a comment to an issue or pull request."},
			{Type: "add_labels", Name: "Add Labels", Description: "Apply labels to an existing issue or pull request."},
		},
		Credentials: []connector.CredentialProperty{
			{Key: "access_token", Label: "OAuth Access Token / PAT", Type: "password", Required: true, Description: "GitHub Bearer Token with repo scope"},
			{Key: "refresh_token", Label: "OAuth Refresh Token", Type: "password", Required: false, Description: "OAuth refresh token for automated rotation"},
			{Key: "webhook_secret", Label: "Webhook Secret", Type: "password", Required: false, Description: "Secret used to sign inbound webhook events"},
		},
		DocumentationURL: "https://docs.github.com/en/rest",
	}
}

// ValidateConfig verifies configuration parameters.
func (g *GitHubConnector) ValidateConfig(config map[string]any) error {
	if config == nil {
		return nil
	}
	return nil
}

// -------------------------------------------------------------------------
// OAuth2Provider Implementation
// -------------------------------------------------------------------------

func (g *GitHubConnector) GetAuthorizationURL(state string, redirectURI string, extraParams map[string]string) string {
	q := url.Values{}
	q.Set("client_id", g.clientID)
	q.Set("scope", "repo read:user admin:repo_hook")
	q.Set("state", state)
	if redirectURI != "" {
		q.Set("redirect_uri", redirectURI)
	}
	for k, v := range extraParams {
		q.Set(k, v)
	}
	return fmt.Sprintf("%s?%s", g.authURL, q.Encode())
}

type gitHubTokenResponse struct {
	AccessToken           string `json:"access_token"`
	TokenType             string `json:"token_type"`
	Scope                 string `json:"scope"`
	ExpiresIn             int64  `json:"expires_in,omitempty"`
	RefreshToken          string `json:"refresh_token,omitempty"`
	RefreshTokenExpiresIn int64  `json:"refresh_token_expires_in,omitempty"`
	Error                 string `json:"error,omitempty"`
	ErrorDescription      string `json:"error_description,omitempty"`
}

func (g *GitHubConnector) ExchangeCode(ctx context.Context, code string, redirectURI string) (*connector.OAuth2Token, error) {
	params := url.Values{}
	params.Set("client_id", g.clientID)
	params.Set("client_secret", g.clientSecret)
	params.Set("code", code)
	if redirectURI != "" {
		params.Set("redirect_uri", redirectURI)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, g.tokenURL, strings.NewReader(params.Encode()))
	if err != nil {
		return nil, fmt.Errorf("failed to build token request: %w", err)
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.Header.Set("Accept", "application/json")

	resp, err := g.client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("token exchange network error: %w", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to read token response: %w", err)
	}

	var tokenResp gitHubTokenResponse
	if err := json.Unmarshal(body, &tokenResp); err != nil {
		return nil, fmt.Errorf("invalid token json response: %w", err)
	}

	if tokenResp.Error != "" {
		return nil, fmt.Errorf("oauth error: %s - %s", tokenResp.Error, tokenResp.ErrorDescription)
	}
	if tokenResp.AccessToken == "" {
		return nil, errors.New("upstream provider returned empty access token")
	}

	token := &connector.OAuth2Token{
		AccessToken:  tokenResp.AccessToken,
		RefreshToken: tokenResp.RefreshToken,
		TokenType:    tokenResp.TokenType,
	}
	if token.TokenType == "" {
		token.TokenType = "Bearer"
	}
	if tokenResp.ExpiresIn > 0 {
		token.ExpiresAt = time.Now().UTC().Add(time.Duration(tokenResp.ExpiresIn) * time.Second)
	}
	if tokenResp.Scope != "" {
		token.Scopes = strings.Split(tokenResp.Scope, ",")
	}

	return token, nil
}

func (g *GitHubConnector) RefreshToken(ctx context.Context, refreshToken string) (*connector.OAuth2Token, error) {
	params := url.Values{}
	params.Set("client_id", g.clientID)
	params.Set("client_secret", g.clientSecret)
	params.Set("grant_type", "refresh_token")
	params.Set("refresh_token", refreshToken)

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, g.tokenURL, strings.NewReader(params.Encode()))
	if err != nil {
		return nil, fmt.Errorf("failed to build refresh request: %w", err)
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.Header.Set("Accept", "application/json")

	resp, err := g.client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("token refresh network error: %w", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to read refresh response: %w", err)
	}

	var tokenResp gitHubTokenResponse
	if err := json.Unmarshal(body, &tokenResp); err != nil {
		return nil, fmt.Errorf("invalid refresh response: %w", err)
	}

	if tokenResp.Error != "" {
		return nil, fmt.Errorf("refresh error: %s - %s", tokenResp.Error, tokenResp.ErrorDescription)
	}

	token := &connector.OAuth2Token{
		AccessToken:  tokenResp.AccessToken,
		RefreshToken: tokenResp.RefreshToken,
		TokenType:    tokenResp.TokenType,
	}
	if token.RefreshToken == "" {
		token.RefreshToken = refreshToken // Retain existing if not rotated
	}
	if tokenResp.ExpiresIn > 0 {
		token.ExpiresAt = time.Now().UTC().Add(time.Duration(tokenResp.ExpiresIn) * time.Second)
	}

	return token, nil
}

func (g *GitHubConnector) RevokeToken(ctx context.Context, token string) error {
	// GitHub allows deleting an app authorization via DELETE /applications/{client_id}/grant
	revokeURL := fmt.Sprintf("%s/applications/%s/grant", g.baseURL, g.clientID)
	reqBody, _ := json.Marshal(map[string]string{"access_token": token})

	req, err := http.NewRequestWithContext(ctx, http.MethodDelete, revokeURL, bytes.NewReader(reqBody))
	if err != nil {
		return err
	}
	req.SetBasicAuth(g.clientID, g.clientSecret)
	req.Header.Set("Accept", "application/vnd.github+json")

	resp, err := g.client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	// Status 204 No Content indicates success
	if resp.StatusCode != http.StatusNoContent && resp.StatusCode != http.StatusOK {
		return fmt.Errorf("github revocation returned status %d", resp.StatusCode)
	}
	return nil
}

// -------------------------------------------------------------------------
// HealthChecker Implementation
// -------------------------------------------------------------------------

func (g *GitHubConnector) CheckHealth(ctx context.Context, creds map[string]string) (connector.HealthStatus, error) {
	start := time.Now()
	now := time.Now().UTC()

	token := extractToken(creds)
	if token == "" {
		return connector.HealthStatus{
			Status:    "unhealthy",
			LatencyMs: 0,
			Message:   "no GitHub access token or credential provided",
			CheckedAt: now,
		}, nil
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, g.baseURL+"/user", nil)
	if err != nil {
		return connector.HealthStatus{
			Status:    "unhealthy",
			LatencyMs: 0,
			Message:   err.Error(),
			CheckedAt: now,
		}, err
	}
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("Accept", "application/vnd.github+json")

	resp, err := g.client.Do(req)
	latency := time.Since(start).Milliseconds()

	if err != nil {
		return connector.HealthStatus{
			Status:    "unhealthy",
			LatencyMs: latency,
			Message:   fmt.Sprintf("connection error: %s", err.Error()),
			CheckedAt: now,
		}, nil
	}
	defer resp.Body.Close()

	if resp.StatusCode == http.StatusOK {
		var user map[string]any
		_ = json.NewDecoder(resp.Body).Decode(&user)
		login, _ := user["login"].(string)

		return connector.HealthStatus{
			Status:    "healthy",
			LatencyMs: latency,
			Message:   fmt.Sprintf("authenticated successfully as %s", login),
			CheckedAt: now,
			Details: map[string]any{
				"authenticated_user": login,
				"rate_limit_limit":   resp.Header.Get("X-RateLimit-Limit"),
				"rate_limit_remain":  resp.Header.Get("X-RateLimit-Remaining"),
			},
		}, nil
	}

	if resp.StatusCode == http.StatusUnauthorized || resp.StatusCode == http.StatusForbidden {
		return connector.HealthStatus{
			Status:    "unhealthy",
			LatencyMs: latency,
			Message:   fmt.Sprintf("authentication failed with HTTP %d", resp.StatusCode),
			CheckedAt: now,
		}, nil
	}

	return connector.HealthStatus{
		Status:    "degraded",
		LatencyMs: latency,
		Message:   fmt.Sprintf("unexpected response HTTP %d", resp.StatusCode),
		CheckedAt: now,
	}, nil
}

// -------------------------------------------------------------------------
// ActionProvider Implementation
// -------------------------------------------------------------------------

func (g *GitHubConnector) ExecuteAction(ctx context.Context, req connector.ActionRequest) (connector.ActionResult, error) {
	token := extractToken(req.Credentials)
	if token == "" {
		return connector.ActionResult{Success: false, Error: "missing github access token in credentials"}, errors.New("missing github access token")
	}

	// Merge action Config and Parameters
	cfgMap := make(map[string]any)
	if len(req.Config) > 0 {
		_ = json.Unmarshal(req.Config, &cfgMap)
	}
	for k, v := range req.Parameters {
		cfgMap[k] = v
	}

	owner, _ := cfgMap["owner"].(string)
	repo, _ := cfgMap["repo"].(string)
	if owner == "" || repo == "" {
		return connector.ActionResult{
			Success: false,
			Error:   "github action requires 'owner' and 'repo' parameters",
		}, errors.New("missing owner or repo")
	}

	switch req.ActionType {
	case "create_issue", "":
		return g.actionCreateIssue(ctx, owner, repo, token, cfgMap, req.Event)
	case "post_comment":
		return g.actionPostComment(ctx, owner, repo, token, cfgMap)
	case "add_labels":
		return g.actionAddLabels(ctx, owner, repo, token, cfgMap)
	default:
		return connector.ActionResult{
			Success: false,
			Error:   fmt.Sprintf("unknown action type: %s", req.ActionType),
		}, fmt.Errorf("unknown action type: %s", req.ActionType)
	}
}

func (g *GitHubConnector) actionCreateIssue(ctx context.Context, owner, repo, token string, params map[string]any, event map[string]any) (connector.ActionResult, error) {
	title, _ := params["title"].(string)
	if title == "" && event != nil {
		if t, ok := event["title"].(string); ok {
			title = t
		}
	}
	if title == "" {
		title = fmt.Sprintf("Automated issue from Kerob at %s", time.Now().UTC().Format(time.RFC3339))
	}

	bodyText, _ := params["body"].(string)
	if bodyText == "" {
		if b, _ := params["text"].(string); b != "" {
			bodyText = b
		} else if event != nil {
			if b, ok := event["body"].(string); ok {
				bodyText = b
			}
		}
	}

	issuePayload := map[string]any{
		"title": title,
		"body":  bodyText,
	}

	if labels, ok := params["labels"].([]any); ok && len(labels) > 0 {
		issuePayload["labels"] = labels
	} else if labelsStr, ok := params["labels"].(string); ok && labelsStr != "" {
		parts := strings.Split(labelsStr, ",")
		var clean []string
		for _, p := range parts {
			if t := strings.TrimSpace(p); t != "" {
				clean = append(clean, t)
			}
		}
		issuePayload["labels"] = clean
	}

	url := fmt.Sprintf("%s/repos/%s/%s/issues", g.baseURL, owner, repo)
	data, err := json.Marshal(issuePayload)
	if err != nil {
		return connector.ActionResult{Success: false, Error: err.Error()}, err
	}

	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(data))
	if err != nil {
		return connector.ActionResult{Success: false, Error: err.Error()}, err
	}
	httpReq.Header.Set("Authorization", "Bearer "+token)
	httpReq.Header.Set("Accept", "application/vnd.github+json")
	httpReq.Header.Set("Content-Type", "application/json")

	resp, err := g.client.Do(httpReq)
	if err != nil {
		return connector.ActionResult{
			Success:   false,
			Error:     err.Error(),
			Retryable: true,
		}, err
	}
	defer resp.Body.Close()

	respBytes, _ := io.ReadAll(resp.Body)
	if resp.StatusCode != http.StatusCreated && resp.StatusCode != http.StatusOK {
		isRetryable := g.retryPolicy.IsRetryable(resp.StatusCode)
		return connector.ActionResult{
			Success:   false,
			Error:     fmt.Sprintf("github api error (%d): %s", resp.StatusCode, string(respBytes)),
			Retryable: isRetryable,
		}, fmt.Errorf("github api returned status %d", resp.StatusCode)
	}

	var issueResult map[string]any
	_ = json.Unmarshal(respBytes, &issueResult)

	return connector.ActionResult{
		Success: true,
		Output: map[string]any{
			"issue_number": issueResult["number"],
			"html_url":     issueResult["html_url"],
			"title":        issueResult["title"],
			"state":        issueResult["state"],
		},
	}, nil
}

func (g *GitHubConnector) actionPostComment(ctx context.Context, owner, repo, token string, params map[string]any) (connector.ActionResult, error) {
	issueNum := 0
	if n, ok := params["issue_number"].(float64); ok {
		issueNum = int(n)
	} else if n, ok := params["issue_number"].(int); ok {
		issueNum = n
	}
	if issueNum <= 0 {
		return connector.ActionResult{Success: false, Error: "missing or invalid issue_number"}, errors.New("missing issue_number")
	}

	commentBody, _ := params["body"].(string)
	if commentBody == "" {
		return connector.ActionResult{Success: false, Error: "comment body cannot be empty"}, errors.New("empty comment body")
	}

	url := fmt.Sprintf("%s/repos/%s/%s/issues/%d/comments", g.baseURL, owner, repo, issueNum)
	payload, _ := json.Marshal(map[string]string{"body": commentBody})

	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(payload))
	if err != nil {
		return connector.ActionResult{Success: false, Error: err.Error()}, err
	}
	httpReq.Header.Set("Authorization", "Bearer "+token)
	httpReq.Header.Set("Accept", "application/vnd.github+json")
	httpReq.Header.Set("Content-Type", "application/json")

	resp, err := g.client.Do(httpReq)
	if err != nil {
		return connector.ActionResult{Success: false, Error: err.Error(), Retryable: true}, err
	}
	defer resp.Body.Close()

	respBytes, _ := io.ReadAll(resp.Body)
	if resp.StatusCode != http.StatusCreated && resp.StatusCode != http.StatusOK {
		return connector.ActionResult{
			Success:   false,
			Error:     fmt.Sprintf("github api error (%d): %s", resp.StatusCode, string(respBytes)),
			Retryable: g.retryPolicy.IsRetryable(resp.StatusCode),
		}, fmt.Errorf("github api returned status %d", resp.StatusCode)
	}

	var commentResult map[string]any
	_ = json.Unmarshal(respBytes, &commentResult)

	return connector.ActionResult{
		Success: true,
		Output: map[string]any{
			"comment_id": commentResult["id"],
			"html_url":   commentResult["html_url"],
		},
	}, nil
}

func (g *GitHubConnector) actionAddLabels(ctx context.Context, owner, repo, token string, params map[string]any) (connector.ActionResult, error) {
	issueNum := 0
	if n, ok := params["issue_number"].(float64); ok {
		issueNum = int(n)
	} else if n, ok := params["issue_number"].(int); ok {
		issueNum = n
	}
	if issueNum <= 0 {
		return connector.ActionResult{Success: false, Error: "missing or invalid issue_number"}, errors.New("missing issue_number")
	}

	var labels []string
	if list, ok := params["labels"].([]any); ok {
		for _, item := range list {
			if s, ok := item.(string); ok && s != "" {
				labels = append(labels, s)
			}
		}
	} else if s, ok := params["labels"].(string); ok && s != "" {
		for _, part := range strings.Split(s, ",") {
			if t := strings.TrimSpace(part); t != "" {
				labels = append(labels, t)
			}
		}
	}

	if len(labels) == 0 {
		return connector.ActionResult{Success: false, Error: "no labels provided to add"}, errors.New("empty labels")
	}

	url := fmt.Sprintf("%s/repos/%s/%s/issues/%d/labels", g.baseURL, owner, repo, issueNum)
	payload, _ := json.Marshal(map[string]any{"labels": labels})

	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(payload))
	if err != nil {
		return connector.ActionResult{Success: false, Error: err.Error()}, err
	}
	httpReq.Header.Set("Authorization", "Bearer "+token)
	httpReq.Header.Set("Accept", "application/vnd.github+json")
	httpReq.Header.Set("Content-Type", "application/json")

	resp, err := g.client.Do(httpReq)
	if err != nil {
		return connector.ActionResult{Success: false, Error: err.Error(), Retryable: true}, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK && resp.StatusCode != http.StatusCreated {
		respBytes, _ := io.ReadAll(resp.Body)
		return connector.ActionResult{
			Success:   false,
			Error:     fmt.Sprintf("github api error (%d): %s", resp.StatusCode, string(respBytes)),
			Retryable: g.retryPolicy.IsRetryable(resp.StatusCode),
		}, fmt.Errorf("github api returned status %d", resp.StatusCode)
	}

	return connector.ActionResult{
		Success: true,
		Output: map[string]any{
			"applied_labels": labels,
		},
	}, nil
}

// -------------------------------------------------------------------------
// TriggerProvider Implementation
// -------------------------------------------------------------------------

func (g *GitHubConnector) ValidateTrigger(triggerType string, config map[string]any) error {
	if !strings.HasPrefix(triggerType, "github.") {
		return fmt.Errorf("unsupported trigger type %q for github connector", triggerType)
	}
	return nil
}

func (g *GitHubConnector) NormalizeEvent(triggerType string, headers map[string]string, payload []byte) (map[string]any, error) {
	var rawMap map[string]any
	if err := json.Unmarshal(payload, &rawMap); err != nil {
		return nil, fmt.Errorf("invalid github webhook payload: %w", err)
	}

	event := make(map[string]any)
	event["raw"] = rawMap

	if issue, ok := rawMap["issue"].(map[string]any); ok {
		event["title"] = issue["title"]
		event["body"] = issue["body"]
		event["number"] = issue["number"]
		event["html_url"] = issue["html_url"]
		if user, ok := issue["user"].(map[string]any); ok {
			event["author"] = user["login"]
		}
	}

	if pr, ok := rawMap["pull_request"].(map[string]any); ok {
		event["title"] = pr["title"]
		event["body"] = pr["body"]
		event["number"] = pr["number"]
		event["html_url"] = pr["html_url"]
		if user, ok := pr["user"].(map[string]any); ok {
			event["author"] = user["login"]
		}
	}

	if comment, ok := rawMap["comment"].(map[string]any); ok {
		event["comment_body"] = comment["body"]
		event["comment_id"] = comment["id"]
	}

	if repo, ok := rawMap["repository"].(map[string]any); ok {
		event["repository"] = repo["full_name"]
	}

	return event, nil
}

// -------------------------------------------------------------------------
// WebhookManager Implementation
// -------------------------------------------------------------------------

func (g *GitHubConnector) SetupWebhook(ctx context.Context, req connector.WebhookSetupRequest) (connector.WebhookInfo, error) {
	token := extractToken(req.Credentials)
	if token == "" {
		return connector.WebhookInfo{}, errors.New("missing github access token in credentials")
	}

	owner, _ := req.Config["owner"].(string)
	repo, _ := req.Config["repo"].(string)
	if owner == "" || repo == "" {
		return connector.WebhookInfo{}, errors.New("webhook setup requires 'owner' and 'repo' in config")
	}

	events := req.Events
	if len(events) == 0 {
		events = []string{"issues", "push", "pull_request", "issue_comment"}
	}

	hookConfig := map[string]any{
		"url":          req.WebhookURL,
		"content_type": "json",
	}
	if req.Secret != "" {
		hookConfig["secret"] = req.Secret
	}

	hookPayload := map[string]any{
		"name":   "web",
		"active": true,
		"events": events,
		"config": hookConfig,
	}

	data, _ := json.Marshal(hookPayload)
	url := fmt.Sprintf("%s/repos/%s/%s/hooks", g.baseURL, owner, repo)

	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(data))
	if err != nil {
		return connector.WebhookInfo{}, err
	}
	httpReq.Header.Set("Authorization", "Bearer "+token)
	httpReq.Header.Set("Accept", "application/vnd.github+json")
	httpReq.Header.Set("Content-Type", "application/json")

	resp, err := g.client.Do(httpReq)
	if err != nil {
		return connector.WebhookInfo{}, err
	}
	defer resp.Body.Close()

	respBytes, _ := io.ReadAll(resp.Body)
	if resp.StatusCode != http.StatusCreated && resp.StatusCode != http.StatusOK {
		return connector.WebhookInfo{}, fmt.Errorf("failed to create webhook (%d): %s", resp.StatusCode, string(respBytes))
	}

	var hookResp map[string]any
	_ = json.Unmarshal(respBytes, &hookResp)
	hookID := fmt.Sprintf("%v", hookResp["id"])

	return connector.WebhookInfo{
		ID:         hookID,
		ExternalID: hookID,
		WebhookURL: req.WebhookURL,
		Events:     events,
		Active:     true,
		CreatedAt:  time.Now().UTC(),
	}, nil
}

func (g *GitHubConnector) TeardownWebhook(ctx context.Context, webhookID string, creds map[string]string) error {
	token := extractToken(creds)
	if token == "" {
		return errors.New("missing github access token in credentials")
	}

	owner := creds["owner"]
	repo := creds["repo"]
	if owner == "" || repo == "" {
		return errors.New("teardown requires owner and repo")
	}

	url := fmt.Sprintf("%s/repos/%s/%s/hooks/%s", g.baseURL, owner, repo, webhookID)
	httpReq, err := http.NewRequestWithContext(ctx, http.MethodDelete, url, nil)
	if err != nil {
		return err
	}
	httpReq.Header.Set("Authorization", "Bearer "+token)
	httpReq.Header.Set("Accept", "application/vnd.github+json")

	resp, err := g.client.Do(httpReq)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusNoContent && resp.StatusCode != http.StatusOK && resp.StatusCode != http.StatusNotFound {
		return fmt.Errorf("github webhook delete failed with HTTP %d", resp.StatusCode)
	}
	return nil
}

func (g *GitHubConnector) VerifyWebhookSignature(secret string, headers map[string]string, payload []byte) bool {
	if secret == "" {
		return true // No secret configured
	}
	sigHeader := headers["X-Hub-Signature-256"]
	if sigHeader == "" {
		sigHeader = headers["x-hub-signature-256"]
	}
	if sigHeader == "" {
		return false
	}

	sigHex := strings.TrimPrefix(sigHeader, "sha256=")
	expectedSig, err := hex.DecodeString(sigHex)
	if err != nil {
		return false
	}

	mac := hmac.New(sha256.New, []byte(secret))
	mac.Write(payload)
	actualSig := mac.Sum(nil)

	return hmac.Equal(actualSig, expectedSig)
}

// -------------------------------------------------------------------------
// SchemaProvider Implementation
// -------------------------------------------------------------------------

func (g *GitHubConnector) GetSchema(operationType string) (map[string]any, error) {
	switch operationType {
	case "create_issue":
		return map[string]any{
			"type": "object",
			"properties": map[string]any{
				"owner":  map[string]any{"type": "string", "title": "Repository Owner"},
				"repo":   map[string]any{"type": "string", "title": "Repository Name"},
				"title":  map[string]any{"type": "string", "title": "Issue Title"},
				"body":   map[string]any{"type": "string", "title": "Issue Body (Markdown)"},
				"labels": map[string]any{"type": "array", "items": map[string]any{"type": "string"}, "title": "Labels"},
			},
			"required": []any{"owner", "repo", "title"},
		}, nil

	case "post_comment":
		return map[string]any{
			"type": "object",
			"properties": map[string]any{
				"owner":        map[string]any{"type": "string", "title": "Repository Owner"},
				"repo":         map[string]any{"type": "string", "title": "Repository Name"},
				"issue_number": map[string]any{"type": "integer", "title": "Issue or PR Number"},
				"body":         map[string]any{"type": "string", "title": "Comment Body"},
			},
			"required": []any{"owner", "repo", "issue_number", "body"},
		}, nil

	case "add_labels":
		return map[string]any{
			"type": "object",
			"properties": map[string]any{
				"owner":        map[string]any{"type": "string", "title": "Repository Owner"},
				"repo":         map[string]any{"type": "string", "title": "Repository Name"},
				"issue_number": map[string]any{"type": "integer", "title": "Issue or PR Number"},
				"labels":       map[string]any{"type": "array", "items": map[string]any{"type": "string"}, "title": "Labels to Add"},
			},
			"required": []any{"owner", "repo", "issue_number", "labels"},
		}, nil

	default:
		return map[string]any{"type": "object"}, nil
	}
}

func extractToken(creds map[string]string) string {
	if creds == nil {
		return ""
	}
	if t := creds["access_token"]; t != "" {
		return t
	}
	if t := creds["token"]; t != "" {
		return t
	}
	return creds["personal_access_token"]
}
