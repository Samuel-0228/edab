package delivery

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"math"
	"sync"
	"time"

	"kerob/internal/adapters/destinations"
	"kerob/internal/core/event"
)

// WorkerConfig defines parameters for background job execution and retries.
type WorkerConfig struct {
	PollInterval   time.Duration
	Concurrency    int
	InitialBackoff time.Duration
	MaxBackoff     time.Duration
}

// Worker polls and processes pending delivery jobs concurrently.
type Worker struct {
	jobStore     Store
	eventStore   event.Store
	destStore    destinations.Store
	destRegistry *destinations.Registry
	cfg          WorkerConfig
	wg           sync.WaitGroup
	stopChan     chan struct{}
}

// NewWorker initializes a background delivery worker.
func NewWorker(
	jobStore Store,
	eventStore event.Store,
	destStore destinations.Store,
	destRegistry *destinations.Registry,
	cfg WorkerConfig,
) *Worker {
	if cfg.PollInterval <= 0 {
		cfg.PollInterval = 1 * time.Second
	}
	if cfg.Concurrency <= 0 {
		cfg.Concurrency = 2
	}
	if cfg.InitialBackoff <= 0 {
		cfg.InitialBackoff = 2 * time.Second
	}
	if cfg.MaxBackoff <= 0 {
		cfg.MaxBackoff = 1 * time.Minute
	}

	return &Worker{
		jobStore:     jobStore,
		eventStore:   eventStore,
		destStore:    destStore,
		destRegistry: destRegistry,
		cfg:          cfg,
		stopChan:     make(chan struct{}),
	}
}

// Start launches worker goroutines that process delivery jobs until ctx is cancelled.
func (w *Worker) Start(ctx context.Context) {
	slog.Info("delivery worker started",
		slog.Int("concurrency", w.cfg.Concurrency),
		slog.Duration("poll_interval", w.cfg.PollInterval),
	)

	for i := 0; i < w.cfg.Concurrency; i++ {
		w.wg.Add(1)
		go w.workerLoop(ctx, i)
	}
}

// Stop gracefully waits for in-flight jobs to complete.
func (w *Worker) Stop() {
	close(w.stopChan)
	w.wg.Wait()
	slog.Info("delivery worker stopped")
}

func (w *Worker) workerLoop(ctx context.Context, workerID int) {
	defer w.wg.Done()

	ticker := time.NewTicker(w.cfg.PollInterval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-w.stopChan:
			return
		default:
			claimed, err := w.processNextJob(ctx, workerID)
			if err != nil {
				slog.Error("error processing delivery job",
					slog.Int("worker_id", workerID),
					slog.String("error", err.Error()),
				)
			}
			if !claimed {
				// No jobs ready; wait for next poll tick or cancellation
				select {
				case <-ctx.Done():
					return
				case <-w.stopChan:
					return
				case <-ticker.C:
				}
			}
		}
	}
}

// ProcessNextJob attempts to claim and execute a single job. Returns true if a job was claimed.
func (w *Worker) ProcessNextJob(ctx context.Context) (bool, error) {
	return w.processNextJob(ctx, 0)
}

func (w *Worker) processNextJob(ctx context.Context, workerID int) (bool, error) {
	job, err := w.jobStore.ClaimNextJob(ctx)
	if err != nil {
		return false, fmt.Errorf("failed to claim job: %w", err)
	}
	if job == nil {
		return false, nil // No jobs ready
	}

	slog.Info("delivery started",
		slog.Int("worker_id", workerID),
		slog.String("job_id", job.ID),
		slog.String("event_id", job.EventID),
		slog.String("destination_id", job.DestinationID),
		slog.Int("attempt", job.Attempts),
	)

	// Fetch event
	ev, err := w.eventStore.GetByID(ctx, job.EventID)
	if err != nil {
		w.handleDeliveryFailure(ctx, job, fmt.Sprintf("event not found: %v", err))
		return true, fmt.Errorf("job %s failed loading event: %w", job.ID, err)
	}

	// Fetch destination
	dest, err := w.destStore.GetByID(ctx, job.DestinationID)
	if err != nil {
		w.handleDeliveryFailure(ctx, job, fmt.Sprintf("destination not found: %v", err))
		return true, fmt.Errorf("job %s failed loading destination: %w", job.ID, err)
	}

	if !dest.Enabled {
		w.handleDeliveryFailure(ctx, job, "destination is disabled")
		return true, nil
	}

	// Obtain or instantiate adapter
	adapter, err := w.destRegistry.Create(dest.AdapterType)
	if err != nil {
		w.handleDeliveryFailure(ctx, job, fmt.Sprintf("unregistered adapter %s: %v", dest.AdapterType, err))
		return true, fmt.Errorf("unregistered adapter %s: %w", dest.AdapterType, err)
	}

	if err := adapter.Connect(ctx, dest.Config); err != nil {
		w.handleDeliveryFailure(ctx, job, fmt.Sprintf("failed to connect adapter: %v", err))
		return true, fmt.Errorf("failed to connect adapter: %w", err)
	}

	// Deliver event
	var result json.RawMessage
	var deliverErr error

	if len(job.Result) > 0 {
		slog.Info("delivery job already has destination result, skipping delivery call",
			slog.String("job_id", job.ID),
			slog.String("event_id", job.EventID),
			slog.String("destination_id", job.DestinationID),
		)
		result = job.Result
	} else if reporter, ok := adapter.(destinations.ResultReporter); ok {
		result, deliverErr = reporter.DeliverWithResult(ctx, ev)
	} else {
		deliverErr = adapter.Deliver(ctx, ev)
	}

	if deliverErr != nil {
		w.handleDeliveryFailure(ctx, job, deliverErr.Error())
		return true, nil
	}

	// Delivery succeeded
	var markErr error
	if len(result) > 0 {
		markErr = w.jobStore.MarkDeliveredWithResult(ctx, job.ID, result)
	} else {
		markErr = w.jobStore.MarkDelivered(ctx, job.ID)
	}
	if markErr != nil {
		slog.Error("failed to mark delivery job as delivered",
			slog.String("job_id", job.ID),
			slog.String("error", markErr.Error()),
		)
		return true, markErr
	}

	slog.Info("delivery succeeded",
		slog.String("job_id", job.ID),
		slog.String("event_id", job.EventID),
		slog.String("destination_id", job.DestinationID),
		slog.Int("attempt", job.Attempts),
	)
	return true, nil
}

func (w *Worker) handleDeliveryFailure(ctx context.Context, job *DeliveryJob, errStr string) {
	slog.Warn("delivery failed",
		slog.String("job_id", job.ID),
		slog.String("event_id", job.EventID),
		slog.String("destination_id", job.DestinationID),
		slog.Int("attempt", job.Attempts),
		slog.Int("max_attempts", job.MaxAttempts),
		slog.String("error", errStr),
	)

	if job.Attempts >= job.MaxAttempts {
		if err := w.jobStore.MarkDeadLetter(ctx, job.ID, errStr); err != nil {
			slog.Error("failed to mark job dead_letter", slog.String("job_id", job.ID), slog.String("error", err.Error()))
			return
		}
		slog.Error("delivery job marked as dead_letter",
			slog.String("job_id", job.ID),
			slog.String("event_id", job.EventID),
			slog.String("destination_id", job.DestinationID),
			slog.Int("attempts", job.Attempts),
		)
		return
	}

	backoff := w.CalculateBackoff(job.Attempts)
	nextAttempt := time.Now().UTC().Add(backoff)

	if err := w.jobStore.MarkFailed(ctx, job.ID, errStr, nextAttempt); err != nil {
		slog.Error("failed to mark job failed", slog.String("job_id", job.ID), slog.String("error", err.Error()))
		return
	}

	slog.Info("retry scheduled",
		slog.String("job_id", job.ID),
		slog.Time("next_attempt_at", nextAttempt),
		slog.Duration("backoff", backoff),
	)
}

// CalculateBackoff computes exponential backoff for a given attempt index: initialBackoff * 2^(attempt-1).
func (w *Worker) CalculateBackoff(attempt int) time.Duration {
	if attempt <= 1 {
		return w.cfg.InitialBackoff
	}
	multiplier := math.Pow(2, float64(attempt-1))
	backoff := time.Duration(float64(w.cfg.InitialBackoff) * multiplier)
	if backoff > w.cfg.MaxBackoff {
		return w.cfg.MaxBackoff
	}
	return backoff
}
