package routing

import (
	"context"
	"encoding/json"
	"testing"

	"kerob/internal/core/event"
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
			filters:  `{invalid`,
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

func TestRoutingEngineAndMemoryStore(t *testing.T) {
	ctx := context.Background()
	store := NewMemoryStore()

	// 1. Create Routes
	r1, err := store.Create(ctx, &Route{
		SourceID:      "src-1",
		DestinationID: "dest-1",
		Enabled:       true,
		Filters:       json.RawMessage(`{"event_types":["order.placed"]}`),
	})
	if err != nil || r1.ID == "" {
		t.Fatalf("failed to create route: %v", err)
	}

	r2, err := store.Create(ctx, &Route{
		SourceID:      "src-1",
		DestinationID: "dest-2",
		Enabled:       false, // disabled
		Filters:       json.RawMessage(`{"event_types":["order.placed"]}`),
	})
	if err != nil {
		t.Fatalf("failed to create route: %v", err)
	}

	_, err = store.Create(ctx, &Route{
		SourceID:      "src-2",
		DestinationID: "dest-3",
		Enabled:       true,
	})
	if err != nil {
		t.Fatalf("failed to create route: %v", err)
	}

	// 2. GetByID
	fetched, err := store.GetByID(ctx, r1.ID)
	if err != nil || fetched.DestinationID != "dest-1" {
		t.Fatalf("failed to get route: %v", err)
	}
	if _, err := store.GetByID(ctx, "nonexistent"); err == nil {
		t.Fatal("expected error on nonexistent route")
	}

	// 3. GetBySourceID
	srcRoutes, err := store.GetBySourceID(ctx, "src-1")
	if err != nil || len(srcRoutes) != 2 {
		t.Fatalf("expected 2 routes for src-1, got %d", len(srcRoutes))
	}

	// 4. List
	allRoutes, err := store.List(ctx)
	if err != nil || len(allRoutes) != 3 {
		t.Fatalf("expected 3 routes, got %d", len(allRoutes))
	}

	// 5. Update
	r1.Enabled = false
	updated, err := store.Update(ctx, r1)
	if err != nil || updated.Enabled != false {
		t.Fatalf("failed to update route: %v", err)
	}
	r1.Enabled = true
	_, _ = store.Update(ctx, r1)

	// 6. Delete
	if err := store.Delete(ctx, r2.ID); err != nil {
		t.Fatalf("failed to delete route: %v", err)
	}
	if err := store.Delete(ctx, "nonexistent"); err == nil {
		t.Fatal("expected error deleting nonexistent route")
	}

	// 7. Test Routing Engine ResolveRoutes
	engine := NewRoutingEngine(store)
	evMatching, _ := event.New("src-1", "ext-1", "order.placed", map[string]string{"foo": "bar"}, nil)
	resolved, err := engine.ResolveRoutes(ctx, evMatching)
	if err != nil {
		t.Fatalf("failed to resolve routes: %v", err)
	}
	if len(resolved) != 1 || resolved[0].DestinationID != "dest-1" {
		t.Fatalf("expected 1 resolved route for dest-1, got %d", len(resolved))
	}

	evNonMatching, _ := event.New("src-1", "ext-2", "user.signup", map[string]string{"foo": "bar"}, nil)
	resolvedEmpty, err := engine.ResolveRoutes(ctx, evNonMatching)
	if err != nil {
		t.Fatalf("failed to resolve routes: %v", err)
	}
	if len(resolvedEmpty) != 0 {
		t.Fatalf("expected 0 resolved routes, got %d", len(resolvedEmpty))
	}

	// 8. Update nonexistent route
	_, err = store.Update(ctx, &Route{ID: "nonexistent"})
	if err == nil {
		t.Fatal("expected error updating nonexistent route")
	}

	// 9. RoutingEngine with failing store
	failingStore := &mockFailingStore{}
	failingEngine := NewRoutingEngine(failingStore)
	_, err = failingEngine.ResolveRoutes(ctx, evMatching)
	if err == nil {
		t.Fatal("expected error from failing store in ResolveRoutes")
	}
}

type mockFailingStore struct {
	Store
}

func (m *mockFailingStore) GetBySourceID(ctx context.Context, sourceID string) ([]*Route, error) {
	return nil, context.DeadlineExceeded
}
