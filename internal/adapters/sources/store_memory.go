package sources

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/google/uuid"
)

// MemoryStore provides a thread-safe in-memory implementation of Store.
type MemoryStore struct {
	mu      sync.RWMutex
	sources map[string]*Source
}

func NewMemoryStore() *MemoryStore {
	return &MemoryStore{
		sources: make(map[string]*Source),
	}
}

func (m *MemoryStore) Create(ctx context.Context, s *Source) (*Source, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	if s.ID == "" {
		s.ID = uuid.NewString()
	}
	now := time.Now().UTC()
	s.CreatedAt = now
	s.UpdatedAt = now

	cloned := *s
	m.sources[s.ID] = &cloned
	return &cloned, nil
}

func (m *MemoryStore) GetByID(ctx context.Context, id string) (*Source, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	s, ok := m.sources[id]
	if !ok {
		return nil, fmt.Errorf("%w: %s", ErrSourceNotFound, id)
	}
	cloned := *s
	return &cloned, nil
}

func (m *MemoryStore) Update(ctx context.Context, s *Source) (*Source, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	existing, ok := m.sources[s.ID]
	if !ok {
		return nil, fmt.Errorf("%w: %s", ErrSourceNotFound, s.ID)
	}

	existing.Name = s.Name
	existing.AdapterType = s.AdapterType
	existing.Config = s.Config
	existing.Enabled = s.Enabled
	existing.UpdatedAt = time.Now().UTC()

	cloned := *existing
	return &cloned, nil
}

func (m *MemoryStore) Delete(ctx context.Context, id string) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	if _, ok := m.sources[id]; !ok {
		return fmt.Errorf("%w: %s", ErrSourceNotFound, id)
	}
	delete(m.sources, id)
	return nil
}

func (m *MemoryStore) List(ctx context.Context) ([]*Source, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	list := make([]*Source, 0, len(m.sources))
	for _, s := range m.sources {
		cloned := *s
		list = append(list, &cloned)
	}
	return list, nil
}
