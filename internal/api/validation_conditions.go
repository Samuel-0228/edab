package api

import (
	"fmt"
	"regexp"

	"kerob/internal/core/conditions"
)

// ValidateCondition recursively validates a condition's structure, operator enum, and regexes.
func ValidateCondition(c *conditions.Condition, path string) []FieldError {
	if c == nil {
		return nil
	}
	var errs []FieldError

	validOperators := map[conditions.Operator]bool{
		conditions.OpEquals:         true,
		conditions.OpNotEquals:      true,
		conditions.OpContains:       true,
		conditions.OpNotContains:    true,
		conditions.OpStartsWith:     true,
		conditions.OpEndsWith:       true,
		conditions.OpMatches:        true,
		conditions.OpRegex:          true,
		conditions.OpExists:         true,
		conditions.OpNotExists:      true,
		conditions.OpGreaterThan:    true,
		conditions.OpLessThan:       true,
		conditions.OpGreaterOrEqual: true,
		conditions.OpLessOrEqual:    true,
		conditions.OpBetween:        true,
		conditions.OpIn:             true,
		conditions.OpNotIn:          true,
		conditions.OpIsEmpty:        true,
		conditions.OpIsNotEmpty:     true,
		conditions.OpTimeWindow:     true,
	}

	validLogicals := map[conditions.LogicalOp]bool{
		conditions.LogicalAnd: true,
		conditions.LogicalOr:  true,
		conditions.LogicalNot: true,
		"":                    true,
	}

	if !validLogicals[c.Logical] {
		errs = append(errs, FieldError{
			Field:   path + ".logical",
			Message: fmt.Sprintf("unsupported logical operator %q; must be 'and', 'or', or 'not'", c.Logical),
		})
	}

	if c.Quantifier != "" && c.Quantifier != conditions.QuantifierAny && c.Quantifier != conditions.QuantifierAll {
		errs = append(errs, FieldError{
			Field:   path + ".quantifier",
			Message: fmt.Sprintf("unsupported array quantifier %q; must be 'any' or 'all'", c.Quantifier),
		})
	}

	if c.Operator != "" {
		if !validOperators[c.Operator] {
			errs = append(errs, FieldError{
				Field:   path + ".operator",
				Message: fmt.Sprintf("unsupported condition operator %q", c.Operator),
			})
		}
		if c.Operator == conditions.OpMatches || c.Operator == conditions.OpRegex {
			pattern, ok := c.Value.(string)
			if !ok || pattern == "" {
				errs = append(errs, FieldError{
					Field:   path + ".value",
					Message: fmt.Sprintf("%s operator requires non-empty regex pattern string in 'value'", c.Operator),
				})
			} else if len(pattern) > 256 {
				errs = append(errs, FieldError{
					Field:   path + ".value",
					Message: "regex pattern exceeds max length of 256 characters",
				})
			} else if _, err := regexp.Compile(pattern); err != nil {
				errs = append(errs, FieldError{
					Field:   path + ".value",
					Message: fmt.Sprintf("invalid regex pattern: %v", err),
				})
			}
		}
	}

	if len(c.Field) > 128 {
		errs = append(errs, FieldError{
			Field:   path + ".field",
			Message: "field path exceeds 128 characters",
		})
	}

	for i, rule := range c.Rules {
		rulePath := fmt.Sprintf("%s.rules[%d]", path, i)
		errs = append(errs, ValidateCondition(&rule, rulePath)...)
	}

	return errs
}
