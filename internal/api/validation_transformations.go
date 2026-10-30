package api

import (
	"encoding/json"
	"fmt"
	"strings"

	"kerob/internal/core/transformations"
)

// ValidateTransformation validates transformation type enum, output key, and template syntax.
func ValidateTransformation(t *transformations.Transformation) []FieldError {
	if t == nil {
		return nil
	}
	var errs []FieldError

	validTypes := map[string]bool{
		"template":    true,
		"mapping":     true,
		"json":        true,
		"passthrough": true,
		"":            true,
	}

	if !validTypes[t.Type] {
		errs = append(errs, FieldError{
			Field:   "transformations.type",
			Message: fmt.Sprintf("unsupported transformation type %q; supported: template, mapping, json, passthrough", t.Type),
		})
	}

	if len(t.OutputKey) > 64 {
		errs = append(errs, FieldError{
			Field:   "transformations.output_key",
			Message: "output_key cannot exceed 64 characters",
		})
	}

	if t.Type == "json" && strings.TrimSpace(t.Template) != "" {
		// Verify raw template isn't malformed JSON structure when literal JSON
		trimmed := strings.TrimSpace(t.Template)
		if !strings.Contains(trimmed, "{{") {
			var js map[string]interface{}
			if err := json.Unmarshal([]byte(trimmed), &js); err != nil {
				errs = append(errs, FieldError{
					Field:   "transformations.template",
					Message: "json template is not valid JSON syntax: " + err.Error(),
				})
			}
		}
	}

	return errs
}
