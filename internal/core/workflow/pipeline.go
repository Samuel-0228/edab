package workflow

import (
	"context"
	"fmt"
	"log/slog"

	"kerob/internal/adapters/destinations"
	"kerob/internal/adapters/sources"
	"kerob/internal/core/delivery"
	"kerob/internal/core/event"
	"kerob/internal/core/routing"
)

// Pipeline orchestrates the ingestion, normalization, routing, and delivery job creation.
// Following: SOURCE → INGEST → NORMALIZE → ROUTE → DELIVER → RECORD.
type Pipeline struct {
	sourceStore       sources.Store
	sourceRegistry    *sources.Registry
	eventStore        event.Store
	routingEngine     routing.Engine
	deliveryStore     delivery.Store
	destStore         destinations.Store
	defaultMaxRetries int
}

// NewPipeline creates a new workflow Pipeline.
func NewPipeline(
	sourceStore sources.Store,
	sourceRegistry *sources.Registry,
	eventStore event.Store,
	routingEngine routing.Engine,
	deliveryStore delivery.Store,
	destStore destinations.Store,
	defaultMaxRetries int,
) *Pipeline {
	if defaultMaxRetries <= 0 {
		defaultMaxRetries = 3
	}
	return &Pipeline{
		sourceStore:       sourceStore,
		sourceRegistry:    sourceRegistry,
		eventStore:        eventStore,
		routingEngine:     routingEngine,
		deliveryStore:     deliveryStore,
		destStore:         destStore,
		defaultMaxRetries: defaultMaxRetries,
	}
}

// IngestRaw receives raw external data for a given source, connects the source adapter,
// normalizes the data into a generic Event, and processes it through the pipeline.
func (p *Pipeline) IngestRaw(ctx context.Context, sourceID string, rawData []byte) (*event.Event, error) {
	slog.Info("event received",
		slog.String("source_id", sourceID),
		slog.Int("bytes", len(rawData)),
	)

	// 1. Fetch source configuration
	src, err := p.sourceStore.GetByID(ctx, sourceID)
	if err != nil {
		return nil, fmt.Errorf("source %s not found: %w", sourceID, err)
	}

	if !src.Enabled {
		return nil, fmt.Errorf("source %s is disabled", sourceID)
	}

	// 2. Instantiate source adapter
	adapter, err := p.sourceRegistry.Create(src.AdapterType)
	if err != nil {
		return nil, fmt.Errorf("adapter error for source %s: %w", sourceID, err)
	}

	if err := adapter.Connect(ctx, src.Config); err != nil {
		return nil, fmt.Errorf("failed to connect source adapter: %w", err)
	}

	// 3. Normalize external data into generic Event
	ev, err := adapter.Normalize(ctx, sourceID, rawData)
	if err != nil {
		return nil, fmt.Errorf("normalization failed for source %s: %w", sourceID, err)
	}

	// 4. Continue pipeline processing
	return p.ProcessEvent(ctx, ev)
}

// ProcessEvent persists the event (enforcing idempotency), resolves active routes,
// and enqueues delivery jobs for the background worker.
func (p *Pipeline) ProcessEvent(ctx context.Context, ev *event.Event) (*event.Event, error) {
	// Persist event (idempotency check by source_id + external_event_id)
	persisted, inserted, err := p.eventStore.Create(ctx, ev)
	if err != nil {
		return nil, fmt.Errorf("failed to persist event: %w", err)
	}

	if !inserted {
		slog.Info("duplicate event ignored",
			slog.String("source_id", ev.SourceID),
			slog.String("external_event_id", ev.ExternalEventID),
			slog.String("existing_event_id", persisted.ID),
		)
		return persisted, nil
	}

	slog.Info("event persisted",
		slog.String("event_id", persisted.ID),
		slog.String("source_id", persisted.SourceID),
		slog.String("external_event_id", persisted.ExternalEventID),
		slog.String("event_type", persisted.EventType),
	)

	// Resolve active matching routes
	routes, err := p.routingEngine.ResolveRoutes(ctx, persisted)
	if err != nil {
		return nil, fmt.Errorf("failed to resolve routes for event %s: %w", persisted.ID, err)
	}

	if len(routes) == 0 {
		slog.Info("no routes matched for event",
			slog.String("event_id", persisted.ID),
			slog.String("source_id", persisted.SourceID),
		)
		return persisted, nil
	}

	// Create delivery jobs for all matched routes
	var jobs []*delivery.DeliveryJob
	for _, r := range routes {
		slog.Info("route matched",
			slog.String("event_id", persisted.ID),
			slog.String("route_id", r.ID),
			slog.String("destination_id", r.DestinationID),
		)
		job := delivery.NewJob(persisted.ID, r.DestinationID, p.defaultMaxRetries)
		jobs = append(jobs, job)
	}

	if err := p.deliveryStore.CreateJobs(ctx, jobs); err != nil {
		return nil, fmt.Errorf("failed to enqueue delivery jobs for event %s: %w", persisted.ID, err)
	}

	for _, j := range jobs {
		slog.Info("delivery created",
			slog.String("job_id", j.ID),
			slog.String("event_id", persisted.ID),
			slog.String("destination_id", j.DestinationID),
		)
	}

	return persisted, nil
}
