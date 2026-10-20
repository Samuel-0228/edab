package transformations

import (
	"encoding/json"
	"fmt"
	"regexp"
	"strconv"
	"strings"
	"time"

	"kerob/internal/core/event"
)

var templateVarRegex = regexp.MustCompile(`\{\{\s*([^{}]+?)\s*\}\}`)

// RenderTemplate evaluates a template string containing placeholders such as
// {{event.payload.title}} or {{event.payload.title | default: "Untitled"}}
// against the provided normalized event.
func RenderTemplate(tmpl string, ev *event.Event) string {
	if tmpl == "" || ev == nil {
		return tmpl
	}

	return templateVarRegex.ReplaceAllStringFunc(tmpl, func(match string) string {
		submatches := templateVarRegex.FindStringSubmatch(match)
		if len(submatches) < 2 {
			return match
		}
		expr := strings.TrimSpace(submatches[1])

		// Check for default fallback operator e.g. "path | default: fallback" or "path || fallback"
		var path string
		var defaultVal string
		hasDefault := false

		if strings.Contains(expr, "| default:") {
			parts := strings.SplitN(expr, "| default:", 2)
			path = strings.TrimSpace(parts[0])
			defaultVal = cleanDefault(strings.TrimSpace(parts[1]))
			hasDefault = true
		} else if strings.Contains(expr, "||") {
			parts := strings.SplitN(expr, "||", 2)
			path = strings.TrimSpace(parts[0])
			defaultVal = cleanDefault(strings.TrimSpace(parts[1]))
			hasDefault = true
		} else {
			path = expr
		}

		val, exists := resolveEventPath(path, ev)
		if !exists || val == nil {
			if hasDefault {
				return defaultVal
			}
			return ""
		}

		switch v := val.(type) {
		case string:
			return v
		case time.Time:
			return v.Format(time.RFC3339)
		case []byte:
			return string(v)
		default:
			// JSON marshal complex objects or fmt simple types
			if b, err := json.Marshal(v); err == nil && (strings.HasPrefix(string(b), "{") || strings.HasPrefix(string(b), "[")) {
				return string(b)
			}
			return fmt.Sprintf("%v", v)
		}
	})
}

func cleanDefault(s string) string {
	s = strings.TrimSpace(s)
	if (strings.HasPrefix(s, `"`) && strings.HasSuffix(s, `"`)) ||
		(strings.HasPrefix(s, `'`) && strings.HasSuffix(s, `'`)) {
		if len(s) >= 2 {
			return s[1 : len(s)-1]
		}
	}
	return s
}

// resolveEventPath resolves dot-notation paths against the Event.
// Supported prefixes:
// - "event.payload.x", "payload.x"
// - "event.metadata.x", "metadata.x"
// - "event.id", "event.event_type", "event.source_id", "event.external_event_id", "event.created_at"
func resolveEventPath(path string, ev *event.Event) (any, bool) {
	path = strings.TrimPrefix(path, "event.")
	parts := strings.Split(path, ".")
	if len(parts) == 0 {
		return nil, false
	}

	var root any
	switch parts[0] {
	case "payload":
		if len(ev.Payload) == 0 {
			return nil, false
		}
		var m any
		if err := json.Unmarshal(ev.Payload, &m); err != nil {
			return nil, false
		}
		root = m
	case "metadata":
		if len(ev.Metadata) == 0 {
			return nil, false
		}
		var m any
		if err := json.Unmarshal(ev.Metadata, &m); err != nil {
			return nil, false
		}
		root = m
	case "event_type":
		return ev.EventType, true
	case "source_id":
		return ev.SourceID, true
	case "external_event_id":
		return ev.ExternalEventID, true
	case "id":
		return ev.ID, true
	case "created_at":
		return ev.CreatedAt, true
	default:
		// Attempt direct lookup in payload
		if len(ev.Payload) > 0 {
			var m any
			if err := json.Unmarshal(ev.Payload, &m); err == nil {
				return lookupPath(m, parts)
			}
		}
		return nil, false
	}

	if len(parts) == 1 {
		return root, true
	}

	return lookupPath(root, parts[1:])
}

func lookupPath(data any, parts []string) (any, bool) {
	curr := data
	for _, part := range parts {
		if curr == nil {
			return nil, false
		}
		switch val := curr.(type) {
		case map[string]any:
			v, ok := val[part]
			if !ok {
				return nil, false
			}
			curr = v
		case []any:
			idx, err := strconv.Atoi(part)
			if err != nil || idx < 0 || idx >= len(val) {
				return nil, false
			}
			curr = val[idx]
		default:
			return nil, false
		}
	}
	return curr, true
}
