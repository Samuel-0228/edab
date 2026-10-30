package actions

import (
	"bytes"
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"

	"kerob/internal/adapters/ssrf"
	"kerob/internal/core/workflow"
)

// HTTPActionConfig holds configuration for calling an external HTTP API.
type HTTPActionConfig struct {
	Method        string            `json:"method"`                 // GET, POST, PUT, PATCH, DELETE
	URL           string            `json:"url"`                    // Target URL
	Headers       map[string]string `json:"headers,omitempty"`      // Custom headers
	QueryParams   map[string]string `json:"query_params,omitempty"` // Query parameters
	Body          string            `json:"body,omitempty"`         // Request body (JSON or text)
	TimeoutSec    int               `json:"timeout_seconds,omitempty"`
	SigningSecret string            `json:"signing_secret,omitempty"` // HMAC-SHA256 secret for request signing
	SigningHeader string            `json:"signing_header,omitempty"` // Header name, default: X-Kerob-Signature-256
}

type HTTPActionExecutor struct {
	client *http.Client
}

func NewHTTPActionExecutor() *HTTPActionExecutor {
	return &HTTPActionExecutor{}
}

func NewHTTPActionExecutorWithClient(client *http.Client) *HTTPActionExecutor {
	return &HTTPActionExecutor{client: client}
}

func (h *HTTPActionExecutor) Name() string { return "Generic HTTP Action" }
func (h *HTTPActionExecutor) Type() string { return "http" }

func (h *HTTPActionExecutor) Validate(config json.RawMessage) error {
	var cfg HTTPActionConfig
	if err := json.Unmarshal(config, &cfg); err != nil {
		return fmt.Errorf("invalid http action config: %w", err)
	}
	if cfg.URL == "" {
		return fmt.Errorf("http action url is required")
	}
	method := strings.ToUpper(cfg.Method)
	if method == "" {
		method = http.MethodPost
	}
	validMethods := map[string]bool{
		http.MethodGet:    true,
		http.MethodPost:   true,
		http.MethodPut:    true,
		http.MethodPatch:  true,
		http.MethodDelete: true,
	}
	if !validMethods[method] {
		return fmt.Errorf("unsupported http method: %s", cfg.Method)
	}
	return nil
}

func (h *HTTPActionExecutor) Execute(ctx context.Context, req workflow.ActionRequest) (workflow.ActionResult, error) {
	var cfg HTTPActionConfig
	if err := json.Unmarshal(req.Config, &cfg); err != nil {
		return workflow.ActionResult{Success: false}, fmt.Errorf("invalid http config: %w", err)
	}

	method := strings.ToUpper(cfg.Method)
	if method == "" {
		method = http.MethodPost
	}

	// 1. SSRF Validation
	if err := ssrf.ValidateURL(ctx, cfg.URL); err != nil {
		return workflow.ActionResult{Success: false}, fmt.Errorf("SSRF protection blocked URL %s: %w", cfg.URL, err)
	}

	// 2. Build target URL with query params
	targetURL, err := url.Parse(cfg.URL)
	if err != nil {
		return workflow.ActionResult{Success: false}, fmt.Errorf("failed to parse url: %w", err)
	}
	q := targetURL.Query()
	for k, v := range cfg.QueryParams {
		q.Set(k, v)
	}
	targetURL.RawQuery = q.Encode()

	// 3. Prepare Body
	var bodyReader io.Reader
	if cfg.Body != "" {
		bodyReader = strings.NewReader(cfg.Body)
	} else if len(req.Payload) > 0 && (method == http.MethodPost || method == http.MethodPut || method == http.MethodPatch) {
		payloadBytes, err := json.Marshal(req.Payload)
		if err == nil {
			bodyReader = bytes.NewReader(payloadBytes)
		}
	}

	httpReq, err := http.NewRequestWithContext(ctx, method, targetURL.String(), bodyReader)
	if err != nil {
		return workflow.ActionResult{Success: false}, fmt.Errorf("failed to construct http request: %w", err)
	}

	httpReq.Header.Set("User-Agent", "kerob/1.0 (automation-engine)")
	if bodyReader != nil {
		httpReq.Header.Set("Content-Type", "application/json")
	}

	// Apply configured headers
	for k, v := range cfg.Headers {
		httpReq.Header.Set(k, v)
	}

	// Inject credentials if provided (e.g. Bearer token or API key)
	if apiKey, ok := req.Credentials["api_key"]; ok && apiKey != "" {
		if !strings.HasPrefix(strings.ToLower(apiKey), "bearer ") {
			httpReq.Header.Set("Authorization", "Bearer "+apiKey)
		} else {
			httpReq.Header.Set("Authorization", apiKey)
		}
	}

	// Outbound request signing if configured
	signingSecret := cfg.SigningSecret
	if signingSecret == "" && req.Credentials != nil {
		signingSecret = req.Credentials["signing_secret"]
	}
	if signingSecret != "" {
		signingHeader := cfg.SigningHeader
		if signingHeader == "" {
			signingHeader = "X-Kerob-Signature-256"
		}
		var bodyBytes []byte
		if cfg.Body != "" {
			bodyBytes = []byte(cfg.Body)
		} else if len(req.Payload) > 0 {
			bodyBytes, _ = json.Marshal(req.Payload)
		}
		ts := fmt.Sprintf("%d", time.Now().Unix())
		payloadToSign := append([]byte(ts+"."), bodyBytes...)
		mac := hmac.New(sha256.New, []byte(signingSecret))
		mac.Write(payloadToSign)
		sig := "sha256=" + hex.EncodeToString(mac.Sum(nil))

		httpReq.Header.Set(signingHeader, sig)
		httpReq.Header.Set("X-Kerob-Timestamp", ts)
	}

	// 4. Send request with SSRF-safe client (or injected client)
	timeout := 15 * time.Second
	if cfg.TimeoutSec > 0 {
		timeout = time.Duration(cfg.TimeoutSec) * time.Second
	}
	client := h.client
	if client == nil {
		client = ssrf.NewSafeHTTPClient(timeout)
	}

	resp, err := client.Do(httpReq)
	if err != nil {
		return workflow.ActionResult{Success: false}, fmt.Errorf("http request execution failed: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()

	respBody, _ := io.ReadAll(io.LimitReader(resp.Body, 16*1024)) // limit to 16 KB

	outputData := map[string]any{
		"status_code": resp.StatusCode,
		"status":      resp.Status,
		"body":        string(respBody),
	}
	outputJSON, _ := json.Marshal(outputData)

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return workflow.ActionResult{
			Success: false,
			Output:  outputJSON,
		}, fmt.Errorf("http action returned status %d: %s", resp.StatusCode, string(respBody))
	}

	return workflow.ActionResult{
		Success: true,
		Output:  outputJSON,
		Metadata: map[string]any{
			"status_code": resp.StatusCode,
		},
	}, nil
}
