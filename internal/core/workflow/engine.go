package workflow

import (
	"context"
	"fmt"
	"log/slog"
	"math"
	"time"

	"kerob/internal/core/conditions"
	"kerob/internal/core/credentials"
	"kerob/internal/core/event"
	"kerob/internal/core/transformations"
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
			continue
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
	exec.Attempts++
	_ = e.executions.UpdateStatus(ctx, exec.ID, ExecutionRunning, nil, 0)

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
	_ = e.executions.RecordStep(ctx, triggerStep)

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
		_ = e.executions.RecordStep(ctx, condStep)

		totalDuration := time.Since(start).Milliseconds()
		_ = e.executions.UpdateStatus(ctx, exec.ID, ExecutionFailed, &errStr, totalDuration)
		e.recordDeadLetter(ctx, exec, wf, ev, "", errStr)
		return exec, err
	}

	if !matched {
		// Condition not met: skip execution cleanly
		condStep.Status = "skipped"
		_ = e.executions.RecordStep(ctx, condStep)

		totalDuration := time.Since(start).Milliseconds()
		_ = e.executions.UpdateStatus(ctx, exec.ID, ExecutionSuccess, nil, totalDuration)
		slog.Info("workflow conditions not met, skipped cleanly",
			slog.String("workflow_id", wf.ID),
			slog.String("execution_id", exec.ID),
		)
		return exec, nil
	}

	condStep.Status = "success"
	_ = e.executions.RecordStep(ctx, condStep)

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
		_ = e.executions.RecordStep(ctx, transStep)

		totalDuration := time.Since(start).Milliseconds()
		_ = e.executions.UpdateStatus(ctx, exec.ID, ExecutionFailed, &errStr, totalDuration)
		e.recordDeadLetter(ctx, exec, wf, ev, "", errStr)
		return exec, err
	}

	transStep.Status = "success"
	transStep.Output = transformedPayload
	_ = e.executions.RecordStep(ctx, transStep)

	// Step 4: Execute Actions Sequentially
	for _, act := range wf.Actions {
		actStart := time.Now().UTC()
		actionStep := &ExecutionStep{
			ExecutionID: exec.ID,
			StepType:    "action",
			Name:        fmt.Sprintf("Action: %s (%s)", act.Integration, act.ActionType),
			StartedAt:   actStart,
		}

		// Interpolate action configuration with event data
		renderedConfig, err := transformations.RenderJSONConfig(act.Config, ev)
		if err != nil {
			errStr := fmt.Sprintf("failed to render action config: %v", err)
			actionStep.Status = "failed"
			actionStep.Error = &errStr
			actionStep.CompletedAt = time.Now().UTC()
			actionStep.DurationMs = time.Since(actStart).Milliseconds()
			_ = e.executions.RecordStep(ctx, actionStep)

			return e.handleExecutionFailure(ctx, exec, wf, ev, act.ID, errStr, start)
		}

		// Resolve credentials if needed
		var decryptedCreds map[string]string
		if act.CredentialID != "" && e.creds != nil {
			creds, err := e.creds.GetDecrypted(ctx, act.CredentialID)
			if err != nil {
				errStr := fmt.Sprintf("failed to decrypt action credentials: %v", err)
				actionStep.Status = "failed"
				actionStep.Error = &errStr
				actionStep.CompletedAt = time.Now().UTC()
				actionStep.DurationMs = time.Since(actStart).Milliseconds()
				_ = e.executions.RecordStep(ctx, actionStep)

				return e.handleExecutionFailure(ctx, exec, wf, ev, act.ID, errStr, start)
			}
			decryptedCreds = creds
		}

		// Lookup action executor
		executor, err := e.actions.Get(act.Integration)
		if err != nil {
			errStr := err.Error()
			actionStep.Status = "failed"
			actionStep.Error = &errStr
			actionStep.CompletedAt = time.Now().UTC()
			actionStep.DurationMs = time.Since(actStart).Milliseconds()
			_ = e.executions.RecordStep(ctx, actionStep)

			return e.handleExecutionFailure(ctx, exec, wf, ev, act.ID, errStr, start)
		}

		// Enforce action timeout
		timeout := time.Duration(act.TimeoutSec) * time.Second
		if timeout <= 0 {
			timeout = time.Duration(wf.TimeoutSeconds) * time.Second
		}
		if timeout <= 0 {
			timeout = 30 * time.Second
		}
		actCtx, cancel := context.WithTimeout(ctx, timeout)

		result, err := executor.Execute(actCtx, ActionRequest{
			WorkflowID:  wf.ID,
			ActionID:    act.ID,
			Event:       ev,
			Config:      renderedConfig,
			Payload:     transformedPayload,
			Credentials: decryptedCreds,
		})
		cancel()

		actionStep.CompletedAt = time.Now().UTC()
		actionStep.DurationMs = time.Since(actStart).Milliseconds()

		if err != nil || !result.Success {
			errStr := "action execution failed"
			if err != nil {
				errStr = err.Error()
			}
			actionStep.Status = "failed"
			actionStep.Error = &errStr
			_ = e.executions.RecordStep(ctx, actionStep)

			return e.handleExecutionFailure(ctx, exec, wf, ev, act.ID, errStr, start)
		}

		actionStep.Status = "success"
		actionStep.Output = result.Output
		_ = e.executions.RecordStep(ctx, actionStep)
	}

	// All steps succeeded
	totalDuration := time.Since(start).Milliseconds()
	_ = e.executions.UpdateStatus(ctx, exec.ID, ExecutionSuccess, nil, totalDuration)
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
		_ = e.executions.UpdateStatus(ctx, exec.ID, ExecutionRetrying, &errStr, totalDuration)
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
	_ = e.executions.UpdateStatus(ctx, exec.ID, ExecutionFailed, &errStr, totalDuration)
	e.recordDeadLetter(ctx, exec, wf, ev, actionID, errStr)
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
	_, _ = e.deadLetter.Record(ctx, &DeadLetterRecord{
		ExecutionID: exec.ID,
		WorkflowID:  wf.ID,
		EventID:     ev.ID,
		ActionID:    actionID,
		Error:       errStr,
		Payload:     ev.Payload,
		Attempts:    exec.Attempts,
		FailedAt:    time.Now().UTC(),
	})
}
