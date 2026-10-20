package conditions

import (
	"encoding/json"
	"fmt"
	"reflect"
	"regexp"
	"strconv"
	"strings"

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
	OpExists         Operator = "exists"
	OpNotExists      Operator = "not_exists"
	OpGreaterThan    Operator = "greater_than"
	OpLessThan       Operator = "less_than"
	OpGreaterOrEqual Operator = "greater_or_equal"
	OpLessOrEqual    Operator = "less_or_equal"
	OpIn             Operator = "in"
	OpNotIn          Operator = "not_in"
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
	Field    string      `json:"field,omitempty"`
	Operator Operator    `json:"operator,omitempty"`
	Value    any         `json:"value,omitempty"`
	Logical  LogicalOp   `json:"logical,omitempty"`
	Rules    []Condition `json:"rules,omitempty"`
}

// Evaluate checks whether the condition evaluates to true against the given event.
// Evaluation is safe and deterministic with no arbitrary code execution.
func Evaluate(c *Condition, ev *event.Event) (bool, error) {
	if c == nil {
		return true, nil
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
			// Evaluates the first rule and inverts it (or ALL subrules negated)
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
	if c.Field == "" {
		// Empty rule matches by default
		return true, nil
	}

	fieldVal, exists := extractFieldValue(c.Field, ev)

	switch c.Operator {
	case OpExists:
		return exists, nil

	case OpNotExists:
		return !exists, nil

	case OpEquals:
		if !exists {
			return c.Value == nil, nil
		}
		return compareEqual(fieldVal, c.Value), nil

	case OpNotEquals:
		if !exists {
			return c.Value != nil, nil
		}
		return !compareEqual(fieldVal, c.Value), nil

	case OpContains:
		if !exists || fieldVal == nil {
			return false, nil
		}
		return checkContains(fieldVal, c.Value), nil

	case OpNotContains:
		if !exists || fieldVal == nil {
			return true, nil
		}
		return !checkContains(fieldVal, c.Value), nil

	case OpStartsWith:
		if !exists || fieldVal == nil {
			return false, nil
		}
		s1 := fmt.Sprintf("%v", fieldVal)
		s2 := fmt.Sprintf("%v", c.Value)
		return strings.HasPrefix(s1, s2), nil

	case OpEndsWith:
		if !exists || fieldVal == nil {
			return false, nil
		}
		s1 := fmt.Sprintf("%v", fieldVal)
		s2 := fmt.Sprintf("%v", c.Value)
		return strings.HasSuffix(s1, s2), nil

	case OpMatches:
		if !exists || fieldVal == nil {
			return false, nil
		}
		pattern, ok := c.Value.(string)
		if !ok {
			pattern = fmt.Sprintf("%v", c.Value)
		}
		// Bound regex length to avoid ReDoS
		if len(pattern) > 256 {
			return false, fmt.Errorf("regex pattern exceeds max length of 256 characters")
		}
		re, err := regexp.Compile(pattern)
		if err != nil {
			return false, fmt.Errorf("invalid regex '%s': %w", pattern, err)
		}
		return re.MatchString(fmt.Sprintf("%v", fieldVal)), nil

	case OpGreaterThan, OpLessThan, OpGreaterOrEqual, OpLessOrEqual:
		if !exists || fieldVal == nil {
			return false, nil
		}
		cmp, err := compareNumeric(fieldVal, c.Value)
		if err != nil {
			return false, err
		}
		switch c.Operator {
		case OpGreaterThan:
			return cmp > 0, nil
		case OpLessThan:
			return cmp < 0, nil
		case OpGreaterOrEqual:
			return cmp >= 0, nil
		case OpLessOrEqual:
			return cmp <= 0, nil
		}

	case OpIn:
		if !exists || fieldVal == nil {
			return false, nil
		}
		return checkIn(fieldVal, c.Value), nil

	case OpNotIn:
		if !exists || fieldVal == nil {
			return true, nil
		}
		return !checkIn(fieldVal, c.Value), nil

	default:
		return false, fmt.Errorf("unsupported condition operator: %s", c.Operator)
	}

	return false, nil
}

// extractFieldValue navigates normalized event data by dot-separated path.
// Supported roots: "payload.x.y", "metadata.x", "event_type", "source_id", "external_event_id", "id".
func extractFieldValue(path string, ev *event.Event) (any, bool) {
	parts := strings.Split(path, ".")
	if len(parts) == 0 {
		return nil, false
	}

	var root any
	switch parts[0] {
	case "payload":
		if len(ev.Payload) == 0 {
			return nil, false
		}
		var m any
		if err := json.Unmarshal(ev.Payload, &m); err != nil {
			return nil, false
		}
		root = m
	case "metadata":
		if len(ev.Metadata) == 0 {
			return nil, false
		}
		var m any
		if err := json.Unmarshal(ev.Metadata, &m); err != nil {
			return nil, false
		}
		root = m
	case "event_type":
		if len(parts) == 1 {
			return ev.EventType, true
		}
		return nil, false
	case "source_id":
		if len(parts) == 1 {
			return ev.SourceID, true
		}
		return nil, false
	case "external_event_id":
		if len(parts) == 1 {
			return ev.ExternalEventID, true
		}
		return nil, false
	case "id":
		if len(parts) == 1 {
			return ev.ID, true
		}
		return nil, false
	default:
		// Attempt fallback lookup directly in payload
		if len(ev.Payload) > 0 {
			var m any
			if err := json.Unmarshal(ev.Payload, &m); err == nil {
				return lookupPath(m, parts)
			}
		}
		return nil, false
	}

	if len(parts) == 1 {
		return root, true
	}

	return lookupPath(root, parts[1:])
}

func lookupPath(data any, parts []string) (any, bool) {
	curr := data
	for _, part := range parts {
		if curr == nil {
			return nil, false
		}
		switch val := curr.(type) {
		case map[string]any:
			v, ok := val[part]
			if !ok {
				return nil, false
			}
			curr = v
		case []any:
			idx, err := strconv.Atoi(part)
			if err != nil || idx < 0 || idx >= len(val) {
				return nil, false
			}
			curr = val[idx]
		default:
			return nil, false
		}
	}
	return curr, true
}

func compareEqual(a, b any) bool {
	if a == nil && b == nil {
		return true
	}
	if a == nil || b == nil {
		return false
	}

	// Try numeric comparison first
	if numA, okA := toFloat64(a); okA {
		if numB, okB := toFloat64(b); okB {
			return numA == numB
		}
	}

	// Try boolean
	if boolA, okA := a.(bool); okA {
		if boolB, okB := b.(bool); okB {
			return boolA == boolB
		}
	}

	// Compare string representations
	return fmt.Sprintf("%v", a) == fmt.Sprintf("%v", b)
}

func checkContains(container, target any) bool {
	if container == nil || target == nil {
		return false
	}

	switch c := container.(type) {
	case string:
		targetStr := fmt.Sprintf("%v", target)
		return strings.Contains(strings.ToLower(c), strings.ToLower(targetStr))
	case []any:
		for _, item := range c {
			if compareEqual(item, target) {
				return true
			}
		}
		return false
	default:
		str := fmt.Sprintf("%v", container)
		targetStr := fmt.Sprintf("%v", target)
		return strings.Contains(strings.ToLower(str), strings.ToLower(targetStr))
	}
}

func checkIn(item, list any) bool {
	if list == nil {
		return false
	}
	val := reflect.ValueOf(list)
	if val.Kind() == reflect.Slice || val.Kind() == reflect.Array {
		for i := 0; i < val.Len(); i++ {
			elem := val.Index(i).Interface()
			if compareEqual(item, elem) {
				return true
			}
		}
	}
	return false
}

func compareNumeric(a, b any) (int, error) {
	numA, okA := toFloat64(a)
	if !okA {
		return 0, fmt.Errorf("field value %v is not numeric", a)
	}
	numB, okB := toFloat64(b)
	if !okB {
		return 0, fmt.Errorf("target value %v is not numeric", b)
	}

	if numA < numB {
		return -1, nil
	} else if numA > numB {
		return 1, nil
	}
	return 0, nil
}

func toFloat64(v any) (float64, bool) {
	switch n := v.(type) {
	case float64:
		return n, true
	case float32:
		return float64(n), true
	case int:
		return float64(n), true
	case int64:
		return float64(n), true
	case int32:
		return float64(n), true
	case uint:
		return float64(n), true
	case uint64:
		return float64(n), true
	case string:
		f, err := strconv.ParseFloat(n, 64)
		if err == nil {
			return f, true
		}
	}
	return 0, false
}
