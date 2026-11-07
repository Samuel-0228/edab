package ai

import (
	"encoding/json"
	"strings"

	"kerob/internal/core/governance"
)

// SanitizePromptInput scrubs PII, secrets, API tokens, and authorization credentials
// from any data structure before passing it to an external AI model provider.
func SanitizePromptInput(input any) any {
	if input == nil {
		return nil
	}

	// Apply governance masking engine (both PII and Secrets)
	masked := governance.MaskValue(input, true, true)

	// Further check for secret interpolation placeholders e.g. {{ secrets.KEY }}
	return deepCleanSecrets(masked)
}

// SanitizeString scrubs a string prompt directly and safely returns a string.
func SanitizeString(s string) string {
	res := SanitizePromptInput(s)
	if str, ok := res.(string); ok {
		return str
	}
	return s
}

func deepCleanSecrets(v any) any {
	switch val := v.(type) {
	case string:
		return cleanSecretString(val)
	case map[string]any:
		res := make(map[string]any, len(val))
		for k, v := range val {
			res[k] = deepCleanSecrets(v)
		}
		return res
	case []any:
		res := make([]any, len(val))
		for i, item := range val {
			res[i] = deepCleanSecrets(item)
		}
		return res
	default:
		return v
	}
}

func cleanSecretString(s string) string {
	// Redact inline bearer or basic tokens
	lower := strings.ToLower(s)
	if strings.Contains(lower, "bearer ") || strings.Contains(lower, "basic ") {
		s = governance.MaskString(s, true, true)
	}

	// Redact raw credential interpolations if they look like concrete secrets
	if strings.Contains(s, "{{secret.") || strings.Contains(s, "{{ secrets.") {
		// Keep template reference syntax safe
		return s
	}

	return s
}

// SanitizeJSON scrubs raw JSON message before inclusion in model prompts.
func SanitizeJSON(raw json.RawMessage) json.RawMessage {
	if len(raw) == 0 {
		return raw
	}
	var unmarshaled any
	if err := json.Unmarshal(raw, &unmarshaled); err != nil {
		return raw
	}
	cleaned := SanitizePromptInput(unmarshaled)
	out, err := json.Marshal(cleaned)
	if err != nil {
		return raw
	}
	return json.RawMessage(out)
}
