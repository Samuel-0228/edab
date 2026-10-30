package workflow

import (
	"context"
	"fmt"
	"log/slog"
	"math"
	"time"

	"kerob/internal/core/conditions"
	"kerob/internal/core/credentials"
	"kerob/internal/core/errtracker"
	"kerob/internal/core/event"
	"kerob/internal/core/transformations"
	"kerob/internal/observability"
)

// EngineConfig holds tuning parameters for the workflow engine.
type EngineConfig struct {
	BaseBackoff time.Duration
	MaxBackoff  time.Duration
}

// Engine coordinates event ingestion, condition evaluation, transformation, and action execution.
type Engine struct {
	workflows  Store
	executions ExecutionStore
	deadLetter DeadLetterStore
	creds      credentials.Store
	actions    *ActionRegistry
	cfg        EngineConfig
	errTracker errtracker.Provider
}

// SetErrorTracker sets the error tracker provider for the workflow engine.
func (e *Engine) SetErrorTracker(tracker errtracker.Provider) {
	e.errTracker = tracker
}

func NewEngine(
	wfStore Store,
	execStore ExecutionStore,
	dlStore DeadLetterStore,
	credStore credentials.Store,
	actions *ActionRegistry,
	cfg ...EngineConfig,
) *Engine {
	config := EngineConfig{
		BaseBackoff: 2 * time.Second,
		MaxBackoff:  5 * time.Minute,
	}
	if len(cfg) > 0 {
		config = cfg[0]
	}

	return &Engine{
		workflows:  wfStore,
		executions: execStore,
		deadLetter: dlStore,
		creds:      credStore,
		actions:    actions,
		cfg:        config,
	}
}

// ProcessEvent routes an incoming event to all active matching workflows and executes them.
func (e *Engine) ProcessEvent(ctx context.Context, ev *event.Event) ([]*Execution, error) {
	activeWorkflows, err := e.workflows.ListActive(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to query active workflows: %w", err)
	}

	var results []*Execution

	for _, wf := range activeWorkflows {
		if !wf.Trigger.Matches(ev) {
			continue
		}

		slog.Info("workflow matched event",
			slog.String("workflow_id", wf.ID),
			slog.String("workflow_name", wf.Name),
			slog.String("event_id", ev.ID),
			slog.String("event_type", ev.EventType),
		)

		// Create execution run (idempotent: returns existing execution if duplicate)
		exec, err := e.executions.Create(ctx, &Execution{
			WorkflowID:  wf.ID,
			EventID:     ev.ID,
			Status:      ExecutionPending,
			Attempts:    0,
			MaxAttempts: wf.MaxRetries,
		})
		if err != nil {
			slog.Error("failed to create execution record",
				slog.String("workflow_id", wf.ID),
				slog.String("event_id", ev.ID),
				slog.String("error", err.Error()),
			)
			return results, fmt.Errorf("failed to create execution for workflow %s: %w", wf.ID, err)
		}

		// If execution was already completed successfully, skip re-execution (strict idempotency)
		if exec.Status == ExecutionSuccess {
			slog.Info("workflow execution already succeeded, skipping duplicate",
				slog.String("execution_id", exec.ID),
				slog.String("workflow_id", wf.ID),
				slog.String("event_id", ev.ID),
			)
			results = append(results, exec)
			continue
		}

		// Execute workflow lifecycle
		executed, err := e.ExecuteWorkflow(ctx, wf, ev, exec)
		if err != nil {
			slog.Error("workflow execution failed",
				slog.String("execution_id", exec.ID),
				slog.String("workflow_id", wf.ID),
				slog.String("error", err.Error()),
			)
		}
		results = append(results, executed)
	}

	return results, nil
}

// ExecuteWorkflow executes the complete step lifecycle for a single workflow and event.
func (e *Engine) ExecuteWorkflow(
	ctx context.Context,
	wf *Workflow,
	ev *event.Event,
	exec *Execution,
) (*Execution, error) {
	start := time.Now().UTC()
	if err := ctx.Err(); err != nil {
		return exec, fmt.Errorf("workflow execution cancelled: %w", err)
	}
	exec.Attempts++
	if err := e.executions.UpdateStatus(ctx, exec.ID, ExecutionRunning, nil, 0); err != nil {
		return exec, fmt.Errorf("failed to mark execution running: %w", err)
	}
	observability.RecordExecutionStarted()

	// Step 1: Trigger Received
	triggerStep := &ExecutionStep{
		ExecutionID: exec.ID,
		StepType:    "trigger",
		Name:        fmt.Sprintf("Trigger: %s", wf.Trigger.Type),
		Status:      "success",
		Input:       map[string]any{"event_type": ev.EventType, "source_id": ev.SourceID},
		StartedAt:   start,
		CompletedAt: time.Now().UTC(),
		DurationMs:  time.Since(start).Milliseconds(),
	}
	e.recordStep(ctx, triggerStep)

	// Step 2: Evaluate Conditions
	condStart := time.Now().UTC()
	matched, err := conditions.Evaluate(wf.Conditions, ev)
	condStep := &ExecutionStep{
		ExecutionID: exec.ID,
		StepType:    "condition",
		Name:        "Condition Evaluation",
		StartedAt:   condStart,
		CompletedAt: time.Now().UTC(),
		DurationMs:  time.Since(condStart).Milliseconds(),
	}

	if err != nil {
		errStr := fmt.Sprintf("condition evaluation error: %v", err)
		condStep.Status = "failed"
		condStep.Error = &errStr
		e.recordStep(ctx, condStep)

		totalDuration := time.Since(start).Milliseconds()
		e.updateStatus(ctx, exec.ID, ExecutionFailed, &errStr, totalDuration)
		e.recordDeadLetter(ctx, exec, wf, ev, "", errStr)
		return exec, err
	}

	if !matched {
		// Condition not met: skip execution cleanly
		condStep.Status = "skipped"
		e.recordStep(ctx, condStep)

		totalDuration := time.Since(start).Milliseconds()
		e.updateStatus(ctx, exec.ID, ExecutionSuccess, nil, totalDuration)
		observability.RecordExecutionSucceeded()
		observability.RecordExecutionDuration(time.Since(start))
		slog.Info("workflow conditions not met, skipped cleanly",
			slog.String("workflow_id", wf.ID),
			slog.String("execution_id", exec.ID),
		)
		return exec, nil
	}

	condStep.Status = "success"
	e.recordStep(ctx, condStep)

	// Step 3: Transformation
	transStart := time.Now().UTC()
	transformedPayload, err := transformations.Transform(wf.Transformations, ev)
	transStep := &ExecutionStep{
		ExecutionID: exec.ID,
		StepType:    "transformation",
		Name:        "Data Transformation",
		StartedAt:   transStart,
		CompletedAt: time.Now().UTC(),
		DurationMs:  time.Since(transStart).Milliseconds(),
	}

	if err != nil {
		errStr := fmt.Sprintf("transformation error: %v", err)
		transStep.Status = "failed"
		transStep.Error = &errStr
		e.recordStep(ctx, transStep)

		totalDuration := time.Since(start).Milliseconds()
		e.updateStatus(ctx, exec.ID, ExecutionFailed, &errStr, totalDuration)
		e.recordDeadLetter(ctx, exec, wf, ev, "", errStr)
		return exec, err
	}

	transStep.Status = "success"
	transStep.Output = transformedPayload
	e.recordStep(ctx, transStep)

	// Step 4: Execute Actions Sequentially with data flow & idempotency
	stepStates := e.loadStepStates(ctx, exec.ID)

	for i, act := range wf.Actions {
		if act.ID == "" {
			act.ID = fmt.Sprintf("step_%d", i+1)
		}

		// Idempotent resumption: if this step already succeeded or was skipped in a prior attempt, reuse it
		if prev, ok := stepStates[act.ID]; ok && (prev.Status == "success" || prev.Status == "skipped") {
			slog.Info("skipping already completed action step",
				slog.String("workflow_id", wf.ID),
				slog.String("execution_id", exec.ID),
				slog.String("step_id", act.ID),
				slog.String("status", prev.Status),
			)
			continue
		}

		outcome, err := e.executeActionStep(ctx, wf, ev, exec, transformedPayload, act, stepStates, start)
		if outcome == outcomeFailed {
			return exec, err
		}
		if outcome == outcomeSkipRest {
			break
		}
	}

	// All steps succeeded
	totalDuration := time.Since(start).Milliseconds()
	e.updateStatus(ctx, exec.ID, ExecutionSuccess, nil, totalDuration)
	observability.RecordExecutionSucceeded()
	observability.RecordExecutionDuration(time.Since(start))
	slog.Info("workflow execution completed successfully",
		slog.String("execution_id", exec.ID),
		slog.String("workflow_id", wf.ID),
		slog.Int64("duration_ms", totalDuration),
	)

	return exec, nil
}

func (e *Engine) handleExecutionFailure(
	ctx context.Context,
	exec *Execution,
	wf *Workflow,
	ev *event.Event,
	actionID string,
	errStr string,
	start time.Time,
) (*Execution, error) {
	totalDuration := time.Since(start).Milliseconds()

	if exec.Attempts < exec.MaxAttempts {
		// Schedule retry with exponential backoff
		backoff := e.calculateBackoff(exec.Attempts)
		exec.NextAttemptAt = time.Now().UTC().Add(backoff)
		e.updateStatus(ctx, exec.ID, ExecutionRetrying, &errStr, totalDuration)
		observability.RecordRetry()
		slog.Warn("action execution failed, scheduled retry",
			slog.String("execution_id", exec.ID),
			slog.Int("attempt", exec.Attempts),
			slog.Int("max_attempts", exec.MaxAttempts),
			slog.Duration("backoff", backoff),
			slog.String("error", errStr),
		)
		return exec, fmt.Errorf("%s", errStr)
	}

	// Retries exhausted: mark failed and record in dead-letter
	e.updateStatus(ctx, exec.ID, ExecutionFailed, &errStr, totalDuration)
	observability.RecordExecutionFailed()
	observability.RecordExecutionDuration(time.Since(start))
	observability.RecordActionFailure()
	e.recordDeadLetter(ctx, exec, wf, ev, actionID, errStr)
	if e.errTracker != nil {
		e.errTracker.CaptureError(ctx, fmt.Errorf("%s", errStr), errtracker.Scope{
			WorkflowID:  wf.ID,
			ExecutionID: exec.ID,
			EventID:     ev.ID,
			Tags: map[string]string{
				"action_id": actionID,
			},
		})
	}
	slog.Error("workflow execution permanently failed (retries exhausted)",
		slog.String("execution_id", exec.ID),
		slog.String("workflow_id", wf.ID),
		slog.String("error", errStr),
	)
	return exec, fmt.Errorf("%s", errStr)
}

func (e *Engine) calculateBackoff(attempt int) time.Duration {
	mult := math.Pow(2, float64(attempt))
	backoff := time.Duration(mult) * e.cfg.BaseBackoff
	if backoff > e.cfg.MaxBackoff {
		backoff = e.cfg.MaxBackoff
	}
	return backoff
}

func (e *Engine) recordDeadLetter(
	ctx context.Context,
	exec *Execution,
	wf *Workflow,
	ev *event.Event,
	actionID string,
	errStr string,
) {
	if e.deadLetter == nil {
		return
	}
	if _, err := e.deadLetter.Record(ctx, &DeadLetterRecord{
		ExecutionID: exec.ID,
		WorkflowID:  wf.ID,
		EventID:     ev.ID,
		ActionID:    actionID,
		Error:       errStr,
		Payload:     ev.Payload,
		Attempts:    exec.Attempts,
		FailedAt:    time.Now().UTC(),
	}); err != nil {
		slog.ErrorContext(ctx, "failed to record dead letter",
			slog.String("execution_id", exec.ID),
			slog.String("workflow_id", wf.ID),
			slog.String("error", err.Error()),
		)
	}
}

func (e *Engine) recordStep(ctx context.Context, step *ExecutionStep) {
	if err := e.executions.RecordStep(ctx, step); err != nil {
		slog.ErrorContext(ctx, "failed to persist execution step",
			slog.String("execution_id", step.ExecutionID),
			slog.String("step_type", step.StepType),
			slog.String("error", err.Error()),
		)
	}
}

func (e *Engine) updateStatus(ctx context.Context, id string, status ExecutionStatus, errStr *string, durationMs int64) {
	if err := e.executions.UpdateStatus(ctx, id, status, errStr, durationMs); err != nil {
		slog.ErrorContext(ctx, "failed to persist execution status",
			slog.String("execution_id", id),
			slog.String("status", string(status)),
			slog.String("error", err.Error()),
		)
	}
}
