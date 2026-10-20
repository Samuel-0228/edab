package transformations

import (
	"encoding/json"
	"testing"

	"kerob/internal/core/event"
)

func TestRenderTemplate(t *testing.T) {
	ev, err := event.New("src-github", "gh-101", "github.issue.opened", map[string]any{
		"title": "Bug in worker pool",
		"url":   "https://github.com/org/repo/issues/101",
		"user": map[string]any{
			"login": "octocat",
		},
		"labels": []string{"bug", "critical"},
	}, map[string]any{
		"repo": "org/repo",
	})
	if err != nil {
		t.Fatalf("failed to create event: %v", err)
	}

	tests := []struct {
		name     string
		template string
		expected string
	}{
		{
			name:     "simple interpolation",
			template: "New issue: {{event.payload.title}} by {{event.payload.user.login}}",
			expected: "New issue: Bug in worker pool by octocat",
		},
		{
			name:     "shorthand payload interpolation",
			template: "Link: {{payload.url}}",
			expected: "Link: https://github.com/org/repo/issues/101",
		},
		{
			name:     "metadata interpolation",
			template: "Repository: {{metadata.repo}}",
			expected: "Repository: org/repo",
		},
		{
			name:     "missing field with default",
			template: "Severity: {{payload.severity | default: 'normal'}}",
			expected: "Severity: normal",
		},
		{
			name:     "missing field with alternative default syntax",
			template: "Milestone: {{payload.milestone || 'v1.0'}}",
			expected: "Milestone: v1.0",
		},
		{
			name:     "existing field ignores default",
			template: "Title: {{payload.title | default: 'Default Title'}}",
			expected: "Title: Bug in worker pool",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			actual := RenderTemplate(tt.template, ev)
			if actual != tt.expected {
				t.Errorf("expected %q, got %q", tt.expected, actual)
			}
		})
	}
}

func TestTransformMappingAndDefaults(t *testing.T) {
	ev, err := event.New("src-rss", "rss-42", "rss.item", map[string]any{
		"title":       "Go 1.24 Released",
		"link":        "https://go.dev/blog/go1.24",
		"description": "New language features and standard library additions.",
	}, nil)
	if err != nil {
		t.Fatalf("failed to create event: %v", err)
	}

	tf := &Transformation{
		Type: "mapping",
		FieldMaps: map[string]string{
			"headline": "{{payload.title}}",
			"target":   "payload.link",
			"summary":  "payload.description",
		},
		Defaults: map[string]any{
			"category": "engineering",
		},
	}

	out, err := Transform(tf, ev)
	if err != nil {
		t.Fatalf("transform error: %v", err)
	}

	if out["headline"] != "Go 1.24 Released" {
		t.Errorf("expected headline 'Go 1.24 Released', got %v", out["headline"])
	}
	if out["target"] != "https://go.dev/blog/go1.24" {
		t.Errorf("expected target 'https://go.dev/blog/go1.24', got %v", out["target"])
	}
	if out["category"] != "engineering" {
		t.Errorf("expected default category 'engineering', got %v", out["category"])
	}
}

func TestRenderJSONConfig(t *testing.T) {
	ev, err := event.New("src-webhook", "wh-99", "webhook.alert", map[string]any{
		"service": "billing-api",
		"status":  "critical",
		"code":    503,
	}, nil)
	if err != nil {
		t.Fatalf("failed to create event: %v", err)
	}

	rawConfig := json.RawMessage(`{
		"message": "Alert on {{payload.service}}: status {{payload.status}}",
		"data": {
			"error_code": "{{payload.code}}",
			"constant": "fixed_value"
		}
	}`)

	renderedBytes, err := RenderJSONConfig(rawConfig, ev)
	if err != nil {
		t.Fatalf("RenderJSONConfig failed: %v", err)
	}

	var parsed map[string]any
	if err := json.Unmarshal(renderedBytes, &parsed); err != nil {
		t.Fatalf("failed to unmarshal rendered JSON: %v", err)
	}

	if parsed["message"] != "Alert on billing-api: status critical" {
		t.Errorf("expected message interpolated, got %v", parsed["message"])
	}

	subData, ok := parsed["data"].(map[string]any)
	if !ok || subData["error_code"] != "503" {
		t.Errorf("expected data.error_code to be 503, got %v", subData)
	}
}
