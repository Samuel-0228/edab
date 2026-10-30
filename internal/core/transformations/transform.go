package transformations

import (
	"encoding/json"
	"fmt"
	"strconv"
	"strings"

	"kerob/internal/core/event"
)

// ComputeField specifies an arithmetic or string operation for the "compute" transformation.
type ComputeField struct {
	Target string   `json:"target"` // Target key in output payload
	Op     string   `json:"op"`     // "add", "sub", "mul", "div", "concat"
	Args   []string `json:"args"`   // Path references (e.g. "payload.subtotal") or literals
}

// Transformation represents configuration for converting or formatting an event payload.
type Transformation struct {
	Type      string            `json:"type,omitempty"`       // "template", "mapping", "json", "passthrough", "pick", "omit", "flatten", "merge", "compute"
	Template  string            `json:"template,omitempty"`   // Single string template
	FieldMaps map[string]string `json:"field_maps,omitempty"` // outputKey -> template or path
	Defaults  map[string]any    `json:"defaults,omitempty"`   // fallback values if field is empty/nil
	OutputKey string            `json:"output_key,omitempty"` // key to store single template output in (default "text")

	// Rich Transformations
	PickKeys []string       `json:"pick_keys,omitempty"` // For type == "pick"
	OmitKeys []string       `json:"omit_keys,omitempty"` // For type == "omit"
	Data     map[string]any `json:"data,omitempty"`      // For type == "merge"
	Compute  []ComputeField `json:"compute,omitempty"`   // For type == "compute"
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

	case "pick":
		basePayload := loadPayloadMap(ev)
		for _, key := range t.PickKeys {
			if v, ok := basePayload[key]; ok {
				out[key] = v
			}
		}

	case "omit":
		basePayload := loadPayloadMap(ev)
		omitSet := make(map[string]bool, len(t.OmitKeys))
		for _, k := range t.OmitKeys {
			omitSet[k] = true
		}
		for k, v := range basePayload {
			if !omitSet[k] {
				out[k] = v
			}
		}

	case "flatten":
		basePayload := loadPayloadMap(ev)
		flattenMap("", basePayload, out)

	case "merge":
		basePayload := loadPayloadMap(ev)
		for k, v := range basePayload {
			out[k] = v
		}
		if t.Data != nil {
			mergedData := renderRecursiveWithContext(t.Data, RenderContext{Event: ev})
			if m, ok := mergedData.(map[string]any); ok {
				for k, v := range m {
					out[k] = v
				}
			}
		}

	case "compute":
		basePayload := loadPayloadMap(ev)
		for k, v := range basePayload {
			out[k] = v
		}
		for _, comp := range t.Compute {
			res, err := executeCompute(comp, ev)
			if err != nil {
				return nil, err
			}
			out[comp.Target] = res
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

func loadPayloadMap(ev *event.Event) map[string]any {
	out := make(map[string]any)
	if ev != nil && len(ev.Payload) > 0 {
		_ = json.Unmarshal(ev.Payload, &out)
	}
	return out
}

func flattenMap(prefix string, in map[string]any, out map[string]any) {
	for k, v := range in {
		fullKey := k
		if prefix != "" {
			fullKey = prefix + "." + k
		}
		if subMap, ok := v.(map[string]any); ok {
			flattenMap(fullKey, subMap, out)
		} else {
			out[fullKey] = v
		}
	}
}

func executeCompute(comp ComputeField, ev *event.Event) (any, error) {
	op := strings.ToLower(strings.TrimSpace(comp.Op))
	if op == "concat" {
		var parts []string
		for _, arg := range comp.Args {
			val, exists := resolveEventPath(arg, ev)
			if exists && val != nil {
				parts = append(parts, fmt.Sprintf("%v", val))
			} else {
				parts = append(parts, arg)
			}
		}
		return strings.Join(parts, ""), nil
	}

	// Arithmetic operations: add, sub, mul, div
	var nums []float64
	for _, arg := range comp.Args {
		var num float64
		var found bool
		if val, exists := resolveEventPath(arg, ev); exists && val != nil {
			if n, ok := toFloat64(val); ok {
				num = n
				found = true
			}
		}
		if !found {
			if n, err := strconv.ParseFloat(arg, 64); err == nil {
				num = n
				found = true
			}
		}
		if !found {
			return nil, fmt.Errorf("argument %q could not be parsed as numeric for op %s", arg, comp.Op)
		}
		nums = append(nums, num)
	}

	if len(nums) == 0 {
		return 0.0, nil
	}

	switch op {
	case "add", "+":
		sum := 0.0
		for _, n := range nums {
			sum += n
		}
		return sum, nil

	case "sub", "-":
		res := nums[0]
		for _, n := range nums[1:] {
			res -= n
		}
		return res, nil

	case "mul", "*":
		prod := 1.0
		for _, n := range nums {
			prod *= n
		}
		return prod, nil

	case "div", "/":
		res := nums[0]
		for _, n := range nums[1:] {
			if n == 0 {
				return nil, fmt.Errorf("division by zero in compute field %s", comp.Target)
			}
			res /= n
		}
		return res, nil

	default:
		return nil, fmt.Errorf("unsupported compute op %q", comp.Op)
	}
}
