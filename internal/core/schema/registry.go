package schema

import (
	"context"
	"errors"
	"fmt"
)

// Registry orchestrates schema versioning, compatibility enforcement, and payload validation.
type Registry struct {
	store Store
}

// NewRegistry creates a new schema Registry wrapping the provided Store.
func NewRegistry(store Store) *Registry {
	if store == nil {
		store = NewMemoryStore()
	}
	return &Registry{store: store}
}

// RegisterSchema checks compatibility against the latest active schema and registers a new version.
func (r *Registry) RegisterSchema(ctx context.Context, s *Schema) (*Schema, error) {
	if s == nil {
		return nil, errors.New("cannot register nil schema")
	}
	if s.Name == "" {
		return nil, errors.New("schema name is required")
	}
	if s.Definition == "" {
		return nil, errors.New("schema definition is required")
	}
	if s.Type == "" {
		s.Type = TypeJSONSchema
	}
	if s.Compatibility == "" {
		s.Compatibility = CompatibilityBackward
	}

	// 1. First, validate that the schema definition itself is valid syntax
	if _, err := ValidatePayload(s, []byte("{}")); err != nil && errors.Is(err, ErrInvalidDefinition) {
		return nil, err
	}

	// 2. Fetch latest existing schema for compatibility check
	latest, err := r.store.GetLatest(ctx, s.TenantID, s.Name)
	if err == nil && latest != nil {
		if compatErr := CheckCompatibility(latest, s); compatErr != nil {
			return nil, compatErr
		}
	} else if err != nil && !errors.Is(err, ErrSchemaNotFound) {
		return nil, fmt.Errorf("failed to check existing schemas: %w", err)
	}

	// 3. Persist new schema
	return r.store.Register(ctx, s)
}

// ValidateEvent checks whether an event's payload conforms to its registered schema.
// If no schema is registered for the eventType, it returns (nil, nil) allowing passthrough.
func (r *Registry) ValidateEvent(ctx context.Context, tenantID, eventType string, payload []byte) ([]ValidationError, error) {
	sc, err := r.store.GetLatest(ctx, tenantID, eventType)
	if errors.Is(err, ErrSchemaNotFound) {
		// Passthrough if no schema is configured
		return nil, nil
	}
	if err != nil {
		return nil, err
	}

	valErrs, err := ValidatePayload(sc, payload)
	if err != nil {
		return nil, err
	}
	return valErrs, nil
}

// Store returns the underlying schema Store.
func (r *Registry) Store() Store {
	return r.store
}
