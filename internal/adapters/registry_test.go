package adapters

import (
	"testing"
)

func TestCatalogListsSupportedIntegrationsAndCapabilities(t *testing.T) {
	catalog := Catalog()
	if len(catalog) < 6 {
		t.Fatalf("expected the full integration catalog, got %d entries", len(catalog))
	}

	byID := make(map[string]IntegrationMetadata, len(catalog))
	for _, integration := range catalog {
		if integration.ID == "" || integration.Name == "" || integration.Category == "" {
			t.Fatalf("catalog entry is incomplete: %+v", integration)
		}
		if _, exists := byID[integration.ID]; exists {
			t.Fatalf("duplicate integration id %q", integration.ID)
		}
		byID[integration.ID] = integration
	}

	tests := []struct {
		id            string
		kind          string
		trigger       string
		action        string
		credentialKey string
	}{
		{"telegram", "both", "telegram.message", "send_message", "bot_token"},
		{"discord", "both", "discord.webhook", "send_message", "webhook_url"},
		{"rss", "trigger", "rss.item", "", ""},
		{"github", "both", "github.push", "create_issue", "personal_access_token"},
		{"webhook", "trigger", "webhook.received", "", ""},
		{"http", "action", "", "http_request", "api_key"},
		{"schedule", "trigger", "schedule.tick", "", ""},
	}
	for _, tt := range tests {
		t.Run(tt.id, func(t *testing.T) {
			integration, ok := byID[tt.id]
			if !ok {
				t.Fatalf("missing integration %q", tt.id)
			}
			if integration.Kind != tt.kind {
				t.Errorf("expected kind %q, got %q", tt.kind, integration.Kind)
			}
			if len(integration.Capabilities) == 0 {
				t.Errorf("expected non-empty capabilities for %q", tt.id)
			}
			if tt.trigger != "" && !hasTrigger(integration, tt.trigger) {
				t.Errorf("missing trigger %q", tt.trigger)
			}
			if tt.action != "" && !hasAction(integration, tt.action) {
				t.Errorf("missing action %q", tt.action)
			}
			if tt.credentialKey != "" && !hasCredential(integration, tt.credentialKey) {
				t.Errorf("missing credential %q", tt.credentialKey)
			}
		})
	}
}

func TestRegistryGetAndGetSchema(t *testing.T) {
	// Default registry lookup
	reg := Default()
	tele, ok := reg.Get("telegram")
	if !ok || tele == nil {
		t.Fatalf("expected telegram integration in default registry")
	}

	// Schema lookup default
	schema, ok := reg.GetSchema("telegram", "")
	if !ok || schema == nil {
		t.Fatalf("expected telegram schema")
	}
	if schema["type"] != "object" {
		t.Errorf("expected object type schema, got %v", schema["type"])
	}

	// Schema lookup specific action
	copySchema, ok := reg.GetSchema("telegram", "copy_message")
	if !ok || copySchema == nil {
		t.Fatalf("expected copy_message schema")
	}

	// Unknown integration
	_, found := reg.GetSchema("non_existent", "")
	if found {
		t.Errorf("expected not found for unknown integration")
	}

	// Custom isolated registry
	customReg := NewRegistry()
	customReg.Register(IntegrationFunc(func() Manifest {
		return Manifest{
			ID:   "custom_test",
			Name: "Custom Test",
			Kind: "action",
			ConfigSchema: map[string]any{
				"type": "object",
			},
		}
	}))

	customReg.Register(nil) // should not panic

	m, ok := customReg.GetManifest("custom_test")
	if !ok || m.Name != "Custom Test" {
		t.Errorf("expected custom_test in custom registry")
	}
	list := customReg.List()
	if len(list) != 1 {
		t.Errorf("expected 1 custom item, got %d", len(list))
	}
}

func hasTrigger(integration IntegrationMetadata, wanted string) bool {
	for _, trigger := range integration.Triggers {
		if trigger.Type == wanted {
			return true
		}
	}
	return false
}

func hasAction(integration IntegrationMetadata, wanted string) bool {
	for _, action := range integration.Actions {
		if action.Type == wanted {
			return true
		}
	}
	return false
}

func hasCredential(integration IntegrationMetadata, wanted string) bool {
	for _, credential := range integration.Credentials {
		if credential.Key == wanted && credential.Type == "password" {
			return true
		}
	}
	return false
}
