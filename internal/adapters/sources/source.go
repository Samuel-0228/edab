package sources

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
	ErrSourceNotFound  = errors.New("source not found")
	ErrAdapterNotFound = errors.New("source adapter not found")
)

// Source represents a database-backed external source configuration.
type Source struct {
	ID          string          `json:"id"`
	Name        string          `json:"name"`
	AdapterType string          `json:"adapter_type"`
	Config      json.RawMessage `json:"config"`
	Enabled     bool            `json:"enabled"`
	CreatedAt   time.Time       `json:"created_at"`
	UpdatedAt   time.Time       `json:"updated_at"`
}

// RawEvent represents unnormalized data captured by a source before normalization.
type RawEvent struct {
	ExternalID string          `json:"external_id"`
	Type       string          `json:"type"`
	Data       json.RawMessage `json:"data"`
	Metadata   json.RawMessage `json:"metadata"`
}

// SourceAdapter is the generic interface for any event source.
// Specific source implementations (Mock, Webhook, RSS, Telegram, GitHub, etc.)
// must satisfy this interface.
type SourceAdapter interface {
	Name() string
	Connect(ctx context.Context, config json.RawMessage) error
	Health(ctx context.Context) error
	Poll(ctx context.Context) ([]RawEvent, error)
	Normalize(ctx context.Context, sourceID string, raw []byte) (*event.Event, error)
}

// Factory creates a new instance of a SourceAdapter.
type Factory func() SourceAdapter

// Registry stores and instantiates registered source adapters.
type Registry struct {
	mu        sync.RWMutex
	factories map[string]Factory
}

// NewRegistry creates a new source adapter registry.
func NewRegistry() *Registry {
	return &Registry{
		factories: make(map[string]Factory),
	}
}

// Register registers a source adapter factory for a given adapter type name.
func (r *Registry) Register(adapterType string, factory Factory) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.factories[adapterType] = factory
}

// Create instantiates a new adapter by type name.
func (r *Registry) Create(adapterType string) (SourceAdapter, error) {
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

// Store defines database operations for Sources.
type Store interface {
	Create(ctx context.Context, s *Source) (*Source, error)
	GetByID(ctx context.Context, id string) (*Source, error)
	Update(ctx context.Context, s *Source) (*Source, error)
	Delete(ctx context.Context, id string) error
	List(ctx context.Context) ([]*Source, error)
}
