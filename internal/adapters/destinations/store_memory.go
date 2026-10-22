package destinations

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/google/uuid"
)

// MemoryStore provides a thread-safe in-memory implementation of Store.
type MemoryStore struct {
	mu           sync.RWMutex
	destinations map[string]*Destination
}

func NewMemoryStore() *MemoryStore {
	return &MemoryStore{
		destinations: make(map[string]*Destination),
	}
}

func (m *MemoryStore) Create(ctx context.Context, d *Destination) (*Destination, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	if d.ID == "" {
		d.ID = uuid.NewString()
	}
	now := time.Now().UTC()
	d.CreatedAt = now
	d.UpdatedAt = now

	cloned := *d
	m.destinations[d.ID] = &cloned
	return &cloned, nil
}

func (m *MemoryStore) GetByID(ctx context.Context, id string) (*Destination, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	d, ok := m.destinations[id]
	if !ok {
		return nil, fmt.Errorf("%w: %s", ErrDestinationNotFound, id)
	}
	cloned := *d
	return &cloned, nil
}

func (m *MemoryStore) Update(ctx context.Context, d *Destination) (*Destination, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	existing, ok := m.destinations[d.ID]
	if !ok {
		return nil, fmt.Errorf("%w: %s", ErrDestinationNotFound, d.ID)
	}

	existing.Name = d.Name
	existing.AdapterType = d.AdapterType
	existing.Config = d.Config
	existing.Enabled = d.Enabled
	existing.UpdatedAt = time.Now().UTC()

	cloned := *existing
	return &cloned, nil
}

func (m *MemoryStore) Delete(ctx context.Context, id string) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	if _, ok := m.destinations[id]; !ok {
		return fmt.Errorf("%w: %s", ErrDestinationNotFound, id)
	}
	delete(m.destinations, id)
	return nil
}

func (m *MemoryStore) List(ctx context.Context) ([]*Destination, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	list := make([]*Destination, 0, len(m.destinations))
	for _, d := range m.destinations {
		cloned := *d
		list = append(list, &cloned)
	}
	return list, nil
}
