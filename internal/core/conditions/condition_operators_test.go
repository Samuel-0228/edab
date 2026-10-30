package conditions

import (
	"testing"
	"time"

	"kerob/internal/core/event"
)

func TestConditionsBuiltinFieldsAndEdgeCases(t *testing.T) {
	ev, err := event.New("src-telegram", "msg-1234", "telegram.message", map[string]any{
		"text":   "We have an urgent job opening for backend engineers!",
		"count":  42,
		"tags":   []any{"tech", "jobs", "golang"},
		"active": true,
	}, map[string]any{
		"priority": "high",
	})
	if err != nil {
		t.Fatalf("failed to create event: %v", err)
	}

	tests := []struct {
		name      string
		condition *Condition
		expected  bool
	}{
		{
			name: "match source_id builtin",
			condition: &Condition{
				Field:    "source_id",
				Operator: OpEquals,
				Value:    "src-telegram",
			},
			expected: true,
		},
		{
			name: "match event_type builtin",
			condition: &Condition{
				Field:    "event_type",
				Operator: OpEquals,
				Value:    "telegram.message",
			},
			expected: true,
		},
		{
			name: "match external_event_id builtin",
			condition: &Condition{
				Field:    "external_event_id",
				Operator: OpEquals,
				Value:    "msg-1234",
			},
			expected: true,
		},
		{
			name: "match id builtin",
			condition: &Condition{
				Field:    "id",
				Operator: OpEquals,
				Value:    ev.ID,
			},
			expected: true,
		},
		{
			name: "array indexing payload.tags.0",
			condition: &Condition{
				Field:    "payload.tags.0",
				Operator: OpEquals,
				Value:    "tech",
			},
			expected: true,
		},
		{
			name: "starts_with operator",
			condition: &Condition{
				Field:    "payload.text",
				Operator: OpStartsWith,
				Value:    "We have",
			},
			expected: true,
		},
		{
			name: "ends_with operator",
			condition: &Condition{
				Field:    "payload.text",
				Operator: OpEndsWith,
				Value:    "engineers!",
			},
			expected: true,
		},
		{
			name: "greater_or_equal operator",
			condition: &Condition{
				Field:    "payload.count",
				Operator: OpGreaterOrEqual,
				Value:    42,
			},
			expected: true,
		},
		{
			name: "less_or_equal operator",
			condition: &Condition{
				Field:    "payload.count",
				Operator: OpLessOrEqual,
				Value:    42,
			},
			expected: true,
		},
		{
			name: "not_in operator",
			condition: &Condition{
				Field:    "payload.count",
				Operator: OpNotIn,
				Value:    []any{1, 2, 3},
			},
			expected: true,
		},
		{
			name: "contains on slice",
			condition: &Condition{
				Field:    "payload.tags",
				Operator: OpContains,
				Value:    "golang",
			},
			expected: true,
		},
		{
			name: "fallback lookup in payload directly",
			condition: &Condition{
				Field:    "text",
				Operator: OpContains,
				Value:    "urgent",
			},
			expected: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			res, err := Evaluate(tt.condition, ev)
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if res != tt.expected {
				t.Errorf("expected %v, got %v", tt.expected, res)
			}
		})
	}
}

func TestOperatorsBetween(t *testing.T) {
	ev, _ := event.New("src-1", "ext-1", "test.event", map[string]any{
		"temperature": 23.5,
		"score":       85,
	}, nil)

	// Slice format [min, max]
	res, err := Evaluate(&Condition{
		Field:    "payload.temperature",
		Operator: OpBetween,
		Value:    []any{20.0, 30.0},
	}, ev)
	if err != nil || !res {
		t.Errorf("expected 23.5 between 20 and 30, got res=%v, err=%v", res, err)
	}

	// Out of range
	res, err = Evaluate(&Condition{
		Field:    "payload.temperature",
		Operator: OpBetween,
		Value:    []any{25.0, 30.0},
	}, ev)
	if err != nil || res {
		t.Errorf("expected 23.5 NOT between 25 and 30, got res=%v", res)
	}

	// Map format {min: 80, max: 90}
	res, err = Evaluate(&Condition{
		Field:    "payload.score",
		Operator: OpBetween,
		Value:    map[string]any{"min": 80, "max": 90},
	}, ev)
	if err != nil || !res {
		t.Errorf("expected 85 between 80 and 90, got res=%v, err=%v", res, err)
	}
}

func TestOperatorsRegex(t *testing.T) {
	ev, _ := event.New("src-1", "ext-1", "test.event", map[string]any{
		"email": "dev.ops@example.com",
	}, nil)

	// Valid regex matching
	res, err := Evaluate(&Condition{
		Field:    "payload.email",
		Operator: OpRegex,
		Value:    `^[a-zA-Z0-9._%+-]+@example\.com$`,
	}, ev)
	if err != nil || !res {
		t.Errorf("expected regex match, got res=%v, err=%v", res, err)
	}

	// Valid regex not matching
	res, err = Evaluate(&Condition{
		Field:    "payload.email",
		Operator: OpRegex,
		Value:    `@company\.org$`,
	}, ev)
	if err != nil || res {
		t.Errorf("expected regex mismatch, got res=%v", res)
	}

	// Length complexity guard (> 256 chars)
	longPattern := "^" + string(make([]byte, 300)) + "$"
	_, err = Evaluate(&Condition{
		Field:    "payload.email",
		Operator: OpRegex,
		Value:    longPattern,
	}, ev)
	if err == nil {
		t.Error("expected error for regex pattern exceeding 256 characters")
	}
}

func TestOperatorsIsEmpty(t *testing.T) {
	ev, _ := event.New("src-1", "ext-1", "test.event", map[string]any{
		"empty_str":  "   ",
		"normal_str": "hello",
		"empty_arr":  []any{},
		"normal_arr": []any{1, 2},
		"empty_obj":  map[string]any{},
		"null_val":   nil,
	}, nil)

	tests := []struct {
		field    string
		op       Operator
		expected bool
	}{
		{"payload.empty_str", OpIsEmpty, true},
		{"payload.normal_str", OpIsEmpty, false},
		{"payload.empty_arr", OpIsEmpty, true},
		{"payload.normal_arr", OpIsEmpty, false},
		{"payload.empty_obj", OpIsEmpty, true},
		{"payload.null_val", OpIsEmpty, true},
		{"payload.missing_key", OpIsEmpty, true},
		{"payload.normal_str", OpIsNotEmpty, true},
		{"payload.empty_str", OpIsNotEmpty, false},
	}

	for _, tt := range tests {
		res, err := Evaluate(&Condition{Field: tt.field, Operator: tt.op}, ev)
		if err != nil {
			t.Fatalf("unexpected error for %s %s: %v", tt.field, tt.op, err)
		}
		if res != tt.expected {
			t.Errorf("expected %s %s == %v, got %v", tt.field, tt.op, tt.expected, res)
		}
	}
}

func TestOperatorsTimeWindow(t *testing.T) {
	// Fixed test time: 2026-10-30 14:30:00 UTC
	fixedTime, _ := time.Parse(time.RFC3339, "2026-10-30T14:30:00Z")

	ev, _ := event.New("src-1", "ext-1", "test.event", map[string]any{
		"event_time": "2026-10-30T14:30:00Z",
	}, nil)
	ev.CreatedAt = fixedTime

	// 14:30 UTC is inside 09:00-17:00 UTC
	res, err := Evaluate(&Condition{
		Field:    "payload.event_time",
		Operator: OpTimeWindow,
		Value:    "09:00-17:00 UTC",
	}, ev)
	if err != nil || !res {
		t.Errorf("expected 14:30 UTC inside 09:00-17:00 UTC, got res=%v, err=%v", res, err)
	}

	// 14:30 UTC is 10:30 EDT (America/New_York), which is inside 09:00-17:00 America/New_York
	res, err = Evaluate(&Condition{
		Field:    "payload.event_time",
		Operator: OpTimeWindow,
		Value:    "09:00-17:00 America/New_York",
	}, ev)
	if err != nil || !res {
		t.Errorf("expected 10:30 EDT inside 09:00-17:00 America/New_York, got res=%v, err=%v", res, err)
	}

	// Outside window
	res, err = Evaluate(&Condition{
		Field:    "payload.event_time",
		Operator: OpTimeWindow,
		Value:    "16:00-20:00 UTC",
	}, ev)
	if err != nil || res {
		t.Errorf("expected 14:30 UTC OUTSIDE 16:00-20:00 UTC, got res=%v", res)
	}

	// Overnight window: 22:00 to 06:00 UTC; 14:30 UTC is outside
	res, err = Evaluate(&Condition{
		Field:    "payload.event_time",
		Operator: OpTimeWindow,
		Value:    "22:00-06:00 UTC",
	}, ev)
	if err != nil || res {
		t.Errorf("expected 14:30 UTC OUTSIDE overnight window 22:00-06:00, got res=%v", res)
	}
}

func TestJSONPathAndWildcardQuantifiers(t *testing.T) {
	ev, _ := event.New("src-1", "ext-1", "order.created", map[string]any{
		"items": []any{
			map[string]any{"id": "item-1", "name": "Keyboard", "status": "shipped"},
			map[string]any{"id": "item-2", "name": "Mouse", "status": "pending"},
			map[string]any{"id": "item-3", "name": "Monitor", "status": "shipped"},
		},
	}, nil)

	// 1. Array index access: payload.items[0].name
	res, err := Evaluate(&Condition{
		Field:    "payload.items[0].name",
		Operator: OpEquals,
		Value:    "Keyboard",
	}, ev)
	if err != nil || !res {
		t.Errorf("expected items[0].name == Keyboard, got res=%v, err=%v", res, err)
	}

	// 2. Wildcard quantifier ANY in path: payload.items[*].status equals "pending"
	res, err = Evaluate(&Condition{
		Field:    "payload.items[*].status",
		Operator: OpEquals,
		Value:    "pending",
	}, ev)
	if err != nil || !res {
		t.Errorf("expected any status == pending to be true, got res=%v, err=%v", res, err)
	}

	// 3. Wildcard quantifier ALL in path: payload.items[all].status equals "shipped" -> false
	res, err = Evaluate(&Condition{
		Field:    "payload.items[all].status",
		Operator: OpEquals,
		Value:    "shipped",
	}, ev)
	if err != nil || res {
		t.Errorf("expected all status == shipped to be false, got res=%v", res)
	}

	// 4. Wildcard quantifier with explicit Quantifier: "all"
	res, err = Evaluate(&Condition{
		Field:      "payload.items[*].name",
		Operator:   OpExists,
		Quantifier: QuantifierAll,
	}, ev)
	if err != nil || !res {
		t.Errorf("expected all items to have name, got res=%v, err=%v", res, err)
	}
}

func TestHelperComparisons(t *testing.T) {
	if !compareEqual(nil, nil) {
		t.Error("expected nil == nil")
	}
	if compareEqual(nil, "something") {
		t.Error("expected nil != something")
	}
	if !compareEqual(true, true) {
		t.Error("expected true == true")
	}
	if compareEqual(true, false) {
		t.Error("expected true != false")
	}

	if val, ok := toFloat64(int(10)); !ok || val != 10 {
		t.Errorf("expected 10, got %f", val)
	}
	if val, ok := toFloat64("42.5"); !ok || val != 42.5 {
		t.Errorf("expected 42.5, got %f", val)
	}
}

func TestCheckContainsAndToFloat64(t *testing.T) {
	slice := []any{"apple", "banana", "cherry"}
	if !checkContains(slice, "banana") {
		t.Error("expected slice to contain banana")
	}
	if checkContains(slice, "orange") {
		t.Error("expected slice not to contain orange")
	}
	if checkContains(nil, "something") {
		t.Error("expected checkContains(nil, ...) to return false")
	}

	if val, ok := toFloat64(int(8)); !ok || val != 8.0 {
		t.Errorf("failed int toFloat64: %v", val)
	}
	if val, ok := toFloat64("123.45"); !ok || val != 123.45 {
		t.Errorf("failed string toFloat64: %v", val)
	}
}
