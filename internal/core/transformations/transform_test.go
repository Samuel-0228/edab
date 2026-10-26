package transformations

import (
	"encoding/json"
	"strings"
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

func TestTransformationEdgeCases(t *testing.T) {
	// 1. Nil event
	out, err := Transform(nil, nil)
	if err != nil || len(out) != 0 {
		t.Fatalf("expected empty map for nil event")
	}

	ev, _ := event.New("src-1", "ext-1", "test.event", map[string]any{
		"items": []any{
			map[string]any{"id": "item-1", "price": 10},
			map[string]any{"id": "item-2", "price": 25},
		},
		"simple": "val",
	}, nil)

	// 2. Passthrough
	passOut, err := Transform(&Transformation{Type: "passthrough"}, ev)
	if err != nil || passOut["simple"] != "val" {
		t.Fatalf("expected passthrough to preserve payload: %+v", passOut)
	}

	// 3. Single template type with custom output key
	tplTf := &Transformation{
		Type:      "template",
		Template:  "Item: {{payload.simple}}",
		OutputKey: "formatted_result",
	}
	tplOut, err := Transform(tplTf, ev)
	if err != nil || tplOut["formatted_result"] != "Item: val" {
		t.Fatalf("expected formatted_result 'Item: val', got %+v", tplOut)
	}

	// 4. Invalid rendered JSON template returns error
	_, err = Transform(&Transformation{
		Type:     "json",
		Template: `{invalid_json: true`,
	}, ev)
	if err == nil {
		t.Fatalf("expected error for invalid rendered JSON template")
	}

	// 5. Valid JSON transformation
	jsonOut, err := Transform(&Transformation{
		Type:     "json",
		Template: `{"user_name":"{{payload.simple}}","active":true}`,
	}, ev)
	if err != nil || jsonOut["user_name"] != "val" || jsonOut["active"] != true {
		t.Fatalf("expected valid parsed JSON transformation output, got %+v", jsonOut)
	}

	// 6. RenderJSONConfig with empty/nil rawConfig
	emptyRender, err := RenderJSONConfig(nil, ev)
	if err != nil || len(emptyRender) != 0 {
		t.Fatalf("expected empty bytes for nil config")
	}

	// 7. RenderTemplate with unclosed tags and nested missing paths
	unclosed := RenderTemplate("Hello {{payload.missing_key without close", ev)
	if !strings.Contains(unclosed, "Hello") {
		t.Errorf("expected string to be returned safely: %s", unclosed)
	}
}

func TestTransformDefaultsAndPathLookups(t *testing.T) {
	ev, _ := event.New("src-1", "ext-1", "test.event", map[string]any{
		"user": map[string]any{
			"profile": map[string]any{
				"email": "user@example.com",
			},
		},
		"tags": []any{"production", "backend"},
	}, nil)

	// 1. Defaults applied when fields are missing
	tfWithDefaults := &Transformation{
		Type: "mapping",
		Defaults: map[string]any{
			"env":     "development",
			"retries": 3,
		},
		FieldMaps: map[string]string{
			"email":   "payload.user.profile.email",
			"tag":     "payload.tags.0",
			"missing": "payload.user.missing_field",
		},
	}

	out, err := Transform(tfWithDefaults, ev)
	if err != nil {
		t.Fatalf("Transform failed: %v", err)
	}

	if out["env"] != "development" {
		t.Errorf("expected default env 'development', got %v", out["env"])
	}
	if out["retries"] != 3 {
		t.Errorf("expected default retries 3, got %v", out["retries"])
	}
	if out["email"] != "user@example.com" {
		t.Errorf("expected mapped email 'user@example.com', got %v", out["email"])
	}
	if out["tag"] != "production" {
		t.Errorf("expected mapped tag 'production', got %v", out["tag"])
	}

	// 2. Unknown transformation type falls back to template render
	tfFallback := &Transformation{
		Type:      "unknown_type",
		Template:  "User: {{payload.user.profile.email}}",
		OutputKey: "summary",
	}
	fallbackOut, err := Transform(tfFallback, ev)
	if err != nil {
		t.Fatalf("Transform fallback failed: %v", err)
	}
	if fallbackOut["summary"] != "User: user@example.com" {
		t.Errorf("expected fallback summary 'User: user@example.com', got %v", fallbackOut["summary"])
	}
}

func TestRenderTemplateEventTopLevelPaths(t *testing.T) {
	ev, _ := event.New("src-test", "ext-test-1", "test.topic", map[string]any{"direct_field": "val"}, nil)

	tests := []struct {
		tpl      string
		expected string
	}{
		{"ID: {{event.id}}", "ID: " + ev.ID},
		{"Type: {{event.event_type}}", "Type: test.topic"},
		{"Source: {{event.source_id}}", "Source: src-test"},
		{"Ext: {{event.external_event_id}}", "Ext: ext-test-1"},
		{"Direct: {{direct_field}}", "Direct: val"},
	}

	for _, tt := range tests {
		got := RenderTemplate(tt.tpl, ev)
		if got != tt.expected {
			t.Errorf("RenderTemplate(%q) = %q, expected %q", tt.tpl, got, tt.expected)
		}
	}
}

func TestRenderJSONConfigNested(t *testing.T) {
	ev, _ := event.New("src", "ext", "alert", map[string]any{"msg": "service down", "code": 503}, nil)

	input := json.RawMessage(`{
		"items": ["{{payload.msg}}", "{{payload.code}}", "static"],
		"nested": {
			"detail": "{{payload.msg}}"
		}
	}`)

	rendered, err := RenderJSONConfig(input, ev)
	if err != nil {
		t.Fatalf("RenderJSONConfig failed: %v", err)
	}

	var parsed map[string]any
	if err := json.Unmarshal(rendered, &parsed); err != nil {
		t.Fatalf("failed unmarshaling rendered JSON: %v", err)
	}

	items, ok := parsed["items"].([]any)
	if !ok {
		t.Fatalf("expected []any items in rendered JSON, got %T", parsed["items"])
	}
	if items[0] != "service down" || items[1] != "503" || items[2] != "static" {
		t.Errorf("unexpected rendered items: %v", items)
	}
}
