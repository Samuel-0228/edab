package streaming

import (
	"context"
	"fmt"
	"time"

	"github.com/google/uuid"
	"kerob/internal/core/event"
	"kerob/internal/core/workflow"
)

// ReplayResult details the outcome of an event stream replay operation.
type ReplayResult struct {
	TotalMatched int      `json:"total_matched"`
	Replayed     int      `json:"replayed"`
	EventIDs     []string `json:"event_ids"`
	ExecutionIDs []string `json:"execution_ids"`
}

// ReplayEngine handles targeted reprocessing of streaming events and dead-letter queue records.
type ReplayEngine struct {
	eventStore     event.Store
	workflowEngine *workflow.Engine
	dlqStore       DeadLetterStore
	producer       Producer
}

// NewReplayEngine creates a new stream ReplayEngine.
func NewReplayEngine(
	eventStore event.Store,
	workflowEngine *workflow.Engine,
	dlqStore DeadLetterStore,
	producer Producer,
) *ReplayEngine {
	return &ReplayEngine{
		eventStore:     eventStore,
		workflowEngine: workflowEngine,
		dlqStore:       dlqStore,
		producer:       producer,
	}
}

// ReplayEvents replays historical events matching the filter through the workflow engine.
func (r *ReplayEngine) ReplayEvents(ctx context.Context, filter ReplayFilter) (*ReplayResult, error) {
	// Query events from event store matching criteria
	listFilter := event.ListFilter{
		SourceID:  filter.SourceID,
		EventType: filter.EventType,
		Limit:     filter.Limit,
	}
	if listFilter.Limit <= 0 {
		listFilter.Limit = 100
	}

	events, err := r.eventStore.List(ctx, listFilter)
	if err != nil {
		return nil, fmt.Errorf("failed to list historical events for replay: %w", err)
	}

	res := &ReplayResult{
		EventIDs:     make([]string, 0),
		ExecutionIDs: make([]string, 0),
	}

	for _, ev := range events {
		// Filter by Tenant
		if filter.TenantID != "" && ev.TenantID != "" && ev.TenantID != filter.TenantID {
			continue
		}

		// Filter by Time Range
		if !filter.FromTime.IsZero() && ev.CreatedAt.Before(filter.FromTime) {
			continue
		}
		if !filter.ToTime.IsZero() && ev.CreatedAt.After(filter.ToTime) {
			continue
		}

		res.TotalMatched++

		targetEv := ev
		if filter.BypassIdempotency {
			cloned := *ev
			cloned.ID = uuid.NewString()
			cloned.ExternalEventID = fmt.Sprintf("%s-stream-replay-%d", ev.ExternalEventID, time.Now().UnixNano())
			cloned.CreatedAt = time.Now().UTC()
			targetEv = &cloned
		}

		// Execute workflows
		if r.workflowEngine != nil {
			execs, err := r.workflowEngine.ProcessEvent(ctx, targetEv)
			if err == nil {
				res.Replayed++
				res.EventIDs = append(res.EventIDs, targetEv.ID)
				for _, ex := range execs {
					res.ExecutionIDs = append(res.ExecutionIDs, ex.ID)
				}
			}
		}
	}

	return res, nil
}

// ReplayDeadLetterEvent recovers an event from DLQ and re-executes it through the engine.
func (r *ReplayEngine) ReplayDeadLetterEvent(ctx context.Context, dlqID string) (*event.Event, []*workflow.Execution, error) {
	if r.dlqStore == nil {
		return nil, nil, fmt.Errorf("dead-letter store not configured")
	}

	dlq, err := r.dlqStore.GetByID(ctx, dlqID)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to get dead-letter event: %w", err)
	}

	// Reconstruct normalized Event
	ev, err := event.New(
		"stream.dlq",
		fmt.Sprintf("dlq-retry-%s-%d", dlq.ID, time.Now().UnixNano()),
		dlq.EventType,
		dlq.Payload,
		map[string]any{
			"replayed_from_dlq": true,
			"original_dlq_id":   dlq.ID,
			"original_topic":    dlq.Topic,
			"original_offset":   dlq.Offset,
		},
	)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to rebuild event from dead-letter record: %w", err)
	}
	ev.TenantID = dlq.TenantID

	var execs []*workflow.Execution
	if r.workflowEngine != nil {
		execs, err = r.workflowEngine.ProcessEvent(ctx, ev)
		if err != nil {
			return nil, nil, fmt.Errorf("failed to process replayed DLQ event: %w", err)
		}
	}

	// Mark DLQ item resolved
	_ = r.dlqStore.MarkResolved(ctx, dlq.ID)

	return ev, execs, nil
}
