package conditions

import (
	"encoding/json"
	"fmt"
	"reflect"
	"regexp"
	"strconv"
	"strings"
	"time"

	"kerob/internal/core/event"
)

func evaluateLeaf(op Operator, fieldVal any, targetVal any, exists bool, ev *event.Event) (bool, error) {
	switch op {
	case OpExists:
		return exists, nil

	case OpNotExists:
		return !exists, nil

	case OpIsEmpty:
		return evaluateIsEmpty(fieldVal, exists), nil

	case OpIsNotEmpty:
		return !evaluateIsEmpty(fieldVal, exists), nil

	case OpEquals:
		if !exists {
			return targetVal == nil, nil
		}
		return compareEqual(fieldVal, targetVal), nil

	case OpNotEquals:
		if !exists {
			return targetVal != nil, nil
		}
		return !compareEqual(fieldVal, targetVal), nil

	case OpContains:
		if !exists || fieldVal == nil {
			return false, nil
		}
		return checkContains(fieldVal, targetVal), nil

	case OpNotContains:
		if !exists || fieldVal == nil {
			return true, nil
		}
		return !checkContains(fieldVal, targetVal), nil

	case OpStartsWith:
		if !exists || fieldVal == nil {
			return false, nil
		}
		s1 := fmt.Sprintf("%v", fieldVal)
		s2 := fmt.Sprintf("%v", targetVal)
		return strings.HasPrefix(s1, s2), nil

	case OpEndsWith:
		if !exists || fieldVal == nil {
			return false, nil
		}
		s1 := fmt.Sprintf("%v", fieldVal)
		s2 := fmt.Sprintf("%v", targetVal)
		return strings.HasSuffix(s1, s2), nil

	case OpMatches, OpRegex:
		if !exists || fieldVal == nil {
			return false, nil
		}
		pattern, ok := targetVal.(string)
		if !ok {
			pattern = fmt.Sprintf("%v", targetVal)
		}
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
		cmp, err := compareNumeric(fieldVal, targetVal)
		if err != nil {
			return false, err
		}
		switch op {
		case OpGreaterThan:
			return cmp > 0, nil
		case OpLessThan:
			return cmp < 0, nil
		case OpGreaterOrEqual:
			return cmp >= 0, nil
		case OpLessOrEqual:
			return cmp <= 0, nil
		}

	case OpBetween:
		if !exists || fieldVal == nil {
			return false, nil
		}
		return evaluateBetween(fieldVal, targetVal)

	case OpIn:
		if !exists || fieldVal == nil {
			return false, nil
		}
		return checkIn(fieldVal, targetVal), nil

	case OpNotIn:
		if !exists || fieldVal == nil {
			return true, nil
		}
		return !checkIn(fieldVal, targetVal), nil

	case OpTimeWindow:
		return evaluateTimeWindow(fieldVal, targetVal, exists, ev)

	default:
		return false, fmt.Errorf("unsupported condition operator: %s", op)
	}

	return false, nil
}

func evaluateIsEmpty(val any, exists bool) bool {
	if !exists || val == nil {
		return true
	}
	switch v := val.(type) {
	case string:
		return strings.TrimSpace(v) == ""
	case []any:
		return len(v) == 0
	case map[string]any:
		return len(v) == 0
	case []byte:
		return len(v) == 0
	case json.RawMessage:
		return len(v) == 0 || string(v) == "null" || string(v) == `""` || string(v) == "[]" || string(v) == "{}"
	default:
		rv := reflect.ValueOf(val)
		switch rv.Kind() {
		case reflect.Slice, reflect.Array, reflect.Map, reflect.String:
			return rv.Len() == 0
		default:
			return false
		}
	}
}

func evaluateBetween(val any, target any) (bool, error) {
	numVal, ok := toFloat64(val)
	if !ok {
		return false, fmt.Errorf("field value %v is not numeric for between comparison", val)
	}

	var minVal, maxVal float64
	switch t := target.(type) {
	case []any:
		if len(t) < 2 {
			return false, fmt.Errorf("between operator requires array of [min, max]")
		}
		n1, ok1 := toFloat64(t[0])
		n2, ok2 := toFloat64(t[1])
		if !ok1 || !ok2 {
			return false, fmt.Errorf("between array elements must be numeric")
		}
		minVal, maxVal = n1, n2
	case map[string]any:
		rawMin := t["min"]
		if rawMin == nil {
			rawMin = t["from"]
		}
		rawMax := t["max"]
		if rawMax == nil {
			rawMax = t["to"]
		}
		n1, ok1 := toFloat64(rawMin)
		n2, ok2 := toFloat64(rawMax)
		if !ok1 || !ok2 {
			return false, fmt.Errorf("between map requires numeric min/max or from/to")
		}
		minVal, maxVal = n1, n2
	default:
		return false, fmt.Errorf("between target must be [min, max] or {min, max}")
	}

	return numVal >= minVal && numVal <= maxVal, nil
}

func evaluateTimeWindow(fieldVal any, targetVal any, exists bool, ev *event.Event) (bool, error) {
	// 1. Determine evaluation timestamp
	var t time.Time
	if exists && fieldVal != nil {
		switch fv := fieldVal.(type) {
		case time.Time:
			t = fv
		case string:
			parsed, err := time.Parse(time.RFC3339, fv)
			if err != nil {
				parsed, err = time.Parse("2006-01-02 15:04:05", fv)
			}
			if err != nil {
				return false, fmt.Errorf("field time parsing failed: %w", err)
			}
			t = parsed
		default:
			t = time.Now().UTC()
		}
	} else if ev != nil && !ev.CreatedAt.IsZero() {
		t = ev.CreatedAt
	} else {
		t = time.Now().UTC()
	}

	// 2. Parse time window (e.g. "09:00-17:00") and timezone (default "UTC")
	var windowStr, tzStr string
	switch tv := targetVal.(type) {
	case string:
		parts := strings.Fields(tv)
		if len(parts) > 0 {
			windowStr = parts[0]
		}
		if len(parts) > 1 {
			tzStr = parts[1]
		}
	case map[string]any:
		if w, ok := tv["window"].(string); ok {
			windowStr = w
		} else if s, ok := tv["start"].(string); ok {
			if e, ok := tv["end"].(string); ok {
				windowStr = fmt.Sprintf("%s-%s", s, e)
			}
		}
		if tz, ok := tv["timezone"].(string); ok {
			tzStr = tz
		}
	default:
		return false, fmt.Errorf("invalid time_window target format")
	}

	if tzStr == "" {
		tzStr = "UTC"
	}
	loc, err := time.LoadLocation(tzStr)
	if err != nil {
		return false, fmt.Errorf("invalid timezone %q: %w", tzStr, err)
	}

	// 3. Compare in localized timezone
	locTime := t.In(loc)
	currentMinutes := locTime.Hour()*60 + locTime.Minute()

	winParts := strings.Split(windowStr, "-")
	if len(winParts) != 2 {
		return false, fmt.Errorf("time_window must be formatted as 'HH:MM-HH:MM'")
	}

	startMinutes, err := parseHHMM(winParts[0])
	if err != nil {
		return false, fmt.Errorf("invalid window start: %w", err)
	}
	endMinutes, err := parseHHMM(winParts[1])
	if err != nil {
		return false, fmt.Errorf("invalid window end: %w", err)
	}

	if startMinutes <= endMinutes {
		// Normal window within same day, e.g. 09:00 to 17:00
		return currentMinutes >= startMinutes && currentMinutes <= endMinutes, nil
	}
	// Overnight window, e.g. 22:00 to 06:00
	return currentMinutes >= startMinutes || currentMinutes <= endMinutes, nil
}

func parseHHMM(s string) (int, error) {
	s = strings.TrimSpace(s)
	parts := strings.Split(s, ":")
	if len(parts) != 2 {
		return 0, fmt.Errorf("expected HH:MM format, got %q", s)
	}
	h, err := strconv.Atoi(parts[0])
	if err != nil || h < 0 || h > 23 {
		return 0, fmt.Errorf("invalid hour in %q", s)
	}
	m, err := strconv.Atoi(parts[1])
	if err != nil || m < 0 || m > 59 {
		return 0, fmt.Errorf("invalid minute in %q", s)
	}
	return h*60 + m, nil
}

func compareEqual(a, b any) bool {
	if a == nil && b == nil {
		return true
	}
	if a == nil || b == nil {
		return false
	}

	if numA, okA := toFloat64(a); okA {
		if numB, okB := toFloat64(b); okB {
			return numA == numB
		}
	}

	if boolA, okA := a.(bool); okA {
		if boolB, okB := b.(bool); okB {
			return boolA == boolB
		}
	}

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
