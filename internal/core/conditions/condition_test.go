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
