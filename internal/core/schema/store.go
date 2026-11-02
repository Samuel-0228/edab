package schema

import (
	"context"
	"fmt"
	"sort"
	"sync"
	"time"

	"github.com/google/uuid"
)

// Store defines persistent storage operations for event schemas.
type Store interface {
	Register(ctx context.Context, s *Schema) (*Schema, error)
	GetByID(ctx context.Context, id string) (*Schema, error)
	GetLatest(ctx context.Context, tenantID, name string) (*Schema, error)
	GetByVersion(ctx context.Context, tenantID, name string, version int) (*Schema, error)
	List(ctx context.Context, tenantID string) ([]*Schema, error)
	ListVersions(ctx context.Context, tenantID, name string) ([]*Schema, error)
	Delete(ctx context.Context, id string) error
}

// MemoryStore implements an in-memory Store for schema registry testing and local execution.
type MemoryStore struct {
	mu      sync.RWMutex
	schemas map[string]*Schema // key: ID
}

// NewMemoryStore creates a new in-memory schema store.
func NewMemoryStore() *MemoryStore {
	return &MemoryStore{
		schemas: make(map[string]*Schema),
	}
}

func (m *MemoryStore) Register(ctx context.Context, s *Schema) (*Schema, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	if s.Name == "" {
		return nil, fmt.Errorf("schema name is required")
	}

	// Calculate next version if not set
	if s.Version <= 0 {
		highestVersion := 0
		for _, existing := range m.schemas {
			if existing.TenantID == s.TenantID && existing.Name == s.Name {
				if existing.Version > highestVersion {
					highestVersion = existing.Version
				}
			}
		}
		s.Version = highestVersion + 1
	} else {
		// Check for conflict
		for _, existing := range m.schemas {
			if existing.TenantID == s.TenantID && existing.Name == s.Name && existing.Version == s.Version {
				return nil, fmt.Errorf("%w: version %d for %s already exists", ErrSchemaConflict, s.Version, s.Name)
			}
		}
	}

	if s.ID == "" {
		s.ID = uuid.NewString()
	}
	if s.Type == "" {
		s.Type = TypeJSONSchema
	}
	if s.Compatibility == "" {
		s.Compatibility = CompatibilityBackward
	}
	s.CreatedAt = time.Now().UTC()

	// Store copy
	clone := *s
	m.schemas[s.ID] = &clone
	return &clone, nil
}

func (m *MemoryStore) GetByID(ctx context.Context, id string) (*Schema, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	s, ok := m.schemas[id]
	if !ok {
		return nil, ErrSchemaNotFound
	}
	clone := *s
	return &clone, nil
}

func (m *MemoryStore) GetLatest(ctx context.Context, tenantID, name string) (*Schema, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	var latest *Schema
	for _, s := range m.schemas {
		if (tenantID == "" || s.TenantID == "" || s.TenantID == tenantID) && s.Name == name {
			if latest == nil || s.Version > latest.Version {
				latest = s
			}
		}
	}

	if latest == nil {
		return nil, ErrSchemaNotFound
	}
	clone := *latest
	return &clone, nil
}

func (m *MemoryStore) GetByVersion(ctx context.Context, tenantID, name string, version int) (*Schema, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	for _, s := range m.schemas {
		if (tenantID == "" || s.TenantID == "" || s.TenantID == tenantID) && s.Name == name && s.Version == version {
			clone := *s
			return &clone, nil
		}
	}
	return nil, ErrSchemaNotFound
}

func (m *MemoryStore) List(ctx context.Context, tenantID string) ([]*Schema, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	latestMap := make(map[string]*Schema)
	for _, s := range m.schemas {
		if tenantID == "" || s.TenantID == "" || s.TenantID == tenantID {
			curr, exists := latestMap[s.Name]
			if !exists || s.Version > curr.Version {
				latestMap[s.Name] = s
			}
		}
	}

	result := make([]*Schema, 0, len(latestMap))
	for _, s := range latestMap {
		clone := *s
		result = append(result, &clone)
	}

	sort.Slice(result, func(i, j int) bool {
		return result[i].Name < result[j].Name
	})

	return result, nil
}

func (m *MemoryStore) ListVersions(ctx context.Context, tenantID, name string) ([]*Schema, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	var result []*Schema
	for _, s := range m.schemas {
		if (tenantID == "" || s.TenantID == "" || s.TenantID == tenantID) && s.Name == name {
			clone := *s
			result = append(result, &clone)
		}
	}

	sort.Slice(result, func(i, j int) bool {
		return result[i].Version < result[j].Version
	})

	return result, nil
}

func (m *MemoryStore) Delete(ctx context.Context, id string) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	if _, ok := m.schemas[id]; !ok {
		return ErrSchemaNotFound
	}
	delete(m.schemas, id)
	return nil
}
