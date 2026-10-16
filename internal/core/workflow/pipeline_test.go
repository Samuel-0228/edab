package workflow

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"
	"time"

	"automation-engine/internal/adapters/destinations"
	"automation-engine/internal/adapters/sources"
	"automation-engine/internal/core/delivery"
	"automation-engine/internal/core/event"
	"automation-engine/internal/core/routing"
)

// Mock Stores for Pipeline Testing

type mockSourceStore struct {
	sources map[string]*sources.Source
}

func (m *mockSourceStore) Create(ctx context.Context, s *sources.Source) (*sources.Source, error) {
	m.sources[s.ID] = s
	return s, nil
}
func (m *mockSourceStore) GetByID(ctx context.Context, id string) (*sources.Source, error) {
	s, ok := m.sources[id]
	if !ok {
		return nil, sources.ErrSourceNotFound
	}
	return s, nil
}
func (m *mockSourceStore) Update(ctx context.Context, s *sources.Source) (*sources.Source, error) {
	m.sources[s.ID] = s
	return s, nil
}
func (m *mockSourceStore) Delete(ctx context.Context, id string) error {
	delete(m.sources, id)
	return nil
}
func (m *mockSourceStore) List(ctx context.Context) ([]*sources.Source, error) {
	res := make([]*sources.Source, 0, len(m.sources))
	for _, s := range m.sources {
		res = append(res, s)
	}
	return res, nil
}

type mockEventStore struct {
	events map[string]*event.Event
}

func (m *mockEventStore) Create(ctx context.Context, e *event.Event) (*event.Event, bool, error) {
	key := e.SourceID + ":" + e.ExternalEventID
	if existing, ok := m.events[key]; ok {
		return existing, false, nil
	}
	m.events[key] = e
	return e, true, nil
}
func (m *mockEventStore) GetByID(ctx context.Context, id string) (*event.Event, error) {
	return nil, nil
}
func (m *mockEventStore) GetByExternalID(ctx context.Context, sourceID, externalID string) (*event.Event, error) {
	return nil, nil
}
func (m *mockEventStore) List(ctx context.Context, filter event.ListFilter) ([]*event.Event, error) {
	return nil, nil
}

type mockRoutingEngine struct {
	routes []*routing.Route
}

func (m *mockRoutingEngine) ResolveRoutes(ctx context.Context, ev *event.Event) ([]*routing.Route, error) {
	var matched []*routing.Route
	for _, r := range m.routes {
		if r.SourceID == ev.SourceID && r.Matches(ev) {
			matched = append(matched, r)
		}
	}
	return matched, nil
}

type mockDeliveryStore struct {
	createdJobs []*delivery.DeliveryJob
}

func (m *mockDeliveryStore) CreateJob(ctx context.Context, job *delivery.DeliveryJob) error {
	m.createdJobs = append(m.createdJobs, job)
	return nil
}
func (m *mockDeliveryStore) CreateJobs(ctx context.Context, jobs []*delivery.DeliveryJob) error {
	m.createdJobs = append(m.createdJobs, jobs...)
	return nil
}
func (m *mockDeliveryStore) GetByID(ctx context.Context, id string) (*delivery.DeliveryJob, error) {
	return nil, nil
}
func (m *mockDeliveryStore) List(ctx context.Context, filter delivery.ListFilter) ([]*delivery.DeliveryJob, error) {
	return nil, nil
}
func (m *mockDeliveryStore) ClaimNextJob(ctx context.Context) (*delivery.DeliveryJob, error) {
	return nil, nil
}
func (m *mockDeliveryStore) MarkDelivered(ctx context.Context, id string) error {
	return nil
}
func (m *mockDeliveryStore) MarkDeliveredWithResult(ctx context.Context, id string, result json.RawMessage) error {
	return nil
}
func (m *mockDeliveryStore) Stats(ctx context.Context) (delivery.Stats, error) {
	return delivery.Stats{}, nil
}
func (m *mockDeliveryStore) MarkFailed(ctx context.Context, id string, errStr string, nextAttempt time.Time) error {
	return nil
}
func (m *mockDeliveryStore) MarkDeadLetter(ctx context.Context, id string, errStr string) error {
	return nil
}
func (m *mockDeliveryStore) ResetForRetry(ctx context.Context, id string) (*delivery.DeliveryJob, error) {
	return nil, nil
}

type mockDestStore struct {
	destinations map[string]*destinations.Destination
}

func (m *mockDestStore) Create(ctx context.Context, d *destinations.Destination) (*destinations.Destination, error) {
	m.destinations[d.ID] = d
	return d, nil
}
func (m *mockDestStore) GetByID(ctx context.Context, id string) (*destinations.Destination, error) {
	d, ok := m.destinations[id]
	if !ok {
		return nil, destinations.ErrDestinationNotFound
	}
	return d, nil
}
func (m *mockDestStore) Update(ctx context.Context, d *destinations.Destination) (*destinations.Destination, error) {
	m.destinations[d.ID] = d
	return d, nil
}
func (m *mockDestStore) Delete(ctx context.Context, id string) error {
	delete(m.destinations, id)
	return nil
}
func (m *mockDestStore) List(ctx context.Context) ([]*destinations.Destination, error) {
	res := make([]*destinations.Destination, 0, len(m.destinations))
	for _, d := range m.destinations {
		res = append(res, d)
	}
	return res, nil
}

func TestPipelineIngestValidation(t *testing.T) {
	ctx := context.Background()

	srcStore := &mockSourceStore{sources: make(map[string]*sources.Source)}
	srcReg := sources.NewRegistry()
	evStore := &mockEventStore{events: make(map[string]*event.Event)}
	rEngine := &mockRoutingEngine{}
	delStore := &mockDeliveryStore{}
	dstStore := &mockDestStore{destinations: make(map[string]*destinations.Destination)}

	pipeline := NewPipeline(srcStore, srcReg, evStore, rEngine, delStore, dstStore, 3)

	// 1. Source not found
	_, err := pipeline.IngestRaw(ctx, "nonexistent-src", []byte(`{}`))
	if err == nil || !errors.Is(err, sources.ErrSourceNotFound) {
		t.Fatalf("expected ErrSourceNotFound, got %v", err)
	}

	// 2. Source disabled
	srcStore.sources["disabled-src"] = &sources.Source{
		ID:          "disabled-src",
		Name:        "Disabled",
		AdapterType: "mock",
		Enabled:     false,
	}
	_, err = pipeline.IngestRaw(ctx, "disabled-src", []byte(`{}`))
	if err == nil || !strings.Contains(err.Error(), "disabled") {
		t.Fatalf("expected disabled error, got %v", err)
	}

	// 3. Adapter not registered
	srcStore.sources["active-src"] = &sources.Source{
		ID:          "active-src",
		Name:        "Active",
		AdapterType: "unregistered",
		Enabled:     true,
	}
	_, err = pipeline.IngestRaw(ctx, "active-src", []byte(`{}`))
	if err == nil || !errors.Is(err, sources.ErrAdapterNotFound) {
		t.Fatalf("expected ErrAdapterNotFound, got %v", err)
	}
}

func TestPipelineIngestAndProcess(t *testing.T) {
	ctx := context.Background()

	srcStore := &mockSourceStore{sources: make(map[string]*sources.Source)}
	srcReg := sources.NewRegistry()
	srcReg.Register("mock", func() sources.SourceAdapter {
		return sources.NewMockSourceAdapter()
	})

	evStore := &mockEventStore{events: make(map[string]*event.Event)}
	delStore := &mockDeliveryStore{}
	dstStore := &mockDestStore{destinations: make(map[string]*destinations.Destination)}

	// Setup destination
	dstStore.destinations["dest-1"] = &destinations.Destination{
		ID:      "dest-1",
		Name:    "Target 1",
		Enabled: true,
	}

	// Setup route
	rEngine := &mockRoutingEngine{
		routes: []*routing.Route{
			{
				ID:            "r-1",
				SourceID:      "src-ok",
				DestinationID: "dest-1",
				Enabled:       true,
			},
		},
	}

	pipeline := NewPipeline(srcStore, srcReg, evStore, rEngine, delStore, dstStore, 3)

	srcStore.sources["src-ok"] = &sources.Source{
		ID:          "src-ok",
		Name:        "OK Source",
		AdapterType: "mock",
		Config:      json.RawMessage(`{}`),
		Enabled:     true,
	}

	rawData := []byte(`{"external_id":"evt-100","type":"order.placed","payload":{"total":99}}`)
	ev, err := pipeline.IngestRaw(ctx, "src-ok", rawData)
	if err != nil {
		t.Fatalf("ingest raw failed: %v", err)
	}

	if ev.ExternalEventID != "evt-100" {
		t.Errorf("expected evt-100, got %s", ev.ExternalEventID)
	}
	if len(delStore.createdJobs) != 1 {
		t.Fatalf("expected 1 delivery job created, got %d", len(delStore.createdJobs))
	}

	// Duplicate Ingest: must NOT create new delivery jobs
	_, err = pipeline.IngestRaw(ctx, "src-ok", rawData)
	if err != nil {
		t.Fatalf("duplicate ingest failed: %v", err)
	}
	if len(delStore.createdJobs) != 1 {
		t.Fatalf("duplicate event must not create additional delivery jobs, got %d", len(delStore.createdJobs))
	}
}
