# Integration & Connector Developer Guide

This guide explains how to add new integrations, custom connectors, and OAuth 2.0 providers to **Kerob** without modifying core engine logic.

---

## 1. Architecture Overview

Kerob supports two levels of integration:
1. **Legacy Builtin Adapters (`internal/adapters`)**: Built-in actions and destinations compiled directly into the binary.
2. **Extensible Connector SDK (`pkg/connector`)**: Exported Go SDK allowing third-party developers, plugins, and custom integrations to implement triggers, actions, dynamic polling, real-time webhooks, OAuth 2.0 authorization flows, and dynamic health probes.

```
       +--------------------------------------------------------+
       |                  Workflow Engine / API                  |
       +--------------------------------------------------------+
                  |                                  |
       +--------------------+              +--------------------+
       |  adapters.Registry  |              |  ActionRegistry    |
       +--------------------+              +--------------------+
                  ^                                  ^
                  |                                  |
          BridgeToSystem()                   NewActionBridge()
                  |                                  |
       +--------------------------------------------------------+
       |               Kerob Connector SDK                      |
       |                (pkg/connector)                         |
       +--------------------------------------------------------+
            |                  |                    |
     [OAuth 2.0 Flow]    [Actions/Triggers]    [Health Checks]
            |                  |                    |
       GitHub Connector  Custom Connectors   Third-Party Plugins
```

---

## 2. Connector SDK Stable Interfaces (`pkg/connector`)

Every connector implements `connector.Connector` and one or more specialized capability interfaces:

### Core Interface
```go
type Connector interface {
    Manifest() Manifest
    ValidateConfig(config map[string]any) error
}
```

### Capability Interfaces
- **`ActionProvider`**: Dispatches workflow actions with custom parameters and credentials.
  ```go
  type ActionProvider interface {
      Connector
      ExecuteAction(ctx context.Context, req ActionRequest) (ActionResult, error)
  }
  ```
- **`TriggerProvider`**: Validates trigger configurations and normalizes inbound event payloads.
  ```go
  type TriggerProvider interface {
      Connector
      ValidateTrigger(triggerType string, config map[string]any) error
      NormalizeEvent(triggerType string, headers map[string]string, payload []byte) (map[string]any, error)
  }
  ```
- **`PollingProvider`**: Scheduled incremental state polling.
  ```go
  type PollingProvider interface {
      Connector
      Poll(ctx context.Context, req PollRequest) (PollResult, error)
  }
  ```
- **`WebhookManager`**: Registers and unregisters webhooks with external providers, and verifies cryptographic signatures (HMAC).
  ```go
  type WebhookManager interface {
      Connector
      SetupWebhook(ctx context.Context, req WebhookSetupRequest) (WebhookInfo, error)
      TeardownWebhook(ctx context.Context, webhookID string, creds map[string]string) error
      VerifyWebhookSignature(secret string, headers map[string]string, payload []byte) bool
  }
  ```
- **`HealthChecker`**: Live connectivity and authentication health probe.
  ```go
  type HealthChecker interface {
      Connector
      CheckHealth(ctx context.Context, creds map[string]string) (HealthStatus, error)
  }
  ```
- **`OAuth2Provider`**: Handles standard OAuth 2.0 code exchange, refresh token rotation, and revocation.
  ```go
  type OAuth2Provider interface {
      Connector
      GetAuthorizationURL(state string, redirectURI string, extraParams map[string]string) string
      ExchangeCode(ctx context.Context, code string, redirectURI string) (*OAuth2Token, error)
      RefreshToken(ctx context.Context, refreshToken string) (*OAuth2Token, error)
      RevokeToken(ctx context.Context, token string) error
  }
  ```
- **`SchemaProvider`**: Exposes JSON Schemas for dynamic frontend form generation.
  ```go
  type SchemaProvider interface {
      Connector
      GetSchema(operationType string) (map[string]any, error)
  }
  ```

---

## 3. OAuth 2.0 Architecture & Security

Kerob includes a hardened OAuth 2.0 subsystem in `internal/core/oauth`:

### Security Controls:
1. **CSRF State Protection**: High-entropy 32-byte cryptographically random nonces (`crypto/rand`) stored in an in-memory State Manager with a 10-minute TTL. States are single-use and bound to the initiating tenant ID and user ID.
2. **Encrypted Token Vault**: Exchanged tokens (`access_token`, `refresh_token`) are encrypted at rest using AES-256-GCM.
3. **Zero Secret Leakage**: `OAuth2Token` has `json:"-"` struct tags on secret fields. HTTP API responses, SSE streams, metrics, audit logs, and frontend models only expose masked strings (e.g. `••••••••1234`).
4. **Automated Refresh Rotation**: The `OAuthManager.RefreshToken()` API rotates both access and refresh tokens, ensuring compliance with expiring token providers (e.g., GitHub, Slack, Google).
5. **Revocation Support**: `RevokeToken()` invalidates tokens upstream at the provider before deleting credentials from the vault.

### OAuth API Endpoints:
- `GET /api/v1/oauth/{connector}/authorize`: Initiates authorization, returns upstream URL and state.
- `GET|POST /api/v1/oauth/{connector}/callback`: Verifies state, exchanges code for tokens, stores encrypted credentials in vault.
- `POST /api/v1/oauth/{connector}/refresh`: Rotates refresh tokens and updates encrypted credentials.
- `POST /api/v1/oauth/{connector}/revoke`: Revokes authorization upstream and deletes credentials.

---

## 4. Reference Implementation: GitHub Connector

The reference OAuth connector is implemented in `pkg/connector/builtin/github/`:
- **Capabilities**: `oauth2`, `actions`, `triggers`, `webhooks`, `health_check`.
- **Supported Actions**:
  - `create_issue`: Opens issues with title, body, and labels.
  - `post_comment`: Adds comments to existing issues or pull requests.
  - `add_labels`: Adds labels to issues or PRs.
- **Webhook Management**: Automates creation and teardown of repository webhooks; validates inbound `X-Hub-Signature-256` HMAC-SHA256 headers.
- **Health Verification**: Calls `GET /user` to verify Bearer token authenticity and report API latency.

---

## 5. Building a Third-Party Connector

See `examples/custom_connector/main.go` for a complete standalone example.

### Minimal Connector Example:
```go
package main

import (
    "context"
    "fmt"
    "kerob/pkg/connector"
)

type MyCustomConnector struct{}

func (c *MyCustomConnector) Manifest() connector.Manifest {
    return connector.Manifest{
        ID:           "my_custom",
        Name:         "My Custom Service",
        Version:      "1.0.0",
        Capabilities: []string{connector.CapabilityAction, connector.CapabilityHealthCheck},
        AuthType:     "apiKey",
        Actions: []connector.ActionMetadata{
            {Type: "send_ping", Name: "Send Ping", Description: "Dispatches a ping request"},
        },
    }
}

func (c *MyCustomConnector) ValidateConfig(config map[string]any) error {
    return nil
}

func (c *MyCustomConnector) CheckHealth(ctx context.Context, creds map[string]string) (connector.HealthStatus, error) {
    return connector.HealthStatus{Status: "healthy", LatencyMs: 10}, nil
}

func (c *MyCustomConnector) ExecuteAction(ctx context.Context, req connector.ActionRequest) (connector.ActionResult, error) {
    return connector.ActionResult{
        Success: true,
        Output:  map[string]any{"status": "ok"},
    }, nil
}

func main() {
    // Register with Kerob SDK
    _ = connector.Register(&MyCustomConnector{})
}
```

---

## 6. Conformance & Contract Testing

Kerob provides a contract verification suite in `pkg/connector/contract_test.go`. Any connector can be tested for full specification conformance:

```go
func TestMyConnectorContract(t *testing.T) {
    c := NewMyCustomConnector()
    connector_test.AssertConnectorContract(t, c)
}
```

---

## 7. Best Practices Checklist for Connectors

- [ ] Declare capabilities accurately in `Manifest().Capabilities`.
- [ ] Implement `HealthChecker` to enable dashboard live status indicators.
- [ ] Use `ssrf.NewSafeHTTPClient()` or configurable HTTP clients for all network calls.
- [ ] Never log, print, or serialize plaintext tokens or secrets.
- [ ] Implement exponential backoff retry policies for rate-limited (HTTP 429) and transient (HTTP 5xx) errors.
- [ ] Run contract conformance tests with `go test -v -race ./pkg/connector/...`.
