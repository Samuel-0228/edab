package routing

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/google/uuid"
)

// MemoryStore provides a thread-safe in-memory implementation of Store.
type MemoryStore struct {
	mu     sync.RWMutex
	routes map[string]*Route
}

func NewMemoryStore() *MemoryStore {
	return &MemoryStore{
		routes: make(map[string]*Route),
	}
}

func (m *MemoryStore) Create(ctx context.Context, r *Route) (*Route, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	if r.ID == "" {
		r.ID = uuid.NewString()
	}
	now := time.Now().UTC()
	r.CreatedAt = now
	r.UpdatedAt = now

	cloned := *r
	m.routes[r.ID] = &cloned
	return &cloned, nil
}

func (m *MemoryStore) GetByID(ctx context.Context, id string) (*Route, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	r, ok := m.routes[id]
	if !ok {
		return nil, fmt.Errorf("%w: %s", ErrRouteNotFound, id)
	}
	cloned := *r
	return &cloned, nil
}

func (m *MemoryStore) GetBySourceID(ctx context.Context, sourceID string) ([]*Route, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	var list []*Route
	for _, r := range m.routes {
		if r.SourceID == sourceID {
			cloned := *r
			list = append(list, &cloned)
		}
	}
	return list, nil
}

func (m *MemoryStore) Update(ctx context.Context, r *Route) (*Route, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	existing, ok := m.routes[r.ID]
	if !ok {
		return nil, fmt.Errorf("%w: %s", ErrRouteNotFound, r.ID)
	}

	existing.SourceID = r.SourceID
	existing.DestinationID = r.DestinationID
	existing.Filters = r.Filters
	existing.Enabled = r.Enabled
	existing.UpdatedAt = time.Now().UTC()

	cloned := *existing
	return &cloned, nil
}

func (m *MemoryStore) Delete(ctx context.Context, id string) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	if _, ok := m.routes[id]; !ok {
		return fmt.Errorf("%w: %s", ErrRouteNotFound, id)
	}
	delete(m.routes, id)
	return nil
}

func (m *MemoryStore) List(ctx context.Context) ([]*Route, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	list := make([]*Route, 0, len(m.routes))
	for _, r := range m.routes {
		cloned := *r
		list = append(list, &cloned)
	}
	return list, nil
}
