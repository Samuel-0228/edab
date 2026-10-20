package routing

import (
	"context"
	"encoding/json"
	"errors"
	"time"

	"kerob/internal/core/event"
)

var (
	ErrRouteNotFound = errors.New("route not found")
)

// Route connects a source to a destination with optional filtering.
type Route struct {
	ID            string          `json:"id"`
	SourceID      string          `json:"source_id"`
	DestinationID string          `json:"destination_id"`
	Enabled       bool            `json:"enabled"`
	Filters       json.RawMessage `json:"filters"`
	CreatedAt     time.Time       `json:"created_at"`
	UpdatedAt     time.Time       `json:"updated_at"`
}

// FilterConfig holds parsed route filter options.
type FilterConfig struct {
	EventTypes []string `json:"event_types,omitempty"`
}

// Matches checks if the route's filters match the given event.
// Designed so additional filter criteria can easily be added.
func (r *Route) Matches(ev *event.Event) bool {
	if !r.Enabled {
		return false
	}

	if len(r.Filters) == 0 || string(r.Filters) == "{}" {
		return true
	}

	var fc FilterConfig
	if err := json.Unmarshal(r.Filters, &fc); err != nil {
		// If filters are not unmarshable as standard filter config, allow by default
		return true
	}

	// Filter by event_types if specified
	if len(fc.EventTypes) > 0 {
		matched := false
		for _, et := range fc.EventTypes {
			if et == "*" || et == ev.EventType {
				matched = true
				break
			}
		}
		if !matched {
			return false
		}
	}

	return true
}

// Store defines database operations for Routes.
type Store interface {
	Create(ctx context.Context, r *Route) (*Route, error)
	GetByID(ctx context.Context, id string) (*Route, error)
	GetBySourceID(ctx context.Context, sourceID string) ([]*Route, error)
	Update(ctx context.Context, r *Route) (*Route, error)
	Delete(ctx context.Context, id string) error
	List(ctx context.Context) ([]*Route, error)
}

// Engine evaluates and resolves active routes for an event.
type Engine interface {
	ResolveRoutes(ctx context.Context, ev *event.Event) ([]*Route, error)
}
