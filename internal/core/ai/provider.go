package ai

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"
)

// RawAIResult contains the model provider's raw generation response.
type RawAIResult struct {
	Content     json.RawMessage `json:"content"`
	Explanation string          `json:"explanation"`
	Model       string          `json:"model"`
	TokensUsed  int             `json:"tokens_used"`
}

// Provider defines the interface for an external or local AI model provider.
type Provider interface {
	Name() string
	Generate(ctx context.Context, task TaskType, systemPrompt string, userPrompt string) (*RawAIResult, error)
}

// --- 1. Mock Deterministic Provider (Offline / Local / CI) ---

type MockProvider struct {
	ModelName string
}

func NewMockProvider() *MockProvider {
	return &MockProvider{ModelName: "kerob-mock-model-v1"}
}

func (m *MockProvider) Name() string {
	return "mock"
}

func (m *MockProvider) Generate(_ context.Context, task TaskType, _ string, userPrompt string) (*RawAIResult, error) {
	// Security testing hook: if caller explicitly tests malicious prompt injection
	if strings.Contains(userPrompt, "test_malicious_exec") {
		return &RawAIResult{
			Content:     json.RawMessage(`{"name":"Attack Workflow","trigger":{"type":"webhook"},"actions":[{"id":"sh1","action_type":"execute_command","config":{"command":"rm -rf /"}}]}`),
			Explanation: "Attempted malicious script execution",
			Model:       m.ModelName,
			TokensUsed:  50,
		}, nil
	}
	if strings.Contains(userPrompt, "test_malicious_ssrf") {
		return &RawAIResult{
			Content:     json.RawMessage(`{"name":"SSRF Exfil","trigger":{"type":"webhook"},"actions":[{"id":"http1","integration":"http","action_type":"request","config":{"url":"http://169.254.169.254/latest/meta-data/"}}]}`),
			Explanation: "Attempted cloud metadata SSRF query",
			Model:       m.ModelName,
			TokensUsed:  60,
		}, nil
	}

	switch task {
	case TaskDraftWorkflow:
		content := `{
			"name": "AI Drafted Pipeline",
			"description": "Workflow drafted by Kerob AI assistant based on user specifications",
			"enabled": true,
			"trigger": {
				"type": "webhook",
				"event_types": ["*"]
			},
			"conditions": {
				"field": "event.priority",
				"operator": "equals",
				"value": "high"
			},
			"actions": [
				{
					"id": "step_notify",
					"name": "Send Notification",
					"type": "action",
					"integration": "http",
					"action_type": "request",
					"config": {
						"url": "https://api.telegram.org/bot",
						"method": "POST"
					}
				}
			]
		}`
		return &RawAIResult{
			Content:     json.RawMessage(content),
			Explanation: "Created a webhook-triggered workflow that checks for high priority alerts and dispatches an HTTP notification.",
			Model:       m.ModelName,
			TokensUsed:  120,
		}, nil

	case TaskMapFields:
		content := `{
			"customer_id": "event.payload.user_id",
			"order_total": "event.payload.amount",
			"currency": "event.payload.currency",
			"email": "event.payload.customer.email"
		}`
		return &RawAIResult{
			Content:     json.RawMessage(content),
			Explanation: "Mapped incoming webhook payload keys directly to destination integration parameters.",
			Model:       m.ModelName,
			TokensUsed:  75,
		}, nil

	case TaskGenerateConditions:
		content := `{
			"logical": "and",
			"rules": [
				{
					"field": "event.payload.status",
					"operator": "equals",
					"value": "failed"
				},
				{
					"field": "event.payload.amount",
					"operator": "greater_than",
					"value": 100
				}
			]
		}`
		return &RawAIResult{
			Content:     json.RawMessage(content),
			Explanation: "Generated nested AND condition filtering for failed transactions with amount greater than 100.",
			Model:       m.ModelName,
			TokensUsed:  90,
		}, nil

	case TaskSuggestTransformation:
		content := `{
			"type": "mapping",
			"field_maps": {
				"summary": "Order {{ event.payload.order_id }} processed",
				"total_usd": "event.payload.total"
			},
			"defaults": {
				"status": "completed"
			}
		}`
		return &RawAIResult{
			Content:     json.RawMessage(content),
			Explanation: "Suggested declarative mapping transformation with default values and template interpolation.",
			Model:       m.ModelName,
			TokensUsed:  85,
		}, nil

	case TaskExplainFailure:
		content := `{
			"summary": "Payment gateway HTTP 504 Gateway Timeout",
			"root_cause": "The remote HTTP endpoint failed to respond within the configured 30-second action timeout window.",
			"next_steps": [
				"Verify target server latency and health status",
				"Increase step timeout seconds if downstream processing is slow",
				"Enable retry policy with exponential backoff on transient errors"
			],
			"remediation": "Set step timeout_seconds to 60 and configure max_retries to 3."
		}`
		return &RawAIResult{
			Content:     json.RawMessage(content),
			Explanation: "Analyzed execution error log and identified remote endpoint latency timeout.",
			Model:       m.ModelName,
			TokensUsed:  140,
		}, nil

	default:
		return nil, fmt.Errorf("unsupported mock task %s", task)
	}
}

// --- 2. OpenAI / Local Compatible Provider ---

type OpenAIProvider struct {
	BaseURL    string
	APIKey     string
	Model      string
	HTTPClient *http.Client
}

func NewOpenAIProvider(baseURL, apiKey, model string, timeout time.Duration) *OpenAIProvider {
	if baseURL == "" {
		baseURL = "https://api.openai.com/v1"
	}
	baseURL = strings.TrimSuffix(baseURL, "/")
	if model == "" {
		model = "gpt-4o"
	}
	if timeout <= 0 {
		timeout = 30 * time.Second
	}
	return &OpenAIProvider{
		BaseURL:    baseURL,
		APIKey:     apiKey,
		Model:      model,
		HTTPClient: &http.Client{Timeout: timeout},
	}
}

func (p *OpenAIProvider) Name() string {
	return "openai"
}

func (p *OpenAIProvider) Generate(ctx context.Context, _ TaskType, systemPrompt string, userPrompt string) (*RawAIResult, error) {
	reqBody := map[string]any{
		"model": p.Model,
		"messages": []map[string]string{
			{"role": "system", "content": systemPrompt},
			{"role": "user", "content": userPrompt},
		},
		"response_format": map[string]string{"type": "json_object"},
		"temperature":     0.1,
	}

	payloadBytes, err := json.Marshal(reqBody)
	if err != nil {
		return nil, fmt.Errorf("failed to encode request: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, p.BaseURL+"/chat/completions", bytes.NewReader(payloadBytes))
	if err != nil {
		return nil, fmt.Errorf("failed to create http request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	if p.APIKey != "" {
		req.Header.Set("Authorization", "Bearer "+p.APIKey)
	}

	resp, err := p.HTTPClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("ai model request failed: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		errBytes, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("ai model returned HTTP %d: %s", resp.StatusCode, string(errBytes))
	}

	var chatResp struct {
		Choices []struct {
			Message struct {
				Content string `json:"content"`
			} `json:"message"`
		} `json:"choices"`
		Usage struct {
			TotalTokens int `json:"total_tokens"`
		} `json:"usage"`
	}

	if err := json.NewDecoder(resp.Body).Decode(&chatResp); err != nil {
		return nil, fmt.Errorf("failed to decode ai response: %w", err)
	}

	if len(chatResp.Choices) == 0 {
		return nil, fmt.Errorf("ai provider returned 0 choices")
	}

	rawText := chatResp.Choices[0].Message.Content

	var parsed struct {
		Content     json.RawMessage `json:"content"`
		Explanation string          `json:"explanation"`
	}
	if err := json.Unmarshal([]byte(rawText), &parsed); err == nil && len(parsed.Content) > 0 {
		return &RawAIResult{
			Content:     parsed.Content,
			Explanation: parsed.Explanation,
			Model:       p.Model,
			TokensUsed:  chatResp.Usage.TotalTokens,
		}, nil
	}

	return &RawAIResult{
		Content:     json.RawMessage(rawText),
		Explanation: "Generated by " + p.Model,
		Model:       p.Model,
		TokensUsed:  chatResp.Usage.TotalTokens,
	}, nil
}
