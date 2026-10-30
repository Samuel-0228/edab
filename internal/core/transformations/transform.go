package transformations

import (
	"encoding/json"
	"fmt"
	"strings"

	"kerob/internal/core/event"
)

// Transformation represents configuration for converting or formatting an event payload.
type Transformation struct {
	Type      string            `json:"type,omitempty"`       // "template", "mapping", "json", "passthrough"
	Template  string            `json:"template,omitempty"`   // Single string template
	FieldMaps map[string]string `json:"field_maps,omitempty"` // outputKey -> template or path
	Defaults  map[string]any    `json:"defaults,omitempty"`   // fallback values if field is empty/nil
	OutputKey string            `json:"output_key,omitempty"` // key to store single template output in (default "text")
}

// Transform executes the transformation rules on the given event.
// Always returns a safe, clean map[string]any representing the transformed payload.
func Transform(t *Transformation, ev *event.Event) (map[string]any, error) {
	if ev == nil {
		return map[string]any{}, nil
	}

	// 1. If transformation is nil or passthrough, parse existing payload
	if t == nil || t.Type == "" || t.Type == "passthrough" {
		out := make(map[string]any)
		if len(ev.Payload) > 0 {
			_ = json.Unmarshal(ev.Payload, &out)
		}
		return out, nil
	}

	out := make(map[string]any)

	// Apply default values first
	for k, v := range t.Defaults {
		out[k] = v
	}

	switch t.Type {
	case "template":
		outputKey := t.OutputKey
		if outputKey == "" {
			outputKey = "text"
		}
		rendered := RenderTemplate(t.Template, ev)
		out[outputKey] = rendered

	case "mapping":
		for outKey, expr := range t.FieldMaps {
			if strings.Contains(expr, "{{") {
				out[outKey] = RenderTemplate(expr, ev)
			} else {
				val, exists := resolveEventPath(expr, ev)
				if exists && val != nil {
					out[outKey] = val
				}
			}
		}

	case "json":
		// Render field maps if present
		for outKey, expr := range t.FieldMaps {
			if strings.Contains(expr, "{{") {
				out[outKey] = RenderTemplate(expr, ev)
			} else {
				val, exists := resolveEventPath(expr, ev)
				if exists && val != nil {
					out[outKey] = val
				}
			}
		}
		if t.Template != "" {
			renderedJSON := RenderTemplate(t.Template, ev)
			var parsed map[string]any
			if err := json.Unmarshal([]byte(renderedJSON), &parsed); err == nil {
				for k, v := range parsed {
					out[k] = v
				}
			} else {
				return nil, fmt.Errorf("rendered JSON template is invalid: %w", err)
			}
		}

	default:
		// Fallback: render template into outputKey
		if t.Template != "" {
			outputKey := t.OutputKey
			if outputKey == "" {
				outputKey = "text"
			}
			out[outputKey] = RenderTemplate(t.Template, ev)
		}
	}

	return out, nil
}

// RenderJSONConfig recursively navigates arbitrary JSON data and renders any template strings within it.
func RenderJSONConfig(raw json.RawMessage, ev *event.Event) (json.RawMessage, error) {
	return RenderJSONConfigWithContext(raw, RenderContext{Event: ev})
}

// RenderJSONConfigWithContext recursively renders templates in JSON data with the provided RenderContext.
func RenderJSONConfigWithContext(raw json.RawMessage, ctx RenderContext) (json.RawMessage, error) {
	if len(raw) == 0 || string(raw) == "{}" || string(raw) == "[]" {
		return raw, nil
	}

	var data any
	if err := json.Unmarshal(raw, &data); err != nil {
		return raw, err
	}

	rendered := renderRecursiveWithContext(data, ctx)
	return json.Marshal(rendered)
}

func renderRecursiveWithContext(v any, ctx RenderContext) any {
	switch val := v.(type) {
	case string:
		if strings.Contains(val, "{{") {
			return RenderTemplateWithContext(val, ctx)
		}
		return val
	case map[string]any:
		res := make(map[string]any, len(val))
		for k, item := range val {
			res[k] = renderRecursiveWithContext(item, ctx)
		}
		return res
	case []any:
		res := make([]any, len(val))
		for i, item := range val {
			res[i] = renderRecursiveWithContext(item, ctx)
		}
		return res
	default:
		return v
	}
}
