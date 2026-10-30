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
		conditions.OpExists:         true,
		conditions.OpNotExists:      true,
		conditions.OpGreaterThan:    true,
		conditions.OpLessThan:       true,
		conditions.OpGreaterOrEqual: true,
		conditions.OpLessOrEqual:    true,
		conditions.OpIn:             true,
		conditions.OpNotIn:          true,
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

	if c.Operator != "" {
		if !validOperators[c.Operator] {
			errs = append(errs, FieldError{
				Field:   path + ".operator",
				Message: fmt.Sprintf("unsupported condition operator %q", c.Operator),
			})
		}
		if c.Operator == conditions.OpMatches {
			pattern, ok := c.Value.(string)
			if !ok || pattern == "" {
				errs = append(errs, FieldError{
					Field:   path + ".value",
					Message: "matches operator requires non-empty regex pattern string in 'value'",
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
