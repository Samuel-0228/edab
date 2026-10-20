# Integration Developer Guide

This guide explains how to add a new integration or adapter to **Kerob** without modifying core engine logic.

---

## 1. Integration Concepts

An integration in Kerob consists of:
1. **Metadata**: Registered in `internal/adapters/registry.go` with its capabilities (triggers, actions, credential keys).
2. **Trigger (Optional)**: Ingests external updates, normalizes data, and publishes an `event.Event`.
3. **Action (Optional)**: Implements `workflow.ActionExecutor` to execute work on the external system.

---

## 2. Implementing an Action Adapter

All actions implement the `workflow.ActionExecutor` interface in `internal/core/workflow/action.go`:

```go
type ActionExecutor interface {
    Name() string
    Type() string
    Validate(config json.RawMessage) error
    Execute(ctx context.Context, req ActionRequest) (ActionResult, error)
}
```

### Example: Slack Message Action

Create a new file `internal/adapters/actions/slack_action.go`:

```go
package actions

import (
    "bytes"
    "context"
    "encoding/json"
    "fmt"
    "net/http"
    "time"

    "kerob/internal/adapters/ssrf"
    "kerob/internal/core/workflow"
)

type SlackActionConfig struct {
    Channel string `json:"channel"`
    Text    string `json:"text"`
}

type SlackActionExecutor struct{}

func NewSlackActionExecutor() *SlackActionExecutor {
    return &SlackActionExecutor{}
}

func (s *SlackActionExecutor) Name() string { return "Slack" }
func (s *SlackActionExecutor) Type() string { return "slack" }

func (s *SlackActionExecutor) Validate(config json.RawMessage) error {
    var cfg SlackActionConfig
    if err := json.Unmarshal(config, &cfg); err != nil {
        return fmt.Errorf("invalid slack config: %w", err)
    }
    if cfg.Channel == "" {
        return fmt.Errorf("slack channel is required")
    }
    return nil
}

func (s *SlackActionExecutor) Execute(ctx context.Context, req workflow.ActionRequest) (workflow.ActionResult, error) {
    var cfg SlackActionConfig
    if err := json.Unmarshal(req.Config, &cfg); err != nil {
        return workflow.ActionResult{Success: false}, err
    }

    token := req.Credentials["bot_token"]
    if token == "" {
        return workflow.ActionResult{Success: false}, fmt.Errorf("slack bot_token is required")
    }

    text := cfg.Text
    if text == "" {
        if t, ok := req.Payload["text"].(string); ok {
            text = t
        }
    }

    payload := map[string]any{
        "channel": cfg.Channel,
        "text":    text,
    }
    body, _ := json.Marshal(payload)

    httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, "https://slack.com/api/chat.postMessage", bytes.NewReader(body))
    if err != nil {
        return workflow.ActionResult{Success: false}, err
    }
    httpReq.Header.Set("Authorization", "Bearer "+token)
    httpReq.Header.Set("Content-Type", "application/json")

    // Use SSRF-protected safe HTTP client
    client := ssrf.NewSafeHTTPClient(15 * time.Second)
    resp, err := client.Do(httpReq)
    if err != nil {
        return workflow.ActionResult{Success: false}, err
    }
    defer resp.Body.Close()

    if resp.StatusCode != http.StatusOK {
        return workflow.ActionResult{Success: false}, fmt.Errorf("slack api error HTTP %d", resp.StatusCode)
    }

    return workflow.ActionResult{
        Success: true,
        Output:  json.RawMessage(`{"status":"delivered"}`),
    }, nil
}
```

### Registering the Action

In `cmd/server/main.go`:
```go
actionRegistry.Register(actions.NewSlackActionExecutor())
```

---

## 3. Implementing a Trigger Adapter

Triggers produce normalized `event.Event` structures:

```go
ev, err := event.New(
    sourceID,                         // e.g. "slack-listener"
    fmt.Sprintf("slack:%s", msgID),   // Unique external event ID
    "slack.message.created",          // Normalized event type
    payloadMap,                       // Event payload
    metadataMap,                      // Context metadata
)
if err != nil {
    return err
}

// Ingest into engine
_, err = workflowEngine.ProcessEvent(ctx, ev)
```

---

## 4. Best Practices Checklist for New Integrations

- [ ] Register metadata in `internal/adapters/registry.go`.
- [ ] Implement `Validate()` for the action configuration.
- [ ] Use `ssrf.NewSafeHTTPClient()` for all outbound HTTP requests.
- [ ] Never log, print, or leak credentials from `req.Credentials`.
- [ ] Support templated string interpolation in configurations.
- [ ] Write unit tests verifying successful execution and failure paths.
- [ ] Run `go test -race ./...` to verify zero data races.
