package routing

import (
	"encoding/json"
	"testing"

	"automation-engine/internal/core/event"
)

func TestRouteMatches(t *testing.T) {
	ev, err := event.New("src-1", "ext-1", "payment.succeeded", `{"amount": 100}`, nil)
	if err != nil {
		t.Fatalf("failed to create event: %v", err)
	}

	tests := []struct {
		name     string
		enabled  bool
		filters  string
		expected bool
	}{
		{
			name:     "disabled route should never match",
			enabled:  false,
			filters:  "",
			expected: false,
		},
		{
			name:     "enabled route with empty filter matches everything",
			enabled:  true,
			filters:  "",
			expected: true,
		},
		{
			name:     "enabled route with empty json object matches everything",
			enabled:  true,
			filters:  "{}",
			expected: true,
		},
		{
			name:     "matching event type",
			enabled:  true,
			filters:  `{"event_types": ["user.created", "payment.succeeded"]}`,
			expected: true,
		},
		{
			name:     "non-matching event type",
			enabled:  true,
			filters:  `{"event_types": ["user.created", "user.deleted"]}`,
			expected: false,
		},
		{
			name:     "wildcard event type matches anything",
			enabled:  true,
			filters:  `{"event_types": ["*"]}`,
			expected: true,
		},
		{
			name:     "malformed filter JSON allows by default",
			enabled:  true,
			filters:  `{not-valid-json}`,
			expected: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			r := &Route{
				ID:            "route-1",
				SourceID:      "src-1",
				DestinationID: "dest-1",
				Enabled:       tt.enabled,
				Filters:       json.RawMessage(tt.filters),
			}

			if got := r.Matches(ev); got != tt.expected {
				t.Errorf("Matches() = %v, expected %v", got, tt.expected)
			}
		})
	}
}
