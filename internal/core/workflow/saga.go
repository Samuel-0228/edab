package workflow

import (
	"context"
	"fmt"
	"log/slog"
	"time"

	"kerob/internal/core/transformations"
	"kerob/internal/observability"
)

// CompensateExecution executes reverse saga compensation for all completed steps in an execution.
func (e *Engine) CompensateExecution(ctx context.Context, executionID string, reason string) (*Execution, error) {
	exec, err := e.executions.GetByID(ctx, executionID)
	if err != nil {
		return nil, fmt.Errorf("failed to load execution for compensation: %w", err)
	}

	if exec.Status == ExecutionCompensated {
		slog.Info("execution already fully compensated, skipping", slog.String("execution_id", executionID))
		return exec, nil
	}

	wf, err := e.workflows.GetByID(ctx, exec.WorkflowID)
	if err != nil {
		return nil, fmt.Errorf("failed to load workflow %s for compensation: %w", exec.WorkflowID, err)
	}

	start := time.Now().UTC()
	slog.Info("starting saga compensation rollback",
		slog.String("execution_id", exec.ID),
		slog.String("workflow_id", wf.ID),
		slog.String("reason", reason),
	)

	// Transition execution to compensating
	exec.Status = ExecutionCompensating
	reasonStr := fmt.Sprintf("compensation triggered: %s", reason)
	_ = e.executions.UpdateStatus(ctx, exec.ID, ExecutionCompensating, &reasonStr, 0)
	if e.listener != nil {
		e.listener.OnExecutionCompensating(exec)
	}

	// Fetch event for interpolation context
	var evMap map[string]any
	stepStates := e.loadStepStates(ctx, exec.ID)

	// Build map of action steps by ID from workflow definition
	actionMap := make(map[string]ActionStep)
	collectActionSteps(wf.Actions, actionMap)

	// Filter successfully completed steps from execution history in REVERSE order (LIFO)
	var stepsToCompensate []ExecutionStep
	for i := len(exec.Steps) - 1; i >= 0; i-- {
		st := exec.Steps[i]
		if st.Status == "success" || st.Status == "approved" {
			stepsToCompensate = append(stepsToCompensate, st)
		}
	}

	var hasFailures bool
	var compErrStr string

	for _, st := range stepsToCompensate {
		act, found := actionMap[st.StepID]
		if !found || act.Compensation == nil {
			// No compensation action registered for this step
			continue
		}

		compAction := *act.Compensation
		compStepID := fmt.Sprintf("compensate_%s", st.StepID)

		// Idempotency check: if already compensated in a prior run, skip
		if prev, ok := stepStates[compStepID]; ok && prev.Status == "success" {
			slog.Info("step compensation already succeeded, skipping",
				slog.String("execution_id", exec.ID),
				slog.String("step_id", st.StepID),
			)
			continue
		}

		compStart := time.Now().UTC()
		compStepName := compAction.Name
		if compStepName == "" {
			compStepName = fmt.Sprintf("Compensate: %s", st.Name)
		}

		executionCompStep := &ExecutionStep{
			ExecutionID: exec.ID,
			StepID:      compStepID,
			StepType:    "compensation",
			Name:        compStepName,
			StartedAt:   compStart,
			Input:       map[string]any{"target_step_id": st.StepID, "reason": reason},
		}

		// 1. Resolve credentials if needed
		var decryptedCreds map[string]string
		if compAction.CredentialID != "" && e.creds != nil {
			creds, cErr := e.creds.GetDecrypted(ctx, compAction.CredentialID)
			if cErr != nil {
				errStr := fmt.Sprintf("failed to decrypt compensation credentials: %v", cErr)
				executionCompStep.Status = "failed"
				executionCompStep.Error = &errStr
				executionCompStep.CompletedAt = time.Now().UTC()
				executionCompStep.DurationMs = time.Since(compStart).Milliseconds()
				e.recordStep(ctx, executionCompStep)
				hasFailures = true
				compErrStr = errStr
				continue
			}
			decryptedCreds = creds
		}

		// 2. Interpolate compensation action config
		renderCtx := transformations.RenderContext{
			Steps:   stepStates,
			Secrets: decryptedCreds,
			Variables: map[string]any{
				"reason":              reason,
				"compensated_step_id": st.StepID,
				"step_output":         st.Output,
			},
		}
		if evMap != nil {
			renderCtx.Variables["event"] = evMap
		}

		renderedConfig, rErr := transformations.RenderJSONConfigWithContext(compAction.Config, renderCtx)
		if rErr != nil {
			errStr := fmt.Sprintf("failed to render compensation config: %v", rErr)
			executionCompStep.Status = "failed"
			executionCompStep.Error = &errStr
			executionCompStep.CompletedAt = time.Now().UTC()
			executionCompStep.DurationMs = time.Since(compStart).Milliseconds()
			e.recordStep(ctx, executionCompStep)
			hasFailures = true
			compErrStr = errStr
			continue
		}

		// 3. Resolve executor
		executor, aErr := e.actions.Get(compAction.Integration)
		if aErr != nil {
			errStr := fmt.Sprintf("compensation action executor not found (%s): %v", compAction.Integration, aErr)
			executionCompStep.Status = "failed"
			executionCompStep.Error = &errStr
			executionCompStep.CompletedAt = time.Now().UTC()
			executionCompStep.DurationMs = time.Since(compStart).Milliseconds()
			e.recordStep(ctx, executionCompStep)
			hasFailures = true
			compErrStr = errStr
			continue
		}

		// 4. Execute compensation with timeout
		timeout := time.Duration(compAction.TimeoutSec) * time.Second
		if timeout <= 0 {
			timeout = 30 * time.Second
		}
		compCtx, cancel := context.WithTimeout(ctx, timeout)
		res, execErr := executor.Execute(compCtx, ActionRequest{
			WorkflowID:  wf.ID,
			ActionID:    compStepID,
			Config:      renderedConfig,
			Credentials: decryptedCreds,
		})
		cancel()

		executionCompStep.CompletedAt = time.Now().UTC()
		executionCompStep.DurationMs = time.Since(compStart).Milliseconds()

		if execErr != nil || !res.Success {
			errStr := "compensation action execution failed"
			if execErr != nil {
				errStr = execErr.Error()
			}
			executionCompStep.Status = "failed"
			executionCompStep.Error = &errStr
			e.recordStep(ctx, executionCompStep)
			stepStates[compStepID] = transformations.StepOutput{
				Status:     "failed",
				Error:      errStr,
				DurationMs: executionCompStep.DurationMs,
			}
			hasFailures = true
			compErrStr = errStr
			slog.Error("saga compensation step failed",
				slog.String("execution_id", exec.ID),
				slog.String("step_id", st.StepID),
				slog.String("error", errStr),
			)
		} else {
			executionCompStep.Status = "success"
			executionCompStep.Output = res.Output
			e.recordStep(ctx, executionCompStep)
			stepStates[compStepID] = transformations.StepOutput{
				Status:     "success",
				Output:     executionCompStep.Output,
				DurationMs: executionCompStep.DurationMs,
			}
			slog.Info("saga compensation step succeeded",
				slog.String("execution_id", exec.ID),
				slog.String("step_id", st.StepID),
			)
		}
	}

	totalDuration := time.Since(start).Milliseconds()

	if hasFailures {
		exec.Status = ExecutionCompensationFailed
		finalErr := fmt.Sprintf("saga compensation encountered failures: %s", compErrStr)
		exec.Error = &finalErr
		_ = e.executions.UpdateStatus(ctx, exec.ID, ExecutionCompensationFailed, &finalErr, totalDuration)
		observability.RecordActionFailure()
	} else {
		exec.Status = ExecutionCompensated
		_ = e.executions.UpdateStatus(ctx, exec.ID, ExecutionCompensated, &reasonStr, totalDuration)
	}

	if e.listener != nil {
		e.listener.OnExecutionCompensated(exec)
	}

	slog.Info("saga compensation finished",
		slog.String("execution_id", exec.ID),
		slog.String("status", string(exec.Status)),
		slog.Int64("duration_ms", totalDuration),
	)

	return exec, nil
}

// HasCompensableSteps checks whether a workflow defines any compensation actions
// for steps that have already run successfully in the given execution.
func (e *Engine) HasCompensableSteps(wf *Workflow, exec *Execution) bool {
	if wf == nil || exec == nil {
		return false
	}
	actionMap := make(map[string]ActionStep)
	collectActionSteps(wf.Actions, actionMap)

	steps := exec.Steps
	if len(steps) == 0 && e.executions != nil {
		if loaded, err := e.executions.GetByID(context.Background(), exec.ID); err == nil && loaded != nil {
			steps = loaded.Steps
		}
	}

	for _, st := range steps {
		if st.Status == "success" || st.Status == "approved" {
			if act, ok := actionMap[st.StepID]; ok && act.Compensation != nil {
				return true
			}
		}
	}
	return false
}

// collectActionSteps recursively indexes all action steps by ID across branches, loops, and parallel blocks.
func collectActionSteps(steps []ActionStep, m map[string]ActionStep) {
	for i, st := range steps {
		stepID := st.ID
		if stepID == "" {
			stepID = fmt.Sprintf("step_%d", i+1)
		}
		m[stepID] = st

		// Check branches
		for _, bCase := range st.Cases {
			collectActionSteps(bCase.Steps, m)
		}
		collectActionSteps(st.Default, m)

		// Check child steps (for_each, parallel)
		collectActionSteps(st.Steps, m)
	}
}
