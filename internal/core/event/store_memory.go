package event

import (
	"context"
	"sync"
	"time"

	"github.com/google/uuid"
)

// MemoryStore provides a thread-safe in-memory implementation of Store.
type MemoryStore struct {
	mu     sync.RWMutex
	events map[string]*Event
	byExt  map[string]string // "sourceID:externalID" -> eventID
}

func NewMemoryStore() *MemoryStore {
	return &MemoryStore{
		events: make(map[string]*Event),
		byExt:  make(map[string]string),
	}
}

func (m *MemoryStore) Create(ctx context.Context, e *Event) (*Event, bool, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	key := e.SourceID + ":" + e.ExternalEventID
	if existingID, ok := m.byExt[key]; ok {
		cloned := *m.events[existingID]
		return &cloned, false, nil
	}

	if e.ID == "" {
		e.ID = uuid.NewString()
	}
	if e.CreatedAt.IsZero() {
		e.CreatedAt = time.Now().UTC()
	}

	cloned := *e
	m.events[e.ID] = &cloned
	m.byExt[key] = e.ID
	return &cloned, true, nil
}

func (m *MemoryStore) GetByID(ctx context.Context, id string) (*Event, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	ev, ok := m.events[id]
	if !ok {
		return nil, ErrEventNotFound
	}
	cloned := *ev
	return &cloned, nil
}

func (m *MemoryStore) GetByExternalID(ctx context.Context, sourceID, externalID string) (*Event, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	key := sourceID + ":" + externalID
	id, ok := m.byExt[key]
	if !ok {
		return nil, ErrEventNotFound
	}
	cloned := *m.events[id]
	return &cloned, nil
}

func (m *MemoryStore) List(ctx context.Context, filter ListFilter) ([]*Event, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	var result []*Event
	for _, ev := range m.events {
		if filter.SourceID != "" && ev.SourceID != filter.SourceID {
			continue
		}
		if filter.EventType != "" && ev.EventType != filter.EventType {
			continue
		}
		cloned := *ev
		result = append(result, &cloned)
	}

	if filter.Offset > 0 && filter.Offset < len(result) {
		result = result[filter.Offset:]
	} else if filter.Offset >= len(result) && filter.Offset > 0 {
		return []*Event{}, nil
	}

	if filter.Limit > 0 && filter.Limit < len(result) {
		result = result[:filter.Limit]
	}

	return result, nil
}
