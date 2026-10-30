package conditions

import (
	"encoding/json"
	"strconv"
	"strings"

	"kerob/internal/core/event"
)

type extractionResult struct {
	values     []any
	exists     bool
	isWildcard bool
}

// extractFieldValues navigates event data supporting bracket indexing and array wildcard quantifiers.
func extractFieldValues(path string, ev *event.Event) (extractionResult, string, error) {
	if ev == nil || path == "" {
		return extractionResult{exists: false}, "", nil
	}

	// Detect wildcard quantifier in path syntax
	var detectedQuantifier string
	if strings.Contains(path, "[*]") {
		detectedQuantifier = QuantifierAny
		path = strings.ReplaceAll(path, "[*]", ".*.")
	} else if strings.Contains(path, "[any]") {
		detectedQuantifier = QuantifierAny
		path = strings.ReplaceAll(path, "[any]", ".*.")
	} else if strings.Contains(path, "[all]") {
		detectedQuantifier = QuantifierAll
		path = strings.ReplaceAll(path, "[all]", ".*.")
	}

	// Normalize remaining bracket indexes: e.g. items[0].name -> items.0.name
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

	if len(parts) == 0 {
		return extractionResult{exists: false}, detectedQuantifier, nil
	}

	// Determine root data structure
	var root any
	switch parts[0] {
	case "payload":
		if len(ev.Payload) == 0 {
			return extractionResult{exists: false}, detectedQuantifier, nil
		}
		var m any
		if err := json.Unmarshal(ev.Payload, &m); err != nil {
			return extractionResult{exists: false}, detectedQuantifier, nil
		}
		root = m
	case "metadata":
		if len(ev.Metadata) == 0 {
			return extractionResult{exists: false}, detectedQuantifier, nil
		}
		var m any
		if err := json.Unmarshal(ev.Metadata, &m); err != nil {
			return extractionResult{exists: false}, detectedQuantifier, nil
		}
		root = m
	case "event_type":
		if len(parts) == 1 {
			return extractionResult{values: []any{ev.EventType}, exists: true}, detectedQuantifier, nil
		}
		return extractionResult{exists: false}, detectedQuantifier, nil
	case "source_id":
		if len(parts) == 1 {
			return extractionResult{values: []any{ev.SourceID}, exists: true}, detectedQuantifier, nil
		}
		return extractionResult{exists: false}, detectedQuantifier, nil
	case "external_event_id":
		if len(parts) == 1 {
			return extractionResult{values: []any{ev.ExternalEventID}, exists: true}, detectedQuantifier, nil
		}
		return extractionResult{exists: false}, detectedQuantifier, nil
	case "id":
		if len(parts) == 1 {
			return extractionResult{values: []any{ev.ID}, exists: true}, detectedQuantifier, nil
		}
		return extractionResult{exists: false}, detectedQuantifier, nil
	case "created_at":
		if len(parts) == 1 {
			return extractionResult{values: []any{ev.CreatedAt}, exists: true}, detectedQuantifier, nil
		}
		return extractionResult{exists: false}, detectedQuantifier, nil
	default:
		// Attempt direct lookup inside payload
		if len(ev.Payload) > 0 {
			var m any
			if err := json.Unmarshal(ev.Payload, &m); err == nil {
				res := traverse(m, parts)
				return res, detectedQuantifier, nil
			}
		}
		return extractionResult{exists: false}, detectedQuantifier, nil
	}

	if len(parts) == 1 {
		return extractionResult{values: []any{root}, exists: true}, detectedQuantifier, nil
	}

	res := traverse(root, parts[1:])
	return res, detectedQuantifier, nil
}

func traverse(curr any, parts []string) extractionResult {
	if len(parts) == 0 {
		return extractionResult{values: []any{curr}, exists: curr != nil}
	}

	part := parts[0]
	rem := parts[1:]

	// Unmarshal if curr is raw JSON
	if raw, ok := curr.(json.RawMessage); ok {
		var m any
		if err := json.Unmarshal(raw, &m); err == nil {
			curr = m
		}
	} else if rawB, ok := curr.([]byte); ok {
		var m any
		if err := json.Unmarshal(rawB, &m); err == nil {
			curr = m
		}
	}

	// Wildcard expansion: iterate over all slice elements
	if part == "*" {
		sliceVal, ok := curr.([]any)
		if !ok {
			return extractionResult{exists: false, isWildcard: true}
		}

		var collected []any
		for _, elem := range sliceVal {
			subRes := traverse(elem, rem)
			if subRes.exists {
				collected = append(collected, subRes.values...)
			}
		}
		return extractionResult{
			values:     collected,
			exists:     len(collected) > 0,
			isWildcard: true,
		}
	}

	switch val := curr.(type) {
	case map[string]any:
		child, ok := val[part]
		if !ok {
			return extractionResult{exists: false}
		}
		return traverse(child, rem)

	case []any:
		idx, err := strconv.Atoi(part)
		if err != nil || idx < 0 || idx >= len(val) {
			return extractionResult{exists: false}
		}
		return traverse(val[idx], rem)

	default:
		return extractionResult{exists: false}
	}
}
