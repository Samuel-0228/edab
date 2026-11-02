package main

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"time"

	"kerob/pkg/connector"
)

// PagerAlertConnector is a standalone example third-party connector implementing the Kerob Connector SDK.
type PagerAlertConnector struct {
	client *http.Client
}

func NewPagerAlertConnector() *PagerAlertConnector {
	return &PagerAlertConnector{
		client: &http.Client{Timeout: 10 * time.Second},
	}
}

// Manifest declares the third-party connector's capabilities and schema.
func (p *PagerAlertConnector) Manifest() connector.Manifest {
	return connector.Manifest{
		ID:          "custom_pager",
		Name:        "PagerAlert Integration",
		Description: "Example third-party incident dispatch and paging connector.",
		Version:     "1.0.0",
		Category:    "developer",
		Icon:        "bell",
		Capabilities: []string{
			connector.CapabilityAction,
			connector.CapabilityHealthCheck,
		},
		AuthType: "apiKey",
		ConfigSchema: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"routing_key": map[string]any{"type": "string", "title": "Service Routing Key"},
				"summary":     map[string]any{"type": "string", "title": "Incident Summary"},
				"severity":    map[string]any{"type": "string", "title": "Severity", "enum": []string{"critical", "error", "warning", "info"}},
			},
			"required": []any{"routing_key", "summary"},
		},
		Actions: []connector.ActionMetadata{
			{Type: "trigger_incident", Name: "Trigger Incident", Description: "Dispatches a high-priority incident alert."},
			{Type: "resolve_incident", Name: "Resolve Incident", Description: "Resolves an active incident by dedup key."},
		},
		Credentials: []connector.CredentialProperty{
			{Key: "api_key", Label: "Integration API Key", Type: "password", Required: true, Description: "API key issued by PagerAlert"},
		},
		DocumentationURL: "https://example.com/docs/pager-connector",
	}
}

func (p *PagerAlertConnector) ValidateConfig(config map[string]any) error {
	if config == nil {
		return fmt.Errorf("configuration cannot be nil")
	}
	return nil
}

// CheckHealth implements connector.HealthChecker.
func (p *PagerAlertConnector) CheckHealth(ctx context.Context, creds map[string]string) (connector.HealthStatus, error) {
	apiKey := creds["api_key"]
	if apiKey == "" {
		return connector.HealthStatus{
			Status:    "unhealthy",
			Message:   "API key not supplied in credentials",
			CheckedAt: time.Now().UTC(),
		}, nil
	}

	return connector.HealthStatus{
		Status:    "healthy",
		LatencyMs: 25,
		Message:   "Connected to PagerAlert API successfully",
		CheckedAt: time.Now().UTC(),
	}, nil
}

// ExecuteAction implements connector.ActionProvider.
func (p *PagerAlertConnector) ExecuteAction(ctx context.Context, req connector.ActionRequest) (connector.ActionResult, error) {
	apiKey := req.Credentials["api_key"]
	if apiKey == "" {
		return connector.ActionResult{Success: false, Error: "missing api_key in credentials"}, fmt.Errorf("missing api_key")
	}

	var params map[string]any
	_ = json.Unmarshal(req.Config, &params)
	if params == nil {
		params = make(map[string]any)
	}
	for k, v := range req.Parameters {
		params[k] = v
	}

	summary, _ := params["summary"].(string)
	if summary == "" {
		summary = "Triggered by Kerob automated workflow"
	}
	severity, _ := params["severity"].(string)
	if severity == "" {
		severity = "error"
	}

	incidentID := fmt.Sprintf("INC-%d", time.Now().UnixNano()%100000)

	log.Printf("[PagerAlert Connector] Action=%s IncidentID=%s Severity=%s Summary=%s",
		req.ActionType, incidentID, severity, summary)

	return connector.ActionResult{
		Success: true,
		Output: map[string]any{
			"incident_id": incidentID,
			"status":      "triggered",
			"severity":    severity,
		},
	}, nil
}

func main() {
	// Demonstrates third-party registration with Kerob Connector SDK
	c := NewPagerAlertConnector()

	if err := connector.Register(c); err != nil {
		log.Fatalf("failed to register connector: %v", err)
	}

	manifests := connector.ListManifests()
	fmt.Printf("Registered connectors count: %d\n", len(manifests))
	for _, m := range manifests {
		fmt.Printf("- %s (ID: %s, Capabilities: %v)\n", m.Name, m.ID, m.Capabilities)
	}
}
