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
		"pick":        true,
		"omit":        true,
		"flatten":     true,
		"merge":       true,
		"compute":     true,
		"":            true,
	}

	if !validTypes[t.Type] {
		errs = append(errs, FieldError{
			Field:   "transformations.type",
			Message: fmt.Sprintf("unsupported transformation type %q; supported: template, mapping, json, passthrough, pick, omit, flatten, merge, compute", t.Type),
		})
	}

	if t.Type == "pick" && len(t.PickKeys) == 0 {
		errs = append(errs, FieldError{
			Field:   "transformations.pick_keys",
			Message: "pick transformation requires non-empty 'pick_keys'",
		})
	}

	if t.Type == "omit" && len(t.OmitKeys) == 0 {
		errs = append(errs, FieldError{
			Field:   "transformations.omit_keys",
			Message: "omit transformation requires non-empty 'omit_keys'",
		})
	}

	if t.Type == "compute" {
		if len(t.Compute) == 0 {
			errs = append(errs, FieldError{
				Field:   "transformations.compute",
				Message: "compute transformation requires at least one compute field",
			})
		}
		validOps := map[string]bool{
			"add": true, "+": true,
			"sub": true, "-": true,
			"mul": true, "*": true,
			"div": true, "/": true,
			"concat": true,
		}
		for i, comp := range t.Compute {
			if strings.TrimSpace(comp.Target) == "" {
				errs = append(errs, FieldError{
					Field:   fmt.Sprintf("transformations.compute[%d].target", i),
					Message: "target field name is required",
				})
			}
			op := strings.ToLower(strings.TrimSpace(comp.Op))
			if !validOps[op] {
				errs = append(errs, FieldError{
					Field:   fmt.Sprintf("transformations.compute[%d].op", i),
					Message: fmt.Sprintf("invalid compute op %q; must be add, sub, mul, div, or concat", comp.Op),
				})
			}
			if len(comp.Args) == 0 {
				errs = append(errs, FieldError{
					Field:   fmt.Sprintf("transformations.compute[%d].args", i),
					Message: "compute field requires at least one argument",
				})
			}
		}
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
