package conditions

import (
	"testing"

	"kerob/internal/core/event"
)

func TestConditionsEvaluation(t *testing.T) {
	ev, err := event.New("src-telegram", "msg-1234", "telegram.message", map[string]any{
		"text":   "We have an urgent job opening for backend engineers!",
		"author": "Alice",
		"count":  42,
		"user": map[string]any{
			"username": "alice_dev",
			"active":   true,
			"score":    99.5,
		},
		"tags": []string{"tech", "jobs", "golang"},
	}, map[string]any{
		"channel_id": "-100123456",
		"priority":   "high",
	})
	if err != nil {
		t.Fatalf("failed to create test event: %v", err)
	}

	tests := []struct {
		name      string
		condition *Condition
		expected  bool
	}{
		{
			name:      "nil condition matches everything",
			condition: nil,
			expected:  true,
		},
		{
			name: "empty field matches everything",
			condition: &Condition{
				Field: "",
			},
			expected: true,
		},
		{
			name: "equals string matched",
			condition: &Condition{
				Field:    "payload.author",
				Operator: OpEquals,
				Value:    "Alice",
			},
			expected: true,
		},
		{
			name: "equals string mismatched",
			condition: &Condition{
				Field:    "payload.author",
				Operator: OpEquals,
				Value:    "Bob",
			},
			expected: false,
		},
		{
			name: "not equals matched",
			condition: &Condition{
				Field:    "payload.author",
				Operator: OpNotEquals,
				Value:    "Bob",
			},
			expected: true,
		},
		{
			name: "contains case-insensitive",
			condition: &Condition{
				Field:    "payload.text",
				Operator: OpContains,
				Value:    "JOB",
			},
			expected: true,
		},
		{
			name: "not contains",
			condition: &Condition{
				Field:    "payload.text",
				Operator: OpNotContains,
				Value:    "crypto",
			},
			expected: true,
		},
		{
			name: "starts with matched",
			condition: &Condition{
				Field:    "payload.text",
				Operator: OpStartsWith,
				Value:    "We have",
			},
			expected: true,
		},
		{
			name: "ends with matched",
			condition: &Condition{
				Field:    "payload.text",
				Operator: OpEndsWith,
				Value:    "engineers!",
			},
			expected: true,
		},
		{
			name: "regex matches",
			condition: &Condition{
				Field:    "payload.user.username",
				Operator: OpMatches,
				Value:    "^alice_[a-z]+$",
			},
			expected: true,
		},
		{
			name: "exists true",
			condition: &Condition{
				Field:    "payload.user.score",
				Operator: OpExists,
			},
			expected: true,
		},
		{
			name: "exists false",
			condition: &Condition{
				Field:    "payload.user.missing_field",
				Operator: OpExists,
			},
			expected: false,
		},
		{
			name: "not exists true",
			condition: &Condition{
				Field:    "payload.user.missing_field",
				Operator: OpNotExists,
			},
			expected: true,
		},
		{
			name: "numeric greater than",
			condition: &Condition{
				Field:    "payload.count",
				Operator: OpGreaterThan,
				Value:    40,
			},
			expected: true,
		},
		{
			name: "numeric less than",
			condition: &Condition{
				Field:    "payload.count",
				Operator: OpLessThan,
				Value:    50,
			},
			expected: true,
		},
		{
			name: "numeric greater or equal",
			condition: &Condition{
				Field:    "payload.user.score",
				Operator: OpGreaterOrEqual,
				Value:    99.5,
			},
			expected: true,
		},
		{
			name: "in list",
			condition: &Condition{
				Field:    "metadata.priority",
				Operator: OpIn,
				Value:    []any{"high", "urgent", "critical"},
			},
			expected: true,
		},
		{
			name: "not in list",
			condition: &Condition{
				Field:    "metadata.priority",
				Operator: OpNotIn,
				Value:    []any{"low", "trivial"},
			},
			expected: true,
		},
		{
			name: "and compound condition all matched",
			condition: &Condition{
				Logical: LogicalAnd,
				Rules: []Condition{
					{
						Field:    "event_type",
						Operator: OpEquals,
						Value:    "telegram.message",
					},
					{
						Field:    "payload.count",
						Operator: OpGreaterThan,
						Value:    10,
					},
				},
			},
			expected: true,
		},
		{
			name: "and compound condition one failed",
			condition: &Condition{
				Logical: LogicalAnd,
				Rules: []Condition{
					{
						Field:    "event_type",
						Operator: OpEquals,
						Value:    "telegram.message",
					},
					{
						Field:    "payload.count",
						Operator: OpGreaterThan,
						Value:    100, // Fails
					},
				},
			},
			expected: false,
		},
		{
			name: "or compound condition one matches",
			condition: &Condition{
				Logical: LogicalOr,
				Rules: []Condition{
					{
						Field:    "payload.author",
						Operator: OpEquals,
						Value:    "Zack", // Fails
					},
					{
						Field:    "payload.author",
						Operator: OpEquals,
						Value:    "Alice", // Matches
					},
				},
			},
			expected: true,
		},
		{
			name: "not compound condition inverts inner match",
			condition: &Condition{
				Logical: LogicalNot,
				Rules: []Condition{
					{
						Field:    "payload.author",
						Operator: OpEquals,
						Value:    "Alice",
					},
				},
			},
			expected: false,
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

	// toFloat64 conversions
	if val, ok := toFloat64(int(10)); !ok || val != 10 {
		t.Errorf("expected 10, got %f", val)
	}
	if val, ok := toFloat64(int32(20)); !ok || val != 20 {
		t.Errorf("expected 20, got %f", val)
	}
	if val, ok := toFloat64(float32(3.5)); !ok || val != 3.5 {
		t.Errorf("expected 3.5, got %f", val)
	}
	if val, ok := toFloat64("42.5"); !ok || val != 42.5 {
		t.Errorf("expected 42.5, got %f", val)
	}
	if _, ok := toFloat64("not-a-number"); ok {
		t.Error("expected false for invalid numeric string")
	}
	if _, ok := toFloat64(struct{}{}); ok {
		t.Error("expected false for struct")
	}
}

func TestConditionsInvalidInputAndEdgeCases(t *testing.T) {
	ev, _ := event.New("src-test", "ext-1", "test.event", map[string]any{"count": 10}, nil)

	// 1. Unsupported operator
	condUnsupported := &Condition{
		Field:    "payload.count",
		Operator: "invalid_unsupported_op",
		Value:    10,
	}
	_, err := Evaluate(condUnsupported, ev)
	if err == nil {
		t.Error("expected error for unsupported operator, got nil")
	}

	// 2. Invalid regex pattern
	condBadRegex := &Condition{
		Field:    "payload.count",
		Operator: OpMatches,
		Value:    "[unclosed",
	}
	_, err = Evaluate(condBadRegex, ev)
	if err == nil {
		t.Error("expected error for invalid regex, got nil")
	}

	// 3. Logical NOT operator
	condNot := &Condition{
		Logical: LogicalNot,
		Rules: []Condition{
			{
				Field:    "payload.count",
				Operator: OpEquals,
				Value:    10,
			},
		},
	}
	res, err := Evaluate(condNot, ev)
	if err != nil {
		t.Fatalf("unexpected error for logical NOT: %v", err)
	}
	if res != false {
		t.Errorf("expected logical NOT of true condition to be false, got %v", res)
	}

	// 4. Logical NOT with empty subconditions
	condNotNil := &Condition{Logical: LogicalNot}
	res, err = Evaluate(condNotNil, ev)
	if err != nil || !res {
		t.Errorf("expected true when LogicalNot has no subconditions, got %v", res)
	}

	// 5. Evaluate with nil event
	resNilEvent, err := Evaluate(&Condition{Field: "payload.count", Operator: OpExists}, nil)
	if err != nil || resNilEvent {
		t.Errorf("expected false without error when evaluating nil event, got res=%v, err=%v", resNilEvent, err)
	}
}

func TestCheckContainsAndToFloat64(t *testing.T) {
	// checkContains with slice
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

	// toFloat64 conversions
	if val, ok := toFloat64(int(8)); !ok || val != 8.0 {
		t.Errorf("failed int toFloat64: %v", val)
	}
	if val, ok := toFloat64(int64(16)); !ok || val != 16.0 {
		t.Errorf("failed int64 toFloat64: %v", val)
	}
	if val, ok := toFloat64(int32(32)); !ok || val != 32.0 {
		t.Errorf("failed int32 toFloat64: %v", val)
	}
	if val, ok := toFloat64(uint(42)); !ok || val != 42.0 {
		t.Errorf("failed uint toFloat64: %v", val)
	}
	if val, ok := toFloat64(uint64(64)); !ok || val != 64.0 {
		t.Errorf("failed uint64 toFloat64: %v", val)
	}
	if val, ok := toFloat64(float32(3.14)); !ok || float32(val) != float32(3.14) {
		t.Errorf("failed float32 toFloat64: %v", val)
	}
	if val, ok := toFloat64("123.45"); !ok || val != 123.45 {
		t.Errorf("failed string toFloat64: %v", val)
	}
	if _, ok := toFloat64(struct{}{}); ok {
		t.Error("expected false for unsupported struct type")
	}
}
