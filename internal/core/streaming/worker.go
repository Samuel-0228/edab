package streaming

import (
	"context"
	"fmt"
	"log/slog"
	"sync"
	"time"

	"kerob/internal/core/event"
	"kerob/internal/core/schema"
	"kerob/internal/core/workflow"
)

// WorkerConfig configures the stream subscriber worker.
type WorkerConfig struct {
	GroupID     string
	Topics      []string
	Concurrency int
}

// StreamWorker consumes messages from partitioned event streams, validates schemas,
// dispatches to the workflow engine with idempotency, and routes failures to DLQ.
type StreamWorker struct {
	adapter        StreamAdapter
	schemaRegistry *schema.Registry
	workflowEngine *workflow.Engine
	dlqStore       DeadLetterStore
	eventStore     event.Store
	cfg            WorkerConfig
	cancel         context.CancelFunc
	wg             sync.WaitGroup
}

// NewStreamWorker creates a new StreamWorker.
func NewStreamWorker(
	adapter StreamAdapter,
	schemaRegistry *schema.Registry,
	workflowEngine *workflow.Engine,
	dlqStore DeadLetterStore,
	eventStore event.Store,
	cfg WorkerConfig,
) *StreamWorker {
	if cfg.GroupID == "" {
		cfg.GroupID = "kerob-engine-workers"
	}
	if len(cfg.Topics) == 0 {
		cfg.Topics = []string{"events"}
	}
	if cfg.Concurrency <= 0 {
		cfg.Concurrency = 4
	}

	return &StreamWorker{
		adapter:        adapter,
		schemaRegistry: schemaRegistry,
		workflowEngine: workflowEngine,
		dlqStore:       dlqStore,
		eventStore:     eventStore,
		cfg:            cfg,
	}
}

// Start initiates topic subscription and background stream processing.
func (w *StreamWorker) Start(ctx context.Context) error {
	consumer, err := w.adapter.Consumer()
	if err != nil {
		return fmt.Errorf("failed to get stream consumer: %w", err)
	}

	workerCtx, cancel := context.WithCancel(ctx)
	w.cancel = cancel

	handler := func(msgCtx context.Context, record *Record) error {
		return w.processRecord(msgCtx, record)
	}

	slog.Info("starting stream worker subscription",
		slog.String("group_id", w.cfg.GroupID),
		slog.Any("topics", w.cfg.Topics),
	)

	return consumer.Subscribe(workerCtx, w.cfg.GroupID, w.cfg.Topics, handler)
}

// Stop terminates the worker.
func (w *StreamWorker) Stop() {
	if w.cancel != nil {
		w.cancel()
	}
	w.wg.Wait()
}

func (w *StreamWorker) processRecord(ctx context.Context, record *Record) error {
	if record == nil || record.Event == nil {
		return nil
	}

	ev := record.Event

	// 1. Schema Validation Pre-check
	if w.schemaRegistry != nil {
		valErrs, err := w.schemaRegistry.ValidateEvent(ctx, ev.TenantID, ev.EventType, ev.Payload)
		if err != nil || len(valErrs) > 0 {
			var errReason string
			if err != nil {
				errReason = fmt.Sprintf("schema error: %s", err.Error())
			} else {
				errReason = fmt.Sprintf("schema violations: %v", valErrs)
			}

			slog.Warn("streaming event rejected by schema validation",
				slog.String("event_id", ev.ID),
				slog.String("event_type", ev.EventType),
				slog.String("reason", errReason),
			)

			// Route to DLQ so poison pill does not halt stream consumption
			if w.dlqStore != nil {
				_, _ = w.dlqStore.Store(ctx, &DeadLetterEvent{
					TenantID:    ev.TenantID,
					Topic:       record.Topic,
					Partition:   record.Partition,
					Offset:      record.Offset,
					EventID:     ev.ID,
					OrderingKey: record.OrderingKey,
					EventType:   ev.EventType,
					Payload:     ev.Payload,
					ErrorReason: errReason,
					CreatedAt:   time.Now().UTC(),
				})
			}
			return nil // Return nil so offset is committed and stream advances
		}
	}

	// 2. Persist event with idempotency
	if w.eventStore != nil {
		_, _, err := w.eventStore.Create(ctx, ev)
		if err != nil {
			slog.Error("failed to persist stream event",
				slog.String("event_id", ev.ID),
				slog.String("error", err.Error()),
			)
			return err
		}
	}

	// 3. Dispatch to Workflow Engine
	if w.workflowEngine != nil {
		_, err := w.workflowEngine.ProcessEvent(ctx, ev)
		if err != nil {
			slog.Error("workflow engine error on stream event",
				slog.String("event_id", ev.ID),
				slog.String("error", err.Error()),
			)

			// Record failure in Dead Letter Queue
			if w.dlqStore != nil {
				_, _ = w.dlqStore.Store(ctx, &DeadLetterEvent{
					TenantID:    ev.TenantID,
					Topic:       record.Topic,
					Partition:   record.Partition,
					Offset:      record.Offset,
					EventID:     ev.ID,
					OrderingKey: record.OrderingKey,
					EventType:   ev.EventType,
					Payload:     ev.Payload,
					ErrorReason: err.Error(),
					CreatedAt:   time.Now().UTC(),
				})
			}
			return nil
		}
	}

	return nil
}
