package connector

import (
	"context"
)

// Connector is the foundational interface that all Kerob connectors must implement.
type Connector interface {
	// Manifest returns the declarative metadata and capability declarations.
	Manifest() Manifest

	// ValidateConfig validates connector-level configuration parameters.
	ValidateConfig(config map[string]any) error
}

// ActionProvider is implemented by connectors that can execute automation actions.
type ActionProvider interface {
	Connector
	// ExecuteAction runs an action operation using the provided credentials and parameters.
	ExecuteAction(ctx context.Context, req ActionRequest) (ActionResult, error)
}

// TriggerProvider is implemented by connectors that support inbound trigger normalization.
type TriggerProvider interface {
	Connector
	// ValidateTrigger verifies trigger-specific parameters and filters.
	ValidateTrigger(triggerType string, config map[string]any) error

	// NormalizeEvent converts a raw external payload and headers into an event map.
	NormalizeEvent(triggerType string, headers map[string]string, payload []byte) (map[string]any, error)
}

// PollingProvider is implemented by connectors that discover events by scheduled polling.
type PollingProvider interface {
	Connector
	// Poll queries the external resource using an incremental cursor.
	Poll(ctx context.Context, req PollRequest) (PollResult, error)
}

// WebhookManager is implemented by connectors that manage real-time webhook subscriptions.
type WebhookManager interface {
	Connector
	// SetupWebhook registers a webhook endpoint at the upstream provider.
	SetupWebhook(ctx context.Context, req WebhookSetupRequest) (WebhookInfo, error)

	// TeardownWebhook deletes or disables the webhook at the upstream provider.
	TeardownWebhook(ctx context.Context, webhookID string, creds map[string]string) error

	// VerifyWebhookSignature checks authenticity of inbound webhook requests.
	VerifyWebhookSignature(secret string, headers map[string]string, payload []byte) bool
}

// HealthChecker is implemented by connectors that support runtime health and connectivity verification.
type HealthChecker interface {
	Connector
	// CheckHealth verifies connectivity and API authentication with the external service.
	CheckHealth(ctx context.Context, creds map[string]string) (HealthStatus, error)
}

// OAuth2Provider is implemented by connectors that authenticate via OAuth 2.0.
type OAuth2Provider interface {
	Connector
	// GetAuthorizationURL builds the authorization URL with state, redirect URI, and scopes.
	GetAuthorizationURL(state string, redirectURI string, extraParams map[string]string) string

	// ExchangeCode exchanges an authorization code for an OAuth2 token set.
	ExchangeCode(ctx context.Context, code string, redirectURI string) (*OAuth2Token, error)

	// RefreshToken exchanges an active refresh token for a newly issued token set.
	RefreshToken(ctx context.Context, refreshToken string) (*OAuth2Token, error)

	// RevokeToken invalidates the specified token at the upstream OAuth server.
	RevokeToken(ctx context.Context, token string) error
}

// SchemaProvider is implemented by connectors that provide dynamic JSON schemas for actions/triggers.
type SchemaProvider interface {
	Connector
	// GetSchema returns the JSON Schema for the specified action or trigger type.
	GetSchema(operationType string) (map[string]any, error)
}
