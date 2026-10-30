package transformations

import (
	"testing"
	"time"

	"kerob/internal/core/event"
)

func TestFilterLibrary(t *testing.T) {
	fixedTime, _ := time.Parse(time.RFC3339, "2026-10-30T15:04:05Z")

	ev, err := event.New("src-1", "ext-1", "test.event", map[string]any{
		"title":       "  hello world  ",
		"desc":        "The quick brown fox jumps over the lazy dog",
		"price":       19.9567,
		"timestamp":   fixedTime.Format(time.RFC3339),
		"tags":        []string{"go", "svelte", "automation"},
		"csv":         "red,green,blue",
		"email":       "support+vip@example.com",
		"nested_json": map[string]any{"service": "kerob", "v": 2},
		"empty_field": "",
	}, nil)
	if err != nil {
		t.Fatalf("failed to create event: %v", err)
	}

	tests := []struct {
		name     string
		template string
		expected string
	}{
		// 1. upper
		{"upper filter", "{{payload.title | trim | upper}}", "HELLO WORLD"},

		// 2. lower
		{"lower filter", "{{payload.title | trim | lower}}", "hello world"},

		// 3. trim
		{"trim filter", "[{{payload.title | trim}}]", "[hello world]"},

		// 4. truncate
		{"truncate filter", "{{payload.desc | truncate:15}}", "The quick br..."},

		// 5. replace
		{"replace filter", "{{payload.title | replace:world:universe | trim}}", "hello universe"},

		// 6. split and join
		{"split and join", `{{payload.csv | split:"," | join:" - "}}`, "red - green - blue"},

		// 7. default
		{"default on empty", "{{payload.empty_field | default:'Fallback'}}", "Fallback"},

		// 8. json_encode
		{"json_encode filter", "{{payload.nested_json | json_encode}}", `{"service":"kerob","v":2}`},

		// 9. url_encode
		{"url_encode filter", "{{payload.email | url_encode}}", "support%2Bvip%40example.com"},

		// 10. base64
		{"base64 filter", "{{payload.title | trim | base64}}", "aGVsbG8gd29ybGQ="},

		// 11. date_format
		{"date_format filter", "{{payload.timestamp | date_format:2006-01-02}}", "2026-10-30"},

		// 12. number_format
		{"number_format filter", "{{payload.price | number_format:2}}", "19.96"},

		// 13. regex_extract
		{"regex_extract filter", `{{payload.email | regex_extract:@(.*)$}}`, "example.com"},

		// 14. coalesce
		{"coalesce filter", `{{payload.missing | coalesce:payload.empty_field:"default_val"}}`, "default_val"},

		// 15. length
		{"length filter on string", "{{payload.csv | length}}", "14"},
		{"length filter on slice", "{{payload.tags | length}}", "3"},

		// 16. Chained pipeline
		{"chained filter pipeline", "{{payload.title | trim | upper | replace:HELLO:GREETINGS}}", "GREETINGS WORLD"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := RenderTemplate(tt.template, ev)
			if result != tt.expected {
				t.Errorf("template %q: expected %q, got %q", tt.template, tt.expected, result)
			}
		})
	}
}
