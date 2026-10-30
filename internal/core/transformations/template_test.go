package transformations

import (
	"encoding/json"
	"testing"

	"kerob/internal/core/event"
)

func TestRenderTemplateWithStepOutputs(t *testing.T) {
	ev, _ := event.New("src-1", "ext-1", "user.created", json.RawMessage(`{"user":"bob"}`), nil)

	ctx := RenderContext{
		Event: ev,
		Steps: map[string]StepOutput{
			"enrich": {
				Status: "success",
				Output: map[string]any{
					"role":  "admin",
					"badge": "gold",
					"stats": map[string]any{
						"login_count": 42,
					},
					"tags": []any{"vip", "staff"},
				},
			},
			"auth_check": {
				Status: "failed",
				Error:  "invalid session",
			},
		},
		Secrets: map[string]string{
			"api_token": "secret-xyz-123",
		},
	}

	tests := []struct {
		name     string
		template string
		expected string
	}{
		{
			name:     "reference earlier step output string",
			template: "Role is {{steps.enrich.output.role}}",
			expected: "Role is admin",
		},
		{
			name:     "reference earlier step nested path",
			template: "Logins: {{steps.enrich.output.stats.login_count}}",
			expected: "Logins: 42",
		},
		{
			name:     "reference earlier step array index",
			template: "Primary tag: {{steps.enrich.output.tags[0]}}",
			expected: "Primary tag: vip",
		},
		{
			name:     "reference earlier step status",
			template: "Enrich: {{steps.enrich.status}}, Auth: {{steps.auth_check.status}}",
			expected: "Enrich: success, Auth: failed",
		},
		{
			name:     "reference earlier step error",
			template: "Error reason: {{steps.auth_check.error}}",
			expected: "Error reason: invalid session",
		},
		{
			name:     "reference vault secret",
			template: "Bearer {{secrets.api_token}}",
			expected: "Bearer secret-xyz-123",
		},
		{
			name:     "combine event, step output, and default fallback",
			template: "{{payload.user}} ({{steps.enrich.output.badge}}) is {{steps.unknown.output.missing | default: 'guest'}}",
			expected: "bob (gold) is guest",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			res := RenderTemplateWithContext(tt.template, ctx)
			if res != tt.expected {
				t.Errorf("expected %q, got %q", tt.expected, res)
			}
		})
	}
}

func TestRenderJSONConfigWithStepContext(t *testing.T) {
	raw := json.RawMessage(`{
		"recipient": "{{payload.email}}",
		"subject": "Status: {{steps.fetch.status}}",
		"details": {
			"score": "{{steps.fetch.output.score}}",
			"token": "{{secrets.api_key}}"
		}
	}`)

	ev, _ := event.New("src-1", "ext-1", "score.ready", json.RawMessage(`{"email":"dev@example.com"}`), nil)
	ctx := RenderContext{
		Event: ev,
		Steps: map[string]StepOutput{
			"fetch": {
				Status: "success",
				Output: map[string]any{"score": 98},
			},
		},
		Secrets: map[string]string{
			"api_key": "vault-secret-abc",
		},
	}

	rendered, err := RenderJSONConfigWithContext(raw, ctx)
	if err != nil {
		t.Fatalf("unexpected render error: %v", err)
	}

	var parsed map[string]any
	if err := json.Unmarshal(rendered, &parsed); err != nil {
		t.Fatalf("rendered JSON unmarshal failed: %v", err)
	}

	if parsed["recipient"] != "dev@example.com" {
		t.Errorf("expected recipient dev@example.com, got %v", parsed["recipient"])
	}
	if parsed["subject"] != "Status: success" {
		t.Errorf("expected subject 'Status: success', got %v", parsed["subject"])
	}
	details, ok := parsed["details"].(map[string]any)
	if !ok {
		t.Fatalf("expected details map, got %T", parsed["details"])
	}
	if details["score"] != "98" {
		t.Errorf("expected score 98, got %v", details["score"])
	}
	if details["token"] != "vault-secret-abc" {
		t.Errorf("expected token vault-secret-abc, got %v", details["token"])
	}
}
