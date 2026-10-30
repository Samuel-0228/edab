package workflow

import (
	"context"
	"fmt"
	"log/slog"
	"time"

	"kerob/internal/core/event"
	"kerob/internal/core/transformations"
	"kerob/internal/observability"
)

type stepExecutionOutcome int

const (
	outcomeSuccess stepExecutionOutcome = iota
	outcomeContinue
	outcomeSkipRest
	outcomeFailed
)

// loadStepStates reconstructs the outcomes of previously completed steps for an execution.
// If an action step previously succeeded or was skipped, its output and status are re-used
// so that retries resume at the failed step rather than re-running already completed actions.
func (e *Engine) loadStepStates(ctx context.Context, execID string) map[string]transformations.StepOutput {
	stepStates := make(map[string]transformations.StepOutput)
	if e.executions == nil || execID == "" {
		return stepStates
	}

	exec, err := e.executions.GetByID(ctx, execID)
	if err != nil || exec == nil {
		return stepStates
	}

	for _, step := range exec.Steps {
		stepKey := step.StepID
		if stepKey == "" {
			stepKey = step.Name
		}
		var errStr string
		if step.Error != nil {
			errStr = *step.Error
		}
		stepStates[stepKey] = transformations.StepOutput{
			Status:     step.Status,
			Output:     step.Output,
			Error:      errStr,
			DurationMs: step.DurationMs,
		}
	}

	return stepStates
}

func (e *Engine) executeActionStep(
	ctx context.Context,
	wf *Workflow,
	ev *event.Event,
	exec *Execution,
	transformedPayload map[string]any,
	act ActionStep,
	stepStates map[string]transformations.StepOutput,
	start time.Time,
) (stepExecutionOutcome, error) {
	if err := ctx.Err(); err != nil {
		_, fErr := e.handleExecutionFailure(ctx, exec, wf, ev, act.ID, err.Error(), start)
		return outcomeFailed, fErr
	}

	actStart := time.Now().UTC()
	stepName := act.Name
	if stepName == "" {
		stepName = fmt.Sprintf("Action: %s (%s)", act.Integration, act.ActionType)
	}

	actionStep := &ExecutionStep{
		ExecutionID: exec.ID,
		StepID:      act.ID,
		StepType:    "action",
		Name:        stepName,
		StartedAt:   actStart,
	}

	// 1. Resolve credentials if configured
	var decryptedCreds map[string]string
	if act.CredentialID != "" && e.creds != nil {
		creds, err := e.creds.GetDecrypted(ctx, act.CredentialID)
		if err != nil {
			errStr := fmt.Sprintf("failed to decrypt action credentials: %v", err)
			actionStep.Status = "failed"
			actionStep.Error = &errStr
			actionStep.CompletedAt = time.Now().UTC()
			actionStep.DurationMs = time.Since(actStart).Milliseconds()
			e.recordStep(ctx, actionStep)

			return e.applyOnError(ctx, exec, wf, ev, act, errStr, start)
		}
		decryptedCreds = creds
	}

	// 2. Interpolate action configuration with event data, step outputs, and vault secrets
	renderCtx := transformations.RenderContext{
		Event:   ev,
		Steps:   stepStates,
		Secrets: decryptedCreds,
	}

	renderedConfig, err := transformations.RenderJSONConfigWithContext(act.Config, renderCtx)
	if err != nil {
		errStr := fmt.Sprintf("failed to render action config: %v", err)
		actionStep.Status = "failed"
		actionStep.Error = &errStr
		actionStep.CompletedAt = time.Now().UTC()
		actionStep.DurationMs = time.Since(actStart).Milliseconds()
		e.recordStep(ctx, actionStep)

		return e.applyOnError(ctx, exec, wf, ev, act, errStr, start)
	}

	// 3. Lookup registered action executor
	executor, err := e.actions.Get(act.Integration)
	if err != nil {
		errStr := err.Error()
		actionStep.Status = "failed"
		actionStep.Error = &errStr
		actionStep.CompletedAt = time.Now().UTC()
		actionStep.DurationMs = time.Since(actStart).Milliseconds()
		e.recordStep(ctx, actionStep)

		return e.applyOnError(ctx, exec, wf, ev, act, errStr, start)
	}

	// 4. Enforce action timeout
	timeout := time.Duration(act.TimeoutSec) * time.Second
	if timeout <= 0 {
		timeout = time.Duration(wf.TimeoutSeconds) * time.Second
	}
	if timeout <= 0 {
		timeout = 30 * time.Second
	}
	actCtx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	result, err := executor.Execute(actCtx, ActionRequest{
		WorkflowID:  wf.ID,
		ActionID:    act.ID,
		Event:       ev,
		Config:      renderedConfig,
		Payload:     transformedPayload,
		Credentials: decryptedCreds,
	})

	actionStep.CompletedAt = time.Now().UTC()
	actionStep.DurationMs = time.Since(actStart).Milliseconds()

	if err != nil || !result.Success {
		errStr := "action execution failed"
		if err != nil {
			errStr = err.Error()
		}
		actionStep.Status = "failed"
		actionStep.Error = &errStr
		e.recordStep(ctx, actionStep)
		observability.RecordActionFailure()

		stepStates[act.ID] = transformations.StepOutput{
			Status:     "failed",
			Error:      errStr,
			DurationMs: actionStep.DurationMs,
		}

		return e.applyOnError(ctx, exec, wf, ev, act, errStr, start)
	}

	actionStep.Status = "success"
	actionStep.Output = result.Output
	e.recordStep(ctx, actionStep)

	stepStates[act.ID] = transformations.StepOutput{
		Status:     "success",
		Output:     result.Output,
		DurationMs: actionStep.DurationMs,
	}

	return outcomeSuccess, nil
}

func (e *Engine) applyOnError(
	ctx context.Context,
	exec *Execution,
	wf *Workflow,
	ev *event.Event,
	act ActionStep,
	errStr string,
	start time.Time,
) (stepExecutionOutcome, error) {
	switch act.OnError {
	case OnErrorContinue:
		slog.Warn("action failed but on_error is continue; proceeding to next step",
			slog.String("workflow_id", wf.ID),
			slog.String("action_id", act.ID),
			slog.String("error", errStr),
		)
		return outcomeContinue, nil

	case OnErrorSkipRest:
		slog.Info("action failed with on_error=skip_rest; finishing workflow without further steps",
			slog.String("workflow_id", wf.ID),
			slog.String("action_id", act.ID),
			slog.String("error", errStr),
		)
		return outcomeSkipRest, nil

	default: // OnErrorFail or empty
		exec.CurrentStepID = act.ID
		_, err := e.handleExecutionFailure(ctx, exec, wf, ev, act.ID, errStr, start)
		return outcomeFailed, err
	}
}
