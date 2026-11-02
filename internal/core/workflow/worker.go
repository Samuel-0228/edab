package workflow

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"sync"
	"time"

	"github.com/google/uuid"

	"kerob/internal/core/event"
	"kerob/internal/observability"
)

// WorkerConfig defines parameters for background workflow execution, leases, and concurrency.
type WorkerConfig struct {
	PollInterval      time.Duration
	Concurrency       int
	LeaseDuration     time.Duration
	HeartbeatInterval time.Duration
	WorkerID          string
}

// Worker polls, claims, and executes pending, delayed, and retrying workflow executions.
type Worker struct {
	engine     *Engine
	execStore  ExecutionStore
	wfStore    Store
	eventStore event.Store
	cfg        WorkerConfig
	wg         sync.WaitGroup
	stopChan   chan struct{}
}

// NewWorker initializes a distributed workflow execution worker.
func NewWorker(
	engine *Engine,
	execStore ExecutionStore,
	wfStore Store,
	eventStore event.Store,
	cfg WorkerConfig,
) *Worker {
	if cfg.PollInterval <= 0 {
		cfg.PollInterval = 1 * time.Second
	}
	if cfg.Concurrency <= 0 {
		cfg.Concurrency = 2
	}
	if cfg.LeaseDuration <= 0 {
		cfg.LeaseDuration = 30 * time.Second
	}
	if cfg.HeartbeatInterval <= 0 {
		cfg.HeartbeatInterval = cfg.LeaseDuration / 3
	}
	if cfg.WorkerID == "" {
		hostname, _ := os.Hostname()
		if hostname == "" {
			hostname = "wf-worker"
		}
		cfg.WorkerID = fmt.Sprintf("%s-%s", hostname, uuid.NewString()[:8])
	}

	return &Worker{
		engine:     engine,
		execStore:  execStore,
		wfStore:    wfStore,
		eventStore: eventStore,
		cfg:        cfg,
		stopChan:   make(chan struct{}),
	}
}

// Start launches worker goroutines that process workflow executions until ctx is cancelled.
func (w *Worker) Start(ctx context.Context) {
	slog.Info("workflow execution worker started",
		slog.String("worker_id", w.cfg.WorkerID),
		slog.Int("concurrency", w.cfg.Concurrency),
		slog.Duration("poll_interval", w.cfg.PollInterval),
		slog.Duration("lease_duration", w.cfg.LeaseDuration),
		slog.Duration("heartbeat_interval", w.cfg.HeartbeatInterval),
	)

	for i := 0; i < w.cfg.Concurrency; i++ {
		w.wg.Add(1)
		go w.workerLoop(ctx, i)
	}
}

// Stop gracefully signals workers to stop and waits for in-flight executions to finish.
func (w *Worker) Stop() {
	close(w.stopChan)
	w.wg.Wait()
	slog.Info("workflow execution worker stopped", slog.String("worker_id", w.cfg.WorkerID))
}

func (w *Worker) workerLoop(ctx context.Context, goroutineID int) {
	defer w.wg.Done()
	workerIdentifier := fmt.Sprintf("%s-%d", w.cfg.WorkerID, goroutineID)

	ticker := time.NewTicker(w.cfg.PollInterval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-w.stopChan:
			return
		default:
			claimed, err := w.processNextExecution(ctx, workerIdentifier)
			if err != nil {
				slog.Error("error processing workflow execution",
					slog.String("worker_id", workerIdentifier),
					slog.String("error", err.Error()),
				)
			}
			if !claimed {
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

// ProcessNextExecution attempts to claim and execute a single execution. Returns true if an execution was claimed.
func (w *Worker) ProcessNextExecution(ctx context.Context) (bool, error) {
	return w.processNextExecution(ctx, fmt.Sprintf("%s-0", w.cfg.WorkerID))
}

func (w *Worker) processNextExecution(ctx context.Context, workerIdentifier string) (bool, error) {
	exec, err := w.execStore.ClaimPendingWithLease(ctx, workerIdentifier, w.cfg.LeaseDuration)
	if err != nil {
		return false, fmt.Errorf("failed to claim pending execution: %w", err)
	}
	if exec == nil {
		return false, nil // No executions ready
	}

	observability.RecordLeaseAcquired()
	slog.Info("workflow execution lease claimed",
		slog.String("worker_id", workerIdentifier),
		slog.String("execution_id", exec.ID),
		slog.String("workflow_id", exec.WorkflowID),
		slog.String("event_id", exec.EventID),
		slog.Int("attempt", exec.Attempts),
		slog.Duration("lease_duration", w.cfg.LeaseDuration),
	)

	// Lease renewal heartbeat goroutine
	heartbeatCtx, cancelHeartbeat := context.WithCancel(ctx)
	defer cancelHeartbeat()

	heartbeatDone := make(chan struct{})
	defer close(heartbeatDone)

	go func() {
		ticker := time.NewTicker(w.cfg.HeartbeatInterval)
		defer ticker.Stop()
		for {
			select {
			case <-heartbeatCtx.Done():
				return
			case <-heartbeatDone:
				return
			case <-ticker.C:
				if err := w.execStore.RenewLease(heartbeatCtx, exec.ID, workerIdentifier, w.cfg.LeaseDuration); err != nil {
					slog.Warn("failed to renew workflow execution lease",
						slog.String("execution_id", exec.ID),
						slog.String("worker_id", workerIdentifier),
						slog.String("error", err.Error()),
					)
					observability.RecordLeaseLoss()
					return
				}
				observability.RecordLeaseRenewed()
			}
		}
	}()

	// Fetch workflow
	wf, err := w.wfStore.GetByID(ctx, exec.WorkflowID)
	if err != nil {
		errStr := fmt.Sprintf("workflow not found: %v", err)
		_ = w.execStore.UpdateStatus(ctx, exec.ID, ExecutionFailed, &errStr, 0)
		return true, fmt.Errorf("execution %s failed loading workflow %s: %w", exec.ID, exec.WorkflowID, err)
	}
	if !wf.Enabled {
		errStr := "workflow is disabled"
		_ = w.execStore.UpdateStatus(ctx, exec.ID, ExecutionFailed, &errStr, 0)
		return true, nil
	}

	// Fetch event
	ev, err := w.eventStore.GetByID(ctx, exec.EventID)
	if err != nil {
		errStr := fmt.Sprintf("event not found: %v", err)
		_ = w.execStore.UpdateStatus(ctx, exec.ID, ExecutionFailed, &errStr, 0)
		return true, fmt.Errorf("execution %s failed loading event %s: %w", exec.ID, exec.EventID, err)
	}

	// Execute workflow
	_, execErr := w.engine.ExecuteWorkflow(ctx, wf, ev, exec)
	if execErr != nil {
		slog.Warn("workflow execution finished with error",
			slog.String("execution_id", exec.ID),
			slog.String("workflow_id", wf.ID),
			slog.String("error", execErr.Error()),
		)
	}
	return true, nil
}
