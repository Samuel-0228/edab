package conditions

import (
	"fmt"

	"kerob/internal/core/event"
)

// Operator represents a condition comparison operator.
type Operator string

const (
	OpEquals         Operator = "equals"
	OpNotEquals      Operator = "not_equals"
	OpContains       Operator = "contains"
	OpNotContains    Operator = "not_contains"
	OpStartsWith     Operator = "starts_with"
	OpEndsWith       Operator = "ends_with"
	OpMatches        Operator = "matches"
	OpRegex          Operator = "regex"
	OpExists         Operator = "exists"
	OpNotExists      Operator = "not_exists"
	OpGreaterThan    Operator = "greater_than"
	OpLessThan       Operator = "less_than"
	OpGreaterOrEqual Operator = "greater_or_equal"
	OpLessOrEqual    Operator = "less_or_equal"
	OpBetween        Operator = "between"
	OpIn             Operator = "in"
	OpNotIn          Operator = "not_in"
	OpIsEmpty        Operator = "is_empty"
	OpIsNotEmpty     Operator = "is_not_empty"
	OpTimeWindow     Operator = "time_window"
)

// Quantifier represents a quantification mode over array items ("any" or "all").
const (
	QuantifierAny = "any"
	QuantifierAll = "all"
)

// LogicalOp represents a boolean operator combining multiple conditions.
type LogicalOp string

const (
	LogicalAnd LogicalOp = "and"
	LogicalOr  LogicalOp = "or"
	LogicalNot LogicalOp = "not"
)

// Condition defines a single rule or a nested group of rules.
type Condition struct {
	Field      string      `json:"field,omitempty"`
	Operator   Operator    `json:"operator,omitempty"`
	Value      any         `json:"value,omitempty"`
	Quantifier string      `json:"quantifier,omitempty"` // "any", "all", or empty
	Logical    LogicalOp   `json:"logical,omitempty"`
	Rules      []Condition `json:"rules,omitempty"`
}

// Evaluate checks whether the condition evaluates to true against the given event.
// Evaluation is pure, safe, and deterministic with no arbitrary code execution.
func Evaluate(c *Condition, ev *event.Event) (bool, error) {
	if c == nil {
		return true, nil
	}
	if ev == nil {
		return false, nil
	}

	// 1. Group / compound condition
	if len(c.Rules) > 0 || c.Logical != "" {
		logical := c.Logical
		if logical == "" {
			logical = LogicalAnd
		}

		switch logical {
		case LogicalAnd:
			for _, rule := range c.Rules {
				matched, err := Evaluate(&rule, ev)
				if err != nil {
					return false, err
				}
				if !matched {
					return false, nil
				}
			}
			return true, nil

		case LogicalOr:
			if len(c.Rules) == 0 {
				return true, nil
			}
			for _, rule := range c.Rules {
				matched, err := Evaluate(&rule, ev)
				if err != nil {
					return false, err
				}
				if matched {
					return true, nil
				}
			}
			return false, nil

		case LogicalNot:
			if len(c.Rules) == 0 {
				return true, nil
			}
			matched, err := Evaluate(&c.Rules[0], ev)
			if err != nil {
				return false, err
			}
			return !matched, nil

		default:
			return false, fmt.Errorf("unsupported logical operator: %s", logical)
		}
	}

	// 2. Leaf condition: evaluate field against operator and value
	if c.Field == "" && c.Operator != OpTimeWindow {
		// Empty rule matches by default (unless time_window without explicit field, which uses event.CreatedAt)
		return true, nil
	}

	extracted, quant, err := extractFieldValues(c.Field, ev)
	if err != nil {
		return false, err
	}

	// Determine effective quantifier (explicit property takes precedence if set)
	effectiveQuant := c.Quantifier
	if effectiveQuant == "" {
		effectiveQuant = quant
	}

	// Array wildcard evaluation (e.g. payload.items[*].status)
	if effectiveQuant != "" && extracted.isWildcard {
		if len(extracted.values) == 0 {
			if effectiveQuant == QuantifierAll {
				return true, nil
			}
			return false, nil
		}

		for _, val := range extracted.values {
			matched, err := evaluateLeaf(c.Operator, val, c.Value, true, ev)
			if err != nil {
				return false, err
			}
			if effectiveQuant == QuantifierAny && matched {
				return true, nil
			}
			if effectiveQuant == QuantifierAll && !matched {
				return false, nil
			}
		}

		if effectiveQuant == QuantifierAny {
			return false, nil
		}
		return true, nil
	}

	// Single field evaluation
	var singleVal any
	if len(extracted.values) > 0 {
		singleVal = extracted.values[0]
	}
	return evaluateLeaf(c.Operator, singleVal, c.Value, extracted.exists, ev)
}
