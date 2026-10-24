package adapters

import "testing"

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
		trigger       string
		action        string
		credentialKey string
	}{
		{"telegram", "telegram.message", "send_message", "bot_token"},
		{"discord", "discord.webhook", "send_message", "webhook_url"},
		{"rss", "rss.item", "", ""},
		{"github", "github.push", "create_issue", "personal_access_token"},
		{"webhook", "webhook.received", "", ""},
		{"http", "", "http_request", "api_key"},
	}
	for _, tt := range tests {
		t.Run(tt.id, func(t *testing.T) {
			integration, ok := byID[tt.id]
			if !ok {
				t.Fatalf("missing integration %q", tt.id)
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
