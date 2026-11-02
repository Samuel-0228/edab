package connector

import (
	"fmt"
	"sync"
)

// Registry manages registered connectors and their manifests.
type Registry struct {
	mu         sync.RWMutex
	connectors map[string]Connector
	order      []string
}

// NewRegistry creates a new empty connector registry.
func NewRegistry() *Registry {
	return &Registry{
		connectors: make(map[string]Connector),
		order:      make([]string, 0),
	}
}

// Register registers a connector. Fails if connector is nil or ID is empty.
func (r *Registry) Register(c Connector) error {
	if c == nil {
		return fmt.Errorf("cannot register nil connector")
	}
	manifest := c.Manifest()
	if manifest.ID == "" {
		return fmt.Errorf("connector manifest missing ID")
	}

	r.mu.Lock()
	defer r.mu.Unlock()

	if _, exists := r.connectors[manifest.ID]; !exists {
		r.order = append(r.order, manifest.ID)
	}
	r.connectors[manifest.ID] = c
	return nil
}

// Get returns a connector by its unique identifier.
func (r *Registry) Get(id string) (Connector, bool) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	c, ok := r.connectors[id]
	return c, ok
}

// Has checks if a connector is registered.
func (r *Registry) Has(id string) bool {
	r.mu.RLock()
	defer r.mu.RUnlock()

	_, ok := r.connectors[id]
	return ok
}

// List returns all registered connectors in insertion order.
func (r *Registry) List() []Connector {
	r.mu.RLock()
	defer r.mu.RUnlock()

	result := make([]Connector, 0, len(r.order))
	for _, id := range r.order {
		if c, ok := r.connectors[id]; ok {
			result = append(result, c)
		}
	}
	return result
}

// ListManifests returns the manifests of all registered connectors.
func (r *Registry) ListManifests() []Manifest {
	r.mu.RLock()
	defer r.mu.RUnlock()

	result := make([]Manifest, 0, len(r.order))
	for _, id := range r.order {
		if c, ok := r.connectors[id]; ok {
			result = append(result, c.Manifest())
		}
	}
	return result
}

// Default singleton registry for global connector registration.
var defaultRegistry = NewRegistry()

// Default returns the default global connector registry.
func Default() *Registry {
	return defaultRegistry
}

// Register registers a connector in the global registry.
func Register(c Connector) error {
	return defaultRegistry.Register(c)
}

// Get retrieves a connector from the global registry.
func Get(id string) (Connector, bool) {
	return defaultRegistry.Get(id)
}

// List retrieves all registered connectors from the global registry.
func List() []Connector {
	return defaultRegistry.List()
}

// ListManifests retrieves all connector manifests from the global registry.
func ListManifests() []Manifest {
	return defaultRegistry.ListManifests()
}
