package adapters

import (
	"sync"
)

// Registry manages registered integrations and their manifests.
type Registry struct {
	mu           sync.RWMutex
	integrations map[string]Integration
	order        []string
}

// NewRegistry creates a new, empty integration registry.
func NewRegistry() *Registry {
	return &Registry{
		integrations: make(map[string]Integration),
		order:        make([]string, 0),
	}
}

// Register adds an integration to the registry.
func (r *Registry) Register(i Integration) {
	if i == nil {
		return
	}
	r.mu.Lock()
	defer r.mu.Unlock()

	manifest := i.Manifest()
	if _, exists := r.integrations[manifest.ID]; !exists {
		r.order = append(r.order, manifest.ID)
	}
	r.integrations[manifest.ID] = i
}

// Get returns an integration by its unique ID.
func (r *Registry) Get(id string) (Integration, bool) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	i, ok := r.integrations[id]
	return i, ok
}

// GetManifest returns the declarative manifest of an integration.
func (r *Registry) GetManifest(id string) (Manifest, bool) {
	i, ok := r.Get(id)
	if !ok {
		return Manifest{}, false
	}
	return i.Manifest(), true
}

// List returns all registered integration manifests in order.
func (r *Registry) List() []Manifest {
	r.mu.RLock()
	defer r.mu.RUnlock()

	result := make([]Manifest, 0, len(r.order))
	for _, id := range r.order {
		if i, ok := r.integrations[id]; ok {
			result = append(result, i.Manifest())
		}
	}
	return result
}

// GetSchema returns the JSON Schema for an integration, optionally selecting
// a specific action or trigger schema if operationType is provided.
func (r *Registry) GetSchema(id string, operationType string) (map[string]any, bool) {
	m, ok := r.GetManifest(id)
	if !ok {
		return nil, false
	}

	if operationType != "" && m.Schemas != nil {
		if schema, found := m.Schemas[operationType]; found {
			return schema, true
		}
	}

	if m.ConfigSchema != nil {
		return m.ConfigSchema, true
	}

	return map[string]any{"type": "object"}, true
}

// defaultRegistry is the singleton registry populated with all builtin integrations.
var defaultRegistry *Registry

func init() {
	defaultRegistry = NewRegistry()
	for _, item := range BuiltinIntegrations() {
		defaultRegistry.Register(item)
	}
}

// Default returns the global integration registry singleton.
func Default() *Registry {
	return defaultRegistry
}

// Register registers an integration into the global registry.
func Register(i Integration) {
	defaultRegistry.Register(i)
}

// Get looks up an integration in the global registry.
func Get(id string) (Integration, bool) {
	return defaultRegistry.Get(id)
}

// GetManifest returns the manifest for an integration in the global registry.
func GetManifest(id string) (Manifest, bool) {
	return defaultRegistry.GetManifest(id)
}

// Catalog returns all available integrations in the Kerob platform (backward-compatible).
func Catalog() []IntegrationMetadata {
	return defaultRegistry.List()
}

// GetSchema returns the JSON schema for an integration in the global registry.
func GetSchema(id string, operationType string) (map[string]any, bool) {
	return defaultRegistry.GetSchema(id, operationType)
}
