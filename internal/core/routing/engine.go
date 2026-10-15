package routing

import (
	"context"
	"fmt"

	"automation-engine/internal/core/event"
)

// RoutingEngine determines which destinations should receive an event.
type RoutingEngine struct {
	store Store
}

// NewRoutingEngine creates a new routing engine instance.
func NewRoutingEngine(store Store) *RoutingEngine {
	return &RoutingEngine{store: store}
}

// ResolveRoutes finds all enabled and filter-matching routes for the given event's source.
func (e *RoutingEngine) ResolveRoutes(ctx context.Context, ev *event.Event) ([]*Route, error) {
	routes, err := e.store.GetBySourceID(ctx, ev.SourceID)
	if err != nil {
		return nil, fmt.Errorf("failed to fetch routes for source %s: %w", ev.SourceID, err)
	}

	var matched []*Route
	for _, r := range routes {
		if r.Matches(ev) {
			matched = append(matched, r)
		}
	}

	return matched, nil
}
