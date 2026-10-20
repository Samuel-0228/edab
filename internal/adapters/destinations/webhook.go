package destinations

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"time"

	"kerob/internal/core/event"
)

// WebhookConfig defines options for dispatching events to an HTTP endpoint.
type WebhookConfig struct {
	TargetURL string            `json:"target_url"`
	Headers   map[string]string `json:"headers,omitempty"`
	Timeout   time.Duration     `json:"timeout,omitempty"`
}

// WebhookDestinationAdapter delivers events via HTTP POST.
type WebhookDestinationAdapter struct {
	mu        sync.Mutex
	client    *http.Client
	config    WebhookConfig
	connected bool
}

func NewWebhookDestinationAdapter() *WebhookDestinationAdapter {
	return &WebhookDestinationAdapter{
		client: &http.Client{
			Timeout: 10 * time.Second,
			CheckRedirect: func(req *http.Request, via []*http.Request) error {
				return fmt.Errorf("redirects are not allowed for webhook destinations")
			},
		},
	}
}

func (w *WebhookDestinationAdapter) Name() string {
	return "webhook"
}

func (w *WebhookDestinationAdapter) Connect(ctx context.Context, config json.RawMessage) error {
	w.mu.Lock()
	defer w.mu.Unlock()

	if len(config) > 0 && string(config) != "{}" {
		var cfg struct {
			TargetURL      string            `json:"target_url"`
			Headers        map[string]string `json:"headers"`
			TimeoutSeconds int               `json:"timeout_seconds"`
		}
		if err := json.Unmarshal(config, &cfg); err != nil {
			return fmt.Errorf("invalid webhook destination config: %w", err)
		}
		if err := validateTargetURL(ctx, cfg.TargetURL); err != nil {
			return err
		}
		w.config.TargetURL = cfg.TargetURL
		w.config.Headers = cfg.Headers
		if cfg.TimeoutSeconds > 0 {
			w.config.Timeout = time.Duration(cfg.TimeoutSeconds) * time.Second
			w.client.Timeout = w.config.Timeout
		}

	}

	w.connected = true
	return nil
}

func validateTargetURL(ctx context.Context, rawURL string) error {
	parsed, err := url.Parse(strings.TrimSpace(rawURL))
	if err != nil {
		return fmt.Errorf("invalid webhook target URL: %w", err)
	}
	if parsed.Scheme != "http" && parsed.Scheme != "https" {
		return fmt.Errorf("webhook target URL must use http or https")
	}
	if parsed.User != nil || parsed.Hostname() == "" {
		return fmt.Errorf("webhook target URL must not contain credentials")
	}

	host := strings.ToLower(parsed.Hostname())
	if host == "localhost" || strings.HasSuffix(host, ".localhost") || host == "metadata.google.internal" {
		return fmt.Errorf("webhook target URL points to a blocked host")
	}

	ips, err := net.DefaultResolver.LookupIP(ctx, "ip", host)
	if err != nil {
		return fmt.Errorf("failed to resolve webhook target host: %w", err)
	}
	for _, ip := range ips {
		if isBlockedIP(ip) {
			return fmt.Errorf("webhook target URL points to a private or local address")
		}
	}
	return nil
}

func isBlockedIP(ip net.IP) bool {
	return ip.IsLoopback() || ip.IsPrivate() || ip.IsLinkLocalUnicast() ||
		ip.IsUnspecified() || ip.IsMulticast()
}

func (w *WebhookDestinationAdapter) Health(ctx context.Context) error {
	w.mu.Lock()
	defer w.mu.Unlock()
	if !w.connected {
		return fmt.Errorf("webhook destination adapter is not connected")
	}
	return nil
}

func (w *WebhookDestinationAdapter) Deliver(ctx context.Context, ev *event.Event) error {
	w.mu.Lock()
	targetURL := w.config.TargetURL
	headers := w.config.Headers
	client := w.client
	w.mu.Unlock()

	if targetURL == "" {
		return fmt.Errorf("target_url is not configured for webhook destination")
	}

	body, err := json.Marshal(ev)
	if err != nil {
		return fmt.Errorf("failed to marshal event for webhook: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, targetURL, bytes.NewReader(body))
	if err != nil {
		return fmt.Errorf("failed to build webhook request: %w", err)
	}

	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("User-Agent", "kerob/1.0")
	for k, v := range headers {
		req.Header.Set(k, v)
	}

	resp, err := client.Do(req)
	if err != nil {
		return fmt.Errorf("webhook request failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		respBody, _ := io.ReadAll(io.LimitReader(resp.Body, 1024))
		return fmt.Errorf("webhook returned non-2xx status %d: %s", resp.StatusCode, string(respBody))
	}

	return nil
}
