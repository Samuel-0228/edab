package destinations

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"sync"
	"time"

	"automation-engine/internal/core/event"
)

var (
	ErrDestinationNotFound = errors.New("destination not found")
	ErrAdapterNotFound     = errors.New("destination adapter not found")
)

// Destination represents a database-backed external target configuration.
type Destination struct {
	ID          string          `json:"id"`
	Name        string          `json:"name"`
	AdapterType string          `json:"adapter_type"`
	Config      json.RawMessage `json:"config"`
	Enabled     bool            `json:"enabled"`
	CreatedAt   time.Time       `json:"created_at"`
	UpdatedAt   time.Time       `json:"updated_at"`
}

// DestinationAdapter is the generic interface for any event destination/target.
// The core engine delivers generic Events through this interface without knowing
// the underlying service implementation.
type DestinationAdapter interface {
	Name() string
	Connect(ctx context.Context, config json.RawMessage) error
	Health(ctx context.Context) error
	Deliver(ctx context.Context, ev *event.Event) error
}

// Factory creates a new instance of a DestinationAdapter.
type Factory func() DestinationAdapter

// Registry stores and instantiates registered destination adapters.
type Registry struct {
	mu        sync.RWMutex
	factories map[string]Factory
}

// NewRegistry creates a new destination adapter registry.
func NewRegistry() *Registry {
	return &Registry{
		factories: make(map[string]Factory),
	}
}

// Register registers a destination adapter factory for a given adapter type name.
func (r *Registry) Register(adapterType string, factory Factory) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.factories[adapterType] = factory
}

// Create instantiates a new adapter by type name.
func (r *Registry) Create(adapterType string) (DestinationAdapter, error) {
	r.mu.RLock()
	factory, ok := r.factories[adapterType]
	r.mu.RUnlock()
	if !ok {
		return nil, fmt.Errorf("%w: %s", ErrAdapterNotFound, adapterType)
	}
	return factory(), nil
}

// RegisteredTypes returns a list of all registered adapter type names.
func (r *Registry) RegisteredTypes() []string {
	r.mu.RLock()
	defer r.mu.RUnlock()
	types := make([]string, 0, len(r.factories))
	for k := range r.factories {
		types = append(types, k)
	}
	return types
}

// Store defines database operations for Destinations.
type Store interface {
	Create(ctx context.Context, d *Destination) (*Destination, error)
	GetByID(ctx context.Context, id string) (*Destination, error)
	Update(ctx context.Context, d *Destination) (*Destination, error)
	Delete(ctx context.Context, id string) error
	List(ctx context.Context) ([]*Destination, error)
}
