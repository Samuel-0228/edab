package connector

import (
	"encoding/json"
	"strings"
	"time"
)

// Standard capability flags for capability discovery.
const (
	CapabilityTrigger     = "triggers"
	CapabilityPolling     = "polling"
	CapabilityWebhook     = "webhooks"
	CapabilityAction      = "actions"
	CapabilityOAuth2      = "oauth2"
	CapabilityHealthCheck = "health_check"
)

// Manifest defines the declarative metadata, capabilities, schemas,
// and credential requirements for a Kerob connector.
type Manifest struct {
	ID                     string                    `json:"id"`
	Name                   string                    `json:"name"`
	Description            string                    `json:"description"`
	Version                string                    `json:"version"`
	Category               string                    `json:"category"` // "messaging", "developer", "feeds", "network", "system"
	Icon                   string                    `json:"icon"`
	Capabilities           []string                  `json:"capabilities"`
	AuthType               string                    `json:"auth_type"` // "oauth2", "token", "apiKey", "basic", "none"
	OAuth2Config           *OAuth2Config             `json:"oauth2_config,omitempty"`
	RequiredCredentialType string                    `json:"required_credential_type,omitempty"`
	ConfigSchema           map[string]any            `json:"config_schema,omitempty"`
	Schemas                map[string]map[string]any `json:"schemas,omitempty"`
	Triggers               []TriggerMetadata         `json:"triggers"`
	Actions                []ActionMetadata          `json:"actions"`
	Credentials            []CredentialProperty      `json:"credentials"`
	DocumentationURL       string                    `json:"documentation_url,omitempty"`
}

// HasCapability checks if a connector declares a specific capability.
func (m Manifest) HasCapability(cap string) bool {
	for _, c := range m.Capabilities {
		if strings.EqualFold(c, cap) {
			return true
		}
	}
	return false
}

// TriggerMetadata describes a trigger event emitted or accepted by a connector.
type TriggerMetadata struct {
	Type        string `json:"type"`
	Name        string `json:"name"`
	Description string `json:"description"`
}

// ActionMetadata describes an action operation provided by a connector.
type ActionMetadata struct {
	Type        string `json:"type"`
	Name        string `json:"name"`
	Description string `json:"description"`
}

// CredentialProperty defines a required or optional credential secret for this connector.
type CredentialProperty struct {
	Key         string `json:"key"`
	Label       string `json:"label"`
	Type        string `json:"type"` // "text", "password"
	Required    bool   `json:"required"`
	Description string `json:"description"`
}

// HealthStatus represents the runtime health of a connector and its external service.
type HealthStatus struct {
	Status    string         `json:"status"` // "healthy", "degraded", "unhealthy"
	LatencyMs int64          `json:"latency_ms"`
	Message   string         `json:"message,omitempty"`
	CheckedAt time.Time      `json:"checked_at"`
	Details   map[string]any `json:"details,omitempty"`
}

// ActionRequest contains input parameters for an action execution.
type ActionRequest struct {
	ExecutionID string            `json:"execution_id"`
	StepID      string            `json:"step_id"`
	ActionType  string            `json:"action_type"`
	Config      json.RawMessage   `json:"config"`
	Parameters  map[string]any    `json:"parameters"`
	Credentials map[string]string `json:"-"` // Redacted from serialization
	Event       map[string]any    `json:"event,omitempty"`
}

// ActionResult is returned by a connector action after execution.
type ActionResult struct {
	Success    bool           `json:"success"`
	Output     map[string]any `json:"output,omitempty"`
	Error      string         `json:"error,omitempty"`
	Retryable  bool           `json:"retryable,omitempty"`
	RetryAfter time.Duration  `json:"retry_after,omitempty"`
}

// PollRequest contains parameters for polling triggers.
type PollRequest struct {
	TriggerType string            `json:"trigger_type"`
	Config      map[string]any    `json:"config"`
	Cursor      map[string]string `json:"cursor,omitempty"`
	Limit       int               `json:"limit,omitempty"`
	Credentials map[string]string `json:"-"`
}

// PollResult represents events discovered by a polling trigger.
type PollResult struct {
	Events    []map[string]any  `json:"events"`
	NewCursor map[string]string `json:"new_cursor,omitempty"`
	HasMore   bool              `json:"has_more"`
}

// WebhookSetupRequest specifies configuration to register an external webhook.
type WebhookSetupRequest struct {
	WebhookURL  string            `json:"webhook_url"`
	Secret      string            `json:"secret"`
	Events      []string          `json:"events"`
	Config      map[string]any    `json:"config"`
	Credentials map[string]string `json:"-"`
}

// WebhookInfo describes an active webhook subscription.
type WebhookInfo struct {
	ID         string    `json:"id"`
	ExternalID string    `json:"external_id"`
	WebhookURL string    `json:"webhook_url"`
	Events     []string  `json:"events"`
	Active     bool      `json:"active"`
	CreatedAt  time.Time `json:"created_at"`
}

// OAuth2Config defines static OAuth2 client settings.
// Secrets like ClientSecret should be supplied via environment variables or encrypted credentials.
type OAuth2Config struct {
	ClientID        string            `json:"client_id"`
	ClientSecret    string            `json:"-"` // Never expose in JSON
	AuthURL         string            `json:"auth_url"`
	TokenURL        string            `json:"token_url"`
	RevokeURL       string            `json:"revoke_url,omitempty"`
	Scopes          []string          `json:"scopes"`
	PKCEEnabled     bool              `json:"pkce_enabled"`
	ExtraAuthParams map[string]string `json:"extra_auth_params,omitempty"`
}

// OAuth2Token represents an active OAuth2 token response.
// Sensitive token strings are strictly excluded from JSON serialization
// to prevent accidental leakage in HTTP logs or client responses.
type OAuth2Token struct {
	AccessToken  string         `json:"-"` // Excluded from JSON
	RefreshToken string         `json:"-"` // Excluded from JSON
	TokenType    string         `json:"token_type"`
	ExpiresAt    time.Time      `json:"expires_at"`
	Scopes       []string       `json:"scopes,omitempty"`
	Raw          map[string]any `json:"-"`
}

// Expired checks whether the access token has passed its expiration time.
func (t *OAuth2Token) Expired() bool {
	if t.ExpiresAt.IsZero() {
		return false
	}
	return time.Now().UTC().After(t.ExpiresAt)
}

// MaskedAccessToken returns a safely redacted representation for UI/logs.
func (t *OAuth2Token) MaskedAccessToken() string {
	return maskSecret(t.AccessToken)
}

// MaskedRefreshToken returns a safely redacted representation for UI/logs.
func (t *OAuth2Token) MaskedRefreshToken() string {
	return maskSecret(t.RefreshToken)
}

// ToCredentialMap stores the token fields in an encrypted credential dictionary.
func (t *OAuth2Token) ToCredentialMap() map[string]string {
	m := make(map[string]string)
	if t.AccessToken != "" {
		m["access_token"] = t.AccessToken
	}
	if t.RefreshToken != "" {
		m["refresh_token"] = t.RefreshToken
	}
	if t.TokenType != "" {
		m["token_type"] = t.TokenType
	}
	if !t.ExpiresAt.IsZero() {
		m["expires_at"] = t.ExpiresAt.Format(time.RFC3339)
	}
	if len(t.Scopes) > 0 {
		m["scopes"] = strings.Join(t.Scopes, " ")
	}
	return m
}

// FromCredentialMap reconstructs an OAuth2Token from a decrypted credential dictionary.
func FromCredentialMap(values map[string]string) *OAuth2Token {
	if values == nil {
		return &OAuth2Token{}
	}
	t := &OAuth2Token{
		AccessToken:  values["access_token"],
		RefreshToken: values["refresh_token"],
		TokenType:    values["token_type"],
	}
	if expStr, ok := values["expires_at"]; ok && expStr != "" {
		if parsed, err := time.Parse(time.RFC3339, expStr); err == nil {
			t.ExpiresAt = parsed
		}
	}
	if scopesStr, ok := values["scopes"]; ok && scopesStr != "" {
		t.Scopes = strings.Split(scopesStr, " ")
	}
	return t
}

func maskSecret(secret string) string {
	secret = strings.TrimSpace(secret)
	if len(secret) == 0 {
		return ""
	}
	if len(secret) <= 4 {
		return "••••••••"
	}
	visibleLen := 4
	if len(secret) > 20 {
		visibleLen = 6
	}
	return "••••••••" + secret[len(secret)-visibleLen:]
}

// RetryPolicy defines backoff and retry rules for outbound connector calls.
type RetryPolicy struct {
	MaxRetries           int
	InitialInterval      time.Duration
	MaxInterval          time.Duration
	BackoffMultiplier    float64
	RetryableStatusCodes []int
}

// DefaultRetryPolicy returns sensible defaults for HTTP connectors.
func DefaultRetryPolicy() RetryPolicy {
	return RetryPolicy{
		MaxRetries:           3,
		InitialInterval:      500 * time.Millisecond,
		MaxInterval:          10 * time.Second,
		BackoffMultiplier:    2.0,
		RetryableStatusCodes: []int{429, 500, 502, 503, 504},
	}
}

// IsRetryable checks if an HTTP status code is marked as retryable.
func (p RetryPolicy) IsRetryable(statusCode int) bool {
	for _, code := range p.RetryableStatusCodes {
		if code == statusCode {
			return true
		}
	}
	return false
}
