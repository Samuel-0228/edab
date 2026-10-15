package tests

import (
	"context"
	"sync"
	"time"

	"automation-engine/internal/adapters/destinations"
	"automation-engine/internal/adapters/sources"
	"automation-engine/internal/core/delivery"
	"automation-engine/internal/core/event"
	"automation-engine/internal/core/routing"

	"github.com/google/uuid"
)

// In-memory Event Store
type MemoryEventStore struct {
	mu     sync.RWMutex
	events map[string]*event.Event
	byExt  map[string]string // key: "sourceID:externalID" -> eventID
}

func NewMemoryEventStore() *MemoryEventStore {
	return &MemoryEventStore{
		events: make(map[string]*event.Event),
		byExt:  make(map[string]string),
	}
}

func (m *MemoryEventStore) Create(ctx context.Context, e *event.Event) (*event.Event, bool, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	key := e.SourceID + ":" + e.ExternalEventID
	if existingID, ok := m.byExt[key]; ok {
		return m.events[existingID], false, nil
	}

	if e.ID == "" {
		e.ID = uuid.NewString()
	}
	if e.CreatedAt.IsZero() {
		e.CreatedAt = time.Now().UTC()
	}

	m.events[e.ID] = e
	m.byExt[key] = e.ID
	return e, true, nil
}

func (m *MemoryEventStore) GetByID(ctx context.Context, id string) (*event.Event, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	ev, ok := m.events[id]
	if !ok {
		return nil, event.ErrEventNotFound
	}
	return ev, nil
}

func (m *MemoryEventStore) GetByExternalID(ctx context.Context, sourceID, externalID string) (*event.Event, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	id, ok := m.byExt[sourceID+":"+externalID]
	if !ok {
		return nil, event.ErrEventNotFound
	}
	return m.events[id], nil
}

func (m *MemoryEventStore) List(ctx context.Context, filter event.ListFilter) ([]*event.Event, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	var list []*event.Event
	for _, ev := range m.events {
		if filter.SourceID != "" && ev.SourceID != filter.SourceID {
			continue
		}
		if filter.EventType != "" && ev.EventType != filter.EventType {
			continue
		}
		list = append(list, ev)
	}
	return list, nil
}

// In-memory Source Store
type MemorySourceStore struct {
	mu      sync.RWMutex
	sources map[string]*sources.Source
}

func NewMemorySourceStore() *MemorySourceStore {
	return &MemorySourceStore{sources: make(map[string]*sources.Source)}
}

func (m *MemorySourceStore) Create(ctx context.Context, s *sources.Source) (*sources.Source, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if s.ID == "" {
		s.ID = uuid.NewString()
	}
	now := time.Now().UTC()
	s.CreatedAt = now
	s.UpdatedAt = now
	m.sources[s.ID] = s
	return s, nil
}

func (m *MemorySourceStore) GetByID(ctx context.Context, id string) (*sources.Source, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	s, ok := m.sources[id]
	if !ok {
		return nil, sources.ErrSourceNotFound
	}
	return s, nil
}

func (m *MemorySourceStore) Update(ctx context.Context, s *sources.Source) (*sources.Source, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	existing, ok := m.sources[s.ID]
	if !ok {
		return nil, sources.ErrSourceNotFound
	}
	s.CreatedAt = existing.CreatedAt
	s.UpdatedAt = time.Now().UTC()
	m.sources[s.ID] = s
	return s, nil
}

func (m *MemorySourceStore) Delete(ctx context.Context, id string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if _, ok := m.sources[id]; !ok {
		return sources.ErrSourceNotFound
	}
	delete(m.sources, id)
	return nil
}

func (m *MemorySourceStore) List(ctx context.Context) ([]*sources.Source, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	var list []*sources.Source
	for _, s := range m.sources {
		list = append(list, s)
	}
	return list, nil
}

// In-memory Destination Store
type MemoryDestinationStore struct {
	mu           sync.RWMutex
	destinations map[string]*destinations.Destination
}

func NewMemoryDestinationStore() *MemoryDestinationStore {
	return &MemoryDestinationStore{destinations: make(map[string]*destinations.Destination)}
}

func (m *MemoryDestinationStore) Create(ctx context.Context, d *destinations.Destination) (*destinations.Destination, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if d.ID == "" {
		d.ID = uuid.NewString()
	}
	now := time.Now().UTC()
	d.CreatedAt = now
	d.UpdatedAt = now
	m.destinations[d.ID] = d
	return d, nil
}

func (m *MemoryDestinationStore) GetByID(ctx context.Context, id string) (*destinations.Destination, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	d, ok := m.destinations[id]
	if !ok {
		return nil, destinations.ErrDestinationNotFound
	}
	return d, nil
}

func (m *MemoryDestinationStore) Update(ctx context.Context, d *destinations.Destination) (*destinations.Destination, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	existing, ok := m.destinations[d.ID]
	if !ok {
		return nil, destinations.ErrDestinationNotFound
	}
	d.CreatedAt = existing.CreatedAt
	d.UpdatedAt = time.Now().UTC()
	m.destinations[d.ID] = d
	return d, nil
}

func (m *MemoryDestinationStore) Delete(ctx context.Context, id string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if _, ok := m.destinations[id]; !ok {
		return destinations.ErrDestinationNotFound
	}
	delete(m.destinations, id)
	return nil
}

func (m *MemoryDestinationStore) List(ctx context.Context) ([]*destinations.Destination, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	var list []*destinations.Destination
	for _, d := range m.destinations {
		list = append(list, d)
	}
	return list, nil
}

// In-memory Route Store
type MemoryRouteStore struct {
	mu     sync.RWMutex
	routes map[string]*routing.Route
}

func NewMemoryRouteStore() *MemoryRouteStore {
	return &MemoryRouteStore{routes: make(map[string]*routing.Route)}
}

func (m *MemoryRouteStore) Create(ctx context.Context, r *routing.Route) (*routing.Route, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if r.ID == "" {
		r.ID = uuid.NewString()
	}
	now := time.Now().UTC()
	r.CreatedAt = now
	r.UpdatedAt = now
	m.routes[r.ID] = r
	return r, nil
}

func (m *MemoryRouteStore) GetByID(ctx context.Context, id string) (*routing.Route, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	r, ok := m.routes[id]
	if !ok {
		return nil, routing.ErrRouteNotFound
	}
	return r, nil
}

func (m *MemoryRouteStore) GetBySourceID(ctx context.Context, sourceID string) ([]*routing.Route, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	var list []*routing.Route
	for _, r := range m.routes {
		if r.SourceID == sourceID && r.Enabled {
			list = append(list, r)
		}
	}
	return list, nil
}

func (m *MemoryRouteStore) Update(ctx context.Context, r *routing.Route) (*routing.Route, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	existing, ok := m.routes[r.ID]
	if !ok {
		return nil, routing.ErrRouteNotFound
	}
	r.CreatedAt = existing.CreatedAt
	r.UpdatedAt = time.Now().UTC()
	m.routes[r.ID] = r
	return r, nil
}

func (m *MemoryRouteStore) Delete(ctx context.Context, id string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if _, ok := m.routes[id]; !ok {
		return routing.ErrRouteNotFound
	}
	delete(m.routes, id)
	return nil
}

func (m *MemoryRouteStore) List(ctx context.Context) ([]*routing.Route, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	var list []*routing.Route
	for _, r := range m.routes {
		list = append(list, r)
	}
	return list, nil
}

// In-memory Delivery Store
type MemoryDeliveryStore struct {
	mu    sync.Mutex
	jobs  map[string]*delivery.DeliveryJob
	byKey map[string]string // key: "eventID:destID" -> jobID (enforces unique event_id + destination_id)
}

func NewMemoryDeliveryStore() *MemoryDeliveryStore {
	return &MemoryDeliveryStore{
		jobs:  make(map[string]*delivery.DeliveryJob),
		byKey: make(map[string]string),
	}
}

func (m *MemoryDeliveryStore) CreateJob(ctx context.Context, job *delivery.DeliveryJob) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	key := job.EventID + ":" + job.DestinationID
	if _, exists := m.byKey[key]; exists {
		// Idempotency: ON CONFLICT DO NOTHING
		return nil
	}
	if job.ID == "" {
		job.ID = uuid.NewString()
	}
	now := time.Now().UTC()
	if job.CreatedAt.IsZero() {
		job.CreatedAt = now
	}
	if job.UpdatedAt.IsZero() {
		job.UpdatedAt = now
	}
	if job.NextAttemptAt.IsZero() {
		job.NextAttemptAt = now
	}
	m.jobs[job.ID] = job
	m.byKey[key] = job.ID
	return nil
}

func (m *MemoryDeliveryStore) CreateJobs(ctx context.Context, jobs []*delivery.DeliveryJob) error {
	for _, j := range jobs {
		if err := m.CreateJob(ctx, j); err != nil {
			return err
		}
	}
	return nil
}

func (m *MemoryDeliveryStore) GetByID(ctx context.Context, id string) (*delivery.DeliveryJob, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	job, ok := m.jobs[id]
	if !ok {
		return nil, delivery.ErrJobNotFound
	}
	return job, nil
}

func (m *MemoryDeliveryStore) List(ctx context.Context, filter delivery.ListFilter) ([]*delivery.DeliveryJob, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	var list []*delivery.DeliveryJob
	for _, j := range m.jobs {
		if filter.EventID != "" && j.EventID != filter.EventID {
			continue
		}
		if filter.DestinationID != "" && j.DestinationID != filter.DestinationID {
			continue
		}
		if filter.Status != "" && j.Status != filter.Status {
			continue
		}
		list = append(list, j)
	}
	return list, nil
}

func (m *MemoryDeliveryStore) ClaimNextJob(ctx context.Context) (*delivery.DeliveryJob, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	now := time.Now().UTC()
	var candidate *delivery.DeliveryJob

	for _, j := range m.jobs {
		if (j.Status == delivery.StatusPending || j.Status == delivery.StatusFailed) && !j.NextAttemptAt.After(now) {
			if candidate == nil || j.NextAttemptAt.Before(candidate.NextAttemptAt) {
				candidate = j
			}
		}
	}

	if candidate == nil {
		return nil, nil
	}

	candidate.Status = delivery.StatusProcessing
	candidate.Attempts++
	candidate.UpdatedAt = now
	return candidate, nil
}

func (m *MemoryDeliveryStore) MarkDelivered(ctx context.Context, id string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	j, ok := m.jobs[id]
	if !ok {
		return delivery.ErrJobNotFound
	}
	j.Status = delivery.StatusDelivered
	j.UpdatedAt = time.Now().UTC()
	return nil
}

func (m *MemoryDeliveryStore) MarkFailed(ctx context.Context, id string, errStr string, nextAttempt time.Time) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	j, ok := m.jobs[id]
	if !ok {
		return delivery.ErrJobNotFound
	}
	j.Status = delivery.StatusFailed
	j.LastError = &errStr
	j.NextAttemptAt = nextAttempt
	j.UpdatedAt = time.Now().UTC()
	return nil
}

func (m *MemoryDeliveryStore) MarkDeadLetter(ctx context.Context, id string, errStr string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	j, ok := m.jobs[id]
	if !ok {
		return delivery.ErrJobNotFound
	}
	j.Status = delivery.StatusDeadLetter
	j.LastError = &errStr
	j.UpdatedAt = time.Now().UTC()
	return nil
}

func (m *MemoryDeliveryStore) ResetForRetry(ctx context.Context, id string) (*delivery.DeliveryJob, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	j, ok := m.jobs[id]
	if !ok {
		return nil, delivery.ErrJobNotFound
	}
	j.Status = delivery.StatusPending
	j.Attempts = 0
	j.NextAttemptAt = time.Now().UTC()
	j.LastError = nil
	j.UpdatedAt = time.Now().UTC()
	return j, nil
}
