package auth

import (
	"strings"
)

// sensitiveKeyPatterns contains lowercase substrings that denote sensitive secret fields.
var sensitiveKeyPatterns = []string{
	"secret",
	"token",
	"password",
	"passwd",
	"key",
	"auth",
	"credential",
	"bearer",
	"private",
	"cert",
	"cookie",
	"session",
	"jwt",
	"webhook_secret",
	"api_key",
}

// isSensitiveKey returns true if a field key is deemed sensitive.
func isSensitiveKey(k string) bool {
	lower := strings.ToLower(k)
	for _, pattern := range sensitiveKeyPatterns {
		if strings.Contains(lower, pattern) {
			return true
		}
	}
	return false
}

// ScrubAuditDetails recursively traverses a metadata details map and masks all secret fields.
func ScrubAuditDetails(details map[string]any) map[string]any {
	if details == nil {
		return map[string]any{}
	}
	scrubbed := make(map[string]any, len(details))
	for k, v := range details {
		if isSensitiveKey(k) {
			scrubbed[k] = "[REDACTED]"
			continue
		}
		scrubbed[k] = scrubValue(v)
	}
	return scrubbed
}

func scrubValue(v any) any {
	switch val := v.(type) {
	case map[string]any:
		return ScrubAuditDetails(val)
	case map[string]string:
		res := make(map[string]any, len(val))
		for k, s := range val {
			if isSensitiveKey(k) {
				res[k] = "[REDACTED]"
			} else {
				res[k] = s
			}
		}
		return res
	case []any:
		res := make([]any, len(val))
		for i, item := range val {
			res[i] = scrubValue(item)
		}
		return res
	case string:
		// Also scrub inline authorization headers or tokens embedded in string values
		if strings.HasPrefix(strings.ToLower(val), "bearer ") || strings.HasPrefix(strings.ToLower(val), "basic ") {
			return "[REDACTED]"
		}
		return val
	default:
		return val
	}
}
