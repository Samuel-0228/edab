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

// StepOutput represents the recorded state of an executed step available to downstream steps.
type StepOutput struct {
	Status     string `json:"status"`
	Output     any    `json:"output,omitempty"`
	Error      string `json:"error,omitempty"`
	DurationMs int64  `json:"duration_ms,omitempty"`
}

// RenderContext provides contextual scope for template rendering including events, step outputs, and secrets.
type RenderContext struct {
	Event     *event.Event
	Steps     map[string]StepOutput
	Secrets   map[string]string
	Variables map[string]any
}

// RenderTemplate evaluates a template string containing placeholders such as
// {{event.payload.title}} or {{event.payload.title | default: "Untitled"}}
// against the provided normalized event.
func RenderTemplate(tmpl string, ev *event.Event) string {
	return RenderTemplateWithContext(tmpl, RenderContext{Event: ev})
}

// RenderTemplateWithContext evaluates a template string against a full RenderContext
// including earlier step outputs ({{steps.<id>.output.<path>}}) and vault secrets ({{secrets.<name>}}).
func RenderTemplateWithContext(tmpl string, ctx RenderContext) string {
	if tmpl == "" {
		return tmpl
	}

	return templateVarRegex.ReplaceAllStringFunc(tmpl, func(match string) string {
		submatches := templateVarRegex.FindStringSubmatch(match)
		if len(submatches) < 2 {
			return match
		}
		expr := strings.TrimSpace(submatches[1])

		// Check for || fallback shorthand
		if strings.Contains(expr, "||") {
			parts := strings.SplitN(expr, "||", 2)
			expr = fmt.Sprintf("%s | default: %s", parts[0], parts[1])
		}

		segments := strings.Split(expr, "|")
		path := strings.TrimSpace(segments[0])
		var filters []string
		if len(segments) > 1 {
			filters = segments[1:]
		}

		val, exists := resolvePath(path, ctx)
		finalVal := applyFilterPipeline(val, exists, filters, ctx)
		if finalVal == nil {
			return ""
		}

		switch v := finalVal.(type) {
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

func splitPath(path string) []string {
	// Normalize bracket indexing: items[0].name -> items.0.name
	path = strings.ReplaceAll(path, "[", ".")
	path = strings.ReplaceAll(path, "]", "")
	rawParts := strings.Split(path, ".")
	var parts []string
	for _, p := range rawParts {
		p = strings.TrimSpace(p)
		if p != "" {
			parts = append(parts, p)
		}
	}
	return parts
}

// ResolvePath resolves dot-notation paths against Context Variables, Step outputs, Secrets, or the Event.
func ResolvePath(path string, ctx RenderContext) (any, bool) {
	clean := strings.TrimSpace(path)
	clean = strings.TrimPrefix(clean, "{{")
	clean = strings.TrimSuffix(clean, "}}")
	clean = strings.TrimSpace(clean)
	return resolvePath(clean, ctx)
}

// resolvePath resolves dot-notation paths against the Event, Step outputs, or Secrets.
func resolvePath(path string, ctx RenderContext) (any, bool) {
	if ctx.Variables != nil {
		parts := splitPath(path)
		if len(parts) > 0 {
			if val, ok := ctx.Variables[parts[0]]; ok {
				if len(parts) == 1 {
					return val, true
				}
				return lookupPath(val, parts[1:])
			}
		}
	}
	if strings.HasPrefix(path, "steps.") {
		trimmed := strings.TrimPrefix(path, "steps.")
		parts := splitPath(trimmed)
		if len(parts) == 0 || ctx.Steps == nil {
			return nil, false
		}
		stepID := parts[0]
		step, ok := ctx.Steps[stepID]
		if !ok {
			return nil, false
		}
		if len(parts) == 1 {
			return step, true
		}
		switch parts[1] {
		case "status":
			return step.Status, true
		case "error":
			return step.Error, true
		case "duration_ms":
			return step.DurationMs, true
		case "output":
			if len(parts) == 2 {
				return step.Output, true
			}
			return lookupPath(step.Output, parts[2:])
		default:
			return lookupPath(step.Output, parts[1:])
		}
	}

	if strings.HasPrefix(path, "secrets.") {
		secretKey := strings.TrimPrefix(path, "secrets.")
		if ctx.Secrets != nil {
			if val, ok := ctx.Secrets[secretKey]; ok {
				return val, true
			}
		}
		return nil, false
	}

	return resolveEventPath(path, ctx.Event)
}

// resolveEventPath resolves dot-notation paths against the Event.
func resolveEventPath(path string, ev *event.Event) (any, bool) {
	if ev == nil {
		return nil, false
	}
	path = strings.TrimPrefix(path, "event.")
	parts := splitPath(path)
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
		case json.RawMessage:
			var m any
			if err := json.Unmarshal(val, &m); err != nil {
				return nil, false
			}
			curr = m
		case []byte:
			var m any
			if err := json.Unmarshal(val, &m); err != nil {
				return nil, false
			}
			curr = m
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
